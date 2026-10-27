"""Actual pinned payments in an owned, disconnected, disposable MySQL target.

No app DSN, source DB connection, provider call or production target is accepted.
Private snapshot and target tables live in tmpfs, target keys stay in Go memory.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import subprocess
import time
import uuid

ROOT = Path(__file__).resolve().parents[2]
LABEL = 'eurorefurb.payment-history-rehearsal'
PINNED_SHA = 'b58a26812eeea8e75c6bd3e4ec4635b6260d8e704bc9a63fd095f0e7d9f13985'


def docker(*args, input=None, timeout=120):
    return subprocess.run(['docker', *args], input=input, capture_output=True, timeout=timeout)


def ignored(path):
    return subprocess.run(['git', 'check-ignore', '--quiet', str(path)], cwd=ROOT, capture_output=True).returncode == 0


def digest(path):
    return hashlib.sha256(path.read_bytes().replace(b'\r\n', b'\n')).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--snapshot', type=Path, required=True)
    args = parser.parse_args()
    snapshot = args.snapshot.resolve()
    if not snapshot.is_file() or not ignored(snapshot) or hashlib.sha256(snapshot.read_bytes()).hexdigest() != PINNED_SHA:
        raise RuntimeError('Pinned private v2 snapshot required')
    ddl = ROOT / 'docs/migration/mysql-relational-review.sql'
    review = json.loads((ROOT / 'docs/migration/mysql-relational-review.json').read_text(encoding='utf-8'))
    if digest(ddl) != review['ddl_sha256']:
        raise RuntimeError('Reviewed DDL checksum differs')
    if docker('image', 'inspect', 'mysql:8.4', '--format', '{{.Id}}').returncode:
        raise RuntimeError('Local mysql:8.4 image required; no pull performed')
    owner = uuid.uuid4().hex
    executable = ROOT / 'logs/gin-migration' / ('payment-rehearsal-' + owner + '.test')
    if not ignored(executable) or executable.exists():
        raise RuntimeError('New ignored test binary path required')
    environment = os.environ.copy()
    environment.update(GOOS='linux', GOARCH='amd64', CGO_ENABLED='0')
    container = None
    binary_owned = False
    password = secrets.token_urlsafe(32)
    report = None
    try:
        # Unique output path; the compiler emits no private source/target data.
        binary_owned = True
        compiled = subprocess.run(['go', 'test', '-c', '-o', str(executable), './internal/rowimport'],
                                  cwd=ROOT / 'retail/backend', env=environment, capture_output=True, timeout=120)
        if compiled.returncode:
            raise RuntimeError('Rehearsal test runner compilation failed')
        started = docker('run', '-d', '--name', 'eurorefurb-payment-rehearsal-' + owner[:12],
                         '--label', LABEL + '=' + owner, '--network', 'none', '--cpus', '1',
                         '--memory', '768m', '--memory-swap', '768m', '--pids-limit', '192',
                         '--tmpfs', '/var/lib/mysql:rw,size=512m', '--tmpfs', '/tmp:rw,size=128m,mode=1777',
                         '-e', 'MYSQL_ROOT_PASSWORD=' + password, '-e', 'MYSQL_DATABASE=payment_rehearsal',
                         'mysql:8.4', '--skip-log-bin', '--innodb-buffer-pool-size=64M',
                         '--performance-schema=OFF', '--mysqlx=OFF', '--max-connections=8')
        if started.returncode:
            raise RuntimeError('Isolated rehearsal creation failed')
        container = started.stdout.decode('ascii').strip()

        def sql(query):
            result = docker('exec', '-i', '-e', 'MYSQL_PWD=' + password, container,
                            'mysql', '--user=root', '--protocol=socket', '--batch', '--raw',
                            '--skip-column-names', '--default-character-set=utf8mb4', 'payment_rehearsal',
                            input=query.encode('utf-8'))
            if result.returncode:
                raise RuntimeError('Isolated rehearsal SQL failed; private output suppressed')
            return result.stdout.decode('utf-8').strip()

        for _ in range(100):
            ready = docker('exec', '-e', 'MYSQL_PWD=' + password, container, 'mysql', '-uroot', '--protocol=socket',
                           '--batch', '--skip-column-names', '-e', 'SELECT 1', 'payment_rehearsal', timeout=10)
            if ready.returncode == 0:
                break
            time.sleep(0.5)
        else:
            raise RuntimeError('Isolated rehearsal MySQL did not become ready')
        version = sql('SELECT VERSION();')
        for local, remote in [(executable, '/opt/payment-rehearsal.test'), (snapshot, '/tmp/payment-source.ndjson')]:
            # Stream private source directly into the running tmpfs mount.
            # The non-private executable uses the container filesystem because
            # this host mounts tmpfs without executable permission.
            # remote is one of two fixed literals above, never user input.
            with local.open('rb') as private_input:
                transfer = subprocess.run(['docker', 'exec', '-i', container, '/bin/sh', '-c',
                                           'umask 077; cat > ' + remote], stdin=private_input,
                                          stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=60)
            if transfer.returncode:
                raise RuntimeError('Private rehearsal input transfer failed')
        if docker('exec', container, 'chmod', '700', '/opt/payment-rehearsal.test').returncode:
            raise RuntimeError('Rehearsal runner preparation failed')
        tested = docker('exec', '-e', 'MYSQL_PWD=' + password, '-e', 'ROWIMPORT_PAYMENT_REHEARSAL=1', container,
                        '/opt/payment-rehearsal.test', '-test.v', '-test.run', '^TestActualPaymentHistoryMySQL$', timeout=150)
        # Never echo captured test stdout/stderr. Only the aggregate marker is
        # decoded after success; private driver errors are not report content.
        markers = [line.split('PAYMENT_MYSQL_REHEARSAL ', 1)[1]
                   for line in tested.stdout.decode('utf-8').splitlines() if 'PAYMENT_MYSQL_REHEARSAL ' in line]
        if tested.returncode or len(markers) != 1:
            reasons = re.findall(r't\.Fatal\("([^"\n]+)"\)',
                                 (ROOT / 'retail/backend/internal/rowimport/payment_rehearsal_test.go').read_text(encoding='utf-8'))
            reason = next((item for item in reasons if item.encode() in tested.stdout), None)
            if reason is None and b'permission denied' in tested.stdout + tested.stderr:
                reason = 'temporary runner execution denied'
            raise RuntimeError('Actual payment rehearsal failed: ' + (reason or 'unclassified runner failure') +
                               '; runner_started=' + str(b'=== RUN' in tested.stdout) + '; exit=' + str(tested.returncode) + '; private output suppressed')
        counts = json.loads(markers[0])
        files = ['retail/scripts/rehearse-payment-history.py', 'retail/backend/internal/rowimport/payment_rehearsal_test.go',
                 'retail/backend/internal/rowimport/history_payment_sql.go', 'retail/backend/internal/rowimport/history_payment.go',
                 'retail/backend/internal/rowimport/payment_data.go', 'retail/backend/internal/rowimport/payment_context.go',
                 'retail/backend/internal/rowimport/payment_context_envelope.go', 'retail/backend/internal/rowimport/convert.go',
                 'retail/backend/internal/rowimport/field_plans.json', 'retail/backend/migrations/0005_migration_payment_dispositions.sql',
                 'docs/migration/mysql-relational-review.sql']
        files += [str(p.relative_to(ROOT)).replace('\\','/') for p in (ROOT/'retail/backend/migrations').glob('*.sql')]
        files += ['retail/backend/internal/schema/run.go','retail/backend/internal/schema/plan.go','retail/backend/internal/payments/schema.go','retail/backend/internal/payments/schema_plan.json']
        report = {'format_version': 1, 'scope': 'Actual pinned payment subgraph under all 23 formal migrations in disposable tmpfs MySQL only; no full migration or payment runtime acceptance',
                  'snapshot_sha256': PINNED_SHA, 'mysql_version': version, 'counts': counts,
                  'limits': {'cpus': 1, 'memory_mib': 768, 'mysql_tmpfs_mib': 512, 'private_input_tmpfs_mib': 128, 'network': 'none', 'published_ports': 0},
                  'target_business_rows_written': True, 'target_operation_holds_written': True, 'production_target_used': False,
                  'source_database_connection': False, 'provider_called': False, 'new_keys_persisted': False,
                  'private_values_in_report': False, 'relational_acceptance': False,
                  'implementation_sha256': {file: digest(ROOT / file) for file in files}}
    finally:
        password = ''
        if container:
            ownership = docker('inspect', container, '--format', '{{index .Config.Labels "' + LABEL + '"}}')
            if ownership.returncode or ownership.stdout.decode().strip() != owner:
                raise RuntimeError('Rehearsal ownership mismatch; cleanup refused')
            if docker('rm', '-f', '-v', container).returncode:
                raise RuntimeError('Owned rehearsal cleanup failed')
        if binary_owned and executable.exists():
            executable.unlink()  # Exact newly owned non-private test binary only.
    if report:
        report['cleanup'] = 'owned container removed after label verification; tmpfs source/target copies and temporary runner removed'
        (ROOT / 'docs/migration/payment-mysql-rehearsal-report.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
        print(json.dumps({'counts': report['counts'], 'cleanup': 'complete', 'production_target_used': False}))


if __name__ == '__main__':
    main()
