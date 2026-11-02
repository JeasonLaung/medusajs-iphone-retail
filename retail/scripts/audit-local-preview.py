"""Audit the existing local preview using metadata reads only.

Never changes schema, server configuration, gates, data or service processes.
Connection credentials are used only in the child environment, never reported.
"""
import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess

ROOT = Path(__file__).resolve().parents[2]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    if args.output.exists():
        raise RuntimeError('Refusing to replace existing evidence')
    values = [line.split('=', 1)[1].strip().strip('"').strip("'")
              for line in (ROOT / 'retail/backend/.env').read_text().splitlines()
              if line.startswith('MYSQL_DSN=')]
    if len(values) != 1:
        raise RuntimeError('One local preview connection required')
    match = re.fullmatch(r'([^:]+):(.*)@tcp\(127\.0\.0\.1:3306\)/eurorefurb_local_trial\?.*', values[0])
    if not match:
        raise RuntimeError('Only the existing local preview database is allowed')
    cmd = ['/opt/homebrew/bin/mysql', '--no-defaults', '--protocol=TCP',
           '-h127.0.0.1', '-P3306', '-u' + match[1], '-N', 'eurorefurb_local_trial']
    env = {**os.environ, 'MYSQL_PWD': match[2]}

    def query(sql):
        run = subprocess.run(cmd, input=sql, text=True, capture_output=True,
                             timeout=30, env=env)
        if run.returncode:
            raise RuntimeError('Local metadata query failed; credentials suppressed')
        return [row.split('\t') for row in run.stdout.splitlines()]

    journal = query('SELECT version,checksum FROM schema_migrations ORDER BY version;')
    source = {p.name: hashlib.sha256(p.read_bytes()).hexdigest()
              for p in sorted((ROOT / 'retail/backend/migrations').glob('*.sql'))}
    matching = all(source.get(name) == digest for name, digest in journal)
    prefix = [row[0] for row in journal] == list(source)[:len(journal)]
    runtime = query('SELECT VERSION(),@@port,@@innodb_native_foreign_keys,@@foreign_key_checks;')[0]
    size = query("SELECT COUNT(*),COALESCE(SUM(data_length+index_length),0) FROM information_schema.tables WHERE table_schema=DATABASE();")[0]
    foreign_keys = query("SELECT COUNT(*) FROM information_schema.table_constraints WHERE constraint_schema=DATABASE() AND constraint_type='FOREIGN KEY';")[0][0]
    other_schemas = query("SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name NOT IN ('information_schema','mysql','performance_schema','sys',DATABASE());")[0][0]
    pending = query("SELECT COUNT(*) FROM schema_migration_steps WHERE status<>'applied' OR inflight=TRUE OR next_statement<>statement_count;")[0][0]
    price_gate = query('SELECT enabled FROM catalog_price_edit_runtime WHERE id=1;')[0][0]
    result = {'at_utc': datetime.datetime.now(datetime.timezone.utc).isoformat(),
              'metadata_reads_only': True, 'business_rows_exported': False,
              'server': {'version': runtime[0], 'port': int(runtime[1]),
                         'native_foreign_keys': runtime[2] == '1',
                         'foreign_key_checks': runtime[3] == '1',
                         'other_visible_user_schemas': int(other_schemas)},
              'preview_database': 'eurorefurb_local_trial',
              'estimated_data_and_index_bytes': int(size[1]), 'tables': int(size[0]),
              'declared_foreign_keys': int(foreign_keys),
              'applied_migrations': len(journal), 'source_migrations': len(source),
              'journal_checksums_match_source': matching, 'journal_is_source_prefix': prefix,
              'pending_or_inflight_steps': int(pending), 'price_write_gate': price_gate == '1',
              'missing_migrations': [name for name in source if name not in dict(journal)],
              'main_database_modified': False, 'services_restarted': False,
              'backup_restore_verified': False, 'upgrade_ready': False}
    args.output.parent.mkdir(parents=True, exist_ok=True)
    with args.output.open('x') as out:
        json.dump(result, out, indent=2)
        out.write('\n')
    print(json.dumps(result))


if __name__ == '__main__':
    main()
