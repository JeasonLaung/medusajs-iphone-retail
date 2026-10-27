"""Explicit opt-in source export; no default database/container, no credentials.

Writes only a new Git-ignored path. Never overwrites the fixed v1 baseline.
All data queries run in ONE psql session / repeatable-read read-only transaction.
"""
import argparse
import hashlib
import importlib.util
import json
from pathlib import Path
import re
import subprocess

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('cell_sql', Path(__file__).with_name('cell-record-sql.py'))
builder = importlib.util.module_from_spec(spec)
spec.loader.exec_module(builder)
shape_spec = importlib.util.spec_from_file_location('source_shape', Path(__file__).with_name('source-shape-sql.py'))
shape = importlib.util.module_from_spec(shape_spec)
shape_spec.loader.exec_module(shape)


def program_sql():
    plans = builder.plans()
    plan_path = ROOT / 'retail/backend/internal/rowimport/field_plans.json'
    digest = hashlib.sha256(plan_path.read_bytes().replace(b'\r\n', b'\n')).hexdigest()
    statements = ['BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;',
                  "SET LOCAL TIME ZONE 'UTC'; SET LOCAL statement_timeout='120s'; SET LOCAL lock_timeout='5s';",
                  'LOCK TABLE ' + ','.join('public.' + builder.quote(p['source']) for p in plans) + ' IN ACCESS SHARE MODE;']
    # Validate every source shape before emitting ANY business record.
    statements.extend(shape.guard(plan['source']) for plan in plans)

    def emit(record):
        # Constant protocol metadata only, no business values interpolated.
        encoded = json.dumps(record, separators=(',', ':'))
        statements.append("SELECT '" + encoded + "';")

    emit({'kind': 'snapshot', 'format_version': 2, 'plan_sha256': digest})
    for plan in plans:
        table = plan['source']
        emit({'kind': 'table_begin', 'table': table})
        statements.append(builder.query_for(plan))
        statements.append("SELECT json_build_object('kind','table_end','table','" + table + "','rows',count(*))::text FROM public." + builder.quote(table) + ';')
    statements.append('COMMIT;')
    emit({'kind': 'snapshot_complete', 'tables': len(plans)})
    return '\n'.join(statements) + '\n'


def export(container, user, database, output):
    for value in (container, user, database):
        if not re.fullmatch(r'[a-zA-Z0-9_][a-zA-Z0-9_.-]{0,127}', value):
            raise ValueError('Explicit safe source connection identifiers required')
    output = Path(output).resolve()
    if subprocess.run(['git', 'check-ignore', '--quiet', str(output)], cwd=ROOT).returncode:
        raise ValueError('Output must be Git-ignored')
    if output.exists() or output.with_suffix(output.suffix + '.sha256').exists():
        raise ValueError('Output already exists; refusing overwrite')
    output.parent.mkdir(parents=True, exist_ok=True)
    # psql stdin is a generated SQL file, stdout streams directly to disk. No
    # row-sized stdout buffers, JSON.parse, or stderr echo of business context.
    script = output.with_suffix(output.suffix + '.query.tmp')
    if script.exists():
        raise ValueError('Temporary query path already exists')
    query_owned = False
    try:
        with script.open('x', encoding='utf-8', newline='\n') as generated:
            query_owned = True
            generated.write(program_sql())
        with script.open('rb') as queries, output.open('xb') as target:
            result = subprocess.run(['docker', 'exec', '-i', container, 'psql', '-X', '-q', '-A', '-t',
                                     '-v', 'ON_ERROR_STOP=1', '-U', user, '-d', database],
                                    stdin=queries, stdout=target, stderr=subprocess.DEVNULL, timeout=600)
        if result.returncode:
            raise RuntimeError('Read-only cell export failed; partial file retained without completion checksum')
        digest = hashlib.sha256()
        with output.open('rb') as data:
            for block in iter(lambda: data.read(1 << 20), b''):
                digest.update(block)
        # This checksum is transport completeness evidence, not an authenticated
        # origin signature. Go must still verify the full protocol and counts.
        with output.with_suffix(output.suffix + '.sha256').open('x', encoding='ascii') as checksum:
            checksum.write(digest.hexdigest() + '\n')
        return digest.hexdigest()
    finally:
        if query_owned and script.exists():
            script.unlink()  # Exact newly created ignored metadata file only.


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--container', required=True)
    parser.add_argument('--user', required=True)
    parser.add_argument('--database', required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    print(json.dumps({'sha256': export(args.container, args.user, args.database, args.output), 'tables': 159}))
