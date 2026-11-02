"""Read-only lexical oracle from the original local PostgreSQL storefront.

No source business tables are accessed. Connection secrets are never printed.
The output is a new evidence file, not a replacement for the checked-in oracle.
"""
import argparse
import datetime
import json
import os
from pathlib import Path
import subprocess
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parents[2]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--baseline', type=Path, default=ROOT / 'retail/backend/internal/catalog/testdata/search-postgres-lexemes.json')
    args = parser.parse_args()
    if args.output.exists():
        raise RuntimeError('Refusing to replace existing evidence')
    source = ROOT / 'medusa-store/apps/backend/.env'
    values = [line.split('=', 1)[1].strip().strip('"').strip("'")
              for line in source.read_text().splitlines() if line.startswith('DATABASE_URL=')]
    if len(values) != 1:
        raise RuntimeError('One source database configuration required')
    uri = urlsplit(values[0])
    if uri.scheme not in ('postgres', 'postgresql') or uri.hostname not in ('localhost', '127.0.0.1'):
        raise RuntimeError('Only the original local PostgreSQL is allowed')
    baseline = args.baseline
    cases = json.loads(baseline.read_text())
    words = ','.join("('%s')" % row['input'].replace("'", "''") for row in cases)
    sql = """BEGIN READ ONLY;
SET LOCAL statement_timeout='10s';
SELECT json_build_object('version',current_setting('server_version'));
SELECT json_build_object('input',word,
 'terms',ARRAY(SELECT unnest(lexemes) FROM ts_debug('english',word) WHERE lexemes IS NOT NULL),
 'tokens',(SELECT json_agg(json_build_object('kind',alias,'text',token,'lexemes',lexemes))
           FROM ts_debug('english',word))) FROM (VALUES """ + words + ") v(word); ROLLBACK;"
    cmd = ['/opt/homebrew/opt/libpq/bin/psql', '-X', '-h', uri.hostname,
           '-p', str(uri.port or 5432), '-U', unquote(uri.username or ''),
           '-d', uri.path.lstrip('/'), '-A', '-t', '-v', 'ON_ERROR_STOP=1']
    run = subprocess.run(cmd, input=sql, text=True, capture_output=True, timeout=20,
                         env={**os.environ, 'PGPASSWORD': unquote(uri.password or '')})
    if run.returncode:
        raise RuntimeError('Local read-only dictionary query failed')
    rows = [json.loads(line) for line in run.stdout.splitlines() if line.startswith('{')]
    if len(rows) != len(cases) + 1 or 'version' not in rows[0]:
        raise RuntimeError('Incomplete source dictionary response')
    result = {'at_utc': datetime.datetime.now(datetime.timezone.utc).isoformat(),
              'version': rows[0]['version'], 'transaction_read_only': True,
              'business_tables_accessed': False, 'cases': rows[1:],
              'matches_checked_in_oracle': rows[1:] == cases}
    args.output.parent.mkdir(parents=True, exist_ok=True)
    with args.output.open('x') as output:
        json.dump(result, output, ensure_ascii=False, indent=2)
        output.write('\n')
    print(json.dumps({'cases': len(cases), 'matches_checked_in_oracle': result['matches_checked_in_oracle']}))
    if not result['matches_checked_in_oracle']:
        raise RuntimeError('Source dictionary differs from checked-in oracle')


if __name__ == '__main__':
    main()
