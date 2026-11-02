"""Upgrade only the audited, private local99 preview to explicit100.

Stops the sole API before its final backup/restore rehearsal. No config or gate
changes, no application on clone. Preserves old binary/facts and rollback data.
"""
import hashlib
import importlib.util
import json
from pathlib import Path
import re
import shutil
import subprocess

HERE=Path(__file__).resolve().parent

def load(name,path):
    spec=importlib.util.spec_from_file_location(name,path)
    module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module);return module

manager=load('manager100',HERE/'preview-api.py')
verify=load('verify100',HERE/'verify-local-preview.py')
m=manager.mysql
api=manager.api
sha=lambda path:hashlib.sha256(path.read_bytes()).hexdigest()

def main():
    status=manager.status()
    facts_path=api.BASE/'cutover.json';old_facts_bytes=facts_path.read_bytes();old_facts=json.loads(old_facts_bytes)
    if not status['running'] or status['schema_migrations']!=99:raise RuntimeError('Healthy audited99 preview required')
    owner='85413f54d3b3'
    frozen=api.ROOT/'logs/gin-migration'/('native-sql-local-'+owner)
    manifest=json.loads((frozen/'manifest.json').read_text());sql=json.loads((frozen/'result.json').read_text())
    if sql['suite_count']!=23 or not sql['complete_original_sql_regression'] or sql['migrations']!=100:raise RuntimeError('Complete100 original23 SQL required')
    if not json.loads((frozen/'mysql-cleanup.json').read_text())['server_terminal']:raise RuntimeError('SQL resource cleanup required')
    for row in sql['suites']:
        if row['exit_code']!=0 or sha(frozen/(row['suite']+'.log'))!=row['log_sha256']:raise RuntimeError('SQL evidence changed')
    for name,digest in manifest['source_sha256'].items():
        if name.startswith('retail/backend/') and (sha(api.ROOT/name)!=digest or sha(frozen/name)!=manifest['adapted_test_sha256'].get(name,digest)):raise RuntimeError('Frozen/current backend source changed')
    base=api.BASE/'schema-upgrade-100';base.mkdir(mode=0o700)
    (base/'previous-cutover.json').write_bytes(old_facts_bytes);(base/'previous-cutover.json').chmod(0o600)
    previous=base/'previous-api';shutil.copy2(api.NEW,previous)
    candidate=base/'candidate-api'
    with (base/'compile.log').open('w') as out:
        subprocess.run(['/opt/homebrew/opt/go/bin/go','build','-trimpath','-o',str(candidate),'./cmd/api'],cwd=frozen/'retail/backend',env=m.ENV,stdout=out,stderr=subprocess.STDOUT,check=True,timeout=120)
    values=manager.configuration();env_sha=sha(api.ENVFILE)
    report={'previous_pid':status['pid'],'previous_binary_sha256':sha(previous),'new_binary_sha256':sha(candidate),'frozen_source_owner':owner,'old_schema':99,'target_schema':100,'configuration_changed':False,'shared_mysql_modified':False,'real_provider_called':False,'goal_complete':False}
    stopped=False;rehearsal_base=None
    try:
        manager.stop();stopped=True
        with (base/'fresh-rehearsal.log').open('w') as out:
            run=subprocess.run(['python3',str(HERE/'rehearse-local-preview-100.py')],stdout=out,stderr=subprocess.STDOUT,timeout=600)
        if run.returncode:raise RuntimeError('Final stopped-writer backup rehearsal failed')
        progress=[json.loads(line) for line in (base/'fresh-rehearsal.log').read_text().splitlines() if line.startswith('{')]
        rehearsal_owner=progress[0]['owner'];assert re.fullmatch('[a-f0-9]{12}',rehearsal_owner)
        rehearsal_base=api.ROOT/'logs/gin-migration'/('preview-100-upgrade-'+rehearsal_owner)
        evidence=json.loads((rehearsal_base/'report.json').read_text())
        if not evidence['rehearsal_passed'] or not evidence['cleanup']['server_terminal'] or evidence['new_tables']!=['seo_pages','seo_settings']:raise RuntimeError('Final100 rehearsal incomplete')
        if sha(rehearsal_base/'preview-99-private.sql')!=evidence['backup']['sha256']:raise RuntimeError('Final backup changed')
        c=m.config()
        for index,want in enumerate(['Applied 1, skipped 99 migrations','Applied 0, skipped 100 migrations']):
            run=subprocess.run([str(rehearsal_base/'migrate')],cwd=base,env={**m.ENV,'MYSQL_DSN':'root:'+c['root_password']+'@unix('+str(m.SOCKET)+')/'+m.DATABASE+'?parseTime=true&charset=utf8mb4'},capture_output=True,text=True,timeout=180)
            (base/('migration-'+str(index+1)+'.log')).write_text(run.stdout+run.stderr)
            if run.returncode or run.stdout.strip()!=want:raise RuntimeError('Main100 explicit migration failed')
        names=[r[0] for r in m.query("SELECT table_name FROM information_schema.tables WHERE table_schema=DATABASE() AND table_type='BASE TABLE' ORDER BY table_name;",database=True)]
        expected=json.loads((rehearsal_base/'migrated-fingerprints.json').read_text())
        if set(names)!=set(expected):raise RuntimeError('Main100 table inventory differs')
        dump=verify.dump('schema-upgrade-100/post-upgrade-private-data.sql',['--protocol=SOCKET','--socket='+str(m.SOCKET),'-uroot'],{**m.ENV,'MYSQL_PWD':c['root_password']})
        actual=verify.fingerprints(dump,names)
        if any(actual[n]!=expected[n] for n in names if n not in ['schema_migrations','schema_migration_steps']):raise RuntimeError('Main100 business data differs')
        if dict(m.query('SELECT version,checksum FROM schema_migrations ORDER BY version;',database=True))!=evidence['source_migrations_sha256']:raise RuntimeError('Main100 checksums differ')
        if sha(api.ENVFILE)!=env_sha:raise RuntimeError('Preview configuration changed')
        shutil.copy2(candidate,api.NEW)
        facts=dict(old_facts);facts.update(schema_migrations=100,new_binary_sha256=sha(candidate),schema_upgrade={'owner':rehearsal_owner,'report_sha256':sha(rehearsal_base/'report.json'),'frozen_source_owner':owner})
        api.save('cutover.json',facts)
        current=manager.start()
        report.update(success=True,new_pid=current['pid'],health=current['health'],rehearsal_owner=rehearsal_owner,old_business_tables_unchanged=291,new_tables=['seo_pages','seo_settings'])
        api.save('schema-upgrade-100/result.json',report)
        return report
    except BaseException:
        # Only this migration's additive empty tables may be reversed, and only
        # after proving every pre-existing business table still matches backup.
        pid=api.listener()
        if stopped and pid is not None:api.stop(pid,api.NEW)
        if stopped:
            if rehearsal_base is not None:
                before=json.loads((rehearsal_base/'backup-fingerprints.json').read_text())
                c=m.config();names=[r[0] for r in m.query("SELECT table_name FROM information_schema.tables WHERE table_schema=DATABASE() AND table_type='BASE TABLE';",database=True)]
                p=verify.dump('schema-upgrade-100/rollback-check-private.sql',['--protocol=SOCKET','--socket='+str(m.SOCKET),'-uroot'],{**m.ENV,'MYSQL_PWD':c['root_password']})
                actual=verify.fingerprints(p,names)
                if any(actual[n]!=v for n,v in before.items() if n not in ['schema_migrations','schema_migration_steps']):raise RuntimeError('Rollback stopped: original business data changed; protected backups retained')
                for table in ['seo_pages','seo_settings']:
                    if table in names:
                        if m.query('SELECT COUNT(*) FROM `'+table+'`;',database=True)!=[['0']]:raise RuntimeError('Rollback stopped: new SEO facts exist; retained for recovery')
                        m.query('DROP TABLE `'+table+'`;',database=True)
                m.query("DELETE FROM schema_migration_steps WHERE version='0100_public_seo.sql'; DELETE FROM schema_migrations WHERE version='0100_public_seo.sql';",database=True)
            shutil.copy2(previous,api.NEW);facts_path.write_bytes(old_facts_bytes);facts_path.chmod(0o600)
            restored=manager.start();report.update(success=False,rollback_health=restored['health'])
            api.save('schema-upgrade-100/result.json',report)
        raise

if __name__=='__main__':print(json.dumps(main()))
