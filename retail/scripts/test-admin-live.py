"""Real browser -> Vite -> production Gin binary, with owned artificial SQL/Redis."""
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import shutil
import subprocess
import sys
import time
import urllib.request
import uuid

ROOT = Path(__file__).resolve().parents[2]
LABEL = 'eurorefurb.browser-rehearsal'


def docker(*args, timeout=90):
    return subprocess.run(['docker', *args], capture_output=True, timeout=timeout)


def main():
    if shutil.disk_usage(ROOT).free < 2 * (1 << 30):
        raise RuntimeError('At least 2 GiB free workspace disk required to preserve development services')
    for image in ['mysql:8.4', 'redis:7-alpine']:
        if docker('image', 'inspect', image).returncode:
            raise RuntimeError('Existing fixture images required; no pull performed')
    owner = uuid.uuid4().hex
    binaries = [ROOT / 'logs/gin-migration' / ('browser-' + owner + suffix) for suffix in ['-api', '-prep.test']]
    for binary in binaries:
        if binary.exists() or subprocess.run(['git', 'check-ignore', '--quiet', str(binary)], cwd=ROOT, capture_output=True).returncode:
            raise RuntimeError('New ignored runner paths required')
    env = os.environ.copy()
    env.update(GOOS='linux', GOARCH='amd64', CGO_ENABLED='0')
    containers, network, owned_binaries = [], None, []
    password = secrets.token_urlsafe(32)
    ddl = ROOT / 'docs/migration/mysql-relational-review.sql'
    review = json.loads((ROOT / 'docs/migration/mysql-relational-review.json').read_text(encoding='utf-8'))
    if hashlib.sha256(ddl.read_bytes().replace(b'\r\n', b'\n')).hexdigest() != review['ddl_sha256']:
        raise RuntimeError('Reviewed schema digest differs')
    try:
        for binary, command in zip(binaries, [['go', 'build', '-o', str(binaries[0]), './cmd/api'],
                                              ['go', 'test', '-c', '-o', str(binaries[1]), './internal/identity']]):
            owned_binaries.append(binary)
            if subprocess.run(command, cwd=ROOT / 'retail/backend', env=env, capture_output=True, timeout=120).returncode:
                raise RuntimeError('Browser API runner compilation failed')
        # An internal Docker network suppresses published ports on this engine.
        # Use a dedicated bridge; publish only the API on a random host loopback
        # port. SQL and Redis remain unpublished. This does not restrict egress.
        created = docker('network', 'create', '--label', LABEL + '=' + owner, 'eurorefurb-browser-' + owner[:12])
        if created.returncode:
            raise RuntimeError('Owned bridge network creation failed')
        network = created.stdout.decode().strip()
        created = docker('run', '-d', '--name', 'eurorefurb-browser-mysql-' + owner[:12],
                         '--label', LABEL + '=' + owner, '--network', network, '--cpus', '1',
                         '--memory', '768m', '--memory-swap', '768m', '--pids-limit', '192',
                         '--tmpfs', '/var/lib/mysql:rw,size=512m', '--tmpfs', '/tmp:rw,size=32m,mode=1777',
                         '-p', '127.0.0.1::9100', '-e', 'MYSQL_ROOT_PASSWORD=' + password,
                         '-e', 'MYSQL_DATABASE=browser_identity', 'mysql:8.4', '--skip-log-bin',
                         '--innodb-buffer-pool-size=64M', '--performance-schema=OFF', '--mysqlx=OFF', '--max-connections=16')
        if created.returncode:
            raise RuntimeError('Owned browser MySQL creation failed')
        mysql_id = created.stdout.decode().strip()
        containers.append(mysql_id)
        created = docker('run', '-d', '--name', 'eurorefurb-browser-redis-' + owner[:12],
                         '--label', LABEL + '=' + owner, '--network', 'container:' + mysql_id,
                         '--cpus', '1', '--memory', '128m', '--memory-swap', '128m', '--pids-limit', '64',
                         '--tmpfs', '/data:rw,size=16m', 'redis:7-alpine', 'redis-server', '--bind', '127.0.0.1',
                         '--save', '', '--appendonly', 'no', '--maxmemory', '16mb', '--maxmemory-policy', 'noeviction')
        if created.returncode:
            raise RuntimeError('Owned browser Redis creation failed')
        redis_id = created.stdout.decode().strip()
        containers.append(redis_id)
        for _ in range(100):
            if not docker('exec', '-e', 'MYSQL_PWD=' + password, mysql_id, 'mysql', '-uroot', '--protocol=socket', '-e', 'SELECT 1', timeout=10).returncode:
                break
            time.sleep(.5)
        else:
            raise RuntimeError('Owned browser MySQL not ready')
        if docker('exec', redis_id, 'redis-cli', 'PING').stdout.strip() != b'PONG':
            raise RuntimeError('Owned browser Redis not ready')
        for binary, target in zip(binaries, ['/opt/api', '/opt/prep.test']):
            with binary.open('rb') as source:
                copied = subprocess.run(['docker', 'exec', '-i', mysql_id, '/bin/sh', '-c', 'umask 077; cat > ' + target],
                                        stdin=source, capture_output=True, timeout=60)
            if copied.returncode or docker('exec', mysql_id, 'chmod', '700', target).returncode:
                raise RuntimeError('Browser runner transfer failed')
        prepared = docker('exec', '-e', 'MYSQL_PWD=' + password, '-e', 'BROWSER_IDENTITY_FIXTURE=1', mysql_id,
                          '/opt/prep.test', '-test.v', '-test.run', '^TestPrepareOwnedBrowserAPI$', '-test.timeout', '40s')
        if prepared.returncode or b'OWNED_BROWSER_FIXTURE_READY' not in prepared.stdout:
            raise RuntimeError('Owned browser schema/account preparation failed')
        prepared = docker('exec','-e','MYSQL_PWD=' + password,'-e','BROWSER_IDENTITY_FIXTURE=1',mysql_id,
                          '/opt/prep.test','-test.v','-test.run','^TestPrepareReviewedCatalogFixture$','-test.timeout','40s')
        if prepared.returncode or b'OWNED_REVIEWED_CATALOG_READY' not in prepared.stdout:
            raise RuntimeError('Reviewed artificial catalog preparation failed')
        app_env = {'STOREFRONT_STORE_ID':'fixture-store','GIN_MODE': 'release', 'APP_BIND': '0.0.0.0', 'APP_PORT': '9100', 'APP_ENV': 'development',
                   'MYSQL_DSN': 'root:' + password + '@unix(/var/run/mysqld/mysqld.sock)/browser_identity?parseTime=true&loc=UTC',
                   'SESSION_REDIS_URL': 'redis://127.0.0.1:6379/0', 'SESSION_REDIS_PREFIX': 'eurorefurb:browser_fixture:v1:',
                   'ADMIN_ORIGIN': 'http://127.0.0.1:19200', 'STOREFRONT_ORIGIN': 'http://127.0.0.1:19200', 'SESSION_SECURE': 'false'}
        args = ['exec', '-d', '-w', '/opt']
        for key, value in app_env.items():
            args.extend(['-e', key + '=' + value])
        if docker(*args, mysql_id, '/opt/api').returncode:
            raise RuntimeError('Owned Gin listener launch failed')
        published = docker('port', mysql_id, '9100/tcp')
        match = re.fullmatch(r'127\.0\.0\.1:(\d+)\s*', published.stdout.decode())
        if published.returncode or not match or not 1 <= int(match[1]) <= 65535:
            raise RuntimeError('Expected one loopback-only ephemeral API port')
        target = 'http://127.0.0.1:' + match[1]
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        for _ in range(40):
            try:
                with opener.open(target + '/health', timeout=2) as response:
                    if response.status == 200:
                        break
            except OSError:
                pass
            time.sleep(.25)
        else:
            raise RuntimeError('Owned Gin listener not ready')
        browser_env = os.environ.copy()
        browser_env['ADMIN_TEST_API_TARGET'] = target
        npm = 'npm.cmd' if os.name == 'nt' else 'npm'
        tested = subprocess.run([npm, 'exec', 'playwright', 'test', '--', '--config', 'playwright.live.config.ts', '--reporter=json'],
                                cwd=ROOT / 'retail/admin', env=browser_env, capture_output=True, timeout=180)
        # Private browser cookies or server state never enter a committed report.
        try:
            result = json.loads(tested.stdout)
        except (ValueError,TypeError):
            (ROOT/'logs/gin-migration/admin-live-runner.log').write_bytes(tested.stdout+tested.stderr)
            raise RuntimeError('Live browser runner returned no JSON report; inspect ignored output') from None
        if tested.returncode or result['stats']['expected'] != 2 or any(result['stats'][k] for k in ['unexpected', 'skipped', 'flaky']):
            (ROOT / 'logs/gin-migration/admin-live-result.json').write_bytes(tested.stdout)
            raise RuntimeError('Live browser checks failed; inspect ignored local result')
        # Read back actual durable state, without exporting IDs or credentials.
        durable = docker('exec', '-e', 'MYSQL_PWD=' + password, mysql_id, 'mysql', '-uroot',
                         '--protocol=socket', '--database=browser_identity', '--batch', '--skip-column-names', '-e',
                         "SELECT (SELECT COUNT(*) FROM carts),(SELECT COUNT(*) FROM cart_ownership WHERE active=TRUE AND version=1),"
                         "(SELECT COUNT(*) FROM cart_creation_operations),(SELECT COUNT(*) FROM cart_creation_scopes),"
                         "(SELECT COUNT(*) FROM carts WHERE customer_id IS NOT NULL),(SELECT COUNT(*) FROM cart_line_items)")
        if durable.returncode or durable.stdout.decode().strip() != '1\t1\t1\t1\t0\t0':
            raise RuntimeError('Browser cart durable state differs; no successful report written')
        astro_env=os.environ.copy()
        astro_env['STOREFRONT_TEST_GIN']=target
        astro=subprocess.run([sys.executable,str(ROOT/'retail/scripts/test-storefront-runtime.py')],cwd=ROOT,env=astro_env,capture_output=True,timeout=1140)
        if astro.returncode:
            (ROOT/'logs/gin-migration/astro-owner-result.log').write_bytes(astro.stdout+astro.stderr)
            raise RuntimeError('Owned Astro-to-Gin checks failed; inspect ignored output')
        # Prepare the purchasable fixture only after existing catalog/Astro
        # assertions, so their incomplete-variant checks keep their coverage.
        prepared = docker('exec','-e','MYSQL_PWD=' + password,'-e','BROWSER_IDENTITY_FIXTURE=1',mysql_id,
                          '/opt/prep.test','-test.v','-test.run','^TestPrepareCartItemBrowserFixture$','-test.timeout','40s')
        if prepared.returncode or b'OWNED_CART_ITEMS_BROWSER_READY' not in prepared.stdout:
            (ROOT/'logs/gin-migration/cart-items-browser-prep.log').write_bytes(prepared.stdout+prepared.stderr)
            raise RuntimeError('Owned cart item browser preparation failed')
        item_browser = subprocess.run([npm,'exec','playwright','test','--','--config','playwright.cart-items.config.ts','--reporter=json'],
                                      cwd=ROOT/'retail/admin',env=browser_env,capture_output=True,timeout=120)
        item_result = json.loads(item_browser.stdout)
        if item_browser.returncode or item_result['stats']['expected'] != 1 or any(item_result['stats'][k] for k in ['unexpected','skipped','flaky']):
            (ROOT/'logs/gin-migration/cart-items-live-result.json').write_bytes(item_browser.stdout)
            raise RuntimeError('Live cart item browser checks failed; inspect ignored result')
        button_env = os.environ.copy()
        button_env['STOREFRONT_TEST_GIN'] = target
        button_node = button_env.get('STOREFRONT_TEST_NODE') or shutil.which('node')
        button_test = subprocess.run([button_node,'tests/commerce.browser.mjs'],cwd=ROOT/'retail/storefront',env=button_env,capture_output=True,timeout=150)
        button_markers = [line.split('CART_COMMERCE_BUTTON_BROWSER ',1)[1] for line in button_test.stdout.decode('utf-8').splitlines() if line.startswith('CART_COMMERCE_BUTTON_BROWSER ')]
        if button_test.returncode or len(button_markers)!=1:
            (ROOT/'logs/gin-migration/cart-commerce-button-browser.log').write_bytes(button_test.stdout+button_test.stderr)
            raise RuntimeError('Actual Astro cart button browser failed; inspect ignored output')
        item_durable = docker('exec','-e','MYSQL_PWD=' + password,mysql_id,'mysql','-uroot','--protocol=socket',
                              '--database=browser_identity','--batch','--skip-column-names','-e',
                              "SELECT (SELECT COUNT(*) FROM carts),(SELECT COUNT(*) FROM cart_ownership WHERE active=TRUE AND version=6),"
                              "(SELECT COUNT(*) FROM cart_line_items WHERE variant_id='browser-add-v' AND quantity=1 AND deleted_at IS NOT NULL),"
                              "(SELECT COUNT(*) FROM cart_item_operations),"
                              "(SELECT JSON_UNQUOTE(JSON_EXTRACT(raw_unit_price,'$.value')) FROM cart_line_items WHERE variant_id='browser-add-v' AND quantity=1 AND deleted_at IS NOT NULL),"
                              "(SELECT reserved_quantity=0 FROM inventory_levels WHERE id='browser-add-level'),"
                              "(SELECT COUNT(*) FROM inventory_reservations WHERE inventory_item_id='browser-add-item'),"
                              "(SELECT COUNT(*) FROM cart_line_items WHERE variant_id='browser-add-v' AND quantity=4 AND deleted_at IS NOT NULL)")
        if item_durable.returncode or item_durable.stdout.decode().strip() != '3\t2\t1\t10\t100.000000000000000000001\t1\t0\t1':
            raise RuntimeError('Browser item durable state differs; no successful report written')
        files = ['retail/scripts/test-admin-live.py', 'retail/backend/cmd/api/main.go',
                 'retail/backend/internal/identity/browser_fixture_test.go', 'retail/backend/internal/identity/admin.go',
                 'retail/backend/internal/platform/database.go', 'retail/backend/internal/platform/request.go',
                 'retail/backend/internal/server/router.go', 'retail/backend/internal/sessionstore/store.go',
                 'retail/backend/internal/sessionstore/config.go']
        files += [str(p.relative_to(ROOT)).replace('\\', '/') for folder in ('catalog','pricing') for p in (ROOT / 'retail/backend/internal'/folder).glob('*.go')]
        files += [str(p.relative_to(ROOT)).replace('\\', '/') for p in (ROOT / 'retail/backend/migrations').glob('*.sql')]
        files += ['docs/migration/mysql-relational-review.sql']
        files += [str(p.relative_to(ROOT)).replace('\\','/') for p in (ROOT/'retail/backend/internal/inventory').glob('*') if p.is_file()]
        files += ['retail/backend/internal/schema/ready.go','retail/backend/internal/schema/run.go',
                  'retail/backend/internal/schema/plan.go']
        files += [str(p.relative_to(ROOT)).replace('\\', '/') for p in (ROOT / 'retail/admin').rglob('*')
                  if p.is_file() and not any(part in ('node_modules', 'dist') for part in p.relative_to(ROOT / 'retail/admin').parts)]
        files += ['retail/backend/internal/orders/schema.go','retail/backend/internal/orders/schema_plan.json','retail/scripts/generate-order-schema.py']
        files += ['retail/backend/internal/orders/admin.go','retail/backend/internal/identity/order_read_test.go']
        files += ['retail/backend/internal/orders/amounts.go','retail/backend/internal/orders/amount_json.go','retail/backend/internal/identity/order_amounts_test.go']
        files += [str(p.relative_to(ROOT)).replace('\\','/') for p in (ROOT/'retail/backend/internal/totals').glob('*.go')]
        files += ['retail/backend/internal/payments/schema.go','retail/backend/internal/payments/schema_plan.json']
        files += ['retail/backend/internal/orders/payments.go','retail/backend/internal/identity/order_payments_test.go']
        files += ['retail/backend/migrations/0021_fulfillment_relational.sql','retail/backend/migrations/0022_fulfillment_import_evidence.sql','retail/scripts/generate-fulfillment-schema.py','retail/scripts/generate-fulfillment-import-plan.py','retail/backend/internal/identity/fulfillment_history_test.go']
        files += [str(p.relative_to(ROOT)).replace('\\','/') for p in (ROOT/'retail/backend/internal/fulfillment').glob('*') if p.is_file()]
        files += ['retail/backend/internal/orders/fulfillments.go','retail/backend/internal/identity/order_fulfillments_test.go']
        files += [str(p.relative_to(ROOT)).replace('\\','/') for p in (ROOT/'retail/backend/internal/authorization').glob('*.go')]
        files += [str(p.relative_to(ROOT)).replace('\\','/') for folder in ('cart','sessionstore','tax') for p in (ROOT/'retail/backend/internal'/folder).glob('*') if p.is_file()]
        files += ['retail/storefront/src/lib/cart-client.ts','retail/storefront/src/lib/cart-commerce.ts','retail/storefront/src/lib/cart-quantities.ts',
                  'retail/storefront/src/lib/cart-item-commands.ts','retail/storefront/src/components/cart/gin-cart.tsx',
                  'retail/storefront/tests/cart-item-commands.test.ts',
                  'retail/storefront/src/lib/cart-variants.ts','retail/storefront/tests/cart-variants.test.ts',
                  'retail/storefront/src/lib/products.ts','retail/storefront/src/lib/gin-public.ts',
                  'retail/storefront/src/lib/product-display.ts','retail/storefront/src/lib/money.ts',
                  'retail/storefront/src/lib/locales.ts','retail/storefront/src/lib/footer-data.ts','retail/storefront/tests/runtime.integration.mjs']
        files += [str(p.relative_to(ROOT)).replace('\\','/') for p in (ROOT/'retail/storefront/src/components/products').rglob('*') if p.is_file()]
        files += ['retail/storefront/tests/commerce.browser.mjs','retail/storefront/tests/commerce-fixture/src/pages/[...path].astro']
        report = {'format_version': 1, 'scope': 'Real browser through Vite to actual cmd/api, owned MySQL and Redis; artificial account only',
                  'browser_tests_passed': 3, 'migrations_applied': 28, 'mocked_api_responses': False,
                  'cart_commerce_button_browser': json.loads(button_markers[0]),
                  'cart_items_browser': {'actual_storefront_client': True, 'real_gin_mysql_redis': True,
                                         'concurrent_same_key_requests': 4, 'same_sku_merged': True,
                                         'quantity_tier_price': True, 'exact_raw_amount': True,
                                         'reload_readback': True, 'current_possession_lookup': True, 'immutable_old_receipt': True,
                                         'version_and_stock_rejection': True, 'cookie_ttl_unchanged': True,
                                         'line_items': 0, 'saved_deleted_lines': 1, 'operation_receipts': 5,
                                         'absolute_quantity_and_removal': True,
                                         'private_saved_presentation': True,
                                         'coordinator_lost_committed_response_reload': True,
                                         'coordinator_noncredential_intent_cleared': True, 'inventory_reserved': False,
                                         'production_page_mounted': False, 'full_cart_accepted': False},
                  'cart_browser': {'actual_storefront_client': True, 'real_gin_mysql_redis': True,
                                   'real_browser_cookie_origin_csrf': True, 'concurrent_replays': 4,
                                   'changed_request_conflict': True, 'scope_reuse_no_ttl_extension': True,
                                   'reload_with_http_only_possession': True, 'no_new_browser_storage': True,
                                   'stranger_and_admin_realm_denied': True, 'lost_possession_same_operation_recovered': True,
                                   'durable_carts': 1, 'durable_ownerships': 1, 'durable_receipts': 1,
                                   'customer_accounts_bound': 0, 'line_items': 0,
                                   'persisted_amounts_client': True, 'empty_exact_totals': True,
                                   'production_page_mounted': False, 'tls_secure_cookie_verified': False},
                  'reviewed_relational_schema_used': True, 'empty_draft_tables_retained_by_migration': 7,
                  'explicit_catalog_read_schema_migration_used': True, 'test_only_schema_rewrite': False,
                  'browser_executable_override_used': bool(os.environ.get('ADMIN_TEST_BROWSER')),
                  'checked': ['public categories and collections with configured server store/channel, exact projected fields and scoped product counts before login','public Gin region directory and production Astro SSR health on owned nonempty routing data; no Next/Medusa runtime; missing configuration 503','explicit new-account action grants and current permission snapshot; unknown query and anonymous rejection', 'Ant role create and persisted current directory; employee role assignment; Chinese/English role drawer at three widths', 'wrong password then retry', 'real login and reload', 'HttpOnly and SameSite cookies',
                              'no credentials in browser storage', 'private response cache and request ID',
                              'missing CSRF and foreign Origin rejected', 'logout and old-cookie replay rejected',
                              'explicit product list projection and literal wildcard filtering', 'Ant product table search reload reset and empty state',
                              'reviewed schema product detail and paginated variants', 'per-product option membership and incomplete variant retained',
                              'SKU string precision and image soft-delete filters', 'Ant detail navigation reload missing state and three viewport widths',
                              'Ant edit drawer persists real data across reload', 'version conflicts and immutable operation replay',
                              'product writes require CSRF and authorized Origin', 'Chinese and English edit drawer at three viewport widths',
                              'exact 38-digit configured prices and rules in a paginated Ant drawer',
                              'shared price sets, foreign-product rejection and deleted price/list/link filters',
                              'Chinese and English price drawers at three viewport widths',
                              'exact inventory level balances and reservation mismatch evidence',
                              'unmatched reservations, deleted item/location and shared requirements retained',
                              'Chinese and English inventory drawer at three viewport widths',
                              'Ant stock edit persists exact incoming quantity through Gin and MySQL',
                              'stock version conflict and live reservation mismatch reject consumption',
                              'Chinese and English stock edit drawers at three viewport widths',
                              'Order list filters, current-version snapshot details and exact decimals in Chinese and English at three widths',
                              'Ant order amounts, exact taxes/discounts/shipping/credits and summary at three widths in Chinese and English; anonymous and unknown-query rejection',
                              'Ant order payment collections, sessions, expanded captures/refunds, exact 18-digit fractional money, held/unverified indicators and private payload exclusion; Chinese/English at three widths',
                              'Ant order fulfillment packages, explicit missing/deleted evidence, items with exact quantity and identifiers, labels with HTTP(S) links only, nullable delivery address and private provider payload exclusion; Chinese/English at three widths'],
                  'limits': {'mysql_cpus': 1, 'mysql_memory_mib': 768, 'redis_cpus': 1, 'redis_memory_mib': 128,
                             'network': 'owned bridge network; egress not restricted', 'published_ports': 'one random API port bound to 127.0.0.1 only'},
                  'real_accounts_used': False, 'application_resources_used': False, 'tls_secure_cookie_verified': False,
                  'full_authentication_accepted': False, 'full_admin_accepted': False,
                  'implementation_sha256': {name: hashlib.sha256((ROOT / name).read_bytes().replace(b'\r\n', b'\n')).hexdigest() for name in sorted(files)}}
    finally:
        password = ''
        for container in reversed(containers):
            inspected = docker('inspect', container, '--format', '{{index .Config.Labels "' + LABEL + '"}}')
            if inspected.returncode or inspected.stdout.decode().strip() != owner:
                raise RuntimeError('Browser fixture ownership mismatch; cleanup refused')
            if docker('rm', '-f', '-v', container).returncode:
                raise RuntimeError('Owned browser container cleanup failed')
        if network:
            inspected = docker('network', 'inspect', network, '--format', '{{index .Labels "' + LABEL + '"}}')
            if inspected.returncode or inspected.stdout.decode().strip() != owner:
                raise RuntimeError('Browser network ownership mismatch; cleanup refused')
            if docker('network', 'rm', network).returncode:
                raise RuntimeError('Owned browser network cleanup failed')
        for binary in owned_binaries:
            if binary.exists():
                binary.unlink()
    report['cleanup'] = 'both owned containers, bridge network and temporary binaries removed'
    (ROOT / 'docs/migration/admin-live-report.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'browser_tests': 3, 'real_api': True, 'real_mysql_redis': True, 'cart_browser': True, 'cleanup': 'complete'}))


if __name__ == '__main__':
    main()
