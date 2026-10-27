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
        # against empty artificial source tables with exact pinned column shape.
        # Dropped-column ordinal gaps are recreated as well, never normalized.
        all_plans = builder.plans()
        shape_spec = importlib.util.spec_from_file_location('source_shape_fixture', Path(__file__).with_name('source-shape-sql.py'))
        shape = importlib.util.module_from_spec(shape_spec)
        shape_spec.loader.exec_module(shape)
        source_columns = shape.columns()
        enum_rows = json.loads((ROOT / 'docs/migration/source-schema-constraints.json').read_text(encoding='utf-8'))['enums']
        enum_types = {}
        for enum in enum_rows:
            enum_types.setdefault(enum['type_name'], []).append(enum['label'])
        statements = []
        for name, labels in enum_types.items():
            statements.append('CREATE TYPE ' + builder.quote(name) + ' AS ENUM (' + ','.join(shape.literal(label) for label in labels) + ');')
        for source_plan in all_plans:
            if source_plan['source'] == 'product_tag':
                continue
            members = {f['ordinal_position']: f for f in source_columns[source_plan['source']]}
            definitions = []
            dropped = []
            for ordinal in range(1, max(members) + 1):
                if ordinal in members:
                    field = members[ordinal]
                    definitions.append(builder.quote(field['column_name']) + ' ' + shape.fixture_column(field))
                else:
                    placeholder = 'fixture_dropped_' + str(ordinal)
                    definitions.append(builder.quote(placeholder) + ' int')
                    dropped.append(placeholder)
            columns = ','.join(definitions)
            statements.append('CREATE TABLE public.' + builder.quote(source_plan['source']) + '(' + columns + ');')
            for placeholder in dropped:
                statements.append('ALTER TABLE public.' + builder.quote(source_plan['source']) + ' DROP COLUMN ' + builder.quote(placeholder) + ';')
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
        export_spec = importlib.util.spec_from_file_location('cell_export', Path(__file__).with_name('export-cell-snapshot.py'))
        exporter = importlib.util.module_from_spec(export_spec)
        export_spec.loader.exec_module(exporter)
        stream_path = ROOT / ('logs/gin-migration/cell-stream-' + owner[:12] + '.ndjson')
        stream_hash = exporter.export(container, 'postgres', 'cell_fixture', stream_path)
        env['ROWIMPORT_POSTGRES_STREAM'] = str(stream_path)
        stream_test = subprocess.run(['go', 'test', './internal/rowimport', '-run', '^TestActualPostgresSnapshotStream$', '-count=1'],
                                    cwd=ROOT / 'retail/backend', env=env, capture_output=True, encoding='utf-8', timeout=90)
        if stream_test.returncode:
            raise AssertionError('Artificial full source stream failed: ' + stream_test.stdout + stream_test.stderr)
        # Same column names, changed shape must fail BEFORE any table_begin.
        drift_cases = [
            ('type', 'ALTER TABLE product_tag ALTER COLUMN metadata TYPE json USING metadata::json;', 'ALTER TABLE product_tag ALTER COLUMN metadata TYPE jsonb USING metadata::jsonb;'),
            ('nullable', 'ALTER TABLE product_tag ALTER COLUMN value DROP NOT NULL;', 'ALTER TABLE product_tag ALTER COLUMN value SET NOT NULL;'),
            ('time_precision', 'ALTER TABLE product_tag ALTER COLUMN created_at TYPE timestamptz(3);', 'ALTER TABLE product_tag ALTER COLUMN created_at TYPE timestamptz(6);'),
            ('varchar_length', 'ALTER TABLE cart_payment_collection ALTER COLUMN cart_id TYPE varchar(254);', 'ALTER TABLE cart_payment_collection ALTER COLUMN cart_id TYPE varchar(255);'),
            ('numeric_precision', 'ALTER TABLE price ALTER COLUMN amount TYPE numeric(38,18);', 'ALTER TABLE price ALTER COLUMN amount TYPE numeric;'),
        ]
        for label, mutate, restore in drift_cases:
            sql(mutate)
            rejected = ROOT / ('logs/gin-migration/cell-drift-' + owner[:12] + '-' + label + '.ndjson')
            try:
                exporter.export(container, 'postgres', 'cell_fixture', rejected)
            except RuntimeError:
                assert not Path(str(rejected) + '.sha256').exists()
                assert rejected.read_bytes() == b''
            else:
                raise AssertionError('Source schema drift was silently accepted: ' + label)
            sql(restore)
        report = {'format_version': 1, 'scope': 'Artificial PostgreSQL source SQL to v2 record decoder and mapped Go conversion; not source recapture/import acceptance',
                  'image': 'postgres:16-alpine', 'local_image_id': image.stdout.strip(),
                  'isolation': {'network': 'none', 'published_ports': 0, 'cpus': 1, 'memory_mib': 384},
                  'query_builder_sha256': hashlib.sha256(Path(__file__).with_name('cell-record-sql.py').read_bytes().replace(b'\r\n', b'\n')).hexdigest(),
                  'exporter_sha256': hashlib.sha256(Path(__file__).with_name('export-cell-snapshot.py').read_bytes().replace(b'\r\n', b'\n')).hexdigest(),
                  'source_shape_guard_sha256': hashlib.sha256(Path(__file__).with_name('source-shape-sql.py').read_bytes().replace(b'\r\n', b'\n')).hexdigest(),
                  'cases': {'sql_null_distinct': True, 'json_null_document_distinct': True, 'big_integer_and_decimal_lexemes': True,
                            'go_actual_source_records': 3, 'go_test_passed': True,
                            'other_empty_projection_sql_parsed': 158,
                            'full_read_only_transaction_export': True,
                            'go_159_table_stream_footer_counts_and_hash_verified': True,
                            'all_tables_use_pinned_source_types_and_nullability': True,
                            'source_shape_drift_refused': [case[0] for case in drift_cases],
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
