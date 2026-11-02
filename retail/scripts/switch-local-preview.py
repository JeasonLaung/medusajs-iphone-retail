"""One-time local86 -> dedicated99 Gin cutover, with old-service rollback.

Shared MySQL is read only throughout. Stops the verified old Gin before backup;
never runs two application processes against the same database. No gates enabled.
"""
import hashlib
import json
import os
from pathlib import Path
import re
import signal
import subprocess
import sys
import time
import urllib.request

ROOT = Path(__file__).resolve().parents[2]
BASE = ROOT / 'log/gin-preview-mysql'
BACKEND = ROOT / 'retail/backend'
OLD = ROOT / 'log/gin-local-api-20261031'
NEW = ROOT / 'log/gin-preview-api-99'
ENVFILE = BACKEND / '.env'


def save(name, value):
    p = BASE / name
    p.write_text(json.dumps(value, indent=2) + '\n')
    p.chmod(0o600)


def parse_env(raw):
    values = {}
    for line in raw.splitlines():
        if not line.strip() or line.lstrip().startswith('#'):
            continue
        if '=' not in line:
            raise RuntimeError('Unexpected local environment syntax')
        k, v = line.split('=', 1)
        values[k.strip()] = v.strip().strip('"').strip("'")
    return values


def listener():
    run = subprocess.run(['/usr/sbin/lsof', '-nP', '-t', '-iTCP:9100', '-sTCP:LISTEN'],
                         text=True, capture_output=True)
    pids = set(run.stdout.split())
    if len(pids) > 1:
        raise RuntimeError('More than one local API listener')
    return int(next(iter(pids))) if pids else None


def executable(pid):
    run = subprocess.run(['/usr/sbin/lsof', '-a', '-p', str(pid), '-d', 'txt', '-Fn'],
                         text=True, capture_output=True, check=True)
    files = [Path(line[1:]) for line in run.stdout.splitlines() if line.startswith('n')]
    if not files:
        raise RuntimeError('Cannot identify API binary')
    return files[0]


def stop(pid, expected):
    if executable(pid) != expected:
        raise RuntimeError('Refusing to stop an unowned binary')
    os.kill(pid, signal.SIGTERM)
    deadline = time.monotonic() + 20
    while time.monotonic() < deadline:
        try:
            os.kill(pid, 0)
        except ProcessLookupError:
            if listener() is not None:
                raise RuntimeError('Unexpected API listener after shutdown')
            return
        state = subprocess.run(['/bin/ps', '-p', str(pid), '-o', 'stat='],text=True,capture_output=True).stdout.strip()
        if (not state or state.startswith('Z')) and listener() is None:
            return
        time.sleep(0.2)
    raise RuntimeError('Old API graceful shutdown timed out')


def launch(binary, values, log_name):
    env = {k: os.environ[k] for k in ['PATH', 'HOME', 'TMPDIR'] if k in os.environ}
    env.update(values)
    with (BASE / log_name).open('ab') as output:
        proc = subprocess.Popen([str(binary)], cwd=BACKEND, env=env,
                                stdout=output, stderr=subprocess.STDOUT,
                                stdin=subprocess.DEVNULL, start_new_session=True)
    deadline = time.monotonic() + 25
    while time.monotonic() < deadline:
        if proc.poll() is not None:
            raise RuntimeError('Local API exited; inspect protected local log')
        try:
            with urllib.request.urlopen('http://127.0.0.1:9100/health', timeout=2) as response:
                if response.status == 200 and listener() == proc.pid:
                    return proc
        except (OSError, TimeoutError):
            pass
        time.sleep(0.2)
    if proc.poll() is None:
        proc.terminate()
        proc.wait(timeout=15)
    raise RuntimeError('New API readiness failed')


