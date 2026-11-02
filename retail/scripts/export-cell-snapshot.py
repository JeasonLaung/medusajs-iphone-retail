"""Explicit opt-in source export; no default database/container, no credentials.

Writes only a new Git-ignored path. Never overwrites the fixed v1 baseline.
All data queries run in ONE psql session / repeatable-read read-only transaction.
"""
import argparse
import hashlib
import importlib.util
import json
import os
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


def source_command(container, user, database, host=None, port=None, psql='psql'):
    for value in (user, database):
        if not re.fullmatch(r'[a-zA-Z0-9_][a-zA-Z0-9_.-]{0,127}', value):
            raise ValueError('Explicit safe source connection identifiers required')
    options = ['-X', '-w', '-q', '-A', '-t', '-v', 'ON_ERROR_STOP=1', '-U', user, '-d', database]
    if container is not None:
        if host is not None or port is not None or not re.fullmatch(r'[a-zA-Z0-9_][a-zA-Z0-9_.-]{0,127}', container):
            raise ValueError('Choose exactly one explicit source connection')
        return ['docker', 'exec', '-i', container, 'psql', *options]
    if host not in ('127.0.0.1', '::1', 'localhost') or type(port) is not int or not 1 <= port <= 65535:
        raise ValueError('Native source requires an explicit loopback host and port')
    return [psql, '-h', host, '-p', str(port), *options]


def private_file(path, mode='w'):
    # Source snapshots can include customer and payment history. Never expose
    # them through the caller's default umask, even while the export is running.
    return os.fdopen(os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), mode,
                     **({} if 'b' in mode else {'encoding': 'utf-8', 'newline': '\n'}))


def export(container, user, database, output, *, host=None, port=None, psql='psql'):
    command = source_command(container, user, database, host, port, psql)
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
        with private_file(script) as generated:
            query_owned = True
            generated.write(program_sql())
        with script.open('rb') as queries, private_file(output, 'wb') as target:
            try:
                result = subprocess.run(command, stdin=queries, stdout=target,
                                        stderr=subprocess.DEVNULL, timeout=600)
            except (OSError, subprocess.TimeoutExpired):
                raise RuntimeError('Read-only source export unavailable or timed out; no completion checksum written') from None
        if result.returncode:
            raise RuntimeError('Read-only cell export failed; partial file retained without completion checksum')
        digest = hashlib.sha256()
        with output.open('rb') as data:
            for block in iter(lambda: data.read(1 << 20), b''):
                digest.update(block)
        # This checksum is transport completeness evidence, not an authenticated
        # origin signature. Go must still verify the full protocol and counts.
        with private_file(output.with_suffix(output.suffix + '.sha256')) as checksum:
            checksum.write(digest.hexdigest() + '\n')
        return digest.hexdigest()
    finally:
        if query_owned and script.exists():
            script.unlink()  # Exact newly created ignored metadata file only.


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    source = parser.add_mutually_exclusive_group(required=True)
    source.add_argument('--container')
    source.add_argument('--local', action='store_true', help='Use local psql and libpq PGPASSFILE/PGPASSWORD authentication')
    parser.add_argument('--host')
    parser.add_argument('--port', type=int)
    parser.add_argument('--psql', default='psql', help='Native psql executable')
    parser.add_argument('--user', required=True)
    parser.add_argument('--database', required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    try:
        digest = export(args.container, args.user, args.database, args.output,
                        host=args.host, port=args.port, psql=args.psql)
    except (ValueError, RuntimeError, OSError) as error:
        # Do not print arbitrary OS errors: they can include private file paths.
        parser.exit(1, (str(error) if isinstance(error, (ValueError, RuntimeError)) else 'Private export file unavailable') + '\n')
    print(json.dumps({'sha256': digest, 'tables': 159}))
