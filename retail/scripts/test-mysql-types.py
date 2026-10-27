"""Artificial-only MySQL 8.4 dialect checks in a bounded disposable container."""
import argparse
import json
import hashlib
from pathlib import Path
import secrets
import subprocess
import time
import uuid

ROOT = Path(__file__).resolve().parents[2]
LABEL = 'eurorefurb.mysql-types-fixture'


def docker(*args, input=None, timeout=90):
    return subprocess.run(['docker', *args], input=input, capture_output=True,
                          encoding='utf-8', timeout=timeout)


def literal(value):
    return "CONVERT(X'" + value.encode('utf-8').hex() + "' USING utf8mb4)"


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--registry-test-binary', type=Path)
    args = parser.parse_args()
    image = docker('image', 'inspect', 'mysql:8.4', '--format', '{{.Id}}')
    if image.returncode:
        raise RuntimeError('Existing local mysql:8.4 image required; no pull performed')
    owner = uuid.uuid4().hex
    password = secrets.token_urlsafe(32)
    container = None
    cases = []
    report = None
    mode = 'STRICT_ALL_TABLES,NO_ZERO_DATE,NO_ZERO_IN_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION'
    try:
        started = docker('run', '-d', '--name', 'eurorefurb-mysql-types-' + owner[:12],
                         '--label', LABEL + '=' + owner, '--network', 'none', '--cpus', '1',
                         '--memory', '768m', '--memory-swap', '768m', '--pids-limit', '192',
                         '--tmpfs', '/var/lib/mysql:rw,size=512m',
                         '-e', 'MYSQL_ROOT_PASSWORD=' + password, '-e', 'MYSQL_DATABASE=type_fixture',
                         'mysql:8.4', '--skip-log-bin', '--innodb-buffer-pool-size=64M',
                         '--performance-schema=OFF', '--mysqlx=OFF', '--max-connections=16')
        if started.returncode:
            raise RuntimeError('Isolated fixture creation failed')
        container = started.stdout.strip()

        def sql(query, expected_error=None):
            result = docker('exec', '-i', '-e', 'MYSQL_PWD=' + password, container,
                            'mysql', '--user=root', '--protocol=socket', '--batch', '--raw',
                            '--skip-column-names', '--default-character-set=utf8mb4', 'type_fixture',
                            input="SET SESSION sql_mode='" + mode + "'; SET SESSION time_zone='+00:00';\n" + query)
            if expected_error:
                if result.returncode == 0 or ('ERROR ' + str(expected_error) + ' ') not in result.stderr:
                    raise AssertionError('Expected MySQL error ' + str(expected_error))
                return None
            if result.returncode:
                raise AssertionError('Artificial MySQL SQL failed (no source connection): ' + result.stderr.strip())
            return result.stdout.strip()

        for _ in range(100):
            ready = docker('exec', '-e', 'MYSQL_PWD=' + password, container,
                           'mysql', '-uroot', '--protocol=socket', '--batch', '--skip-column-names',
                           '-e', "SELECT SCHEMA_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME='type_fixture'", timeout=5)
            if ready.returncode == 0 and ready.stdout.strip() == 'type_fixture':
                # Initialization uses a temporary socket server first; only
                # proceed when the final entrypoint exec is mysqld PID 1.
                process = docker('exec', container, 'cat', '/proc/1/comm')
                if process.returncode == 0 and process.stdout.strip() == 'mysqld':
                    break
            time.sleep(0.5)
        else:
            raise RuntimeError('Bounded MySQL fixture did not become ready')

        def expect(name, query, expected):
            actual = sql(query)
            if actual != str(expected):
                raise AssertionError('MySQL fixture assertion failed: ' + name + '; artificial output=' + repr(actual))
            cases.append({'name': name, 'passed': True})

        def denied(name, query, error):
            sql(query, error)
            cases.append({'name': name, 'passed': True, 'expected_mysql_error': error})

        version = sql('SELECT VERSION();')
        expect('0900-binary-no-pad-collation', "SELECT PAD_ATTRIBUTE FROM information_schema.COLLATIONS WHERE COLLATION_NAME='utf8mb4_0900_bin';", 'NO PAD')
        sql('''CREATE TABLE exact_rows (
id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin PRIMARY KEY,
amount DECIMAL(38,18) NOT NULL,
payload LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin,
at_utc DATETIME(6) NOT NULL,
flag BOOLEAN NOT NULL CHECK (flag IN (0,1)),
CHECK(payload IS NULL OR JSON_VALID(payload))
) ENGINE=InnoDB;''')
        amount = '12345678901234567890.123456789012345678'
        payload = '{"big":123456789012345678901234567890,"scaled":1.2300,"null":null,"duplicate":1,"duplicate":2}'
        sql("INSERT INTO exact_rows VALUES('ID'," + amount + ',' + literal(payload) + ",'2026-10-27 01:02:03.123456',false);")
        expect('38-digit-decimal-exact-roundtrip', "SELECT CAST(amount AS CHAR) FROM exact_rows WHERE id='ID';", amount)
        expect('json-raw-bytes-large-number-scale-duplicate-keys-retained', "SELECT HEX(payload) FROM exact_rows WHERE id='ID';", payload.encode().hex().upper())
        expect('datetime-six-fractional-digits-retained', "SELECT DATE_FORMAT(at_utc,'%Y-%m-%d %H:%i:%s.%f') FROM exact_rows WHERE id='ID';", '2026-10-27 01:02:03.123456')
        expect('boolean-false-not-missing', "SELECT flag FROM exact_rows WHERE id='ID';", 0)
        sql("INSERT INTO exact_rows VALUES('id',0,'null','2026-10-27',true),('id ',0,NULL,'2026-10-27',false);")
        expect('case-and-trailing-space-identities-distinct', 'SELECT COUNT(*) FROM exact_rows;', 3)
        expect('json-null-not-sql-null', "SELECT payload IS NULL,JSON_TYPE(payload) FROM exact_rows WHERE id='id';", '0\tNULL')
        expect('sql-null-not-json-null', "SELECT payload IS NULL FROM exact_rows WHERE id='id ';", 1)
        denied('duplicate-identical-id-denied', "INSERT INTO exact_rows VALUES('ID',0,NULL,'2026-10-27',false);", 1062)
        unicode_id = '人工😀' * 40
        sql('INSERT INTO exact_rows VALUES(' + literal(unicode_id) + ",0,'{}','2026-10-27',false);")
        expect('unicode-id-not-ascii-loss', 'SELECT HEX(id) FROM exact_rows WHERE id=' + literal(unicode_id) + ';', unicode_id.encode().hex().upper())
        denied('id-over-191-characters-denied', 'INSERT INTO exact_rows VALUES(' + literal('x' * 192) + ",0,NULL,'2026-10-27',false);", 1406)
        denied('decimal-integer-overflow-denied', "INSERT INTO exact_rows VALUES('overflow',100000000000000000000,NULL,'2026-10-27',false);", 1264)
        sql("INSERT INTO exact_rows VALUES('negative',-0.000000000000000001,NULL,'2026-10-27',false);")
        expect('negative-small-decimal-exact', "SELECT CAST(amount AS CHAR) FROM exact_rows WHERE id='negative';", '-0.000000000000000001')
        denied('invalid-json-check-enforced', "INSERT INTO exact_rows VALUES('bad-json',0,'{bad','2026-10-27',false);", 3819)
        denied('boolean-outside-zero-one-check-enforced', "INSERT INTO exact_rows VALUES('bad-bool',0,NULL,'2026-10-27',2);", 3819)
        denied('zero-date-denied', "INSERT INTO exact_rows VALUES('zero-date',0,NULL,'0000-00-00',false);", 1292)
        denied('invalid-calendar-date-denied', "INSERT INTO exact_rows VALUES('bad-date',0,NULL,'2026-02-30',false);", 1292)
        denied('required-time-null-denied', "INSERT INTO exact_rows VALUES('null-date',0,NULL,NULL,false);", 1048)
        sql("INSERT INTO exact_rows VALUES('rounding',0.1234567890123456789,NULL,'2026-10-27 01:02:03.1234567',false);")
        expect('strict-mode-still-rounds-excess-decimal-scale', "SELECT CAST(amount AS CHAR) FROM exact_rows WHERE id='rounding';", '0.123456789012345679')
        expect('strict-mode-still-rounds-excess-time-precision', "SELECT DATE_FORMAT(at_utc,'%f') FROM exact_rows WHERE id='rounding';", '123457')
        expect('datetime-session-zone-does-not-convert-stored-instant', "SET SESSION time_zone='+08:00'; SELECT DATE_FORMAT(at_utc,'%H:%i:%s.%f') FROM exact_rows WHERE id='ID';", '01:02:03.123456')
        sql('CREATE TABLE native_json_comparison(payload JSON NOT NULL) ENGINE=InnoDB; INSERT INTO native_json_comparison VALUES(' + literal(payload) + ');')
        expect('native-json-does-not-preserve-original-text', 'SELECT HEX(CAST(payload AS CHAR))=' + literal(payload.encode().hex().upper()) + ' FROM native_json_comparison;', 0)
        expect('native-json-large-integer-becomes-double', "SELECT JSON_TYPE(JSON_EXTRACT(payload,'$.big')) FROM native_json_comparison;", 'DOUBLE')
        expect('native-json-normalizes-scale-and-duplicate-key', "SELECT JSON_EXTRACT(payload,'$.scaled'),JSON_EXTRACT(payload,'$.duplicate') FROM native_json_comparison;", '1.23\t2')
        sql('''CREATE TABLE active_identity (
id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin PRIMARY KEY,
handle VARCHAR(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin,
deleted_at DATETIME(6),
active_handle VARCHAR(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin GENERATED ALWAYS AS (IF(deleted_at IS NULL,handle,NULL)) STORED,
UNIQUE KEY active_handle_unique(active_handle)
) ENGINE=InnoDB;''')
        sql("INSERT INTO active_identity(id,handle) VALUES('a','same');")
        denied('active-generated-key-rejects-duplicate', "INSERT INTO active_identity(id,handle) VALUES('b','same');", 1062)
        sql("UPDATE active_identity SET deleted_at='2026-10-27' WHERE id='a'; INSERT INTO active_identity(id,handle) VALUES('b','same');")
        expect('soft-deleted-text-retained-and-active-reuse-supported', "SELECT COUNT(*) FROM active_identity WHERE handle='same';", 2)
        denied('restoring-deleted-conflicting-identity-denied', "UPDATE active_identity SET deleted_at=NULL WHERE id='a';", 1062)
        sql("INSERT INTO active_identity(id,handle) VALUES('c','Same'),('d','same '),('null1',NULL),('null2',NULL);")
        expect('active-case-space-and-null-uniqueness-source-semantics', 'SELECT COUNT(*) FROM active_identity;', 6)
        long_handle = '😀' * 512
        sql('INSERT INTO active_identity(id,handle) VALUES(\'long\',' + literal(long_handle) + ');')
        expect('512-four-byte-handle-full-unique-index-fits', "SELECT CHAR_LENGTH(handle),OCTET_LENGTH(handle) FROM active_identity WHERE id='long';", '512\t2048')
        denied('full-composite-512-utf8-keys-exceed-innodb-limit', '''CREATE TABLE too_wide (
a VARCHAR(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
b VARCHAR(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
UNIQUE KEY full_values(a,b)) ENGINE=InnoDB ROW_FORMAT=DYNAMIC;''', 1071)
        sql('''CREATE TABLE naive_unique (value VARCHAR(100), deleted_at DATETIME(6),UNIQUE KEY wrong(value,deleted_at)) ENGINE=InnoDB;
INSERT INTO naive_unique VALUES('same',NULL),('same',NULL);''')
        expect('value-deleted-at-null-pair-is-not-active-unique', 'SELECT COUNT(*) FROM naive_unique;', 2)
        sql('''CREATE TABLE option_titles (
id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin PRIMARY KEY,
title VARCHAR(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
is_exclusive BOOLEAN NOT NULL CHECK(is_exclusive IN(0,1)),deleted_at DATETIME(6),
active_shared_title VARCHAR(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin
GENERATED ALWAYS AS (IF(deleted_at IS NULL AND is_exclusive=0,title,NULL)) STORED,
UNIQUE KEY shared_title(active_shared_title)) ENGINE=InnoDB;
INSERT INTO option_titles(id,title,is_exclusive) VALUES('shared','Capacity',false),('exclusive1','Capacity',true),('exclusive2','Capacity',true);''')
        expect('exclusive-option-same-title-allowed-alongside-shared', 'SELECT COUNT(*) FROM option_titles;', 3)
        denied('nonexclusive-active-title-global-duplicate-denied', "INSERT INTO option_titles(id,title,is_exclusive) VALUES('shared2','Capacity',false);", 1062)
        denied('exclusive-to-shared-conflict-denied', "UPDATE option_titles SET is_exclusive=false WHERE id='exclusive1';", 1062)
        sql('''CREATE TABLE option_values (
id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin PRIMARY KEY,
option_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin,
value_text VARCHAR(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
deleted_at DATETIME(6),
active_value VARCHAR(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin GENERATED ALWAYS AS(IF(deleted_at IS NULL,value_text,NULL)) STORED,
UNIQUE KEY full_option_value(option_id,active_value),
FOREIGN KEY(option_id) REFERENCES option_titles(id)) ENGINE=InnoDB;
INSERT INTO option_values(id,option_id,value_text) VALUES('v1','shared','128GB'),('vnull1',NULL,'128GB'),('vnull2',NULL,'128GB');''')
        expect('null-parent-composite-unique-retains-source-null-semantics', 'SELECT COUNT(*) FROM option_values;', 3)
        denied('nonnull-parent-active-value-duplicate-denied', "INSERT INTO option_values(id,option_id,value_text) VALUES('v2','shared','128GB');", 1062)
        sql("UPDATE option_values SET deleted_at='2026-10-27' WHERE id='v1'; INSERT INTO option_values(id,option_id,value_text) VALUES('v2','shared','128GB');")
        expect('soft-deleted-composite-value-can-be-reused', "SELECT COUNT(*) FROM option_values WHERE option_id='shared';", 2)
        denied('composite-value-restore-conflict-denied', "UPDATE option_values SET deleted_at=NULL WHERE id='v1';", 1062)
        denied('orphan-parent-foreign-key-denied', "INSERT INTO option_values(id,option_id,value_text) VALUES('orphan','missing','128GB');", 1452)
        denied('foreign-key-parent-id-case-sensitive', "INSERT INTO option_values(id,option_id,value_text) VALUES('case-parent','Shared','256GB');", 1452)
        denied('referenced-parent-delete-restricted', "DELETE FROM option_titles WHERE id='shared';", 1451)
        sql('''CREATE TABLE singleton_counter (id BOOLEAN PRIMARY KEY CHECK(id=1),last_id BIGINT NOT NULL CHECK(last_id>=0)) ENGINE=InnoDB;
INSERT INTO singleton_counter VALUES(true,0);''')
        denied('singleton-false-key-denied', 'INSERT INTO singleton_counter VALUES(false,0);', 3819)
        denied('singleton-second-true-key-denied', 'INSERT INTO singleton_counter VALUES(true,1);', 1062)
        denied('singleton-negative-value-denied', 'UPDATE singleton_counter SET last_id=-1 WHERE id=true;', 3819)
        expect('counter-transaction-rollback', 'START TRANSACTION; UPDATE singleton_counter SET last_id=1 WHERE id=true; ROLLBACK; SELECT last_id FROM singleton_counter;', 0)
        sql('''CREATE TABLE participation_unique (
id INT PRIMARY KEY,parent_id VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin,
value_text VARCHAR(512) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin,
deleted_at DATETIME(6),is_exclusive BOOLEAN NOT NULL CHECK(is_exclusive IN(0,1)),
active_flag TINYINT GENERATED ALWAYS AS(IF(deleted_at IS NULL AND is_exclusive=0,1,NULL)) STORED,
UNIQUE KEY exact_values(parent_id,value_text,active_flag)) ENGINE=InnoDB;
INSERT INTO participation_unique(id,parent_id,value_text,is_exclusive) VALUES(1,'parent','value',false);''')
        denied('participation-flag-full-composite-active-duplicate-denied', "INSERT INTO participation_unique(id,parent_id,value_text,is_exclusive) VALUES(2,'parent','value',false);", 1062)
        sql("UPDATE participation_unique SET deleted_at='2026-10-27' WHERE id=1; INSERT INTO participation_unique(id,parent_id,value_text,is_exclusive) VALUES(2,'parent','value',false),(3,'parent','value',true),(4,'parent','value',true),(5,NULL,'value',false),(6,NULL,'value',false),(7,'parent',NULL,false),(8,'parent',NULL,false);")
        expect('participation-flag-deleted-exclusive-and-null-duplicates-allowed', 'SELECT COUNT(*) FROM participation_unique;', 8)
        denied('participation-flag-restore-conflict-denied', 'UPDATE participation_unique SET deleted_at=NULL WHERE id=1;', 1062)
        denied('participation-flag-exclusive-to-shared-conflict-denied', 'UPDATE participation_unique SET is_exclusive=false WHERE id=3;', 1062)
        sql('''CREATE TABLE text_prefix_access (id INT PRIMARY KEY,
value_text LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL,
INDEX prefix_access(value_text(128))) ENGINE=InnoDB;''')
        prefix = '\U0001f680' * 128
        sql('INSERT INTO text_prefix_access VALUES (1,' + literal(prefix + 'A') + '),(2,' + literal(prefix + 'B') + '),(3,' + literal(prefix + 'A') + ');')
        expect('nonunique-text-prefix-retains-distinct-full-values-and-duplicates', 'SELECT COUNT(*) FROM text_prefix_access;', 3)
        expect('nonunique-text-prefix-full-value-filter-excludes-same-prefix-neighbor', 'SELECT GROUP_CONCAT(id ORDER BY id) FROM text_prefix_access FORCE INDEX(prefix_access) WHERE value_text=' + literal(prefix + 'A') + ';', '1,3')
        expect('nonunique-text-prefix-full-value-comparison-keeps-trailing-space', 'SELECT COUNT(*) FROM text_prefix_access FORCE INDEX(prefix_access) WHERE value_text=' + literal(prefix + 'A ') + ';', 0)
        review_file = ROOT / 'docs/migration/mysql-relational-review.sql'
        review_bytes = review_file.read_bytes().replace(b'\r\n', b'\n')
        review_report = json.loads((ROOT / 'docs/migration/mysql-relational-review.json').read_text(encoding='utf-8'))
        if hashlib.sha256(review_bytes).hexdigest() != review_report['ddl_sha256']:
            raise AssertionError('Review DDL checksum mismatch')
        sql('CREATE DATABASE relational_fixture CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin; USE relational_fixture;\n' + review_bytes.decode())
        expect('all-review-tables-created', "SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA='relational_fixture';", 159)
        expect('all-review-source-columns-created', "SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA='relational_fixture' AND GENERATION_EXPRESSION='';", 1691)
        expect('all-review-generated-index-columns-created', "SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA='relational_fixture' AND GENERATION_EXPRESSION<>'';", review_report['counts']['generated_index_columns'])
        expect('every-review-table-innodb', "SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA='relational_fixture' AND ENGINE='InnoDB';", 159)
        expect('all-emitted-review-indexes-created', "SELECT COUNT(DISTINCT TABLE_NAME,INDEX_NAME) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA='relational_fixture' AND INDEX_NAME LIKE 'idx_source_%';", review_report['counts']['emitted_indexes'])
        expect('nonunique-review-prefix-index-count', "SELECT COUNT(DISTINCT TABLE_NAME,INDEX_NAME) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA='relational_fixture' AND INDEX_NAME LIKE 'idx_source_%' AND SUB_PART=128 AND NON_UNIQUE=1;", review_report['counts']['nonunique_prefix_indexes'])
        # Exercise the actual generated business tables/indexes, not a copied
        # approximation of their predicates. All values below are artificial.
        for table, active_states, inactive_state, base in (
            ('after_sales_requests', ('pending', 'processing'), 'completed',
             {'order_id': 'fixture-order', 'customer_id': 'fixture-subject', 'type': 'return', 'reason': 'fixture', 'items': '[]'}),
            ('device_assignments', ('reserved', 'packed', 'shipped'), 'returned',
             {'device_id': 'fixture-device', 'order_id': 'fixture-order', 'item_id': 'fixture-item', 'source_id': 'fixture-source'})):
            def insert(row_id, status, deleted=False):
                values = {'id': row_id, **base, 'status': status, 'created_at': '2026-10-27 00:00:00',
                          'updated_at': '2026-10-27 00:00:00', 'deleted_at': '2026-10-27 00:00:00' if deleted else None}
                return 'INSERT INTO relational_fixture.`' + table + '` (' + ','.join('`' + key + '`' for key in values) + ') VALUES (' + ','.join('NULL' if value is None else literal(value) for value in values.values()) + ');'

            sql(insert('first', active_states[0]))
            for state in active_states:
                denied(table + '-active-' + state + '-duplicate-denied', insert('conflict', state), 1062)
            sql(insert('inactive1', inactive_state) + insert('inactive2', inactive_state) + insert('deleted', active_states[0], True))
            expect(table + '-inactive-and-deleted-history-retained', 'SELECT COUNT(*) FROM relational_fixture.`' + table + '`;', 4)
            denied(table + '-inactive-to-active-conflict-denied', 'UPDATE relational_fixture.`' + table + "` SET status=" + literal(active_states[-1]) + " WHERE id='inactive1';", 1062)
            denied(table + '-restore-active-conflict-denied', 'UPDATE relational_fixture.`' + table + "` SET deleted_at=NULL WHERE id='deleted';", 1062)
            sql('UPDATE relational_fixture.`' + table + "` SET status=" + literal(inactive_state) + " WHERE id='first';" + insert('replacement', active_states[-1]))
            expect(table + '-completed-or-returned-releases-active-key', 'SELECT COUNT(*) FROM relational_fixture.`' + table + '`;', 5)
        if args.registry_test_binary:
            if not args.registry_test_binary.is_file():
                raise RuntimeError('Compiled Linux registry test binary required')
            copied = docker('cp', str(args.registry_test_binary.resolve()), container + ':/tmp/registry.test')
            if copied.returncode:
                raise RuntimeError('Could not copy isolated registry tests')
            tested = docker('exec', '-e', 'MYSQL_PWD=' + password, '-e', 'UNIQUEKEY_TEST=1', container,
                            '/tmp/registry.test', '-test.v', '-test.timeout=60s', timeout=75)
            if tested.returncode:
                raise AssertionError('Artificial Go registry tests failed: ' + tested.stdout + tested.stderr)
            cases.append({'name': 'go-full-value-registry-collision-lifecycle-rollback-eight-way-concurrency', 'passed': True})
        report = {'format_version': 1, 'scope': 'Actual MySQL dialect on artificial minimal DDL/data only; not full domain migrations, source import, allocator concurrency or application validation acceptance.',
                  'image': 'mysql:8.4', 'local_image_id': image.stdout.strip(), 'server_version': version,
                  'isolation': {'network': 'none', 'published_ports': 0, 'cpus': 1, 'memory_mib': 768, 'data_tmpfs_mib': 512, 'source_business_connection': False},
                  'session_sql_mode': mode, 'session_timezone': '+00:00', 'cases': cases,
                  'relational_review_ddl_sha256': review_report['ddl_sha256'],
                  'required_application_checks': ['Reject decimal scale/integer overflow before SQL; strict mode rounds excess scale',
                      'Reject excess timestamp precision or explicitly normalize with documented policy; strict mode rounds',
                      'Convert source instants to UTC before DATETIME(6), configure every connection UTC',
                      'Composite long-text identity needs full-value transactional registry/collision verification, never prefix uniqueness'],
                  'remaining': ['Remaining held source constraints/indexes and formal domain migrations', 'Exact JSON/number/time importer validation and real data reconciliation', 'Concurrency, failure recovery, Gin and UI behavior']}
    finally:
        password = ''
        if container:
            ownership = docker('inspect', container, '--format', '{{index .Config.Labels "' + LABEL + '"}}')
            if ownership.returncode or ownership.stdout.strip() != owner:
                raise RuntimeError('Fixture ownership mismatch; cleanup refused')
            if docker('rm', '-f', '-v', container).returncode:
                raise RuntimeError('Owned fixture cleanup failed')
    if report:
        report['cleanup'] = 'owned temporary fixture removed after label verification'
        (ROOT / 'docs/migration/mysql-type-fixture-report.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
        print(json.dumps({'cases': len(cases), 'server_version': version, 'cleanup': 'complete', 'scope': 'artificial-mysql-types-only'}))


if __name__ == '__main__':
    main()
