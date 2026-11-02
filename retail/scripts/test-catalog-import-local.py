"""Pinned historical catalog on an owned native MySQL; no shared target or credentials."""
import hashlib
import json
import os
from pathlib import Path
import secrets
import shutil
import socket
import subprocess
import tempfile
import time

ROOT = Path(__file__).resolve().parents[2]
MYSQL = Path('/opt/homebrew/bin/mysql').resolve()
GO = '/opt/homebrew/opt/go/bin/go'
SOURCE = ROOT / 'logs/gin-migration/source-native-20261031-144745.ndjson'
SOURCE_SHA = '3b8c007f7854ab61b46009df87ba8f3a082dacfb3bf03078b0c16dcede367aab'


def sha(p):
    return hashlib.sha256(p.read_bytes()).hexdigest()


def write(p, value):
    p.write_text(json.dumps(value, indent=2) + '\n')
    p.chmod(0o600)


def main():
    if sha(SOURCE) != SOURCE_SHA or shutil.disk_usage(ROOT).free < 2 * 1024**3:
        raise RuntimeError('Pinned source and 2 GiB free required')
    owner = secrets.token_hex(6)
    directory = ROOT / 'logs/gin-migration' / ('catalog-native-98-' + owner)
    directory.mkdir(mode=0o700)
    shutil.copyfile(Path(__file__), directory / 'runner.py')
    backend = directory / 'retail/backend'
    shutil.copytree(ROOT / 'retail/backend', backend, ignore=shutil.ignore_patterns(
        '.env*', '*.log', 'node_modules', 'dist', '__pycache__'))
    files = {str(p.relative_to(backend)): sha(p) for p in backend.rglob('*') if p.is_file()}
    def frozen():
        for n, h in files.items():
            if sha(backend / n) != h or sha(ROOT / 'retail/backend' / n) != h:
                raise RuntimeError('Source changed during verification')
    frozen()
    env = {k: os.environ[k] for k in ['PATH', 'HOME', 'TMPDIR'] if k in os.environ}
    for name, args in [('migrate', ['./cmd/migrate']), ('import', ['./cmd/import-catalog']),
                       ('catalog.test', ['-test', './internal/catalogimport'])]:
        command = [GO, 'test', '-c', '-o', str(directory / name), args[1]] if args[0] == '-test' else [GO, 'build', '-o', str(directory / name), *args]
        with (directory / (name + '-compile.log')).open('w') as out:
            subprocess.run(command, cwd=backend, env=env, stdout=out, stderr=subprocess.STDOUT, check=True, timeout=120)
    sockdir = Path(tempfile.mkdtemp(prefix='ercatalog-' + owner + '-', dir='/private/tmp'))
    address = sockdir / 'mysql.sock'
    data = directory / 'mysql-data'
    with socket.socket() as listener:
        listener.bind(('127.0.0.1', 0))
        port = listener.getsockname()[1]
    database = 'eurorefurb_catalog_' + owner
    password = secrets.token_hex(24)
    write(directory / 'manifest.json', dict(owner=owner, source_sha256=SOURCE_SHA,
          backend_source_sha256=files, migrations=98, database=database, port=port,
          runner_sha256=sha(directory / 'runner.py'),
          binaries={n: sha(directory / n) for n in ['migrate', 'import', 'catalog.test']}))
    server = None
    control_env = dict(env)
    def mysql(sql):
        r = subprocess.run([str(MYSQL), '--no-defaults', '--protocol=SOCKET', '--socket=' + str(address),
                           '--user=root', '--batch', '--skip-column-names', '--binary-as-hex'], input=sql,
                           text=True, capture_output=True, env=control_env, timeout=45)
        if r.returncode:
            raise RuntimeError('Owned SQL control failed; private output suppressed')
        return r.stdout.strip()
    def run(name, command, run_env):
        with (directory / (name + '.log')).open('w') as out:
            r = subprocess.run(command, cwd=backend, env=run_env, stdout=out, stderr=subprocess.STDOUT, timeout=300)
        if r.returncode:
            raise RuntimeError('Catalog verification failed: ' + name + '; private output retained locally')
    def target_fingerprint():
        tables = mysql("SELECT table_name FROM information_schema.tables WHERE table_schema='" + database + "' AND table_type='BASE TABLE' ORDER BY table_name").splitlines()
        digest = hashlib.sha256()
        for table in tables:
            if not table.replace('_', '').isalnum():
                raise RuntimeError('Unexpected owned table name')
            rows = mysql('SELECT * FROM `' + database + '`.`' + table + '` ORDER BY 1').encode()
            digest.update(table.encode() + b'\0' + len(rows).to_bytes(8, 'big') + rows)
        return digest.digest()
    try:
        data.mkdir(mode=0o700)
        subprocess.run([str(MYSQL.with_name('mysqld')), '--no-defaults', '--initialize-insecure',
                        '--datadir=' + str(data), '--innodb-buffer-pool-size=64M', '--innodb-redo-log-capacity=64M',
                        '--log-error=' + str(directory / 'mysql-initialize.log')], env=env, capture_output=True, check=True, timeout=90)
        with (directory / 'mysql-launch.log').open('w') as out:
            server = subprocess.Popen([str(MYSQL.with_name('mysqld')), '--no-defaults', '--datadir=' + str(data),
                '--socket=' + str(address), '--pid-file=' + str(directory / 'mysql.pid'),
                '--log-error=' + str(directory / 'mysql-server.log'), '--bind-address=127.0.0.1', '--port=' + str(port),
                '--mysqlx=OFF', '--skip-log-bin', '--innodb-buffer-pool-size=64M', '--innodb-redo-log-capacity=64M',
                '--innodb-native-foreign-keys=ON'], env=env, stdout=out, stderr=subprocess.STDOUT)
        deadline = time.monotonic() + 45
        while time.monotonic() < deadline:
            if server.poll() is not None:
                raise RuntimeError('Owned MySQL exited')
            if address.exists():
                try:
                    mysql('SELECT 1')
                    break
                except RuntimeError:
                    pass
            time.sleep(0.2)
        else:
            raise RuntimeError('Owned MySQL readiness timed out')
        mysql("ALTER USER 'root'@'localhost' IDENTIFIED BY '" + password + "'")
        control_env['MYSQL_PWD'] = password
        mysql('CREATE DATABASE `' + database + '` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin')
        facts = mysql('SELECT VERSION(),@@innodb_native_foreign_keys,@@foreign_key_checks,@@bind_address,@@port').split('\t')
        assert facts[1:4] == ['1', '1', '127.0.0.1'] and int(facts[4]) == port
        write(directory / 'mysql-runtime.json', dict(version=facts[0], native_foreign_keys=True, foreign_key_checks=True, loopback_only=True))
        dsn = f'root:{password}@tcp(127.0.0.1:{port})/{database}?charset=utf8mb4&parseTime=true&loc=UTC'
        test_env = dict(env, MYSQL_DSN=dsn, NATIVE_CATALOG_DSN=dsn, CATALOG_NATIVE_REHEARSAL='1',
                        NATIVE_CATALOG_SNAPSHOT=str(SOURCE), NATIVE_CATALOG_SHA256=SOURCE_SHA)
        run('migrate', [str(directory / 'migrate')], test_env)
        assert 'Applied 98, skipped 0' in (directory / 'migrate.log').read_text()
        before_rollback = target_fingerprint()
        cli = [str(directory / 'import'), '--snapshot', str(SOURCE), '--sha256', SOURCE_SHA, '--target-database', database]
        run('cli-rollback', cli + ['--rollback'], test_env)
        assert target_fingerprint() == before_rollback
        assert mysql('SELECT COUNT(*) FROM `' + database + '`.catalog_products') == '0'
        run('cli-apply', cli + ['--apply'], test_env)
        run('replay-and-conflict', [str(directory / 'catalog.test'), '-test.v', '-test.run', '^TestNativeCatalogReplayAndConflictRollback$'], test_env)
        replay_log = (directory / 'replay-and-conflict.log').read_text()
        assert '--- PASS: TestNativeCatalogReplayAndConflictRollback' in replay_log and '--- SKIP' not in replay_log
        report = json.loads((directory / 'cli-apply.log').read_text())
        assert report['applied'] and report['catalog_and_dependency_tables'] == 46 and report['catalog_and_dependency_rows'] == 17821
        assert mysql("SELECT COUNT(*) FROM information_schema.triggers WHERE trigger_schema='" + database + "' AND trigger_name LIKE 'catalog_option_%_unique'") == '4'
        counts = mysql('SELECT (SELECT COUNT(*) FROM `' + database + '`.catalog_options),(SELECT COUNT(*) FROM `' + database + '`.catalog_option_values)')
        assert counts.split('\t')[0] == '163'
        credentials = mysql('SELECT (SELECT COUNT(*) FROM `' + database + '`.admin_users),(SELECT COUNT(*) FROM `' + database + '`.customer_accounts),(SELECT COUNT(*) FROM `' + database + '`.admin_sessions)')
        assert credentials == '0\t0\t0'
        frozen()
        write(directory / 'result.json', dict(owner=owner, migrations=98, historical_catalog=report,
              option_and_value_counts=counts, rollback_empty=True, rollback_all_target_fields_unchanged=True,
              exact_replay_and_conflict_rollback=True,
              migration_triggers_installed=True, frozen_source_verified=True, real_payments=False,
              old_credentials_imported=False, login_account_and_session_counts=credentials,
              goal_complete=False))
        print(json.dumps(dict(owner=owner, historical_catalog_98='passed')), flush=True)
    finally:
        if server is not None and server.poll() is None:
            try:
                subprocess.run([str(MYSQL.with_name('mysqladmin')), '--no-defaults', '--protocol=SOCKET',
                                '--socket=' + str(address), '--user=root', 'shutdown'], env=control_env, capture_output=True, timeout=20)
            except subprocess.TimeoutExpired:
                pass
            try:
                server.wait(timeout=30)
            except subprocess.TimeoutExpired:
                server.terminate()
                server.wait(timeout=15)
        if server is None or server.poll() is not None:
            if data.exists():
                shutil.rmtree(data)
            if sockdir.exists() and not list(sockdir.iterdir()):
                sockdir.rmdir()
            write(directory / 'cleanup.json', dict(server_terminal=True, data_removed=not data.exists(),
                  socket_removed=not address.exists(), main_mysql_unchanged=True))


if __name__ == '__main__':
    main()
