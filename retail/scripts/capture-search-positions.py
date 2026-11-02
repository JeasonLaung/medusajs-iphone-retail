"""Capture source English word positions without accessing business tables."""
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
    parser.add_argument('--inputs', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    if args.output.exists():
        raise RuntimeError('Refusing to replace existing evidence')
    inputs = json.loads(args.inputs.read_text())
    values = [line.split('=', 1)[1].strip().strip('"').strip("'")
              for line in (ROOT / 'medusa-store/apps/backend/.env').read_text().splitlines()
              if line.startswith('DATABASE_URL=')]
    if len(values) != 1:
        raise RuntimeError('One source database configuration required')
    uri = urlsplit(values[0])
    if uri.scheme not in ('postgres', 'postgresql') or uri.hostname not in ('localhost', '127.0.0.1'):
        raise RuntimeError('Only the original local PostgreSQL is allowed')
    words = ','.join("('%s')" % word.replace("'", "''") for word in inputs)
    sql = """BEGIN READ ONLY;
SET LOCAL statement_timeout='10s';
SELECT json_build_object('version',current_setting('server_version'));
SELECT json_build_object('input',word,'vector',to_tsvector('english',word)::text,
 'positions',coalesce((SELECT json_agg(json_build_object('term',lexeme,'position',p) ORDER BY p,lexeme)
 FROM unnest(to_tsvector('english',word)),unnest(positions) p),'[]'::json))
 FROM (VALUES """ + words + ") v(word); ROLLBACK;"
    cmd = ['/opt/homebrew/opt/libpq/bin/psql', '-X', '-h', uri.hostname,
           '-p', str(uri.port or 5432), '-U', unquote(uri.username or ''),
           '-d', uri.path.lstrip('/'), '-A', '-t', '-v', 'ON_ERROR_STOP=1']
    run = subprocess.run(cmd, input=sql, text=True, capture_output=True, timeout=20,
                         env={**os.environ, 'PGPASSWORD': unquote(uri.password or '')})
    if run.returncode:
        raise RuntimeError('Local read-only position query failed')
    rows = [json.loads(line) for line in run.stdout.splitlines() if line.startswith('{')]
    if len(rows) != len(inputs) + 1 or 'version' not in rows[0]:
        raise RuntimeError('Incomplete source position response')
    result = {'at_utc': datetime.datetime.now(datetime.timezone.utc).isoformat(),
              'version': rows[0]['version'], 'transaction_read_only': True,
              'business_tables_accessed': False, 'cases': rows[1:]}
    args.output.parent.mkdir(parents=True, exist_ok=True)
    with args.output.open('x') as output:
        json.dump(result, output, ensure_ascii=False, indent=2)
        output.write('\n')
    print(json.dumps({'cases': len(inputs), 'transaction_read_only': True}))


if __name__ == '__main__':
    main()
