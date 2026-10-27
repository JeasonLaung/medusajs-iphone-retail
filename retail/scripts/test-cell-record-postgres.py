"""Artificial PostgreSQL -> v2 cell SQL -> Go DecodeRecord/Convert proof."""
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import secrets
import subprocess
import time
import uuid

ROOT = Path(__file__).resolve().parents[2]
LABEL = 'eurorefurb.cell-record-fixture'
spec = importlib.util.spec_from_file_location('cell_sql', Path(__file__).with_name('cell-record-sql.py'))
builder = importlib.util.module_from_spec(spec)
spec.loader.exec_module(builder)


def docker(*args, input=None, timeout=60):
    return subprocess.run(['docker', *args], input=input, capture_output=True, encoding='utf-8', timeout=timeout)


def main():
    image = docker('image', 'inspect', 'postgres:16-alpine', '--format', '{{.Id}}')
    if image.returncode:
        raise RuntimeError('Existing local postgres:16-alpine image required; no pull')
    owner = uuid.uuid4().hex
    container = None
    fixture_path = ROOT / 'logs/gin-migration/cell-record-fixture.json'
    report = None
    try:
        started = docker('run', '-d', '--name', 'eurorefurb-cells-' + owner[:12], '--label', LABEL + '=' + owner,
                         '--network', 'none', '--cpus', '1', '--memory', '384m', '--memory-swap', '384m',
                         '--pids-limit', '128', '--shm-size', '32m', '--tmpfs', '/var/lib/postgresql/data:rw,size=192m',
                         '-e', 'POSTGRES_PASSWORD=' + secrets.token_urlsafe(32), '-e', 'POSTGRES_DB=cell_fixture',
                         'postgres:16-alpine', 'postgres', '-c', 'max_connections=16', '-c', 'shared_buffers=32MB')
        if started.returncode:
            raise RuntimeError('Could not create bounded artificial fixture')
        container = started.stdout.strip()
        for _ in range(60):
            ready = docker('exec', container, 'pg_isready', '-U', 'postgres', '-d', 'cell_fixture')
            process = docker('exec', container, 'cat', '/proc/1/comm')
            if ready.returncode == 0 and process.stdout.strip() == 'postgres':
                break
            time.sleep(0.5)
        else:
            raise RuntimeError('Artificial PostgreSQL not ready')

        def sql(query):
            result = docker('exec', '-i', container, 'psql', '-X', '-q', '-A', '-t', '-v', 'ON_ERROR_STOP=1',
                            '-U', 'postgres', '-d', 'cell_fixture', input=query)
            if result.returncode:
                raise RuntimeError('Artificial source SQL failed: ' + result.stderr)
            return result.stdout.strip()

        sql('''CREATE TABLE product_tag(id text PRIMARY KEY,value text NOT NULL,metadata jsonb,
created_at timestamptz NOT NULL,updated_at timestamptz NOT NULL,deleted_at timestamptz,external_id text);
INSERT INTO product_tag VALUES
('a','SQL NULL',NULL,'2026-10-27 08:00:00.123456+08','2026-10-27 08:00:00+08',NULL,NULL),
('b','JSON null','null'::jsonb,'2026-10-27 08:00:00.123456+08','2026-10-27 08:00:00+08',NULL,''),
('c','exact number','{"big":9007199254740993,"amount":1.2300}'::jsonb,'2026-10-27 08:00:00.123456+08','2026-10-27 08:00:00+08',NULL,' Case ');''')
        plan = next(p for p in builder.plans() if p['source'] == 'product_tag')
        query = builder.query_for(plan).removesuffix(';') + ' ORDER BY t.id;'
        output = sql('BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY; SET LOCAL TIME ZONE \'UTC\';\n' + query + '\nCOMMIT;')
        records = [json.loads(line) for line in output.splitlines()]
        assert len(records) == 3
        assert records[0]['cells']['metadata'] == {'sql_null': True}
        assert records[1]['cells']['metadata'] == {'sql_null': False, 'value_json': 'null'}
        assert records[2]['cells']['metadata']['value_json'] == '{"big": 9007199254740993, "amount": 1.2300}'
        # Parse/execute every generated projection, including the widest tables,
        # against empty artificial source tables. Text stand-ins prove identifier
        # and SQL structure only; typed value proof above uses actual jsonb/time.
        all_plans = builder.plans()
        statements = []
        for source_plan in all_plans:
            if source_plan['source'] == 'product_tag':
                continue
            columns = ','.join(builder.quote(f['source']) + ' text' for f in source_plan['fields'])
            statements.append('CREATE TABLE public.' + builder.quote(source_plan['source']) + '(' + columns + ');')
            statements.append('SELECT count(*) FROM (' + builder.query_for(source_plan).removesuffix(';') + ') artificial_empty;')
        empty_results = sql('\n'.join(statements)).splitlines()
        assert len(empty_results) == 158 and set(empty_results) == {'0'}
        # Only artificial values are persisted to this ignored Go fixture input.
        fixture_path.parent.mkdir(parents=True, exist_ok=True)
        fixture_path.write_text(json.dumps(records, ensure_ascii=False), encoding='utf-8')
        env = os.environ.copy()
        env['ROWIMPORT_POSTGRES_FIXTURE'] = str(fixture_path)
        tested = subprocess.run(['go', 'test', './internal/rowimport', '-run', '^TestActualPostgresCellRecords$', '-count=1'],
                                cwd=ROOT / 'retail/backend', env=env, capture_output=True, encoding='utf-8', timeout=90)
        if tested.returncode:
            raise AssertionError('Artificial PostgreSQL-to-Go proof failed: ' + tested.stdout + tested.stderr)
        report = {'format_version': 1, 'scope': 'Artificial PostgreSQL source SQL to v2 record decoder and mapped Go conversion; not source recapture/import acceptance',
                  'image': 'postgres:16-alpine', 'local_image_id': image.stdout.strip(),
                  'isolation': {'network': 'none', 'published_ports': 0, 'cpus': 1, 'memory_mib': 384},
                  'query_builder_sha256': hashlib.sha256(Path(__file__).with_name('cell-record-sql.py').read_bytes().replace(b'\r\n', b'\n')).hexdigest(),
                  'cases': {'sql_null_distinct': True, 'json_null_document_distinct': True, 'big_integer_and_decimal_lexemes': True,
                            'go_actual_source_records': 3, 'go_test_passed': True,
                            'other_empty_projection_sql_parsed': 158,
                            'max_mapped_columns': max(len(p['fields']) for p in all_plans)},
                  'source_business_connection': False}
    finally:
        if container:
            owned = docker('inspect', container, '--format', '{{index .Config.Labels "' + LABEL + '"}}')
            if owned.returncode or owned.stdout.strip() != owner:
                raise RuntimeError('Ownership mismatch; cleanup refused')
            if docker('rm', '-f', '-v', container).returncode:
                raise RuntimeError('Owned fixture cleanup failed')
    if report:
        report['cleanup'] = 'owned artificial container removed after label verification'
        (ROOT / 'docs/migration/cell-record-postgres-report.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
        print(json.dumps({'artificial_records': 3, 'postgres_to_go_passed': True, 'cleanup': 'complete'}))


if __name__ == '__main__':
    main()
