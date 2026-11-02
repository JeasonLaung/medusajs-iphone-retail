"""Run shared-option transactions on one owned private-socket native MySQL.

No shared database, application env, provider or server is used. Browser mode
uses only an owned random namespace in the existing local Redis.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import shutil
import signal
import socket
import subprocess
import tempfile
import time
import urllib.request
import urllib.error

ROOT = Path(__file__).resolve().parents[2]
GO = '/opt/homebrew/opt/go/bin/go'
MYSQL = Path('/opt/homebrew/bin/mysql').resolve()


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def write(path, value):
    path.write_text(json.dumps(value, indent=2) + '\n')


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--browser', action='store_true', help='Also run actual private API and Ant UI on temporary ports')
    parser.add_argument('--astro', action='store_true', help='Include built Astro product pages in the browser joint')
    args = parser.parse_args()
    args.browser = args.browser or args.astro
    if shutil.disk_usage(ROOT).free < 2 * 1024**3:
        raise RuntimeError('2 GiB free required')
    owner = secrets.token_hex(6)
    snapshot = ROOT / 'logs/gin-migration' / ('option-definitions-native-' + owner)
    snapshot.mkdir()
    backend = snapshot / 'retail/backend'
    shutil.copytree(ROOT / 'retail/backend', backend, ignore=shutil.ignore_patterns(
        '.env*', '*.log', 'node_modules', 'dist', '__pycache__'))
    files = {str(p.relative_to(backend)): sha(p) for p in backend.rglob('*') if p.is_file()}
    if any(sha(ROOT / 'retail/backend' / name) != digest for name, digest in files.items()):
        raise RuntimeError('Source changed during copy')
    admin = snapshot / 'retail/admin'
    admin_files = {}
    if args.browser:
        shutil.copytree(ROOT / 'retail/admin', admin, ignore=shutil.ignore_patterns(
            'node_modules', '.env*', 'dist', '*.log', 'test-results', 'playwright-report'))
        admin_files = {str(p.relative_to(admin)): sha(p) for p in admin.rglob('*') if p.is_file()}
        if any(sha(ROOT / 'retail/admin' / n) != h for n, h in admin_files.items()):
            raise RuntimeError('Admin source changed during copy')
        (admin / 'node_modules').symlink_to(ROOT / 'retail/admin/node_modules', target_is_directory=True)
    storefront = snapshot / 'retail/storefront'
    storefront_files = {}
    if args.astro:
        shutil.copytree(ROOT / 'retail/storefront', storefront, ignore=shutil.ignore_patterns(
            'node_modules', '.astro', '.env*', 'dist', '*.log', 'test-results', 'playwright-report'))
        storefront_files = {str(p.relative_to(storefront)): sha(p) for p in storefront.rglob('*') if p.is_file()}
        if any(sha(ROOT / 'retail/storefront' / n) != h for n, h in storefront_files.items()):
            raise RuntimeError('Storefront source changed during copy')
        (storefront / 'node_modules').symlink_to(ROOT / 'retail/storefront/node_modules', target_is_directory=True)
    elif args.browser:
        # Admin packages the migrated public catalog images and their exact URL
        # allowlist. Freeze these dependencies even without an Astro rehearsal.
        for relative in ['src/lib/public-image-url.ts', 'public/swappie']:
            source = ROOT / 'retail/storefront' / relative
            target = storefront / relative
            target.parent.mkdir(parents=True, exist_ok=True)
            if source.is_dir():
                shutil.copytree(source, target)
            else:
                shutil.copy2(source, target)
        storefront_files = {str(p.relative_to(storefront)): sha(p) for p in storefront.rglob('*') if p.is_file()}
        if any(sha(ROOT / 'retail/storefront' / n) != h for n, h in storefront_files.items()):
            raise RuntimeError('Catalog image dependencies changed during copy')
    def frozen():
        actual = {str(p.relative_to(backend)): sha(p) for p in backend.rglob('*') if p.is_file()}
        if actual != files:
            raise RuntimeError('Frozen source changed')
        if any(sha(admin / n) != h for n, h in admin_files.items()):
            raise RuntimeError('Frozen Admin source changed')
        if any(sha(storefront / n) != h for n, h in storefront_files.items()):
            raise RuntimeError('Frozen storefront source changed')
    binary = snapshot / 'catalog.test'
    env = {k: os.environ[k] for k in ['PATH', 'HOME', 'TMPDIR'] if k in os.environ}
    if args.astro:
        with (snapshot / 'astro-build.log').open('w') as output:
            subprocess.run(['/opt/homebrew/opt/node/bin/node', 'node_modules/astro/bin/astro.mjs', 'build'],
                cwd=storefront, env=dict(env, ASTRO_TELEMETRY_DISABLED='1'),
                stdout=output, stderr=subprocess.STDOUT, timeout=120, check=True)
    with (snapshot / 'compile.log').open('w') as output:
        subprocess.run([GO, 'test', '-c', '-o', str(binary), './internal/catalog'], cwd=backend,
                       env=env, stdout=output, stderr=subprocess.STDOUT, timeout=120, check=True)
    api_binary = snapshot / 'api'
    if args.browser:
        with (snapshot / 'api-compile.log').open('w') as output:
            subprocess.run([GO, 'build', '-o', str(api_binary), './cmd/api'], cwd=backend,
                           env=env, stdout=output, stderr=subprocess.STDOUT, timeout=120, check=True)
    sockets = Path(tempfile.mkdtemp(prefix='eroptions-' + owner + '-', dir='/private/tmp'))
    address = sockets / 'mysql.sock'
    database = 'eurorefurb_options_' + owner
    redis_prefix = 'eurorefurb:o' + owner + ':v1:'
    data = snapshot / 'mysql-data'
    write(snapshot / 'manifest.json', dict(owner=owner, source_sha256=files,
          binary_sha256=sha(binary), socket=str(address), database=database,
          migrations=99, production_source_adapted=False, admin_source_sha256=admin_files,
          browser=args.browser, api_binary_sha256=sha(api_binary) if args.browser else None,
          redis_prefix=redis_prefix if args.browser else None, astro=args.astro,
          storefront_source_sha256=storefront_files))
    server = None
    api = None
    astro = None
    browser = None
    redis_cleanup_count = 0
    def mysql(query):
        p = subprocess.run([str(MYSQL), '--no-defaults', '--protocol=SOCKET', '--socket=' + str(address),
                            '--user=root', '--batch', '--skip-column-names'], input=query,
                           text=True, capture_output=True, timeout=30, env=env)
        if p.returncode:
            raise RuntimeError('Owned MySQL command failed')
        return p.stdout.strip()
    try:
        data.mkdir(mode=0o700)
        subprocess.run([str(MYSQL.with_name('mysqld')), '--no-defaults', '--initialize-insecure',
                        '--datadir=' + str(data), '--innodb-buffer-pool-size=64M', '--innodb-redo-log-capacity=64M',
                        '--log-error=' + str(snapshot / 'mysql-initialize.log')], env=env, capture_output=True,
                       timeout=90, check=True)
        with (snapshot / 'mysql-launch.log').open('w') as output:
            server = subprocess.Popen([str(MYSQL.with_name('mysqld')), '--no-defaults', '--datadir=' + str(data),
                '--socket=' + str(address), '--pid-file=' + str(snapshot / 'mysql.pid'),
                '--log-error=' + str(snapshot / 'mysql-server.log'), '--skip-networking', '--mysqlx=OFF',
                '--skip-log-bin', '--innodb-buffer-pool-size=64M', '--innodb-redo-log-capacity=64M',
                '--innodb-native-foreign-keys=ON'], env=env, stdout=output, stderr=subprocess.STDOUT)
        deadline = time.monotonic() + 45
        while time.monotonic() < deadline:
            if server.poll() is not None:
                raise RuntimeError('Owned MySQL exited')
            if address.exists():
                try:
                    facts = mysql('SELECT VERSION(),@@innodb_native_foreign_keys,@@foreign_key_checks,@@skip_networking')
                    if facts.split('\t')[1:] == ['1', '1', '1']:
                        write(snapshot / 'mysql-runtime.json', dict(version=facts.split('\t')[0], native_foreign_keys=True,
                              foreign_key_checks=True, networking_disabled=True, pid=server.pid))
                        break
                except RuntimeError:
                    pass
            time.sleep(0.2)
        else:
            raise RuntimeError('Owned MySQL readiness timed out')
        frozen()
        test_env = dict(env, OPTION_DEFINITIONS_NATIVE='1', OPTION_TEST_DATABASE=database,
                        OPTION_TEST_SOCKET=str(address), OPTION_TEST_KEEP_DB='1' if args.browser else '0',
                        OPTION_TEST_ASTRO='1' if args.astro else '0')
        print(json.dumps(dict(owner=owner, running='TestNativeOptionDefinitionTransactions')), flush=True)
        log = snapshot / 'native-test.log'
        with log.open('w') as output:
            result = subprocess.run([str(binary), '-test.run', '^TestNativeOptionDefinitionTransactions$',
                '-test.v', '-test.timeout', '120s'], cwd=backend / 'internal/catalog', env=test_env,
                stdout=output, stderr=subprocess.STDOUT, timeout=140)
        text = log.read_text()
        if result.returncode or '--- PASS: TestNativeOptionDefinitionTransactions' not in text or '--- SKIP' in text:
            raise RuntimeError('Shared-option native test failed: ' + str(log))
        frozen()
        browser_result = None
        if args.browser:
            redis = '/opt/homebrew/bin/redis-cli'
            if subprocess.check_output([redis, '-p', '16379', 'PING'], text=True).strip() != 'PONG':
                raise RuntimeError('Existing native Redis required')
            if subprocess.check_output([redis, '-p', '16379', '--scan', '--pattern', redis_prefix + '*'], text=True).strip():
                raise RuntimeError('Fresh owned Redis namespace required')
            ports = []
            for _ in range(3 if args.astro else 2):
                with socket.socket() as sock:
                    sock.bind(('127.0.0.1', 0))
                    ports.append(sock.getsockname()[1])
            if len(set(ports)) != len(ports):
                raise RuntimeError('Distinct temporary ports required')
            api_origin, admin_origin = ['http://127.0.0.1:' + str(port) for port in ports[:2]]
            api_env = dict(env, APP_BIND='127.0.0.1', APP_PORT=str(ports[0]), APP_ENV='development',
                GIN_MODE='release', MYSQL_DSN='root@unix(' + str(address) + ')/' + database + '?parseTime=true&loc=UTC',
                SESSION_SECURE='false', SESSION_REDIS_URL='redis://127.0.0.1:16379/0',
                SESSION_REDIS_PREFIX=redis_prefix, ADMIN_ORIGIN=admin_origin)
            if args.astro:
                api_env['STOREFRONT_STORE_ID'] = 'option-store'
            with (snapshot / 'api.log').open('w') as output:
                api = subprocess.Popen([str(api_binary)], cwd=backend, env=api_env,
                                       stdout=output, stderr=subprocess.STDOUT)
            opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
            deadline = time.monotonic() + 30
            while time.monotonic() < deadline:
                if api.poll() is not None:
                    raise RuntimeError('Owned API exited; inspect api.log')
                try:
                    with opener.open(api_origin + '/health', timeout=1) as reply:
                        if reply.status == 200:
                            break
                except OSError:
                    pass
                time.sleep(0.2)
            else:
                raise RuntimeError('Owned API readiness timed out')
            astro_origin = ''
            if args.astro:
                astro_origin = 'http://127.0.0.1:' + str(ports[2])
                # No commerce origin: this joint only exercises catalog definitions,
                # never initializes a customer/cart or a payment provider.
                astro_env = dict(env, HOST='127.0.0.1', PORT=str(ports[2]), GIN_API_ORIGIN=api_origin,
                                 ASTRO_TELEMETRY_DISABLED='1')
                with (snapshot / 'astro.log').open('w') as output:
                    astro = subprocess.Popen(['/opt/homebrew/opt/node/bin/node', 'scripts/start.mjs'],
                        cwd=storefront, env=astro_env, stdout=output, stderr=subprocess.STDOUT)
                deadline = time.monotonic() + 30
                while time.monotonic() < deadline:
                    if astro.poll() is not None:
                        raise RuntimeError('Owned Astro exited; inspect astro.log')
                    try:
                        with opener.open(astro_origin + '/dk-en/products/option-browser-product', timeout=3) as reply:
                            if reply.status == 200:
                                break
                    except OSError:
                        pass
                    time.sleep(0.2)
                else:
                    raise RuntimeError('Owned Astro readiness timed out')
            collection_facts_sql = """
