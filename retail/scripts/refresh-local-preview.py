"""Refresh the single audited API from a fully verified frozen SQL rehearsal.

No migrations, config changes, workers or second application process. Retains
the previous binary and cutover facts, and restores both on a failed readiness.
"""
import argparse
import hashlib
import importlib.util
import json
from pathlib import Path
import re
import shutil
import subprocess

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('preview_manager', HERE / 'preview-api.py')
manager = importlib.util.module_from_spec(spec)
spec.loader.exec_module(manager)
api = manager.api


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def refresh(owner, previous_owner):
    if not all(re.fullmatch('[a-f0-9]{12}', value) for value in [owner, previous_owner]):
        raise RuntimeError('Expected an explicit frozen rehearsal owner')
    snapshot = api.ROOT / 'logs/gin-migration' / ('native-sql-local-' + owner)
    facts_path = api.BASE / 'cutover.json'
    facts_bytes = facts_path.read_bytes()
    facts = json.loads(facts_bytes)
    count = facts.get('schema_migrations')
    if count not in (99, 100, 101):
        raise RuntimeError('Audited local99/100/101 preview required')
    manifest = json.loads((snapshot / 'manifest.json').read_text())
    result = json.loads((snapshot / 'result.json').read_text())
    cleanup = json.loads((snapshot / 'mysql-cleanup.json').read_text())
    resources = json.loads((snapshot / 'cleanup.json').read_text())
    if (manifest['owner'] != owner or manifest['migrations'] != count or
            result['suite_count'] != 23 or not result['complete_original_sql_regression'] or
            not result['frozen_source_verified'] or not cleanup['server_terminal'] or
            not cleanup['data_removed'] or not resources['owned_databases_absent'] or
            not resources['redis_prefix_cleaned']):
        raise RuntimeError('Complete, cleaned regression matching the running schema required')
    for row in result['suites']:
        if row['exit_code'] != 0 or sha(snapshot / (row['suite'] + '.log')) != row['log_sha256']:
            raise RuntimeError('SQL regression evidence changed')
    if sha(snapshot / 'identity.test') != result['binary_sha256']:
        raise RuntimeError('Regression binary changed')
    for name, digest in manifest['source_sha256'].items():
        frozen_digest = manifest['adapted_test_sha256'].get(name, digest)
        if sha(snapshot / name) != frozen_digest:
            raise RuntimeError('Frozen source changed: ' + name)
        if name.startswith('retail/backend/') and sha(api.ROOT / name) != digest:
            raise RuntimeError('Current backend changed after freeze: ' + name)
    expected = {name for name in manifest['source_sha256'] if name.startswith('retail/backend/')}
    present = {str(p.relative_to(api.ROOT)) for p in api.BACKEND.rglob('*') if p.is_file()
               and not any(part in ['node_modules', 'dist', '__pycache__', '.medusa'] for part in p.relative_to(api.BACKEND).parts)
               and not p.name.startswith('.env') and not p.name.endswith('.log')}
    if present != expected:
        raise RuntimeError('Current backend inventory changed after freeze')
    latest = facts.get('latest_binary_refresh', {})
    current_owner = latest.get('owner', previous_owner)
    if count in (100, 101) and latest.get('schema_migrations') != count:
        current_owner = facts.get('schema_upgrade', {}).get('frozen_source_owner')
    if current_owner != previous_owner:
        raise RuntimeError('Previous runtime owner differs')
    previous_manifest = json.loads((api.ROOT / 'logs/gin-migration' / ('native-sql-local-' + previous_owner) / 'manifest.json').read_text())
    migrations = lambda m: {n: s for n, s in m['source_sha256'].items() if n.startswith('retail/backend/migrations/')}
    if migrations(previous_manifest) != migrations(manifest):
        raise RuntimeError('Binary-only refresh cannot change migrations')
    initial = manager.status()
    env_digest = sha(api.ENVFILE)
    if not initial['running']:
        raise RuntimeError('A healthy owned preview must be running')
    backup = api.BASE / ('binary-refresh-' + owner)
    backup.mkdir(mode=0o700)
    previous = backup / 'previous-api'
    shutil.copy2(api.NEW, previous)
    (backup / 'previous-cutover.json').write_bytes(facts_bytes)
    (backup / 'previous-cutover.json').chmod(0o600)
    candidate = backup / 'candidate-api'
    with (backup / 'compile.log').open('w') as output:
        subprocess.run(['/opt/homebrew/opt/go/bin/go', 'build', '-trimpath', '-o', str(candidate), './cmd/api'],
                       cwd=snapshot / 'retail/backend', stdout=output, stderr=subprocess.STDOUT, check=True)
    if sha(api.NEW) != facts['new_binary_sha256'] or facts_path.read_bytes() != facts_bytes or sha(api.ENVFILE) != env_digest:
        raise RuntimeError('Preview changed while building; refusing to stop')
    record = {'owner': owner, 'previous_owner': previous_owner, 'previous_pid': initial['pid'], 'previous_binary_sha256': sha(previous),
              'new_binary_sha256': sha(candidate), 'sql_result_sha256': sha(snapshot / 'result.json'),
              'schema_migrations': count, 'configuration_changed': False, 'schema_changed': False}
    try:
        manager.stop()
        shutil.copy2(candidate, api.NEW)
        facts['new_binary_sha256'] = record['new_binary_sha256']
        facts['latest_binary_refresh'] = {'owner': owner, 'schema_migrations': count, 'previous_binary_sha256': record['previous_binary_sha256']}
        api.save('cutover.json', facts)
        current = manager.start()
        record.update(success=True, new_pid=current['pid'], health=current['health'])
        api.save('binary-refresh-' + owner + '/result.json', record)
    except BaseException:
        pid = api.listener()
        if pid is not None:
            api.stop(pid, api.NEW)
        shutil.copy2(previous, api.NEW)
        facts_path.write_bytes(facts_bytes)
        facts_path.chmod(0o600)
        restored = manager.start()
        record.update(success=False, rollback_health=restored['health'])
        api.save('binary-refresh-' + owner + '/result.json', record)
        raise
    return record


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--owner', required=True)
    parser.add_argument('--previous-owner', required=True)
    args = parser.parse_args()
    print(json.dumps(refresh(args.owner, args.previous_owner)))
