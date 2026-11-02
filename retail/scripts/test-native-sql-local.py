"""Run the original 23 SQL suites on owned loopback MySQL/Redis resources.

Only test connection/naming and explicit database guards change in the frozen
copy. No source assertions, migration counts, production code or gates change.
"""
import argparse
import getpass
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import shutil
import subprocess
import sys
import tempfile
import time

ROOT = Path(__file__).resolve().parents[2]
BASE = ROOT / 'logs/gin-migration'
STATE = BASE / 'native-sql-local-state.json'
GO = '/opt/homebrew/opt/go/bin/go'
MYSQL = '/opt/homebrew/bin/mysql'
REDIS = '/opt/homebrew/bin/redis-cli'
SOURCE_PATHS = ['retail/backend', 'docs/migration/fixtures',
                'medusa-store/apps/backend/src', 'medusa-store/apps/storefront/src']
SUITES = ['PromotionUsage', 'PromotionSchema', 'GeoSchema', 'CartPriceRefresh',
          'CartTaxRefresh', 'TaxImport', 'TaxSchema', 'AdminAuthentication',
          'CatalogSchemaTransition', 'ProductEdits', 'ProductPrices',
          'ProductInventory', 'InventoryStockEdits', 'ReservationTransactions',
          'OrderSchema', 'OrderImport', 'CustomerHistory', 'OrderRead',
          'PaymentSchema', 'PaymentImport', 'FulfillmentHistory', 'CartSchema', 'CartRuntime']


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def write(path, value):
    path.write_text(json.dumps(value, indent=2) + '\n')
    path.chmod(0o600)


def prepare(isolated=False):
    if STATE.exists():
        raise RuntimeError('Inspect existing owned state before preparing another snapshot')
    if shutil.disk_usage(ROOT).free < 2 * 1024**3:
        raise RuntimeError('2 GiB free required to preserve development services')
    owner = secrets.token_hex(6)
    snapshot = BASE / ('native-sql-local-' + owner)
    snapshot.mkdir()
    socket_dir = Path(tempfile.mkdtemp(prefix="ersql-" + owner + "-", dir="/private/tmp")) if isolated else None
    transport = "unix" if isolated else "tcp"
    address = str(socket_dir / "mysql.sock") if isolated else "127.0.0.1:3306"
    original = {}
    for name in SOURCE_PATHS:
        shutil.copytree(ROOT / name, snapshot / name, ignore=shutil.ignore_patterns(
            '.env*', '*.log', '__pycache__', 'node_modules', '.medusa', 'dist'))
        for p in (snapshot / name).rglob('*'):
            if p.is_file():
                n = str(p.relative_to(snapshot))
                if sha(p) != sha(ROOT / n):
                    raise RuntimeError('Source changed during copy')
                original[n] = sha(p)
    tests = list((snapshot / 'retail/backend/internal/identity').glob('*_test.go'))
    names = {'identity_rehearsal', 'browser_identity', 'warehouse_permission_upgrade'}
    for p in tests:
        s = p.read_text()
        names.update(re.findall(r'\b[a-z][a-z0-9_]*(?:_rehearsal|_schema_fresh|_schema_existing)\b', s))
        names.update(re.findall(r'\bschema_catalog_[a-z_]+\b', s))
        names.update(re.findall(r'(?:open|openDB)\("([a-z][a-z0-9_]+)"', s))
    mapping = {n: 'eurorefurb_sql_' + owner + '_' + str(i) for i, n in enumerate(sorted(names))}
    redis_prefix = 'eurorefurb:n' + owner + '-'
    namespace_map = {}
    for p in tests:
        s = p.read_text()
        s = s.replace('@unix(/var/run/mysqld/mysqld.sock)', "@" + transport + "(" + address + ")")
        s = s.replace('"unix"', json.dumps(transport)).replace('/var/run/mysqld/mysqld.sock', address)
        s = s.replace('127.0.0.1:6379', '127.0.0.1:16379')
        def namespace(match):
            before = match.group(0)
            after = redis_prefix + hashlib.sha256(before.encode()).hexdigest()[:8] + ':v1:'
            namespace_map[before] = after
            return after
        s = re.sub(r'eurorefurb:[a-z][a-z0-9_-]{2,39}:v1:', namespace, s)
        s = re.sub(r'\b(' + '|'.join(re.escape(n) for n in sorted(names, key=len, reverse=True)) + r')\b', lambda m: mapping[m[0]], s)
        # Guard every connection, including fixture helpers not selected today.
        s = re.sub(r'\b([a-zA-Z][a-zA-Z0-9_]*)\.FormatDSN\(\)', r'nativeSQLDSN(t,\1.FormatDSN())', s)
        # Dynamic names are fixed test constants, additionally checked at use.
        s = re.sub(r'((?:CREATE|DROP) DATABASE[^"\n]*"\s*\+\s*)name\b', r'\1nativeSQLDatabase(t,name)', s)
        for line in s.splitlines():
            if 'CREATE DATABASE' not in line and 'DROP DATABASE' not in line:
                continue
            if '+name' in line or '+ name' in line:
                raise RuntimeError('Unguarded dynamic database name')
            for n in re.findall(r'(?:CREATE|DROP) DATABASE `?([a-z][a-z0-9_]+)', line):
                if n not in mapping.values():
                    raise RuntimeError('Unowned database statement')
        p.write_text(s)
    allowed = ','.join(json.dumps(n) + ':true' for n in mapping.values())
    guard = snapshot / 'retail/backend/internal/identity/native_sql_guard_test.go'
    guard.write_text('''package identity_test
import("testing"; driver "github.com/go-sql-driver/mysql")
var nativeSQLAllowed=map[string]bool{''' + allowed + '''}
func nativeSQLDatabase(t testing.TB,name string)string{t.Helper();if !nativeSQLAllowed[name]{t.Fatal("unowned native database rejected")};return name}
func nativeSQLDSN(t testing.TB,dsn string)string{t.Helper();c,e:=driver.ParseDSN(dsn);if e!=nil||c.Net!=''' + json.dumps(transport) + '''||c.Addr!=''' + json.dumps(address) + '''||c.User!="root"{t.Fatal("unowned native connection rejected")};if c.DBName!=""{nativeSQLDatabase(t,c.DBName)};return dsn}
''')
    subprocess.run([str(Path(GO).with_name('gofmt')), '-w', *map(str, tests), str(guard)], check=True)
    adapted = {n: sha(snapshot / n) for n, digest in original.items() if sha(snapshot / n) != digest}
    if any(not n.startswith('retail/backend/internal/identity/') or not n.endswith('_test.go') for n in adapted):
        raise RuntimeError('Production source adaptation forbidden')
    c = dict(owner=owner, snapshot=str(snapshot), isolated_mysql=isolated,
             transport=transport, address=address, database_mapping=mapping,
             redis_prefix=redis_prefix, namespace_mapping=namespace_map,
             source_sha256=original, adapted_test_sha256=adapted,
             guard_sha256=sha(guard), suites=SUITES, migrations=103)
    write(STATE, c)
    write(snapshot / 'manifest.json', c)
    print(json.dumps(dict(snapshot=str(snapshot), suites=len(SUITES), owned_database_names=len(mapping))))