SELECT * FROM catalog_collections WHERE id='joint-collection' ORDER BY id;
SELECT * FROM catalog_products WHERE id LIKE 'zz-collection-%' ORDER BY id;
SELECT * FROM catalog_variants WHERE product_id LIKE 'zz-collection-%' ORDER BY id;
SELECT * FROM catalog_product_channels WHERE product_id LIKE 'zz-collection-%' ORDER BY id;
SELECT * FROM catalog_options WHERE id IN ('joint-storage','joint-colour') ORDER BY id;
SELECT * FROM catalog_option_values WHERE option_id IN ('joint-storage','joint-colour') ORDER BY id;
SELECT * FROM catalog_product_options WHERE product_id LIKE 'zz-collection-%' ORDER BY id;
SELECT * FROM catalog_product_option_values WHERE id LIKE 'zz-collection-%' ORDER BY id;
SELECT * FROM catalog_variant_options WHERE variant_id LIKE 'zz-collection-%' ORDER BY variant_id,option_value_id;
SELECT * FROM pricing_price_sets WHERE id LIKE 'zz-collection-%' ORDER BY id;
SELECT * FROM pricing_prices WHERE id LIKE 'zz-collection-%' ORDER BY id;
SELECT * FROM catalog_variant_price_sets WHERE variant_id LIKE 'zz-collection-%' ORDER BY id;
"""
            if args.astro:
                collection_before = mysql("USE `" + database + "`;\n" + collection_facts_sql)
                (snapshot / 'collection-facts-before.tsv').write_text(collection_before)
            browser_env = dict(env, ADMIN_TEST_API_TARGET=api_origin, ADMIN_TEST_PORT=str(ports[1]),
                OPTION_BROWSER_OUTPUT=str(snapshot), ADMIN_EMAIL='option-native@example.test',
                ADMIN_PASSWORD='artificial-option-browser-password', OPTION_ASTRO_ORIGIN=astro_origin)
            browser_log = snapshot / 'browser.log'
            with browser_log.open('w') as output:
                browser = subprocess.Popen(['/opt/homebrew/opt/node/bin/node', 'tests/option-definitions.native.mjs'],
                    cwd=admin, env=browser_env, stdout=output, stderr=subprocess.STDOUT, start_new_session=True)
                browser.wait(timeout=360 if args.astro else 180)  # Includes actual collection and search SSR/hydration checks.
            if browser.returncode:
                raise RuntimeError('Owned option browser failed; inspect browser.log')
            lines = [line.removeprefix('OPTION_DEFINITIONS_BROWSER ') for line in browser_log.read_text().splitlines()
                     if line.startswith('OPTION_DEFINITIONS_BROWSER ')]
            if len(lines) != 1:
                raise RuntimeError('Native browser proof marker missing')
            browser_result = json.loads(lines[0])
            if args.astro:
                # Suspend only the Popen API owned by this fixture. Always resume
                # before propagating any failure; no shared service is signaled.
                api.send_signal(signal.SIGSTOP)
                try:
                    try:
                        with opener.open(astro_origin + '/dk-en/collections/native-phones', timeout=12):
                            raise RuntimeError('Unavailable owned API unexpectedly returned a collection')
                    except urllib.error.HTTPError as failure:
                        if failure.code != 503:
                            raise RuntimeError('Real API outage did not produce503')
                        (snapshot / 'collection-unavailable.html').write_bytes(failure.read())
                finally:
                    api.send_signal(signal.SIGCONT)
                with opener.open(astro_origin + '/dk-en/collections/native-phones', timeout=12) as reply:
                    if reply.status != 200 or b'data-testid="collection-page"' not in reply.read():
                        raise RuntimeError('Collection did not recover after owned API resumed')
                browser_result['collection']['actual_api_unavailable_503_and_recovery_200'] = True
                write(snapshot / 'collection-dependency-recovery.json', dict(owned_api_pid=api.pid,
                    unavailable_status=503, recovered_status=200, shared_services_signaled=False))
            if args.astro:
                collection_after = mysql("USE `" + database + "`;\n" + collection_facts_sql)
                (snapshot / 'collection-facts-after.tsv').write_text(collection_after)
                if collection_before != collection_after:
                    raise RuntimeError('Collection browsing changed durable catalog/pricing facts')
                collection = browser_result.get('collection', {})
                if not collection.get('real_gin_mysql_redis_astro') or collection.get('products') != 105 or collection.get('variants') != 263 or collection.get('filtered_products') != 53:
                    raise RuntimeError('Actual collection joint proof missing')
                counts = mysql("USE `" + database + "`;SELECT (SELECT COUNT(*) FROM catalog_products WHERE collection_id='joint-collection'),(SELECT COUNT(*) FROM catalog_variants WHERE product_id LIKE 'zz-collection-%'),(SELECT COUNT(*) FROM pricing_prices WHERE id LIKE 'zz-collection-%')")
                if counts != '105\t263\t263':
                    raise RuntimeError('Collection fixture SQL totals differ')
                collection['sql_facts_unchanged'] = True
                collection['sql_facts_sha256'] = sha(snapshot / 'collection-facts-after.tsv')
                collection['sql_product_variant_price_totals_verified'] = True
            operation, option_id = browser_result.get('operation_id', ''), browser_result.get('definition_id', '')
            if not re.fullmatch(r'[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}', operation) or not re.fullmatch(r'opt_[a-f0-9-]{36}', option_id):
                raise RuntimeError('Browser receipt identity invalid')
            receipt_count = mysql('SELECT COUNT(*) FROM `' + database + "`.catalog_option_definition_edits WHERE operation_id='" + operation + "' AND option_id='" + option_id + "' AND action='create'")
            if receipt_count != '1':
                raise RuntimeError('Exactly one browser create receipt required')
            browser_result['sql_create_receipt_count'] = 1
            metadata_operation = browser_result.get('metadata_operation_id', '')
            if not re.fullmatch(r'[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}', metadata_operation):
                raise RuntimeError('Metadata operation identity invalid')
            metadata_count = mysql('SELECT COUNT(*) FROM `' + database + "`.catalog_option_definition_edits WHERE operation_id='" + metadata_operation + "' AND option_id='" + option_id + "' AND action='edit'")
            if metadata_count != '1':
                raise RuntimeError('Exactly one metadata edit receipt required')
            browser_result['sql_metadata_receipt_count'] = 1
            deletion = browser_result.get('value_delete_operation_id', '')
            delete_option = browser_result.get('value_delete_definition_id', '')
            delete_value = browser_result.get('value_delete_value_id', '')
            if not re.fullmatch(r'[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}', deletion) or not re.fullmatch(r'opt_[a-f0-9-]{36}', delete_option) or not re.fullmatch(r'optval_[a-f0-9-]{36}', delete_value):
                raise RuntimeError('Value deletion identity invalid')
            deletion_facts = mysql('SELECT (SELECT COUNT(*) FROM `' + database + "`.catalog_option_definition_edits WHERE operation_id='" + deletion + "' AND option_id='" + delete_option + "' AND action='delete-value'),(SELECT COUNT(*) FROM `" + database + "`.catalog_option_values WHERE id='" + delete_value + "' AND option_id='" + delete_option + "' AND deleted_at IS NOT NULL)")
            if deletion_facts != '1\t1':
                raise RuntimeError('Value soft deletion and unique receipt not proven')
            browser_result['sql_value_delete_receipt_count'] = 1
            browser_result['sql_value_soft_deleted_preserving_id'] = True
            if args.astro:
                policy = browser_result.get('policy', {})
                operation = policy.get('operation_id', '')
                if not re.fullmatch(r'[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}', operation):
                    raise RuntimeError('Policy operation identity missing')
                if mysql('SELECT COUNT(*) FROM `' + database + "`.store_policy_edits WHERE operation_id='" + operation + "' AND store_id='option-store'") != '1':
                    raise RuntimeError('Unique browser policy receipt not proven')
                policy['sql_receipt_count'] = 1
                regional = browser_result.get('regional_price', {})
                region_operation = regional.get('operation_id', '')
                if not re.fullmatch(r'[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}', region_operation):
                    raise RuntimeError('Regional price operation identity missing')
                region_facts = mysql('SELECT COUNT(*) FROM `' + database + "`.catalog_price_edits e JOIN `" + database + "`.pricing_prices p ON p.id=e.price_id AND p.deleted_at IS NOT NULL JOIN `" + database + "`.pricing_price_rules r ON r.price_id=p.id AND r.deleted_at IS NOT NULL WHERE e.operation_id='" + region_operation + "' AND e.product_id='option-browser-product' AND p.currency_code='eur' AND p.amount=123.45 AND p.rules_count=1 AND r.attribute='region_id' AND r.operator='eq' AND r.value='option-region'")
                if region_facts != '1':
                    raise RuntimeError('Unique browser regional price receipt and rule not proven')
                regional['sql_price_receipt_rule_count'] = 1
                removal = regional.get('removal', {})
                removal_operation, removed_id = removal.get('operation_id', ''), removal.get('price_id', '')
                if not re.fullmatch(r'[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}', removal_operation) or not re.fullmatch(r'price_[a-f0-9-]{36}', removed_id):
                    raise RuntimeError('Price removal identity missing')
                removal_facts = mysql('SELECT COUNT(*) FROM `' + database + "`.catalog_price_edits e JOIN `" + database + "`.pricing_prices p ON p.id=e.price_id JOIN `" + database + "`.pricing_price_rules r ON r.price_id=p.id WHERE e.operation_id='" + removal_operation + "' AND e.price_id='" + removed_id + "' AND JSON_UNQUOTE(JSON_EXTRACT(e.request_json,'$.action'))='remove' AND p.deleted_at IS NOT NULL AND r.deleted_at=p.deleted_at AND JSON_UNQUOTE(JSON_EXTRACT(e.after_json,'$.id'))=p.id")
                if removal_facts != '1':
                    raise RuntimeError('Unique removal receipt and retained price/rule not proven')
                removal['sql_receipt_price_rule_count'] = 1
                tier = regional.get('quantity', {})
                tier_operation, tier_id = tier.get('operation_id', ''), tier.get('price_id', '')
                if not re.fullmatch(r'[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}', tier_operation) or not re.fullmatch(r'price_[a-f0-9-]{36}', tier_id):
                    raise RuntimeError('Quantity price identity missing')
                tier_facts = mysql('SELECT COUNT(*) FROM `' + database + "`.catalog_price_edits e JOIN `" + database + "`.pricing_prices p ON p.id=e.price_id JOIN `" + database + "`.pricing_price_rules r ON r.price_id=p.id WHERE e.operation_id='" + tier_operation + "' AND p.id='" + tier_id + "' AND p.deleted_at IS NULL AND r.deleted_at IS NULL AND p.currency_code='eur' AND p.amount=88.25 AND p.min_quantity IS NULL AND p.max_quantity IS NULL AND p.raw_min_quantity IS NULL AND p.raw_max_quantity IS NULL AND JSON_UNQUOTE(JSON_EXTRACT(e.request_json,'$.min_quantity'))='2' AND JSON_UNQUOTE(JSON_EXTRACT(e.request_json,'$.max_quantity'))='3' AND r.attribute='region_id' AND r.operator='eq' AND r.value='option-region'")
                if tier_facts != '1':
                    raise RuntimeError('Unique tier receipt and exact bounds/rule not proven')
                tier['sql_receipt_exact_bound_rule_count'] = 1
                quantity_edit = tier.get('edit', {})
                for key in ['operation_id', 'clear_operation_id']:
                    if not re.fullmatch(r'[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}', quantity_edit.get(key, '')):
                        raise RuntimeError('Quantity edit identity missing')
                edit_ops = "'" + quantity_edit['operation_id'] + "','" + quantity_edit['clear_operation_id'] + "'"
                edit_facts = mysql('SELECT COUNT(*) FROM `' + database + "`.catalog_price_edits WHERE operation_id IN (" + edit_ops + ") AND price_id='" + tier_id + "' AND JSON_UNQUOTE(JSON_EXTRACT(request_json,'$.action'))='quantity' AND JSON_UNQUOTE(JSON_EXTRACT(before_json,'$.price.id'))=price_id AND JSON_UNQUOTE(JSON_EXTRACT(after_json,'$.price.id'))=price_id AND JSON_EXTRACT(before_json,'$.raw_amount')=JSON_EXTRACT(after_json,'$.raw_amount')")
                if edit_facts != '2':
                    raise RuntimeError('Quantity edit receipts and preserved price/amount not proven')
                quantity_edit['sql_unique_receipts_count'] = 2
                price_list = regional.get('price_list', {})
                for key in ['operation_id', 'draft_operation_id']:
                    if not re.fullmatch(r'[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}', price_list.get(key, '')):
                        raise RuntimeError('Price-list command identity missing')
                list_ops = "'" + price_list['operation_id'] + "','" + price_list['draft_operation_id'] + "'"
                list_facts = mysql('SELECT COUNT(*) FROM `' + database + "`.catalog_price_list_edits WHERE operation_id IN (" + list_ops + ") AND price_list_id='list-browser-sale' AND JSON_EXTRACT(before_json,'$.rules')=JSON_EXTRACT(after_json,'$.rules')")
                if list_facts != '2':
                    raise RuntimeError('Price-list unique receipts and unchanged microseconds/rules not proven')
                list_state = mysql('SELECT COUNT(*) FROM `' + database + "`.pricing_price_lists l JOIN `" + database + "`.pricing_prices p ON p.price_list_id=l.id JOIN `" + database + "`.pricing_price_list_rules r ON r.price_list_id=l.id WHERE l.id='list-browser-sale' AND l.status='active' AND l.title='Browser list draft again' AND l.description='Browser edited description' AND l.type='override' AND l.starts_at IS NULL AND l.ends_at IS NULL AND l.metadata='{\"exact\":9007199254740993}' AND p.id='list-browser-price' AND p.amount=42.75 AND p.raw_amount='{\"value\":\"42.75\",\"precision\":20}' AND r.id='list-browser-rule' AND r.attribute='sales_channel_id' AND r.value='\"option-channel\"'")
                if list_state != '1':
                    raise RuntimeError('Price-list final state and exact unchanged price/metadata/rule not proven')
                schedule_ops = price_list.get('schedule_operation_ids', [])
                if len(schedule_ops) != 2 or any(not re.fullmatch(r'[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}', v) for v in schedule_ops):
                    raise RuntimeError('Price-list schedule command identities missing')
                all_list_ops = list_ops + "," + ','.join("'" + v + "'" for v in schedule_ops)
                if mysql('SELECT COUNT(*) FROM `' + database + "`.catalog_price_list_edits WHERE price_list_id='list-browser-sale' AND operation_id IN (" + all_list_ops + ")") != '4':
                    raise RuntimeError('Price-list schedule unique receipts missing')
                price_list['sql_unique_receipts_count'] = 4
                price_list['sql_exact_unchanged_price_metadata_rule'] = True
                list_removal = price_list.get('removal', {})
                for key in ['operation_id', 'reactivate_operation_id']:
                    if not re.fullmatch(r'[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}', list_removal.get(key, '')):
                        raise RuntimeError('Price-list removal command identity missing')
                remove_list_op = list_removal['operation_id']
                removed_list = mysql('SELECT COUNT(*) FROM `' + database + "`.catalog_price_list_edits e JOIN `" + database + "`.pricing_price_lists l ON l.id=e.price_list_id JOIN `" + database + "`.pricing_prices p ON p.price_list_id=l.id JOIN `" + database + "`.pricing_price_list_rules r ON r.price_list_id=l.id WHERE e.operation_id='" + remove_list_op + "' AND JSON_UNQUOTE(JSON_EXTRACT(e.request_json,'$.action'))='remove' AND l.id='list-browser-sale' AND l.deleted_at IS NOT NULL AND p.id='list-browser-price' AND p.deleted_at=l.deleted_at AND r.id='list-browser-rule' AND r.deleted_at=l.deleted_at AND p.amount=42.75 AND JSON_UNQUOTE(JSON_EXTRACT(p.raw_amount,'$.value'))='42.75'")
                if removed_list != '1':
                    raise RuntimeError('Price-list soft cascade and unique receipt not proven')

                removed_price_rule = mysql('SELECT COUNT(*) FROM `' + database + "`.pricing_price_rules r JOIN `" + database + "`.pricing_prices p ON p.id=r.price_id JOIN `" + database + "`.pricing_price_lists l ON l.id=p.price_list_id WHERE l.id='list-browser-sale' AND r.id='list-browser-price-rule' AND r.attribute='region_id' AND r.operator='eq' AND r.value='option-region' AND r.deleted_at=l.deleted_at AND p.deleted_at=l.deleted_at")
                if removed_price_rule != '1':
                    raise RuntimeError('Price-list cascade did not retain the exact price rule')
                list_removal['sql_unique_receipt_exact_soft_cascade'] = True
                membership = price_list.get('membership', {})
                for key in ['operation_id', 'tier_operation_id']:
                    if not re.fullmatch(r'[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}', membership.get(key, '')):
                        raise RuntimeError('Price-list membership command identity missing')
                for key in ['price_id', 'tier_price_id']:
                    if not re.fullmatch(r'price_[a-f0-9-]{36}', membership.get(key, '')):
                        raise RuntimeError('Price-list membership price identity missing')
                member_ops = "'" + membership['operation_id'] + "','" + membership['tier_operation_id'] + "'"
                member_facts = mysql('SELECT COUNT(*) FROM `' + database + "`.catalog_price_edits e JOIN `" + database + "`.pricing_prices p ON p.id=e.price_id JOIN `" + database + "`.pricing_price_rules r ON r.price_id=p.id WHERE e.operation_id IN (" + member_ops + ") AND p.price_list_id='list-browser-member' AND JSON_UNQUOTE(JSON_EXTRACT(e.request_json,'$.price_list_id'))=p.price_list_id AND p.price_set_id='list-browser-set' AND p.currency_code='eur' AND p.deleted_at IS NULL AND p.rules_count=1 AND r.attribute='region_id' AND r.operator='eq' AND r.value='option-region' AND r.deleted_at IS NULL AND ((p.id='" + membership['price_id'] + "' AND p.amount=37.25 AND JSON_UNQUOTE(JSON_EXTRACT(p.raw_amount,'$.value'))='37.250000000000000000' AND p.min_quantity IS NULL AND p.raw_min_quantity IS NULL AND p.max_quantity IS NULL AND p.raw_max_quantity IS NULL) OR (p.id='" + membership['tier_price_id'] + "' AND p.amount=32.50 AND JSON_UNQUOTE(JSON_EXTRACT(p.raw_amount,'$.value'))='32.500000000000000000' AND p.min_quantity=2 AND p.max_quantity=3 AND JSON_UNQUOTE(JSON_EXTRACT(p.raw_min_quantity,'$.value'))='2' AND JSON_UNQUOTE(JSON_EXTRACT(p.raw_max_quantity,'$.value'))='3'))")
                if member_facts != '2':
                    raise RuntimeError('Price-list membership exact rows/unique receipts not proven')
                if mysql('SELECT COUNT(*) FROM `' + database + "`.pricing_prices WHERE id='list-browser-base' AND price_list_id IS NULL AND amount=100 AND JSON_UNQUOTE(JSON_EXTRACT(raw_amount,'$.value'))='100' AND deleted_at IS NULL") != '1':
                    raise RuntimeError('Membership creation changed the original base price')
                membership['sql_two_unique_receipts_exact_rows_and_base_preserved'] = True
                creation = price_list.get('creation', {})
                create_op, create_id = creation.get('operation_id', ''), creation.get('list_id', '')
                if not re.fullmatch(r'[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}', create_op) or not re.fullmatch(r'plist_[a-f0-9-]{36}', create_id):
                    raise RuntimeError('Price-list creation identity missing')
                create_receipt = mysql('SELECT COUNT(*) FROM `' + database + "`.catalog_price_list_edits e JOIN `" + database + "`.pricing_price_lists l ON l.id=e.price_list_id WHERE e.operation_id='" + create_op + "' AND l.id='" + create_id + "' AND JSON_UNQUOTE(JSON_EXTRACT(e.request_json,'$.action'))='create' AND JSON_LENGTH(JSON_EXTRACT(e.request_json,'$.prices'))=3 AND JSON_LENGTH(JSON_EXTRACT(e.after_json,'$.prices'))=3 AND l.title='Browser created initial list' AND l.description='Two products with regional quantity prices' AND l.status='active' AND l.type='sale' AND l.rules_count=0 AND l.starts_at IS NULL AND l.ends_at IS NULL AND l.deleted_at IS NULL")
                if create_receipt != '1':
                    raise RuntimeError('Unique list creation receipt with initial prices not proven')
                rows = creation.get('initial_prices', [])
                if len(rows) != 3 or set(creation.get('selected_product_ids', [])) != {'list-browser-product', 'option-browser-product'}:
                    raise RuntimeError('Expected two selected products and three initial prices')
                for row in rows:
                    if not re.fullmatch(r'[a-zA-Z0-9_-]+', row.get('variant_id', '')) or not re.fullmatch(r'[0-9]{1,20}\.[0-9]{18}', row.get('amount', '')) or row.get('region_id') != 'option-region' or row.get('currency_code') != 'eur':
                        raise RuntimeError('Unexpected initial price facts')
                    min_bound = "p.min_quantity=" + row['min_quantity'] + " AND JSON_UNQUOTE(JSON_EXTRACT(p.raw_min_quantity,'$.value'))='" + row['min_quantity'] + "'" if row.get('min_quantity') else 'p.min_quantity IS NULL AND p.raw_min_quantity IS NULL'
                    max_bound = "p.max_quantity=" + row['max_quantity'] + " AND JSON_UNQUOTE(JSON_EXTRACT(p.raw_max_quantity,'$.value'))='" + row['max_quantity'] + "'" if row.get('max_quantity') else 'p.max_quantity IS NULL AND p.raw_max_quantity IS NULL'
                    facts = mysql('SELECT COUNT(*) FROM `' + database + "`.pricing_prices p JOIN `" + database + "`.catalog_variant_price_sets vp ON vp.price_set_id=p.price_set_id AND vp.deleted_at IS NULL JOIN `" + database + "`.pricing_price_rules r ON r.price_id=p.id AND r.deleted_at IS NULL WHERE p.price_list_id='" + create_id + "' AND vp.variant_id='" + row['variant_id'] + "' AND p.currency_code='eur' AND p.amount=" + row['amount'] + " AND JSON_UNQUOTE(JSON_EXTRACT(p.raw_amount,'$.value'))='" + row['amount'] + "' AND p.rules_count=1 AND r.attribute='region_id' AND r.operator='eq' AND r.value='option-region' AND p.deleted_at IS NULL AND " + min_bound + " AND " + max_bound)
                    if facts != '1':
                        raise RuntimeError('Initial SKU price exact amount/bounds/rule not proven')
                if mysql('SELECT COUNT(*) FROM `' + database + "`.pricing_prices p JOIN `" + database + "`.catalog_price_list_edits e ON e.price_list_id=p.price_list_id WHERE e.operation_id='" + create_op + "' AND JSON_SEARCH(JSON_EXTRACT(e.after_json,'$.prices'),'one',p.id,NULL,'$[*].id') IS NOT NULL AND p.deleted_at IS NULL") != '3':
                    raise RuntimeError('Duplicate initial list prices detected')
                creation['sql_unique_creation_receipt_three_exact_prices'] = True
                disable_op = creation.get('disable_existing_operation_id', '')
                if not re.fullmatch(r'[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}', disable_op):
                    raise RuntimeError('Existing specific list transition missing')
                if mysql('SELECT COUNT(*) FROM `' + database + "`.catalog_price_list_edits e JOIN `" + database + "`.pricing_price_lists l ON l.id=e.price_list_id WHERE e.operation_id='" + disable_op + "' AND l.id='list-browser-member' AND l.status='draft' AND JSON_UNQUOTE(JSON_EXTRACT(e.request_json,'$.status'))='draft'") != '1':
                    raise RuntimeError('Specific list actual draft transition not proven')
                creation['sql_specific_list_draft_receipt'] = True
                addition = price_list.get('addition', {})
                add_op = addition.get('operation_id', '')
                if not re.fullmatch(r'[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}', add_op) or addition.get('list_id') != create_id:
                    raise RuntimeError('Existing-list batch addition identity missing')
                add_receipt = mysql('SELECT COUNT(*) FROM `' + database + "`.catalog_price_list_edits e WHERE e.operation_id='" + add_op + "' AND e.price_list_id='" + create_id + "' AND JSON_UNQUOTE(JSON_EXTRACT(e.request_json,'$.action'))='add-prices' AND JSON_LENGTH(JSON_EXTRACT(e.request_json,'$.prices'))=2 AND JSON_LENGTH(JSON_EXTRACT(e.after_json,'$.prices'))=2 AND JSON_EXTRACT(e.before_json,'$.rules')=JSON_EXTRACT(e.after_json,'$.rules') AND JSON_EXTRACT(e.before_json,'$.price_count')=3 AND JSON_EXTRACT(e.after_json,'$.price_count')=5")
                if add_receipt != '1':
                    raise RuntimeError('Unique batch addition receipt not proven')
                for row in addition.get('initial_prices', []):
                    if not re.fullmatch(r'[a-zA-Z0-9_-]+', row.get('variant_id', '')) or not re.fullmatch(r'[0-9]{1,20}\.[0-9]{18}', row.get('amount', '')) or row.get('region_id') != 'option-region':
                        raise RuntimeError('Unexpected batch price facts')
                    for key in ['min_quantity','max_quantity']:
                        if row.get(key) and not re.fullmatch(r'[1-9][0-9]{0,19}',row[key]):
                            raise RuntimeError('Unexpected batch quantity')
                    bounds = ''.join(" AND p." + key + "=" + row[key] + " AND JSON_UNQUOTE(JSON_EXTRACT(p.raw_" + key + ",'$.value'))='" + row[key] + "'" if row.get(key) else " AND p." + key + " IS NULL AND p.raw_" + key + " IS NULL" for key in ['min_quantity','max_quantity'])
                    facts = mysql('SELECT COUNT(*) FROM `' + database + "`.pricing_prices p JOIN `" + database + "`.catalog_variant_price_sets vp ON vp.price_set_id=p.price_set_id AND vp.deleted_at IS NULL JOIN `" + database + "`.pricing_price_rules r ON r.price_id=p.id AND r.deleted_at IS NULL WHERE p.price_list_id='" + create_id + "' AND vp.variant_id='" + row['variant_id'] + "' AND p.currency_code='eur' AND p.amount=" + row['amount'] + " AND JSON_UNQUOTE(JSON_EXTRACT(p.raw_amount,'$.value'))='" + row['amount'] + "' AND p.rules_count=1 AND r.attribute='region_id' AND r.operator='eq' AND r.value='option-region' AND p.deleted_at IS NULL" + bounds)
                    if facts != '1':
                        raise RuntimeError('Batch added price exact row not proven')
                if mysql('SELECT COUNT(*) FROM `' + database + "`.pricing_prices WHERE price_list_id='" + create_id + "' AND deleted_at IS NULL") != '5':
                    raise RuntimeError('Existing and added price count changed')
                addition['sql_one_batch_receipt_two_exact_prices_old_rows_preserved'] = True
                batch = price_list.get('batch', {})
                batch_op, batch_id = batch.get('operation_id', ''), batch.get('list_id', '')
                if not re.fullmatch(r'[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}', batch_op) or not re.fullmatch(r'plist_[a-f0-9-]{36}',batch_id):
                    raise RuntimeError('Mixed batch identity missing')
                command = batch.get('command', {})
                if len(command.get('create',[]))!=2 or len(command.get('update',[]))!=2 or len(command.get('delete',[]))!=1:
                    raise RuntimeError('Expected one mixed batch of five changes')
                batch_receipt = mysql('SELECT COUNT(*) FROM `' + database + "`.catalog_price_list_edits e JOIN `" + database + "`.pricing_price_lists l ON l.id=e.price_list_id WHERE e.operation_id='" + batch_op + "' AND e.price_list_id='" + batch_id + "' AND JSON_UNQUOTE(JSON_EXTRACT(e.request_json,'$.action'))='batch-prices' AND JSON_LENGTH(JSON_EXTRACT(e.request_json,'$.create'))=2 AND JSON_LENGTH(JSON_EXTRACT(e.request_json,'$.update'))=2 AND JSON_LENGTH(JSON_EXTRACT(e.request_json,'$.delete'))=1 AND JSON_LENGTH(JSON_EXTRACT(e.after_json,'$.created'))=2 AND JSON_LENGTH(JSON_EXTRACT(e.after_json,'$.updated'))=2 AND JSON_LENGTH(JSON_EXTRACT(e.after_json,'$.deleted'))=1 AND JSON_EXTRACT(e.before_json,'$.rules')=JSON_EXTRACT(e.after_json,'$.rules') AND JSON_EXTRACT(e.before_json,'$.price_count')=3 AND JSON_EXTRACT(e.after_json,'$.price_count')=4 AND l.title='Browser mixed batch list' AND l.description='Atomic existing and replacement prices' AND l.status='active' AND l.type='sale' AND l.metadata IS NULL AND l.rules_count=0 AND l.starts_at IS NULL AND l.ends_at IS NULL AND l.deleted_at IS NULL")
                if batch_receipt != '1':
                    raise RuntimeError('Mixed batch unique receipt and unchanged list not proven')
                for action in ['create','update']:
                    for row in command[action]:
                        if not re.fullmatch(r'[a-zA-Z0-9_-]+',row.get('variant_id','')) or not re.fullmatch(r'[0-9]{1,20}\.[0-9]{18}',row.get('amount','')):
                            raise RuntimeError('Unexpected mixed batch amount/variant')
                        for key in ['min_quantity','max_quantity']:
                            if row.get(key) and not re.fullmatch(r'[1-9][0-9]{0,19}',row[key]):
                                raise RuntimeError('Unexpected mixed batch bound')
                        bounds = ''.join(" AND p." + key + "=" + row[key] + " AND JSON_UNQUOTE(JSON_EXTRACT(p.raw_" + key + ",'$.value'))='" + row[key] + "'" if row.get(key) else " AND p." + key + " IS NULL AND p.raw_" + key + " IS NULL" for key in ['min_quantity','max_quantity'])
                        identity = ''
                        if action=='update':
                            if not re.fullmatch(r'price_[a-f0-9-]{36}',row.get('id','')):
                                raise RuntimeError('Unexpected updated price identity')
                            identity=" AND p.id='"+row['id']+"'"
                        else:
                            identity=" AND JSON_SEARCH(JSON_EXTRACT(e.after_json,'$.created'),'one',p.id,NULL,'$[*].id') IS NOT NULL"
                        exact=mysql('SELECT COUNT(*) FROM `' + database + "`.pricing_prices p JOIN `" + database + "`.catalog_price_list_edits e ON e.price_list_id=p.price_list_id JOIN `" + database + "`.catalog_variant_price_sets vp ON vp.price_set_id=p.price_set_id AND vp.deleted_at IS NULL JOIN `" + database + "`.pricing_price_rules r ON r.price_id=p.id AND r.deleted_at IS NULL WHERE e.operation_id='" + batch_op + "' AND p.price_list_id='" + batch_id + "' AND vp.variant_id='" + row['variant_id'] + "' AND p.currency_code='eur' AND p.amount=" + row['amount'] + " AND JSON_UNQUOTE(JSON_EXTRACT(p.raw_amount,'$.value'))='" + row['amount'] + "' AND p.rules_count=1 AND r.attribute='region_id' AND r.operator='eq' AND r.value='option-region' AND p.deleted_at IS NULL"+identity+bounds)
                        if exact!='1':
                            raise RuntimeError('Mixed batch exact price/identity/rule not proven')
                deleted_id=batch.get('original_base_id','')
                if not re.fullmatch(r'price_[a-f0-9-]{36}',deleted_id) or command['delete'][0].get('id')!=deleted_id:
                    raise RuntimeError('Mixed batch deleted identity missing')
                retained=mysql('SELECT COUNT(*) FROM `' + database + "`.pricing_prices p JOIN `" + database + "`.pricing_price_rules r ON r.price_id=p.id WHERE p.id='"+deleted_id+"' AND p.price_list_id='"+batch_id+"' AND p.amount=7 AND JSON_UNQUOTE(JSON_EXTRACT(p.raw_amount,'$.value'))='7.000000000000000000' AND p.deleted_at IS NOT NULL AND r.deleted_at=p.deleted_at AND r.attribute='region_id' AND r.operator='eq' AND r.value='option-region'")
                if retained!='1' or mysql('SELECT COUNT(*) FROM `' + database + "`.pricing_prices WHERE price_list_id='" + batch_id + "' AND deleted_at IS NULL")!='4':
                    raise RuntimeError('Mixed batch retained delete and four active prices not proven')
                batch['sql_unique_receipt_two_creates_two_updates_one_retained_delete']=True











            frozen()
            mysql('DROP DATABASE `' + database + '`')
        if mysql("SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='" + database + "'") != '0':
            raise RuntimeError('Owned test database cleanup not proven')
        write(snapshot / 'result.json', dict(exit_code=0, migrations=99, native_transactions_passed=True,
              source_files=len(files), frozen_source_verified=True, binary_sha256=sha(binary),
              test_log_sha256=sha(log), owned_database_absent=True, redis_used=args.browser,
              main_mysql_unchanged=True, real_provider_called=False, native_browser_verified=args.browser,
              browser=browser_result, browser_log_sha256=sha(snapshot / 'browser.log') if args.browser else None,
              admin_source_files=len(admin_files),
              astro_joint_verified=args.astro, storefront_source_files=len(storefront_files),
              original23_regression=False, goal_complete=False))
        print(json.dumps(dict(owner=owner, native_transactions_passed=True)), flush=True)
    finally:
        if browser is not None and browser.poll() is None:
            os.killpg(browser.pid, signal.SIGTERM)
            try:
                browser.wait(timeout=10)
            except subprocess.TimeoutExpired:
                os.killpg(browser.pid, signal.SIGKILL)
                browser.wait(timeout=10)
        if astro is not None and astro.poll() is None:
            astro.terminate()
            try:
                astro.wait(timeout=10)
            except subprocess.TimeoutExpired:
                astro.kill()
                astro.wait(timeout=10)
        if api is not None and api.poll() is None:
            api.terminate()
            try:
                api.wait(timeout=15)
            except subprocess.TimeoutExpired:
                api.kill()
                api.wait(timeout=10)
        redis_cleanup_error = None
        if args.browser:
            try:
                redis = '/opt/homebrew/bin/redis-cli'
                keys = subprocess.check_output([redis, '-p', '16379', '--scan', '--pattern', redis_prefix + '*'], text=True, timeout=10).splitlines()
                if any(not key.startswith(redis_prefix) for key in keys):
                    raise RuntimeError('Unowned Redis key rejected')
                for i in range(0, len(keys), 100):
                    subprocess.run([redis, '-p', '16379', 'DEL', *keys[i:i + 100]], check=True, capture_output=True, timeout=10)
                redis_cleanup_count = len(keys)
                if subprocess.check_output([redis, '-p', '16379', '--scan', '--pattern', redis_prefix + '*'], text=True, timeout=10).strip():
                    raise RuntimeError('Owned Redis namespace not empty')
            except (OSError, subprocess.SubprocessError, RuntimeError) as error:
                redis_cleanup_error = type(error).__name__
        if server is not None and server.poll() is None:
            try:
                subprocess.run([str(MYSQL.with_name('mysqladmin')), '--no-defaults', '--protocol=SOCKET',
                                '--socket=' + str(address), '--user=root', 'shutdown'], env=env,
                               capture_output=True, timeout=15)
            except subprocess.TimeoutExpired:
                pass
            try:
                server.wait(timeout=30)
            except subprocess.TimeoutExpired:
                server.terminate()
                server.wait(timeout=15)
        terminal = server is None or server.poll() is not None
        if terminal:
            if data.exists():
                shutil.rmtree(data)
            if sockets.exists() and not list(sockets.iterdir()):
                sockets.rmdir()
        write(snapshot / 'cleanup.json', dict(server_terminal=terminal, data_removed=not data.exists(),
              socket_removed=not address.exists(), main_mysql_unchanged=True, redis_used=args.browser,
              owned_api_terminal=api is None or api.poll() is not None, redis_keys_deleted=redis_cleanup_count,
              owned_astro_terminal=astro is None or astro.poll() is not None,
              owned_browser_terminal=browser is None or browser.poll() is not None,
              redis_cleanup_error=redis_cleanup_error))
        if redis_cleanup_error:
            raise RuntimeError('Owned Redis cleanup failed; MySQL cleanup was still performed')


if __name__ == '__main__':
    main()
