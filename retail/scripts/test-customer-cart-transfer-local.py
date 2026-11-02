"""Actual anonymous-cart -> new-account login -> checkout browser rehearsal.

Reuses the audited SQL snapshot/connection guards; all application sources are
frozen. A private-socket, network-disabled MySQL has only artificial data. No
shared database, provider credential, real mail, or running dev build is used.
"""
import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import secrets
import shutil
import signal
import socket
import subprocess
import time
import urllib.request

ROOT = Path(__file__).resolve().parents[2]
BASE = ROOT / 'logs/gin-migration'
MODULE = ROOT / 'retail/scripts/test-native-sql-local.py'
spec = importlib.util.spec_from_file_location('owned_native_sql', MODULE)
native = importlib.util.module_from_spec(spec)
spec.loader.exec_module(native)
native.STATE = BASE / 'customer-cart-transfer-native-state.json'
NODE = '/opt/homebrew/bin/node'
GO = native.GO
MYSQL = native.MYSQL


def sha(p):
    return hashlib.sha256(p.read_bytes()).hexdigest()


def prepare():
    native.prepare(isolated=True)
    c = native.config()
    snap = Path(c['snapshot'])
    for app in ['admin', 'storefront']:
        name = 'retail/' + app
        shutil.copytree(ROOT / name, snap / name, ignore=shutil.ignore_patterns(
            '.env*', '.astro', 'dist', 'node_modules', '*.log', 'test-results'))
        for p in (snap / name).rglob('*'):
            if p.is_file():
                n = str(p.relative_to(snap))
                if sha(p) != sha(ROOT / n):
                    raise RuntimeError('Source changed while freezing')
                c['source_sha256'][n] = sha(p)
        (snap / name / 'node_modules').symlink_to(ROOT / name / 'node_modules', target_is_directory=True)
    sockets = [socket.socket() for _ in range(2)]
    try:
        for s in sockets:
            s.bind(('127.0.0.1', 0))
        c['api_port'], c['storefront_port'] = [s.getsockname()[1] for s in sockets]
    finally:
        for s in sockets:
            s.close()
    for p in (snap / 'retail/backend/internal/identity').glob('*_test.go'):
        s = p.read_text().replace('19379', str(c['storefront_port'])).replace('0.0.0.0:9100', '127.0.0.1:' + str(c['api_port'])).replace('127.0.0.1:9100', '127.0.0.1:' + str(c['api_port']))
        p.write_text(s)
        n = str(p.relative_to(snap))
        if n in c['source_sha256'] and sha(p) != c['source_sha256'][n]:
            c['adapted_test_sha256'][n] = sha(p)
    p = snap / 'retail/storefront/tests/checkout.live.mjs'
    s = p.read_text()
    if s.count("shop='http://127.0.0.1:19379'") != 1:
        raise RuntimeError('Browser connection anchor changed')
    p.write_text(s.replace("shop='http://127.0.0.1:19379'", "shop='http://127.0.0.1:" + str(c['storefront_port']) + "'").replace("'19379'", "'" + str(c['storefront_port']) + "'"))
    c['adapted_browser_sha256'] = {str(p.relative_to(snap)): sha(p)}
    c['fixture_key'] = secrets.token_hex(32)
    c['controller_sha256'] = sha(MODULE)
    c['runner_sha256'] = sha(Path(__file__))
    native.write(native.STATE, c)
    native.write(snap / 'manifest.json', c)
    (snap / 'logs/gin-migration').mkdir(parents=True)
    print(json.dumps({'owner': c['owner'], 'snapshot': c['snapshot'], 'api_port': c['api_port'], 'storefront_port': c['storefront_port']}))