def main():
    if (BASE / 'cutover.json').exists() or (BASE / 'restore.json').exists():
        raise RuntimeError('Existing cutover state requires inspection; never overwrite')
    raw = ENVFILE.read_text()
    values = parse_env(raw)
    if not re.fullmatch(r'[^:]+:.*@tcp\(127\.0\.0\.1:3306\)/eurorefurb_local_trial\?.*', values.get('MYSQL_DSN', '')):
        raise RuntimeError('Expected original local86 connection')
    for name in ['CHECKOUT_PAYMENT_ENABLED', 'MOLLIE_NOTIFICATIONS_ENABLED', 'ORDER_REFUNDS_ENABLED',
                 'ORDER_MAIL_ENABLED', 'CUSTOMER_VERIFICATION_MAIL_ENABLED']:
        if values.get(name, 'false') not in ('false', ''):
            raise RuntimeError('Provider dispatch must remain disabled during cutover')
    if values.get('APP_PORT') != '9100' or values.get('APP_BIND') != '127.0.0.1':
        raise RuntimeError('Expected original local listener')
    pid = listener()
    if pid is None or executable(pid) != OLD:
        raise RuntimeError('Expected original local86 Gin process')
    if not NEW.is_file():
        raise RuntimeError('Tested new binary missing')
    # A payment/refund/mail worker is not part of this preview launch. Refuse
    # cutover if such a repository-owned executable is currently running.
    running = subprocess.run(['/bin/ps', '-ax', '-o', 'pid=,comm='],
                             text=True, capture_output=True, check=True).stdout
    for line in running.splitlines():
        if str(ROOT) in line and any(name in line for name in ['process-order-', 'send-order-mail', 'send-verification-mail', 'process-mollie-notifications']):
            raise RuntimeError('An owned writer worker is running; coordinate its shutdown first')
    backup_env = BASE / 'original-backend.env'
    if backup_env.exists():
        raise RuntimeError('Existing original environment backup requires inspection')
    backup_env.write_text(raw)
    backup_env.chmod(0o600)
    facts = {'old_pid': pid, 'old_binary': str(OLD), 'new_binary': str(NEW),
             'old_binary_sha256': hashlib.sha256(OLD.read_bytes()).hexdigest(),
             'new_binary_sha256': hashlib.sha256(NEW.read_bytes()).hexdigest(),
             'shared_mysql_modified': False, 'shared_mysql_restarted': False,
             'medusa_restarted': False, 'real_provider_called': False, 'goal_complete': False}
    stopped = False
    new_proc = None
    try:
        print(json.dumps({'phase': 'stopping_verified_old_gin', 'pid': pid}), flush=True)
        stopped = True
        stop(pid, OLD)
        # Fresh backup and full restore rehearsal after all project API writes
        # stop. The script creates no application processes or workers.
        with (BASE / 'fresh-rehearsal.log').open('wb') as output:
            check = subprocess.run([sys.executable, str(ROOT / 'retail/scripts/rehearse-local-preview-upgrade.py')],
                                   cwd=ROOT, stdout=output, stderr=subprocess.STDOUT, timeout=900)
        if check.returncode:
            raise RuntimeError('Fresh frozen backup/rehearsal failed')
        progress = [json.loads(line) for line in (BASE / 'fresh-rehearsal.log').read_text().splitlines() if line.startswith('{')]
        owner = progress[0]['owner']
        evidence = ROOT / 'logs/gin-migration' / ('preview-upgrade-' + owner)
        report = json.loads((evidence / 'report.json').read_text())
        if not report.get('rehearsal_passed'):
            raise RuntimeError('Fresh restore/migration evidence missing')
        facts['fresh_backup_owner'] = owner
        print(json.dumps({'phase': 'fresh_backup_verified', 'owner': owner}), flush=True)
        with (BASE / 'persistent-restore.log').open('wb') as output:
            restored = subprocess.run([sys.executable, str(ROOT / 'retail/scripts/preview-mysql.py'), 'restore',
                           '--backup', str(evidence / 'preview-86-private.sql'), '--migrate', str(evidence / 'migrate')],
                           cwd=ROOT, stdout=output, stderr=subprocess.STDOUT, timeout=900)
        if restored.returncode:
            raise RuntimeError('Persistent preview restore failed')
        new_dsn = parse_env((BASE / 'application.env').read_text())['MYSQL_DSN']
        new_values = {**values, 'MYSQL_DSN': new_dsn}
        new_raw = re.sub(r'^MYSQL_DSN=.*$', 'MYSQL_DSN=' + new_dsn, raw, flags=re.M)
        new_proc = launch(NEW, new_values, 'api.log')
        temporary = BACKEND / '.env.preview-next'
        temporary.write_text(new_raw)
        temporary.chmod(0o600)
        temporary.replace(ENVFILE)
        facts.update(new_pid=new_proc.pid, switched=True, schema_migrations=99,
                     original_backend_env_retained=True, rollback_required=False)
        save('cutover.json', facts)
        print(json.dumps({'switched': True, 'new_pid': new_proc.pid, 'schema_migrations': 99,
                          'shared_mysql_unchanged': True}), flush=True)
    except BaseException:
        if new_proc is not None and new_proc.poll() is None:
            new_proc.terminate()
            new_proc.wait(timeout=15)
        ENVFILE.write_text(raw)
        ENVFILE.chmod(0o600)
        if stopped and listener() is None:
            restored = launch(OLD, values, 'rollback-old-api.log')
            facts['rollback_old_pid'] = restored.pid
        facts.update(switched=False, rollback_required=False, failed=True)
        save('cutover-failed.json', facts)
        raise


if __name__ == '__main__':
    main()
