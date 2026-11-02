"""Manage only this project's persistent, network-disabled local MySQL.

No commands address shared port 3306. Credentials remain in ignored 0600 files.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import time

ROOT = Path(__file__).resolve().parents[2]
BASE = ROOT / 'log/gin-preview-mysql'
SOCKET_DIR = Path('/private/tmp/er-gin-preview')
SOCKET = SOCKET_DIR / 'mysql.sock'
DATABASE = 'eurorefurb_local_trial'
MYSQL = '/opt/homebrew/bin/mysql'
MYSQLD = str(Path(MYSQL).resolve().with_name('mysqld'))
ENV = {k: os.environ[k] for k in ['PATH', 'HOME', 'TMPDIR'] if k in os.environ}


def save(path, data):
    path.write_text(json.dumps(data, indent=2) + '\n')
    path.chmod(0o600)


def config():
    data = json.loads((BASE / 'private.json').read_text())
    if data['datadir'] != str(BASE / 'data') or data['socket'] != str(SOCKET):
        raise RuntimeError('Owned paths differ')
    return data


def query(sql, app=False, database=False):
    c = config()
    run = subprocess.run([MYSQL, '--no-defaults', '--protocol=SOCKET', '--socket=' + str(SOCKET),
                          '-u' + (c['app_user'] if app else 'root'), '-N', '--batch',
                          *([DATABASE] if database else [])], input=sql, text=True,
                         capture_output=True, timeout=45,
                         env={**ENV, 'MYSQL_PWD': c['app_password'] if app else c['root_password']})
    if run.returncode:
        raise RuntimeError('Owned MySQL command failed; private output suppressed')
    return [row.split('\t') for row in run.stdout.splitlines()]


def status():
    c = config()
    rows = query('SELECT @@datadir,@@innodb_native_foreign_keys,@@foreign_key_checks,@@skip_networking,@@event_scheduler;')
    if len(rows) != 1 or Path(rows[0][0]) != Path(c['datadir']) or rows[0][1:] != ['1', '1', '1', 'OFF']:
        raise RuntimeError('Owned MySQL runtime differs')
    return {'running': True, 'datadir': c['datadir'], 'socket': c['socket'],
            'native_foreign_keys': True, 'networking_disabled': True, 'event_scheduler_disabled': True,
            'pid': int((BASE / 'mysql.pid').read_text().strip())}


def start():
    c = config()
    if SOCKET.exists():
        return status()
    SOCKET_DIR.mkdir(mode=0o700, exist_ok=True)
    if SOCKET_DIR.stat().st_uid != os.getuid() or SOCKET_DIR.stat().st_mode & 0o077:
        raise RuntimeError('Private socket directory ownership/permissions differ')
    command = [MYSQLD, '--no-defaults', '--datadir=' + c['datadir'], '--socket=' + str(SOCKET),
               '--pid-file=' + str(BASE / 'mysql.pid'), '--log-error=' + str(BASE / 'mysql.log'),
               '--skip-networking', '--mysqlx=OFF', '--skip-log-bin', '--event-scheduler=OFF',
               '--innodb-buffer-pool-size=64M', '--innodb-redo-log-capacity=64M',
               '--innodb-native-foreign-keys=ON']
    with (BASE / 'launch.log').open('ab') as output:
        server = subprocess.Popen(command, env=ENV, stdout=output, stderr=subprocess.STDOUT,
                                  stdin=subprocess.DEVNULL, start_new_session=True)
    deadline = time.monotonic() + 45
    while time.monotonic() < deadline:
        if server.poll() is not None:
            raise RuntimeError('Owned MySQL exited; inspect local log')
        if SOCKET.exists():
            try:
                return status()
            except RuntimeError:
                pass
        time.sleep(0.2)
    raise RuntimeError('Owned MySQL readiness timed out')


def stop():
    before = status()  # Prove the target's data directory before shutdown.
    c = config()
    run = subprocess.run([str(Path(MYSQLD).with_name('mysqladmin')), '--no-defaults',
                          '--protocol=SOCKET', '--socket=' + str(SOCKET), '-uroot', 'shutdown'],
                         env={**ENV, 'MYSQL_PWD': c['root_password']}, capture_output=True, timeout=30)
    if run.returncode:
        raise RuntimeError('Owned MySQL shutdown failed')
    deadline = time.monotonic() + 30
    while time.monotonic() < deadline:
        if not SOCKET.exists() and not (BASE / 'mysql.pid').exists():
            return {'stopped': True, 'previous_pid': before['pid'], 'data_retained': True}
        time.sleep(0.2)
    raise RuntimeError('Owned MySQL shutdown not proven')


def init():
    if BASE.exists() or SOCKET.exists():
        raise RuntimeError('Refusing to replace existing owned MySQL')
    if shutil.disk_usage(ROOT).free < 2 * 1024**3:
        raise RuntimeError('2 GiB free required')
    BASE.mkdir(mode=0o700)
    datadir = BASE / 'data'
    datadir.mkdir(mode=0o700)
    # Empty root is allowed only during this private, no-network bootstrap.
    c = {'datadir': str(datadir), 'socket': str(SOCKET), 'root_password': '',
         'app_user': 'gin_preview', 'app_password': secrets.token_hex(32)}
    save(BASE / 'private.json', c)
    run = subprocess.run([MYSQLD, '--no-defaults', '--initialize-insecure',
                          '--datadir=' + str(datadir), '--log-error=' + str(BASE / 'initialize.log'),
                          '--innodb-buffer-pool-size=64M', '--innodb-redo-log-capacity=64M'],
                         env=ENV, capture_output=True, timeout=90)
    if run.returncode:
        raise RuntimeError('Owned MySQL initialization failed')
    start()
    root_password = secrets.token_hex(32)
    try:
        query("ALTER USER 'root'@'localhost' IDENTIFIED BY '" + root_password + "';")
        c['root_password'] = root_password
        save(BASE / 'private.json', c)
    except BaseException:
        # Persist the intended password for recovery if ALTER committed before
        # the client lost its response; never expose it in the error output.
        c['root_password'] = root_password
        save(BASE / 'private.json', c)
        raise
    return status()


def restore(backup, binary):
    status()
    c = config()
    evidence = json.loads((backup.parent / 'report.json').read_text())
    if not backup.resolve().is_relative_to(ROOT / 'logs/gin-migration') or backup.name != 'preview-86-private.sql':
        raise RuntimeError('Only the audited local backup is allowed')
    if hashlib.sha256(backup.read_bytes()).hexdigest() != evidence['backup']['sha256']:
        raise RuntimeError('Backup digest differs')
    if hashlib.sha256(binary.read_bytes()).hexdigest() != evidence['migration_binary_sha256']:
        raise RuntimeError('Tested migration binary required')
    if query("SELECT schema_name FROM information_schema.schemata WHERE schema_name='" + DATABASE + "';"):
        raise RuntimeError('Refusing to restore over an existing preview database')
    query('CREATE DATABASE `' + DATABASE + '` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;')
    with backup.open('rb') as inp:
        run = subprocess.run([MYSQL, '--no-defaults', '--protocol=SOCKET', '--socket=' + str(SOCKET),
                              '-uroot', DATABASE], stdin=inp, capture_output=True,
                             env={**ENV, 'MYSQL_PWD': c['root_password']}, timeout=180)
    if run.returncode:
        raise RuntimeError('Owned restore failed; private output suppressed')
    for i, expected in enumerate(['Applied 13, skipped 86 migrations', 'Applied 0, skipped 99 migrations']):
        run = subprocess.run([str(binary)], cwd=BASE, env={**ENV, 'MYSQL_DSN':
              'root:' + c['root_password'] + '@unix(' + str(SOCKET) + ')/' + DATABASE + '?parseTime=true&charset=utf8mb4'},
              capture_output=True, text=True, timeout=600)
        (BASE / ('migration-' + str(i + 1) + '.log')).write_text(run.stdout + run.stderr)
        if run.returncode or run.stdout.strip() != expected:
            raise RuntimeError('Owned migration failed; inspect generic log')
    query("CREATE USER '" + c['app_user'] + "'@'localhost' IDENTIFIED BY '" + c['app_password'] + "';")
    query("GRANT SELECT,INSERT,UPDATE,DELETE,SHOW VIEW ON `" + DATABASE + "`.* TO '" + c['app_user'] + "'@'localhost';")
    if query('SELECT COUNT(*) FROM schema_migrations;', app=True, database=True) != [['99']]:
        raise RuntimeError('Application connection readiness failed')
    dsn = c['app_user'] + ':' + c['app_password'] + '@unix(' + str(SOCKET) + ')/' + DATABASE + '?parseTime=true&charset=utf8mb4'
    (BASE / 'application.env').write_text('MYSQL_DSN=' + dsn + '\n')
    (BASE / 'application.env').chmod(0o600)
    result = {'database': DATABASE, 'backup_sha256': evidence['backup']['sha256'],
              'migrations': 99, 'applied': 13, 'idempotent_skipped': 99,
              'application_global_privileges': False, 'application_schema_ddl_privileges': False,
              'application_connection_verified': True, 'main_shared_database_modified': False,
              'application_started': False}
    save(BASE / 'restore.json', result)
    return result


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('mode', choices=['init', 'start', 'stop', 'status', 'restore'])
    parser.add_argument('--backup', type=Path)
    parser.add_argument('--migrate', type=Path)
    args = parser.parse_args()
    if args.mode == 'restore':
        if not args.backup or not args.migrate:
            parser.error('restore requires --backup and --migrate')
        result = restore(args.backup.resolve(), args.migrate.resolve())
    else:
        result = {'init': init, 'start': start, 'stop': stop, 'status': status}[args.mode]()
    print(json.dumps(result))