def config():
    c = json.loads(STATE.read_text())
    o = c['owner']
    if not re.fullmatch('[a-f0-9]{12}', o) or c['snapshot'] != str(BASE / ('native-sql-local-' + o)) or c['redis_prefix'] != 'eurorefurb:n' + o + '-' or c['suites'] != SUITES:
        raise RuntimeError('Owned resource identity mismatch')
    if any(not re.fullmatch('eurorefurb_sql_' + o + r'_[0-9]+', n) for n in c['database_mapping'].values()):
        raise RuntimeError('Unowned database mapping')
    if c.get('isolated_mysql'):
        parent = Path(c['address']).parent
        if c.get('transport') != 'unix' or parent.parent != Path('/private/tmp') or not parent.name.startswith('ersql-' + o + '-') or Path(c['address']).name != 'mysql.sock' or not parent.is_dir() or parent.stat().st_mode & 0o077:
            raise RuntimeError('Isolated MySQL socket identity mismatch')
    elif c.get('transport', 'tcp') != 'tcp' or c.get('address', '127.0.0.1:3306') != '127.0.0.1:3306':
        raise RuntimeError('Shared MySQL address changed')
    return c


def run(selected_suite=None):
    c = config()
    selected = [selected_suite] if selected_suite else SUITES
    snapshot = Path(c['snapshot'])
    def frozen():
        expected = set(c['source_sha256']) | {'retail/backend/internal/identity/native_sql_guard_test.go'}
        actual = {str(p.relative_to(snapshot)) for name in SOURCE_PATHS for p in (snapshot / name).rglob('*') if p.is_file()}
        if actual != expected:
            raise RuntimeError('Frozen source file inventory changed')
        if len(list((snapshot / 'retail/backend/migrations').glob('*.sql'))) != c['migrations']:
            raise RuntimeError('Frozen migration inventory changed')
        for n, digest in c['source_sha256'].items():
            if sha(snapshot / n) != c['adapted_test_sha256'].get(n, digest):
                raise RuntimeError('Frozen source changed: ' + n)
        if sha(snapshot / 'retail/backend/internal/identity/native_sql_guard_test.go') != c['guard_sha256']:
            raise RuntimeError('Database guard changed')
    frozen()
    if not c.get('isolated_mysql') and not sys.stdin.isatty():
        raise RuntimeError('Root fixture requires a hidden terminal prompt')
    # No application environment/credentials/providers or workers are inherited.
    env = {k: os.environ[k] for k in ['PATH', 'HOME', 'TMPDIR'] if k in os.environ}
    binary = snapshot / 'identity.test'
    with (snapshot / 'compile.log').open('w') as out:
        built = subprocess.run([GO, 'test', '-c', '-o', str(binary), './internal/identity'],
                               cwd=snapshot / 'retail/backend', env=env, stdout=out, stderr=subprocess.STDOUT, timeout=120)
    if built.returncode:
        raise RuntimeError('Owned SQL fixture compile failed')
    env.update(MYSQL_PWD='' if c.get('isolated_mysql') else getpass.getpass('Owned 23-suite root password (hidden): '), IDENTITY_REHEARSAL='1')
    databases = sorted(c['database_mapping'].values())
    def mysql(sql):
        connection = ['--protocol=SOCKET', '--socket=' + c['address']] if c.get('isolated_mysql') else ['--host=127.0.0.1']
        p = subprocess.run([MYSQL, *connection, '--user=root', '--batch', '--skip-column-names'],
                           input=sql, env=env, text=True, capture_output=True, timeout=45)
        if p.returncode:
            raise RuntimeError('Owned database control failed; credentials suppressed')
        return p.stdout.splitlines()
    def present():
        names = ','.join("'" + n + "'" for n in databases)
        return mysql('SELECT schema_name FROM information_schema.schemata WHERE schema_name IN (' + names + ') ORDER BY schema_name')
    results = []
    def cleanup():
        mysql(''.join('DROP DATABASE IF EXISTS `' + n + '`;' for n in databases))
        keys = subprocess.check_output([REDIS, '-p', '16379', '--scan', '--pattern', c['redis_prefix'] + '*'], text=True).splitlines()
        if any(not k.startswith(c['redis_prefix']) for k in keys):
            raise RuntimeError('Unowned Redis key')
        for i in range(0, len(keys), 100):
            subprocess.run([REDIS, '-p', '16379', 'DEL', *keys[i:i + 100]], capture_output=True, check=True)
        if present():
            raise RuntimeError('Owned database cleanup not proven')
        write(snapshot / 'cleanup.json', dict(owned_databases_absent=True, redis_prefix_cleaned=True, last_deleted_keys=len(keys), shared_server_not_restarted=True))
    server = None
    resources_verified = False
    datadir = snapshot / 'mysql-data'
    try:
        if c.get('isolated_mysql'):
            if datadir.exists():
                raise RuntimeError('Isolated MySQL data reuse rejected')
            datadir.mkdir(mode=0o700)
            mysqld = str(Path(MYSQL).resolve().with_name('mysqld'))
            init = subprocess.run([mysqld, '--no-defaults', '--initialize-insecure', '--innodb-buffer-pool-size=64M', '--innodb-redo-log-capacity=64M', '--datadir=' + str(datadir), '--log-error=' + str(snapshot / 'mysql-initialize.log')], env=env, capture_output=True, timeout=90)
            if init.returncode:
                raise RuntimeError('Isolated MySQL initialization failed; inspect mysql-initialize.log')
            command = [mysqld, '--no-defaults', '--datadir=' + str(datadir), '--socket=' + c['address'], '--pid-file=' + str(snapshot / 'mysql.pid'), '--log-error=' + str(snapshot / 'mysql-server.log'), '--skip-networking', '--mysqlx=OFF', '--skip-log-bin', '--innodb-buffer-pool-size=64M', '--innodb-redo-log-capacity=64M', '--innodb-native-foreign-keys=ON']
            with (snapshot / 'mysql-launch.log').open('w') as output:
                server = subprocess.Popen(command, env=env, stdout=output, stderr=subprocess.STDOUT)
            deadline = time.monotonic() + 45
            while time.monotonic() < deadline:
                if server.poll() is not None:
                    raise RuntimeError('Isolated MySQL startup failed')
                if Path(c['address']).exists():
                    try:
                        facts = mysql('SELECT VERSION(),@@innodb_native_foreign_keys,@@foreign_key_checks,@@skip_networking')
                        if len(facts) == 1 and facts[0].split('\t')[1:] == ['1', '1', '1']:
                            write(snapshot / 'mysql-runtime.json', dict(version=facts[0].split('\t')[0], native_foreign_keys=True, foreign_key_checks=True, networking_disabled=True, socket_private=True, pid=server.pid))
                            break
                    except RuntimeError:
                        pass
                time.sleep(0.2)
            else:
                raise RuntimeError('Isolated MySQL readiness timed out')
            print(json.dumps({'isolated_mysql_ready': True, 'networking_disabled': True}), flush=True)
        if present():
            raise RuntimeError('Owned database exists; test fact reuse rejected')
        resources_verified = True
        for suite in selected:
            if shutil.disk_usage(ROOT).free < 1024**3:
                raise RuntimeError('1 GiB free required between suites')
            frozen()
            if suite == 'AdminAuthentication':
                mysql('CREATE DATABASE `' + c['database_mapping']['identity_rehearsal'] + '`')
            print(json.dumps({'running_suite': suite}), flush=True)
            with (snapshot / (suite + '.log')).open('w') as output:
                p = subprocess.run([str(binary), '-test.run', '^TestOwned' + suite + '$', '-test.v', '-test.timeout', '180s'],
                                   cwd=snapshot / 'retail/backend/internal/identity', env=env,
                                   stdout=output, stderr=subprocess.STDOUT, timeout=200)
            text = (snapshot / (suite + '.log')).read_text()
            if p.returncode or '--- PASS: TestOwned' + suite not in text or '\nPASS\n' not in text or '--- SKIP' in text:
                raise RuntimeError('Native SQL suite failed: ' + suite)
            results.append(dict(suite=suite, exit_code=0, log_sha256=sha(snapshot / (suite + '.log'))))
            cleanup()
            write(snapshot / 'progress.json', dict(passed=results, remaining=selected[len(results):]))
        frozen()
        write(snapshot / ('result.json' if not selected_suite else 'diagnostic-' + selected_suite + '-result.json'), dict(suites=results, suite_count=len(selected), migrations=c['migrations'],
              complete_original_sql_regression=selected == SUITES,
              production_source_unchanged=True, frozen_source_verified=True,
              binary_sha256=sha(binary), goal_complete=False))
        print(json.dumps({'native_sql_suites': len(selected), 'passed': True, 'complete_original_sql_regression': selected == SUITES}))
    finally:
        try:
            if resources_verified and (not c.get('isolated_mysql') or server is not None and server.poll() is None):
                cleanup()
        finally:
            if c.get('isolated_mysql'):
                if server is not None and server.poll() is None:
                    try:
                        subprocess.run([str(Path(MYSQL).resolve().with_name('mysqladmin')), '--protocol=SOCKET', '--socket=' + c['address'], '--user=root', 'shutdown'], env=env, capture_output=True, timeout=15)
                    except subprocess.TimeoutExpired:
                        pass
                    try:
                        server.wait(timeout=30)
                    except subprocess.TimeoutExpired:
                        server.terminate()
                        server.wait(timeout=15)
                if server is None or server.poll() is not None:
                    if datadir.exists():
                        shutil.rmtree(datadir)
                    socket_dir = Path(c['address']).parent
                    if socket_dir.exists() and not list(socket_dir.iterdir()):
                        socket_dir.rmdir()
                    write(snapshot / 'mysql-cleanup.json', dict(server_terminal=True, data_removed=not datadir.exists(), socket_removed=not Path(c['address']).exists(), main_mysql_unchanged=True))


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('mode', choices=['prepare', 'run'])
    parser.add_argument('--isolated-mysql', action='store_true', help='Use an owned private-socket MySQL with native InnoDB foreign keys; never reconfigure shared MySQL')
    parser.add_argument('--suite', choices=SUITES + ['SEOPublication', 'SEODiscovery', 'SEOCommands'], help='Focused diagnosis only; does not produce a complete23 result')
    args = parser.parse_args()
    if args.mode == 'prepare' and args.suite:
        parser.error('--suite applies only to run')
    if args.isolated_mysql:
        STATE = BASE / 'native-sql-isolated-state.json'
    prepare(args.isolated_mysql) if args.mode == 'prepare' else run(args.suite)
