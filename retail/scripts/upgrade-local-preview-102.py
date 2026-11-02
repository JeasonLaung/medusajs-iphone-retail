"""Upgrade the audited private101 preview using tested102 and clone recovery proof.

Keeps configuration/grants/gates intact. Stops only the sole Gin API before a
fresh backup/recovery rehearsal. Never starts an application on the clone.
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


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


manager = load('manager102', HERE / 'preview-api.py')
verify = load('verify102', HERE / 'verify-local-preview.py')
recovery = load('recovery102', HERE / 'cart-102-recovery.py')
controller = load('sql102', HERE / 'test-native-sql-local.py')
m, api = manager.mysql, manager.api
sha = lambda path: hashlib.sha256(path.read_bytes()).hexdigest()


def require(ok, message):
    if not ok:
        raise RuntimeError(message)


def table_ddl(path):
    definitions = {}
    for match in re.finditer(rb'CREATE TABLE `([a-zA-Z0-9_]+)` \(.*?\) ENGINE=[^;]+;', path.read_bytes(), re.S):
        canonical = re.sub(rb' CHARACTER SET (\w+) COLLATE (\w+)',
            lambda x: b' COLLATE '+x[2] if x[2].startswith(x[1]+b'_') else x[0], match[0])
        definitions[match[1].decode()] = hashlib.sha256(canonical).hexdigest()
    return definitions


def validate_rehearsal(owner, source_owner):
    require(re.fullmatch('[a-f0-9]{12}', owner) is not None, 'Explicit recovery rehearsal owner required')
    base = api.ROOT / 'logs/gin-migration' / ('preview-102-upgrade-' + owner)
    evidence = json.loads((base / 'report.json').read_text())
    require(evidence['frozen_source_owner'] == source_owner and evidence.get('rehearsal_passed') and evidence.get('rollback_rehearsed_exact_data_ddl_objects') and evidence.get('recovery_reapply102_idempotent') and evidence.get('recovery_activated_gate_refused'), 'Complete102 upgrade/recovery/reapply rehearsal required')
    require(evidence['idempotent_skipped'] == 102 and evidence['existing_role_grants_unchanged'] and evidence['new_gates_default_closed'] and evidence['new_tables'] == ['cart_context_operations','cart_context_runtime'], '102 role/gate preservation proof required')
    require(all(evidence['cleanup'][k] for k in ['server_terminal','data_removed','socket_removed','private_backup_retained']), 'Rehearsal resources must be cleaned')
    require(sha(base / 'preview-101-private.sql') == evidence['backup']['sha256'] and sha(base / 'recovery-authority.json') == evidence['recovery_authority_sha256'] and sha(base / 'migrate') == evidence['migration_binary_sha256'], 'Rehearsal backup/authority/binary changed')
    require(sha(HERE / 'cart-102-recovery.py') == evidence['recovery_helper_sha256'], 'Recovery helper differs from tested version')
    return base, evidence


def main(owner, recovery_owner):
    require(re.fullmatch('[a-f0-9]{12}', owner) is not None, 'Explicit frozen source owner required')
    status = manager.status()
    require(status['running'] and status['schema_migrations'] == 101, 'Healthy audited101 preview required')
    frozen = api.ROOT / 'logs/gin-migration' / ('native-sql-local-' + owner)
    manifest = json.loads((frozen / 'manifest.json').read_text())
    sql = json.loads((frozen / 'result.json').read_text())
    cleanup = json.loads((frozen / 'mysql-cleanup.json').read_text())
    resources = json.loads((frozen / 'cleanup.json').read_text())
    require(manifest['owner'] == owner and manifest['migrations'] == 102 and sql['migrations'] == 102 and sql['suite_count'] == 23 and sql['complete_original_sql_regression'] and sql['frozen_source_verified'], 'Complete tested102 original23 SQL required')
    require([r['suite'] for r in sql['suites']] == controller.SUITES and all(r['exit_code'] == 0 and sha(frozen / (r['suite']+'.log')) == r['log_sha256'] for r in sql['suites']), 'Complete original SQL logs required')
    require(all(cleanup[k] for k in ['server_terminal','data_removed','socket_removed','main_mysql_unchanged']) and resources['owned_databases_absent'] and resources['redis_prefix_cleaned'], 'SQL resources must be cleaned')
    require(sha(frozen / 'identity.test') == sql['binary_sha256'], 'SQL test binary changed')
    expected = {name for name in manifest['source_sha256'] if name.startswith('retail/backend/')}
    present = {str(p.relative_to(api.ROOT)) for p in api.BACKEND.rglob('*') if p.is_file()
        and not any(part in ['node_modules','dist','__pycache__','.medusa'] for part in p.relative_to(api.BACKEND).parts)
        and not p.name.startswith('.env') and not p.name.endswith('.log')}
    require(expected == present, 'Current backend inventory changed')
    for name, digest in manifest['source_sha256'].items():
        require(sha(frozen / name) == manifest['adapted_test_sha256'].get(name, digest), 'Frozen source changed')
        if name in expected:
            require(sha(api.ROOT / name) == digest, 'Current backend changed')
    validate_rehearsal(recovery_owner, owner)
    facts_path = api.BASE / 'cutover.json'
    previous_facts = facts_path.read_bytes()
    old_facts = json.loads(previous_facts)
    env_sha = sha(api.ENVFILE)
    scripts = {name:sha(HERE / name) for name in ['preview-api.py','verify-local-preview.py','cart-102-recovery.py','rehearse-local-preview-102.py','upgrade-local-preview-102.py']}
    base = api.BASE / 'schema-upgrade-102'
    base.mkdir(mode=0o700)
    (base / 'previous-cutover.json').write_bytes(previous_facts)
    (base / 'previous-cutover.json').chmod(0o600)
    previous = base / 'previous-api'
    shutil.copy2(api.NEW, previous)
    candidate = base / 'candidate-api'
    with (base / 'compile.log').open('w') as out:
        subprocess.run(['/opt/homebrew/opt/go/bin/go','build','-trimpath','-o',str(candidate),'./cmd/api'], cwd=frozen / 'retail/backend', env=m.ENV, stdout=out, stderr=subprocess.STDOUT, check=True, timeout=120)
    require(sha(api.NEW) == old_facts['new_binary_sha256'] and facts_path.read_bytes() == previous_facts and sha(api.ENVFILE) == env_sha, 'Main preview changed during build')
    report = {'previous_pid':status['pid'],'previous_binary_sha256':sha(previous),'new_binary_sha256':sha(candidate),'frozen_source_owner':owner,'sql_result_sha256':sha(frozen / 'result.json'),'prior_recovery_owner':recovery_owner,'old_schema':101,'target_schema':102,'configuration_changed':False,'gates_changed':False,'role_grants_changed':False,'shared_mysql_modified':False,'real_provider_called':False,'goal_complete':False}
    stopped = False
    migration_attempted = False
    rehearsal_base = None
    c = m.config()

    def full_dump(name):
        path = base / name
        with path.open('xb') as output:
            path.chmod(0o600)
            run = subprocess.run(['/opt/homebrew/bin/mysqldump','--no-defaults','--protocol=SOCKET','--socket='+str(m.SOCKET),'-uroot','--single-transaction','--skip-lock-tables','--no-tablespaces','--set-gtid-purged=OFF','--hex-blob','--order-by-primary','--skip-extended-insert','--skip-comments','--default-character-set=utf8mb4','--routines','--events','--triggers',m.DATABASE], stdout=output, stderr=subprocess.PIPE, env={**m.ENV,'MYSQL_PWD':c['root_password']}, timeout=120)
        require(run.returncode == 0 and path.stat().st_size > 0, 'Private verification dump failed; output suppressed')
        return path

    def names():
        return [r[0] for r in m.query("SELECT table_name FROM information_schema.tables WHERE table_schema=DATABASE() AND table_type='BASE TABLE' ORDER BY table_name;", database=True)]

    try:
        manager.stop()
        stopped = True
        with (base / 'fresh-rehearsal.log').open('w') as out:
            run = subprocess.run(['python3',str(HERE / 'rehearse-local-preview-102.py'),'--source-owner',owner], stdout=out, stderr=subprocess.STDOUT, timeout=600)
        require(run.returncode == 0, 'Final stopped-writer recovery rehearsal failed')
        events = [json.loads(line) for line in (base / 'fresh-rehearsal.log').read_text().splitlines() if line.startswith('{')]
        require(bool(events), 'Fresh rehearsal identity missing')
        rehearsal_owner = events[0]['owner']
        rehearsal_base, evidence = validate_rehearsal(rehearsal_owner, owner)
        require(all(sha(HERE / name) == digest for name,digest in scripts.items()) and sha(api.ENVFILE) == env_sha, 'Management/configuration changed during rehearsal')
        report['rehearsal_owner'] = rehearsal_owner
        for index, want in enumerate(['Applied 1, skipped 101 migrations','Applied 0, skipped 102 migrations']):
            migration_attempted = True
            run = subprocess.run([str(rehearsal_base / 'migrate')], cwd=base, env={**m.ENV,'MYSQL_DSN':'root:'+c['root_password']+'@unix('+str(m.SOCKET)+')/'+m.DATABASE+'?parseTime=true&charset=utf8mb4'}, capture_output=True, text=True, timeout=180)
            (base / ('migration-'+str(index+1)+'.log')).write_text(run.stdout+run.stderr)
            require(run.returncode == 0 and run.stdout.strip() == want, 'Main102 explicit migration failed')
        expected = json.loads((rehearsal_base / 'migrated-fingerprints.json').read_text())
        current_names = names()
        require(set(current_names) == set(expected), 'Main102 table inventory differs')
        post = full_dump('post-upgrade-private.sql')
        actual = verify.fingerprints(post, current_names)
        require(all(actual[n] == expected[n] for n in expected if n not in ['schema_migrations','schema_migration_steps']), 'Main102 data differs from stopped-writer rehearsal')
        require(table_ddl(post) == table_ddl(rehearsal_base / 'reapplied-102-private.sql'), 'Main102 table definitions differ')
        require(dict(m.query('SELECT version,checksum FROM schema_migrations ORDER BY version;', database=True)) == evidence['source_migrations_sha256'], 'Main102 migration checksums differ')
        require(sha(api.ENVFILE) == env_sha, 'Main configuration changed')
        shutil.copy2(candidate, api.NEW)
        facts = dict(old_facts)
        history = dict(facts.get('schema_upgrade_history', {}))
        history['101'] = old_facts['schema_upgrade']
        facts.update(schema_migrations=102,new_binary_sha256=sha(candidate),schema_upgrade_history=history,schema_upgrade={'owner':rehearsal_owner,'report_sha256':sha(rehearsal_base / 'report.json'),'frozen_source_owner':owner})
        api.save('cutover.json', facts)
        current = manager.start()
        with (base / 'post-verification.log').open('w') as out:
            checked = subprocess.run(['python3',str(HERE / 'verify-local-preview.py')], stdout=out, stderr=subprocess.STDOUT, timeout=180)
        require(checked.returncode == 0, 'Main102 read-only reconciliation failed')
        report.update(success=True,new_pid=current['pid'],health=current['health'],old_tables_unchanged=295,new_tables=['cart_context_operations','cart_context_runtime'],data_and_ddl_match_rehearsal=True,rollback_executed_on_main=False,rollback_rehearsed_on_clone=True)
        api.save('schema-upgrade-102/result.json', report)
        return report
    except BaseException:
        try:
            pid = api.listener()
            if stopped and pid is not None:
                api.stop(pid, api.NEW)
            if stopped:
                if migration_attempted:
                    require(rehearsal_base is not None, 'Fresh recovery authority missing')
                    before = json.loads((rehearsal_base / 'backup-fingerprints.json').read_text())
                    authority = json.loads((rehearsal_base / 'recovery-authority.json').read_text())
                    actual = verify.fingerprints(full_dump('rollback-before-private.sql'), names())
                    report['recovery'] = recovery.restore(lambda sql:m.query(sql,database=True), before, actual, authority)
                    recovered = full_dump('rollback-after-private.sql')
                    require(names() == sorted(before) and verify.fingerprints(recovered, names()) == before and table_ddl(recovered) == table_ddl(rehearsal_base / 'preview-101-private.sql'), 'Recovered101 data/DDL differ; backup retained')
                shutil.copy2(previous, api.NEW)
                facts_path.write_bytes(previous_facts)
                facts_path.chmod(0o600)
                restored = manager.start()
                report.update(success=False,rollback_health=restored['health'],rollback_executed_on_main=migration_attempted)
                api.save('schema-upgrade-102/result.json', report)
        except BaseException:
            report.update(success=False,recovery_required=True)
            api.save('schema-upgrade-102/result.json', report)
            raise
        raise


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source-owner', required=True)
    parser.add_argument('--recovery-owner', required=True)
    args = parser.parse_args()
    print(json.dumps(main(args.source_owner, args.recovery_owner)))
