"""Execute captured legacy triggers in a disposable, bounded, artificial-only DB."""
import concurrent.futures
import hashlib
import json
from pathlib import Path
import secrets
import subprocess
import time
import uuid

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / 'docs/migration'
LABEL = 'eurorefurb.numbering-fixture'


def docker(*args, input=None, timeout=60):
    result = subprocess.run(['docker', *args], input=input, capture_output=True,
                            encoding='utf-8', timeout=timeout)
    return result


def main():
    schema_bytes = (OUT / 'source-numbering-database.json').read_bytes().replace(b'\r\n', b'\n')
    schema = json.loads(schema_bytes)
    if {(t['source_table'], t['trigger_name']) for t in schema['triggers']} != {
        ('order', 'order_timestamp_number'), ('product_variant', 'product_variant_erp_sku')
    }:
        raise ValueError('Reviewed trigger set changed')
    image = docker('image', 'inspect', 'postgres:16-alpine', '--format', '{{.Id}}')
    if image.returncode:
        raise ValueError('Fixture requires existing local postgres:16-alpine image; no pull performed')
    fixture_id = uuid.uuid4().hex
    name = 'eurorefurb-numbering-fixture-' + fixture_id[:12]
    container = None
    cases = []
    report = None
    try:
        created = docker('run', '-d', '--name', name, '--label', LABEL + '=' + fixture_id,
                         '--network', 'none', '--cpus', '1', '--memory', '384m', '--memory-swap', '384m',
                         '--pids-limit', '128', '--shm-size', '32m',
                         '--tmpfs', '/var/lib/postgresql/data:rw,size=192m',
                         '-e', 'POSTGRES_PASSWORD=' + secrets.token_urlsafe(32),
                         '-e', 'POSTGRES_DB=numbering_fixture', 'postgres:16-alpine',
                         'postgres', '-c', 'max_connections=16', '-c', 'shared_buffers=32MB')
        if created.returncode:
            raise ValueError('Could not create bounded fixture container')
        container = created.stdout.strip()
        for _ in range(30):
            ready = docker('exec', container, 'pg_isready', '-U', 'postgres', '-d', 'numbering_fixture', timeout=5)
            if ready.returncode == 0:
                break
            time.sleep(0.5)
        else:
            raise ValueError('Fixture database did not become ready')

        def sql(text, expected_state=None):
            result = docker('exec', '-i', container, 'psql', '-X', '-q', '-t', '-A',
                            '-v', 'ON_ERROR_STOP=1', '-U', 'postgres', '-d', 'numbering_fixture',
                            input='\\set VERBOSITY sqlstate\n' + text)
            if expected_state:
                if result.returncode == 0 or expected_state not in result.stderr:
                    raise AssertionError('Fixture expected SQLSTATE ' + expected_state)
                return None
            if result.returncode:
                raise AssertionError('Artificial SQL fixture failed; no source database involved')
            return result.stdout.strip()

        def expect(name, text, expected):
            if sql(text) != str(expected):
                raise AssertionError('Fixture assertion failed: ' + name)
            cases.append({'name': name, 'passed': True})

        def denied(name, text, state):
            sql(text, state)
            cases.append({'name': name, 'passed': True, 'expected_sqlstate': state})

        sql('''CREATE TABLE public.sku_number_counter(id boolean PRIMARY KEY DEFAULT true CHECK(id),last_id bigint NOT NULL DEFAULT 0 CHECK(last_id>=0));
CREATE TABLE public.order_number_clock(id boolean PRIMARY KEY DEFAULT true CHECK(id),last_tick bigint NOT NULL);
INSERT INTO public.sku_number_counter VALUES(true,0);
INSERT INTO public.order_number_clock VALUES(true,200000000000000);
CREATE TABLE public.product_variant(id text PRIMARY KEY,sku text,deleted_at timestamptz);
CREATE UNIQUE INDEX fixture_active_sku_unique ON public.product_variant(sku) WHERE deleted_at IS NULL;
CREATE SEQUENCE public.order_display_id_seq AS integer;
CREATE TABLE public."order"(id text PRIMARY KEY,display_id integer DEFAULT nextval('public.order_display_id_seq'),custom_display_id text);
''')
        for trigger in schema['triggers']:
            sql(trigger['function_definition'] + ';\n' + trigger['definition'] + ';')
        expect('null-sku-allocates-first', "INSERT INTO product_variant(id) VALUES('v1') RETURNING sku;", 'ERP10001')
        expect('empty-sku-allocates-next', "INSERT INTO product_variant VALUES('v2','',NULL) RETURNING sku;", 'ERP10002')
        expect('whitespace-sku-allocates-next', "INSERT INTO product_variant VALUES('v3',E' \\t\\n',NULL) RETURNING sku;", 'ERP10003')
        expect('clear-generated-sku-retains-identity', "UPDATE product_variant SET sku='' WHERE id='v1' RETURNING sku;", 'ERP10001')
        expect('clearing-generated-does-not-advance', 'SELECT last_id FROM sku_number_counter;', 3)
        expect('manual-sku-preserved', "INSERT INTO product_variant VALUES('manual','custom-SKU',NULL) RETURNING sku;", 'custom-SKU')
        expect('clearing-custom-sku-allocates', "UPDATE product_variant SET sku=NULL WHERE id='manual' RETURNING sku;", 'ERP10004')
        expect('explicit-erp-leading-zero-text-preserved', "INSERT INTO product_variant VALUES('high','ERP0012000',NULL) RETURNING sku;", 'ERP0012000')
        expect('explicit-erp-reserves-numeric-position', 'SELECT last_id FROM sku_number_counter;', 2000)
        expect('low-explicit-erp-does-not-decrement', "INSERT INTO product_variant VALUES('low','ERP010002',NULL); SELECT last_id FROM sku_number_counter;", 2000)
        expect('erp-10000-not-allocated-range', "INSERT INTO product_variant VALUES('zero','ERP10000',NULL); SELECT last_id FROM sku_number_counter;", 2000)
        expect('short-erp-custom-not-counter', "INSERT INTO product_variant VALUES('short','ERP1234',NULL); SELECT last_id FROM sku_number_counter;", 2000)
        expect('next-allocation-follows-explicit-reservation', "INSERT INTO product_variant(id) VALUES('after-high') RETURNING sku;", 'ERP12001')
        denied('active-duplicate-sku-rejected', "INSERT INTO product_variant VALUES('duplicate','ERP12001',NULL);", '23505')
        expect('soft-delete-does-not-release-counter', "UPDATE product_variant SET deleted_at=now() WHERE id='after-high'; SELECT last_id FROM sku_number_counter;", 2001)
        expect('legacy-soft-deleted-code-reuse-allowed-by-partial-index', "INSERT INTO product_variant VALUES('reuse','ERP12001',NULL) RETURNING sku;", 'ERP12001')
        sql('BEGIN; INSERT INTO product_variant(id) VALUES(\'rollback-sku\'); ROLLBACK;')
        expect('rolled-back-sku-counter-restored', 'SELECT last_id FROM sku_number_counter;', 2001)
        expect('rolled-back-sku-row-absent', "SELECT count(*) FROM product_variant WHERE id='rollback-sku';", 0)
        denied('false-singleton-key-rejected', 'INSERT INTO sku_number_counter VALUES(false,0);', '23514')
        denied('second-singleton-row-rejected', 'INSERT INTO sku_number_counter VALUES(true,0);', '23505')
        denied('negative-sku-counter-rejected', 'UPDATE sku_number_counter SET last_id=-1;', '23514')
        sql('UPDATE sku_number_counter SET last_id=9223372036854765807;')
        denied('sku-prefix-offset-overflow-fails', "INSERT INTO product_variant(id) VALUES('overflow-sku');", '22003')
        expect('overflow-does-not-commit-counter', 'SELECT last_id FROM sku_number_counter;', 9223372036854765807)
        sql('DELETE FROM sku_number_counter;')
        denied('missing-counter-auto-sku-fails-strict', "INSERT INTO product_variant(id) VALUES('missing-auto');", 'P0002')
        expect('legacy-explicit-sku-with-missing-counter-succeeds', "INSERT INTO product_variant VALUES('missing-explicit','ERP99999',NULL) RETURNING sku;", 'ERP99999')
        sql('INSERT INTO sku_number_counter VALUES(true,100000);')

        expect('order-trigger-overwrites-supplied-number', '''INSERT INTO "order"(id,custom_display_id) VALUES('order1','client-supplied') RETURNING custom_display_id;''', '2033051811332000001')
        expect('order-clock-advances-one-when-wall-clock-behind', 'SELECT last_tick FROM order_number_clock;', 200000000000001)
        sql('BEGIN; INSERT INTO "order"(id) VALUES(\'rollback-order\'); ROLLBACK;')
        expect('order-clock-rolls-back', 'SELECT last_tick FROM order_number_clock;', 200000000000001)
        expect('native-sequence-does-not-roll-back', 'SELECT last_value FROM order_display_id_seq;', 2)
        expect('next-order-has-sequence-gap', '''INSERT INTO "order"(id) VALUES('order2') RETURNING display_id;''', 3)
        expect('order-trigger-is-insert-only-legacy-update-allowed', '''UPDATE "order" SET custom_display_id='artificial-manual' WHERE id='order2' RETURNING custom_display_id;''', 'artificial-manual')
        sql('DELETE FROM order_number_clock;')
        denied('missing-order-clock-insert-fails', '''INSERT INTO "order"(id) VALUES('missing-clock');''', 'P0002')
        sql('INSERT INTO order_number_clock VALUES(true,9223372036854775807);')
        denied('max-order-clock-increment-fails', '''INSERT INTO "order"(id) VALUES('overflow-clock');''', '22003')
        sql('UPDATE order_number_clock SET last_tick=200000000000000; TRUNCATE product_variant; UPDATE sku_number_counter SET last_id=0; TRUNCATE "order";')

        # Six distinct connections concurrently execute the actual trigger; no
        # Python allocator model or client-generated numbers stand in for SQL.
        def allocate(worker):
            sql(f"INSERT INTO product_variant(id) SELECT 'parallel-sku-{worker}-'||g FROM generate_series(1,10) g;")
            sql(f'''INSERT INTO "order"(id) SELECT 'parallel-order-{worker}-'||g FROM generate_series(1,10) g;''')

        with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
            list(pool.map(allocate, range(6)))
        expect('parallel-sku-60-rows', 'SELECT count(*) FROM product_variant;', 60)
        expect('parallel-sku-60-unique', 'SELECT count(DISTINCT sku) FROM product_variant;', 60)
        expect('parallel-sku-counter-60', 'SELECT last_id FROM sku_number_counter;', 60)
        expect('parallel-sku-contiguous-artificial-run', "SELECT min(substring(sku FROM 4)::bigint)||':'||max(substring(sku FROM 4)::bigint) FROM product_variant;", '10001:10060')
        expect('parallel-order-60-rows', 'SELECT count(*) FROM "order";', 60)
        expect('parallel-order-60-unique', 'SELECT count(DISTINCT custom_display_id) FROM "order";', 60)
        expect('parallel-order-clock-60', 'SELECT last_tick FROM order_number_clock;', 200000000000060)
        expect('parallel-order-19-digit-output', '''SELECT bool_and(custom_display_id ~ '^[0-9]{19}$') FROM "order";''', 't')
        report = {'format_version': 1, 'source_schema_sha256': hashlib.sha256(schema_bytes).hexdigest(),
                  'scope': 'Actual captured PostgreSQL trigger bodies executed in disposable isolated PostgreSQL with minimal artificial tables; not full Medusa workflows, historical import or MySQL implementation acceptance.',
                  'image': 'postgres:16-alpine', 'local_image_id': image.stdout.strip(),
                  'isolation': {'network': 'none', 'published_ports': 0, 'cpus': 1, 'memory_mib': 384,
                                'data': 'bounded temporary tmpfs; no host/database volumes', 'source_business_connection': False},
                  'concurrent_connections': 6, 'parallel_rows_per_domain': 60, 'cases': cases,
                  'observed_legacy_gaps': ['Explicit valid ERP insert may succeed with missing singleton counter',
                                           'Order custom_display_id can be changed by UPDATE because trigger is INSERT-only',
                                           'Active-only unique SKU index permits reuse of soft-deleted code; counter floor alone does not prevent explicit reuse'],
                  'remaining': ['Target Gin/MySQL persistent allocators and full transaction/idempotency tests',
                                'Full schema constraints/foreign keys and all workflow paths',
                                'Formal write freeze and history/sequence seed reconciliation'],
                  'status': 'legacy-isolated-numbering-tested-not-migrated'}
    finally:
        if container:
            ownership = docker('inspect', container, '--format', '{{index .Config.Labels "' + LABEL + '"}}')
            if ownership.returncode or ownership.stdout.strip() != fixture_id:
                raise RuntimeError('Fixture ownership verification failed; automatic cleanup refused')
            removed = docker('rm', '-f', '-v', container)
            if removed.returncode:
                raise RuntimeError('Fixture container cleanup failed')
    if report:
        report['cleanup'] = 'owned fixture container and anonymous volumes removed after verification'
        (OUT / 'numbering-postgres-fixture-report.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
        print(json.dumps({'cases': len(cases), 'parallel_rows_per_domain': 60, 'concurrent_connections': 6,
                          'cleanup': 'complete', 'status': report['status']}))


if __name__ == '__main__':
    main()