def run():
    c = native.config()
    snap = Path(c['snapshot'])
    log = snap / 'logs/gin-migration'
    if c['controller_sha256'] != sha(MODULE) or c['runner_sha256'] != sha(Path(__file__)):
        raise RuntimeError('Rehearsal controller changed')
    expected = dict(c['source_sha256'])
    expected.update(c['adapted_test_sha256'])
    expected.update(c['adapted_browser_sha256'])
    expected['retail/backend/internal/identity/native_sql_guard_test.go'] = c['guard_sha256']
    def frozen():
        for name, digest in expected.items():
            if sha(snap / name) != digest:
                raise RuntimeError('Frozen source changed: ' + name)
    frozen()
    env = {k: os.environ[k] for k in ['HOME', 'TMPDIR'] if k in os.environ}
    env['PATH'] = '/opt/homebrew/opt/go/bin:/opt/homebrew/bin:/usr/bin:/bin'
    env.update(IDENTITY_REHEARSAL='1', MYSQL_PWD='', CHECKOUT_BROWSER_FIXTURE='1',
               CHECKOUT_PRODUCT_BROWSE='1', CHECKOUT_FRESH_CART='1', CHECKOUT_ACCOUNTS='1',
               CHECKOUT_CART_TRANSFER='1', CHECKOUT_FIXTURE_KEY=c['fixture_key'],
               PUBLIC_CUSTOMER_CART_TRANSFER_ENABLED='true', ASTRO_TELEMETRY_DISABLED='1',
               CUSTOMER_ORDERS_TEST_GIN='http://127.0.0.1:' + str(c['api_port']),
               ADMIN_TEST_BROWSER='/Applications/Google Chrome.app/Contents/MacOS/Google Chrome')
    server = None
    processes = []
    ready = False
    datadir = snap / 'mysql-data'
    if datadir.exists():
        raise RuntimeError('Artificial database reuse rejected')
    datadir.mkdir(mode=0o700)
    mysqld = str(Path(MYSQL).resolve().with_name('mysqld'))
    def mysql(sql):
        p = subprocess.run([MYSQL, '--protocol=SOCKET', '--socket=' + c['address'], '--user=root', '--batch', '--skip-column-names'], input=sql, text=True, capture_output=True, env=env, timeout=30)
        if p.returncode:
            raise RuntimeError('Private fixture database control failed')
        return p.stdout.strip()
    def stop(p):
        if p.poll() is None:
            os.killpg(p.pid, signal.SIGTERM)
            try:
                p.wait(timeout=8)
            except subprocess.TimeoutExpired:
                os.killpg(p.pid, signal.SIGKILL)
                p.wait(timeout=5)
    try:
        subprocess.run([mysqld, '--no-defaults', '--initialize-insecure', '--datadir=' + str(datadir), '--innodb-buffer-pool-size=64M', '--innodb-redo-log-capacity=64M', '--log-error=' + str(log / 'mysql-init.log')], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=90, check=True)
        command = [mysqld, '--no-defaults', '--datadir=' + str(datadir), '--socket=' + c['address'], '--pid-file=' + str(snap / 'mysql.pid'), '--log-error=' + str(log / 'mysql.log'), '--skip-networking', '--mysqlx=OFF', '--skip-log-bin', '--innodb-buffer-pool-size=64M', '--innodb-redo-log-capacity=64M', '--innodb-native-foreign-keys=ON']
        server = subprocess.Popen(command, env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        for _ in range(200):
            if server.poll() is not None:
                raise RuntimeError('Private MySQL stopped')
            try:
                facts = mysql('SELECT VERSION(),@@innodb_native_foreign_keys,@@foreign_key_checks,@@skip_networking').split('\t')
                if len(facts) == 4 and facts[1:] == ['1', '1', '1']:
                    native.write(log / 'mysql-runtime.json', dict(version=facts[0], native_foreign_keys=True, foreign_key_checks=True, networking_disabled=True, private_socket=True))
                    ready = True
                    break
            except RuntimeError:
                pass
            time.sleep(.2)
        if not ready:
            raise RuntimeError('Private MySQL readiness deadline')
        frozen()
        with (log / 'compile.log').open('w') as out:
            subprocess.run([GO, 'test', '-c', '-o', str(snap / 'identity.test'), './internal/identity'], cwd=snap / 'retail/backend', env=env, stdout=out, stderr=subprocess.STDOUT, timeout=120, check=True)
        with (log / 'astro-build.log').open('w') as out:
            subprocess.run([NODE, 'node_modules/astro/bin/astro.mjs', 'build'], cwd=snap / 'retail/storefront', env=env, stdout=out, stderr=subprocess.STDOUT, timeout=180, check=True)
        with (log / 'joint-sql.log').open('w') as out:
            backend = subprocess.Popen([str(snap / 'identity.test'), '-test.run', '^TestOwnedCartRuntime$', '-test.v', '-test.timeout', '8m'], cwd=snap / 'retail/backend/internal/identity', env=env, stdout=out, stderr=subprocess.STDOUT, start_new_session=True)
            processes.append(backend)
            op = urllib.request.build_opener(urllib.request.ProxyHandler({}))
            available = False
            for _ in range(500):
                if backend.poll() is not None:
                    raise RuntimeError('SQL fixture stopped before browser readiness')
                try:
                    req = urllib.request.Request(env['CUSTOMER_ORDERS_TEST_GIN'] + '/__fixture/ready', headers={'X-Fixture-Key': c['fixture_key']})
                    with op.open(req, timeout=1) as response:
                        available = response.status == 200
                    if available:
                        break
                except OSError:
                    pass
                time.sleep(.2)
            if not available:
                raise RuntimeError('Browser fixture readiness deadline')
            with (log / 'joint-browser.log').open('w') as output:
                browser = subprocess.Popen([NODE, 'tests/checkout.live.mjs'], cwd=snap / 'retail/storefront', env=env, stdout=output, stderr=subprocess.STDOUT, start_new_session=True)
                processes.append(browser)
                if browser.wait(timeout=300):
                    raise RuntimeError('Cart transfer browser failed')
            if backend.wait(timeout=60):
                raise RuntimeError('Cart transfer SQL postconditions failed')
        frozen()
        text = (log / 'joint-sql.log').read_text()
        browser_text = (log / 'joint-browser.log').read_text()
        markers = [json.loads(line.split('CHECKOUT_CART_TRANSFER_JOINT_SQL ', 1)[1]) for line in text.splitlines() if 'CHECKOUT_CART_TRANSFER_JOINT_SQL ' in line]
        observations = [json.loads(line.split('CHECKOUT_JOINT_BROWSER ', 1)[1]) for line in browser_text.splitlines() if line.startswith('CHECKOUT_JOINT_BROWSER ')]
        if len(markers) != 1 or len(observations) != 1 or '\nPASS\n' not in text or 'CUSTOMER_CART_TRANSFER_REHEARSAL ' not in text or not observations[0].get('anonymous_cart_login_transfer') or not observations[0].get('lost_transfer_reply_replays_original') or observations[0].get('browser_errors') != 0:
            raise RuntimeError('Actual cart transfer evidence incomplete')
        native.write(log / 'joint-result.json', dict(sql=markers[0], browser=observations[0], source_frozen=True, source_files=len(expected), binary_sha256=sha(snap / 'identity.test'), sql_sha256=sha(log / 'joint-sql.log'), browser_sha256=sha(log / 'joint-browser.log'), migrations=95, goal_complete=False))
        print(json.dumps({'cart_transfer_joint': 'passed', 'migrations': 95, 'new_accounts': 4}))
    finally:
        for process in reversed(processes):
            stop(process)
        try:
            if ready and server is not None and server.poll() is None:
                names = list(c['database_mapping'].values())
                mysql(''.join('DROP DATABASE IF EXISTS `' + n + '`;' for n in names))
                present = mysql("SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name IN (" + ','.join("'" + n + "'" for n in names) + ')')
                if present != '0':
                    raise RuntimeError('Owned databases remain')
        finally:
            if server is not None and server.poll() is None:
                subprocess.run([str(Path(MYSQL).resolve().with_name('mysqladmin')), '--protocol=SOCKET', '--socket=' + c['address'], '--user=root', 'shutdown'], env=env, capture_output=True, timeout=15)
                try:
                    server.wait(timeout=30)
                except subprocess.TimeoutExpired:
                    server.terminate()
                    server.wait(timeout=15)
            terminal = server is None or server.poll() is not None
            if terminal:
                shutil.rmtree(datadir)
                parent = Path(c['address']).parent
                if parent.exists() and not list(parent.iterdir()):
                    parent.rmdir()
            keys = subprocess.check_output(['/opt/homebrew/bin/redis-cli', '-p', '16379', '--scan', '--pattern', c['redis_prefix'] + '*'], text=True).splitlines()
            if any(not k.startswith(c['redis_prefix']) for k in keys):
                raise RuntimeError('Unowned Redis key')
            for i in range(0, len(keys), 100):
                subprocess.run(['/opt/homebrew/bin/redis-cli', '-p', '16379', 'DEL', *keys[i:i + 100]], capture_output=True, check=True)
            native.write(log / 'cleanup.json', dict(owned_processes_terminal=all(p.poll() is not None for p in processes), mysql_terminal=terminal, private_data_removed=not datadir.exists(), private_socket_removed=not Path(c['address']).exists(), redis_keys_deleted=len(keys), shared_services_unchanged=True))


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('mode', choices=['prepare', 'run'])
    args = parser.parse_args()
    prepare() if args.mode == 'prepare' else run()
