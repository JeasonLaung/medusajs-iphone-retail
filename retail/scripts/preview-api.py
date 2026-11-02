"""Manage the single, audited local Gin preview without changing its config.

Never builds code, changes gates, starts workers or addresses shared MySQL.
"""
import argparse
import hashlib
import importlib.util
import json
from pathlib import Path
import re


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


HERE = Path(__file__).resolve().parent
api = load('preview_cutover', HERE / 'switch-local-preview.py')
mysql = load('preview_mysql', HERE / 'preview-mysql.py')


def configuration():
    facts = json.loads((api.BASE / 'cutover.json').read_text())
    if not facts.get('switched') or facts.get('schema_migrations') not in (99, 100, 101, 102):
        raise RuntimeError('Successful audited preview cutover required')
    if facts['schema_migrations'] in (100, 101, 102):
        count = facts['schema_migrations']
        upgrade = facts.get('schema_upgrade', {})
        owner = upgrade.get('owner', '')
        if not re.fullmatch('[a-f0-9]{12}', owner):
            raise RuntimeError('Audited restore rehearsal required')
        path = api.ROOT / 'logs/gin-migration' / ('preview-' + str(count) + '-upgrade-' + owner) / 'report.json'
        if hashlib.sha256(path.read_bytes()).hexdigest() != upgrade.get('report_sha256'):
            raise RuntimeError('Audited restore evidence changed')
        evidence = json.loads(path.read_text())
        if not evidence.get('rehearsal_passed') or evidence.get('idempotent_skipped') != count or not evidence['cleanup']['server_terminal']:
            raise RuntimeError('Complete restore rehearsal required')
        if count == 101 and (not evidence.get('rollback_rehearsed_exact_data_ddl_objects') or not evidence.get('recovery_reapply101_idempotent') or not evidence.get('existing_role_grants_unchanged')):
            raise RuntimeError('101 recovery and grant preservation proof required')
        if count == 102 and (not evidence.get('rollback_rehearsed_exact_data_ddl_objects') or not evidence.get('recovery_reapply102_idempotent') or not evidence.get('existing_role_grants_unchanged') or not evidence.get('recovery_activated_gate_refused')):
            raise RuntimeError('102 recovery and original fact preservation proof required')
    if hashlib.sha256(api.NEW.read_bytes()).hexdigest() != facts['new_binary_sha256']:
        raise RuntimeError('Audited preview binary changed; validate before replacing it')
    values = api.parse_env(api.ENVFILE.read_text())
    expected = api.parse_env((api.BASE / 'application.env').read_text())
    if values.get('MYSQL_DSN') != expected['MYSQL_DSN']:
        raise RuntimeError('Preview connection differs; refusing to start another database target')
    if values.get('APP_PORT') != '9100' or values.get('APP_BIND') != '127.0.0.1':
        raise RuntimeError('Preview listener differs')
    return values


def schema_status():
    facts = json.loads((api.BASE / 'cutover.json').read_text())
    count = facts['schema_migrations']
    if mysql.query('SELECT COUNT(*) FROM schema_migrations;', app=True, database=True) != [[str(count)]]:
        raise RuntimeError('Preview schema differs')
    if count in (100, 101, 102):
        path = api.ROOT / 'logs/gin-migration' / ('preview-' + str(count) + '-upgrade-' + facts['schema_upgrade']['owner']) / 'report.json'
        expected = json.loads(path.read_text())['source_migrations_sha256']
        if dict(mysql.query('SELECT version,checksum FROM schema_migrations ORDER BY version;', app=True, database=True)) != expected:
            raise RuntimeError('Preview migration checksums differ')
    return count


def status():
    configuration()
    mysql.status()
    count = schema_status()
    pid = api.listener()
    if pid is None:
        return {'running': False, 'database_running': True, 'schema_migrations': count}
    if api.executable(pid) != api.NEW:
        raise RuntimeError('Port 9100 belongs to a different executable')
    with api.urllib.request.urlopen('http://127.0.0.1:9100/health', timeout=5) as response:
        if response.status != 200:
            raise RuntimeError('Preview API not ready')
    return {'running': True, 'pid': pid, 'health': 200, 'schema_migrations': count}


def start():
    values = configuration()
    # Verify an existing listener before starting dependencies or a new process.
    pid = api.listener()
    if pid is not None:
        return status()
    mysql.start()
    schema_status()
    proc = api.launch(api.NEW, values, 'api.log')
    api.save('api-process.json', {'pid': proc.pid, 'binary': str(api.NEW)})
    return status()


def stop():
    configuration()
    pid = api.listener()
    if pid is not None:
        api.stop(pid, api.NEW)
    return {'stopped': True, 'previous_pid': pid, 'mysql_and_data_retained': True}


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('mode', choices=['start', 'stop', 'status', 'restart'])
    args = parser.parse_args()
    if args.mode == 'restart':
        stop()
        result = start()
    else:
        result = {'start': start, 'stop': stop, 'status': status}[args.mode]()
    print(json.dumps(result))
