"""Read-only PostgreSQL column defaults; never query business row values."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / 'docs/migration/source-column-defaults.json'
SQL = """BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT coalesce(json_agg(x ORDER BY source_table,ordinal_position),'[]'::json) FROM (
 SELECT table_name AS source_table,column_name AS source_column,ordinal_position,
        data_type,udt_name,is_nullable,column_default,is_identity,identity_generation,
        is_generated,generation_expression
 FROM information_schema.columns WHERE table_schema='public'
   AND table_name IN (SELECT table_name FROM information_schema.tables
                     WHERE table_schema='public' AND table_type='BASE TABLE')
) x;
COMMIT;"""


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source-container', default='medusa-trial-postgres-1')
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    result = subprocess.run(['docker', 'exec', '-i', args.source_container, 'psql', '-X', '-q', '-t', '-A',
                             '-v', 'ON_ERROR_STOP=1', '-U', 'medusa', '-d', 'medusa_trial'],
                            input=SQL, capture_output=True, encoding='utf-8', check=True)
    columns = json.loads(result.stdout)
    manifest_path = ROOT / 'docs/migration/data-source-manifest.json'
    manifest_raw = manifest_path.read_bytes().replace(b'\r\n', b'\n')
    manifest = json.loads(manifest_raw)
    expected = {(t['source_table'], c['column_name']) for t in manifest['tables'] for c in t['columns']}
    actual = {(c['source_table'], c['source_column']) for c in columns}
    if expected != actual or len(actual) != len(columns):
        raise ValueError('Column set differs from baseline; inspect schema drift before recapturing')
    output = {'format_version': 1, 'source_commit': manifest['source_commit'],
              'source_manifest_sha256': hashlib.sha256(manifest_raw).hexdigest(),
              'scope': 'Current PostgreSQL metadata defaults/identity/generated flags only; no source row values or target DDL',
              'columns': columns}
    content = json.dumps(output, ensure_ascii=False, indent=2) + '\n'
    if args.check:
        if OUT.read_text(encoding='utf-8-sig') != content:
            raise ValueError('Default metadata drift; review before refreshing')
    else:
        OUT.write_text(content, encoding='utf-8')
    print(json.dumps({'tables': len({c['source_table'] for c in columns}), 'columns': len(columns),
                      'with_default': sum(c['column_default'] is not None for c in columns),
                      'identity': sum(c['is_identity'] == 'YES' for c in columns),
                      'generated': sum(c['is_generated'] != 'NEVER' for c in columns)}))


if __name__ == '__main__':
    main()
