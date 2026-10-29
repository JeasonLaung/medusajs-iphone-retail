"""New artificial admin over real Gin dispatch, owned MySQL and Redis only."""
import hashlib
import json
import os
import re
from pathlib import Path
import secrets
import shutil
import subprocess
import sys
import time
import uuid

ROOT = Path(__file__).resolve().parents[2]
LABEL = 'eurorefurb.identity-rehearsal'


def docker(*args, timeout=90):
    try:
        return subprocess.run(['docker', *args], capture_output=True, timeout=timeout)
    except subprocess.TimeoutExpired:
        raise RuntimeError('Owned fixture Docker operation timed out; arguments suppressed') from None


def main():
    if sys.argv[1:] not in ([], ['--cart-runtime-only'], ['--promotion-usage-only'], ['--customer-accounts-only'], ['--store-collections-only'], ['--cart-contacts-only'], ['--cart-delivery-only'], ['--initial-payment-only'], ['--payment-status-only'], ['--order-numbering-only'], ['--order-snapshot-only']):
        raise RuntimeError('Unknown rehearsal option')
    numbering_only = sys.argv[1:] == ['--order-numbering-only']
    snapshot_only = sys.argv[1:] == ['--order-snapshot-only']
    status_only = snapshot_only or sys.argv[1:] == ['--payment-status-only']
    initial_only = status_only or sys.argv[1:] == ['--initial-payment-only']
    delivery_only = initial_only or sys.argv[1:] == ['--cart-delivery-only']
    contacts_only = delivery_only or sys.argv[1:] == ['--cart-contacts-only']
    cart_only = contacts_only or sys.argv[1:] == ['--cart-runtime-only']
    usage_only = sys.argv[1:] == ['--promotion-usage-only']
    customer_only = sys.argv[1:] == ['--customer-accounts-only']
    collection_only = sys.argv[1:] == ['--store-collections-only']
    if shutil.disk_usage(ROOT).free < 2 * (1 << 30):
        raise RuntimeError('At least 2 GiB free workspace disk required to preserve development services')
    for image in ['mysql:8.4', 'redis:7-alpine']:
        if docker('image', 'inspect', image).returncode:
            raise RuntimeError('Existing MySQL/Redis fixture images required; no pull performed')
    owner = uuid.uuid4().hex
    binary = ROOT / 'logs/gin-migration' / ('identity-' + owner + '.test')
    if binary.exists() or subprocess.run(['git', 'check-ignore', '--quiet', str(binary)], cwd=ROOT, capture_output=True).returncode:
        raise RuntimeError('New ignored runner path required')
    env = os.environ.copy()
    env.update(GOOS='linux', GOARCH='amd64', CGO_ENABLED='0')
    containers, owned_binary = [], False
    password = secrets.token_urlsafe(32)
    try:
        owned_binary = True
        built = subprocess.run(['go', 'test', '-c', '-o', str(binary), './internal/identity'],
                               cwd=ROOT / 'retail/backend', env=env, capture_output=True, timeout=120)
        if built.returncode:
            raise RuntimeError('Identity runner compilation failed')
        mysql = docker('run', '-d', '--name', 'eurorefurb-identity-mysql-' + owner[:12],
                       '--label', LABEL + '=' + owner, '--network', 'none', '--cpus', '1',
                       '--memory', '768m', '--memory-swap', '768m', '--pids-limit', '192',
                       '--tmpfs', '/var/lib/mysql:rw,size=512m', '--tmpfs', '/tmp:rw,size=32m,mode=1777',
                       '-e', 'MYSQL_ROOT_PASSWORD=' + password, '-e', 'MYSQL_DATABASE=identity_rehearsal',
                       'mysql:8.4', '--skip-log-bin', '--innodb-buffer-pool-size=64M',
                       '--performance-schema=OFF', '--mysqlx=OFF', '--max-connections=16',
                       '--table-open-cache=128', '--table-definition-cache=400',
                       '--table-open-cache-instances=1')
        if mysql.returncode:
            raise RuntimeError('Owned MySQL creation failed')
        mysql_id = mysql.stdout.decode().strip()
        containers.append(mysql_id)
        # Redis shares ONLY the owned no-network namespace. Loopback TCP is
        # inside these two containers, never published on the user's machine.
        redis = docker('run', '-d', '--name', 'eurorefurb-identity-redis-' + owner[:12],
                       '--label', LABEL + '=' + owner, '--network', 'container:' + mysql_id,
                       '--cpus', '1', '--memory', '128m', '--memory-swap', '128m', '--pids-limit', '64',
                       '--tmpfs', '/data:rw,size=16m', 'redis:7-alpine', 'redis-server',
                       '--bind', '127.0.0.1', '--save', '', '--appendonly', 'no',
                       '--maxmemory', '16mb', '--maxmemory-policy', 'noeviction')
        if redis.returncode:
            raise RuntimeError('Owned Redis creation failed')
        redis_id = redis.stdout.decode().strip()
        containers.append(redis_id)
        for _ in range(100):
            ready = docker('exec', '-e', 'MYSQL_PWD=' + password, mysql_id, 'mysql', '-uroot', '--protocol=socket', '-e', 'SELECT 1', timeout=10)
            if ready.returncode == 0:
                break
            time.sleep(.5)
        else:
            raise RuntimeError('Owned MySQL did not become ready')
        if docker('exec', redis_id, 'redis-cli', 'PING').stdout.strip() != b'PONG':
            raise RuntimeError('Owned Redis not ready')
        with binary.open('rb') as source:
            copied = subprocess.run(['docker', 'exec', '-i', mysql_id, '/bin/sh', '-c', 'umask 077; cat > /opt/identity.test'],
                                    stdin=source, capture_output=True, timeout=60)
        if copied.returncode or docker('exec', mysql_id, 'chmod', '700', '/opt/identity.test').returncode:
            raise RuntimeError('Identity runner transfer failed')
        # One public artificial oracle; no application configuration or data.
        if not usage_only and (docker('exec', mysql_id, 'mkdir', '-p', '/docs/migration/fixtures').returncode or docker('cp', str(ROOT/'docs/migration/fixtures/cart-promotion-context.json'), mysql_id+':/docs/migration/fixtures/cart-promotion-context.json').returncode):
            raise RuntimeError('Artificial promotion context oracle transfer failed')
        # Each suite owns artificial databases in this uniquely labelled,
        # disconnected server. Release their tmpfs tables before the next suite;
        # otherwise cumulative DDL consumes the bounded fixture memory.
        suites = ['PromotionUsage','PromotionSchema','GeoSchema','CartPriceRefresh','CartTaxRefresh','TaxImport','TaxSchema','AdminAuthentication','CatalogSchemaTransition','ProductEdits','ProductPrices',
                  'ProductInventory','InventoryStockEdits','ReservationTransactions','OrderSchema',
                  'OrderImport','CustomerHistory','OrderRead','PaymentSchema','PaymentImport','FulfillmentHistory','CartSchema','CartRuntime']
        if cart_only:
            suites = ['CartRuntime']
        if usage_only:
            suites = ['PromotionUsage']
        if customer_only:
            suites = ['AdminAuthentication']
        if collection_only:
            suites = ['ProductPrices']
        if snapshot_only:
            suites = ['OrderRead','CartRuntime']
        if numbering_only:
            suites = ['OrderNumbering']
        output, errors = b'', b''
        for suite in suites:
            print(json.dumps({'running_suite': suite}), flush=True)
            tested = docker('exec', '-e', 'MYSQL_PWD=' + password, '-e', 'IDENTITY_REHEARSAL=1', mysql_id,
                            '/opt/identity.test', '-test.v', '-test.run', '^TestOwned' + suite + '$',
                            '-test.timeout', '150s', timeout=180)
            output += tested.stdout
            errors += tested.stderr
            if tested.returncode:
                break
            names = docker('exec','-e','MYSQL_PWD=' + password,mysql_id,'mysql','-uroot','--protocol=socket',
                           '--batch','--skip-column-names','-e',
                           "SELECT schema_name FROM information_schema.schemata WHERE schema_name NOT IN ('mysql','information_schema','performance_schema','sys')")
            if names.returncode:
                raise RuntimeError('Owned artificial database enumeration failed')
            databases = names.stdout.decode('ascii').splitlines()
            if any(not re.fullmatch(r'[A-Za-z0-9_]+',name) for name in databases):
                raise RuntimeError('Unexpected artificial database identifier; cleanup refused')
            cleanup = ''.join('DROP DATABASE `' + name + '`;' for name in databases)
            cleanup += 'CREATE DATABASE identity_rehearsal;'
            if docker('exec','-e','MYSQL_PWD=' + password,mysql_id,'mysql','-uroot','--protocol=socket','-e',cleanup).returncode:
                raise RuntimeError('Owned artificial database cleanup failed')
        tested = subprocess.CompletedProcess([], tested.returncode, output, errors)
        if numbering_only:
            markers = [line.split('ORDER_NUMBERING_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'ORDER_NUMBERING_REHEARSAL ' in line]
            if tested.returncode or len(markers)!=1:
                print(tested.stdout.decode(errors='replace'))
                raise RuntimeError('Owned order numbering verification failed')
            counts = json.loads(markers[0])
            files = ['retail/backend/internal/orders/numbering.go','retail/backend/internal/orders/numbering_test.go','retail/backend/internal/orders/schema.go','retail/backend/internal/orders/schema_plan.json','retail/backend/internal/identity/order_numbering_test.go','retail/scripts/test-admin-auth.py']
            files += [p.relative_to(ROOT).as_posix() for p in (ROOT/'retail/backend/migrations').glob('*.sql')]
            report = {'scope':'Owned MySQL order number cutover and allocation, not checkout/order/stock acceptance',
                      'counts':counts,'migrations':len(list((ROOT/'retail/backend/migrations').glob('*.sql'))),
                      'includes_uncommitted_payment_0037':(ROOT/'retail/backend/migrations/0037_mollie_notification_inbox.sql').exists(),
                      'limits':{'mysql_cpus':1,'mysql_memory_mib':768,'published_ports':0,'external_network':False},
                      'implementation_sha256':{name:hashlib.sha256((ROOT/name).read_bytes().replace(b'\r\n',b'\n')).hexdigest() for name in files}}
        elif contacts_only:
            markers = [line.split('CART_CONTACT_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CART_CONTACT_REHEARSAL ' in line]
            if tested.returncode or len(markers) != 1:
                print(tested.stdout.decode(errors='replace'))
                raise RuntimeError('Owned cart contact checks failed')
            delivery_markers = [line.split('CART_DELIVERY_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CART_DELIVERY_REHEARSAL ' in line]
            delivery_http_markers = [line.split('CART_DELIVERY_HTTP_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CART_DELIVERY_HTTP_REHEARSAL ' in line]
            if len(delivery_markers)!=1 or len(delivery_http_markers)!=1: raise RuntimeError('Cart delivery verification missing')
            initial_markers=[line.split('CART_INITIAL_PAYMENT_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CART_INITIAL_PAYMENT_REHEARSAL ' in line]
            initial_http_markers=[line.split('CART_INITIAL_PAYMENT_HTTP_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CART_INITIAL_PAYMENT_HTTP_REHEARSAL ' in line]
            if len(initial_markers)!=1 or len(initial_http_markers)!=1: raise RuntimeError('Initial payment evidence missing')
            status_markers=[line.split('CART_PAYMENT_OBSERVATION_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CART_PAYMENT_OBSERVATION_REHEARSAL ' in line]
            status_http_markers=[line.split('CART_PAYMENT_OBSERVATION_HTTP_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CART_PAYMENT_OBSERVATION_HTTP_REHEARSAL ' in line]
            order_reader_markers=[line.split('ORDER_READ_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'ORDER_READ_REHEARSAL ' in line]
            if snapshot_only and len(order_reader_markers)!=1:raise RuntimeError('Existing order reader regression evidence missing')
            snapshot_markers=[line.split('CART_ORDER_SNAPSHOT_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CART_ORDER_SNAPSHOT_REHEARSAL ' in line]
            stock_markers=[line.split('CART_ORDER_STOCK_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CART_ORDER_STOCK_REHEARSAL ' in line]
            if len(stock_markers)!=1:raise RuntimeError('Paid order stock evidence missing')
            if len(snapshot_markers)!=1:raise RuntimeError('Paid order snapshot evidence missing')
            if len(status_markers)!=1 or len(status_http_markers)!=1:raise RuntimeError('Payment status verification missing')
            counts = json.loads(markers[0])
            http_markers = [line.split('CART_CONTACT_HTTP_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CART_CONTACT_HTTP_REHEARSAL ' in line]
            if len(http_markers)!=1: raise RuntimeError('Cart contact HTTP verification missing')
            files = [p.relative_to(ROOT).as_posix() for folder in ('cart','schema','identity','platform','sessionstore','payments','orders') for p in (ROOT/'retail/backend/internal'/folder).glob('*.go')]
            files += [p.relative_to(ROOT).as_posix() for p in (ROOT/'retail/backend/migrations').glob('*.sql')]
            files += ['retail/scripts/test-admin-auth.py','retail/backend/cmd/api/main.go']
            report = {'scope':'Owned Gin/MySQL/Redis payment creation and authoritative return observation with isolated adapter; no real external provider, browser, reservation or order acceptance', 'counts':counts,'existing_order_reader':json.loads(order_reader_markers[0]) if snapshot_only else None,'order_snapshot':json.loads(snapshot_markers[0]),'order_stock':json.loads(stock_markers[0]), 'http':json.loads(http_markers[0]), 'payment_status':json.loads(status_markers[0]),'payment_status_http':json.loads(status_http_markers[0]),'initial_payment':json.loads(initial_markers[0]),'initial_payment_http':json.loads(initial_http_markers[0]), 'delivery':json.loads(delivery_markers[0]), 'delivery_http':json.loads(delivery_http_markers[0]),
                      'migrations':len(list((ROOT/'retail/backend/migrations').glob('*.sql'))),
                      'includes_uncommitted_payment_0037':(ROOT/'retail/backend/migrations/0037_mollie_notification_inbox.sql').exists(),
                      'limits':{'mysql_cpus':1,'mysql_memory_mib':768,'redis_cpus':1,'redis_memory_mib':128,'published_ports':0,'external_network':False},
                      'implementation_sha256':{name:hashlib.sha256((ROOT/name).read_bytes().replace(b'\r\n',b'\n')).hexdigest() for name in files}}
        elif collection_only:
            markers = [line.split('STORE_PRODUCT_REHEARSAL ', 1)[1] for line in tested.stdout.decode().splitlines() if 'STORE_PRODUCT_REHEARSAL ' in line]
            if tested.returncode or len(markers) != 1:
                print(tested.stdout.decode(errors='replace'))
                raise RuntimeError('Owned store collection checks failed')
            counts = json.loads(markers[0])
            if not counts.get('same_variant_option_group_filter') or not counts.get('collection_latest_before_pagination'):
                raise RuntimeError('Collection query evidence missing')
            files = ['retail/backend/internal/catalog/store_products.go','retail/backend/internal/catalog/store_products_test.go','retail/backend/internal/identity/store_products_test.go']
            report = {'scope':'owned Gin/MySQL public catalog and collection query verification, no browser', 'counts':counts,
                      'limits':{'mysql_cpus':1,'mysql_memory_mib':768,'redis_cpus':1,'redis_memory_mib':128,'published_ports':0,'external_network':False},
                      'implementation_sha256':{name:hashlib.sha256((ROOT/name).read_bytes().replace(b'\r\n',b'\n')).hexdigest() for name in files}}
        elif customer_only:
            markers = [line.split('CUSTOMER_ACCOUNTS_REHEARSAL ', 1)[1] for line in tested.stdout.decode().splitlines() if 'CUSTOMER_ACCOUNTS_REHEARSAL ' in line]
            if tested.returncode or len(markers) != 1:
                print(tested.stdout.decode(errors='replace'))
                raise RuntimeError('Owned customer account checks failed')
            counts = json.loads(markers[0])
            notification_markers=[line.split('NOTIFICATION_MAIL_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'NOTIFICATION_MAIL_REHEARSAL ' in line]
            if len(notification_markers)!=1:
                raise RuntimeError('Notification mail verification missing')
            profile_markers=[line.split('CUSTOMER_PROFILE_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CUSTOMER_PROFILE_REHEARSAL ' in line]
            if len(profile_markers)!=1:
                raise RuntimeError('Customer profile verification missing')
            delivery_markers=[line.split('CUSTOMER_DELIVERY_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CUSTOMER_DELIVERY_REHEARSAL ' in line]
            if len(delivery_markers)!=1:
                raise RuntimeError('Customer delivery verification missing')
            address_markers=[line.split('CUSTOMER_ADDRESSES_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CUSTOMER_ADDRESSES_REHEARSAL ' in line]
            if len(address_markers)!=1:
                raise RuntimeError('Customer address verification missing')
            http_markers = [line.split('CUSTOMER_HTTP_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CUSTOMER_HTTP_REHEARSAL ' in line]
            if len(http_markers) != 1:
                raise RuntimeError('Customer HTTP verification missing')
            files = ['retail/backend/migrations/0038_customer_accounts.sql', 'retail/backend/internal/customer/accounts.go', 'retail/backend/internal/customer/accounts_test.go', 'retail/backend/internal/identity/customer_accounts_test.go']
            files += ['retail/backend/internal/customer/http.go','retail/backend/internal/customer/bridge.go','retail/backend/internal/customer/profile.go','retail/backend/internal/identity/customer_profile_test.go','retail/backend/internal/customer/bridge_test.go','retail/backend/internal/identity/customer_http_test.go','retail/backend/internal/protected/config.go','retail/backend/internal/protected/config_test.go','retail/backend/cmd/api/main.go','retail/backend/internal/platform/request.go']
            files += ['retail/backend/migrations/0039_customer_account_addresses.sql','retail/backend/internal/customer/addresses.go','retail/backend/internal/customer/addresses_http.go','retail/backend/internal/identity/customer_addresses_test.go']
            files += ['retail/backend/migrations/0040_customer_verification_delivery.sql','retail/backend/internal/customer/delivery.go','retail/backend/internal/maildelivery/resend.go','retail/backend/internal/maildelivery/resend_test.go','retail/backend/internal/identity/customer_delivery_test.go','retail/backend/cmd/send-verification-mail/main.go']
            files += ['retail/backend/migrations/0041_notification_mail_accounts.sql','retail/backend/internal/notificationmail/accounts.go','retail/backend/internal/notificationmail/accounts_test.go','retail/backend/internal/notificationmail/http.go','retail/backend/internal/identity/notification_mail_test.go','retail/backend/internal/authorization/model.go']
            report = {'notification_mail':json.loads(notification_markers[0]),'scope':'New customer identity, profile, addresses and verification delivery; complete storefront and actual external mail not accepted','counts':counts,'http':json.loads(http_markers[0]),'profile':json.loads(profile_markers[0]),'addresses':json.loads(address_markers[0]),'verification_delivery':json.loads(delivery_markers[0]),'application_resources_used':False,'real_accounts_used':False,'working_tree_migration_count':len(list((ROOT/'retail/backend/migrations').glob('*.sql'))),'includes_uncommitted_payment_0037':(ROOT/'retail/backend/migrations/0037_mollie_notification_inbox.sql').exists(),'implementation_sha256':{name:hashlib.sha256((ROOT/name).read_bytes().replace(b'\r\n',b'\n')).hexdigest() for name in files}}
        elif usage_only:
            if tested.returncode:
                print(tested.stdout.decode(errors='replace'))
                raise RuntimeError('Owned promotion usage checks failed')
            markers = [line.split('PROMOTION_USAGE_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'PROMOTION_USAGE_REHEARSAL ' in line]
            if len(markers) != 1:
                raise RuntimeError('Promotion usage evidence missing')
            counts = json.loads(markers[0])
            files = ['retail/scripts/test-admin-auth.py','retail/backend/internal/identity/promotion_usage_test.go']
            files += [p.relative_to(ROOT).as_posix() for p in (ROOT/'retail/backend/internal/promotion').glob('*') if p.is_file()]
            files += [p.relative_to(ROOT).as_posix() for p in (ROOT/'retail/backend/migrations').glob('*.sql')]
            report = {'format_version':1,'scope':'Owned promotion usage only; not complete checkout','promotion_usage':counts,
                      'implementation_sha256':{name:hashlib.sha256((ROOT/name).read_bytes().replace(b'\r\n',b'\n')).hexdigest() for name in files}}
        else:
            geo_markers = [line.split('GEO_SCHEMA_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'GEO_SCHEMA_REHEARSAL ' in line]
            if tested.returncode == 0 and not cart_only and len(geo_markers) != 1:
                raise RuntimeError('Geo schema evidence missing')
            promotion_markers = [line.split('PROMOTION_SCHEMA_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'PROMOTION_SCHEMA_REHEARSAL ' in line]
            if tested.returncode == 0 and not cart_only and len(promotion_markers) != 1:
                raise RuntimeError('Promotion schema evidence missing')
            usage_markers = [line.split('PROMOTION_USAGE_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'PROMOTION_USAGE_REHEARSAL ' in line]
            if tested.returncode == 0 and not cart_only and len(usage_markers) != 1:
                raise RuntimeError('Promotion usage evidence missing')
            attribute_read_markers = [line.split('PROMOTION_ATTRIBUTE_READ_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'PROMOTION_ATTRIBUTE_READ_REHEARSAL ' in line]
            if tested.returncode == 0 and not cart_only and len(attribute_read_markers) != 1:
                raise RuntimeError('Promotion attribute read evidence missing')
            read_markers = [line.split('PROMOTION_READ_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'PROMOTION_READ_REHEARSAL ' in line]
            if tested.returncode == 0 and not cart_only and len(read_markers) != 1:
                raise RuntimeError('Promotion read evidence missing')
            promotion_product_markers = [line.split('PROMOTION_PRODUCTS_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'PROMOTION_PRODUCTS_REHEARSAL ' in line]
            if tested.returncode == 0 and len(promotion_product_markers) != 1:
                raise RuntimeError('Promotion product relation evidence missing')
            promotion_adjustment_markers = [line.split('CART_PROMOTION_ADJUSTMENTS_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CART_PROMOTION_ADJUSTMENTS_REHEARSAL ' in line]
            if tested.returncode == 0 and len(promotion_adjustment_markers) != 1:
                raise RuntimeError('Cart promotion adjustment evidence missing')
            promotion_amount_markers = [line.split('CART_PROMOTION_AMOUNTS_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CART_PROMOTION_AMOUNTS_REHEARSAL ' in line]
            payment_creation_execution_markers = [line.split('CART_PAYMENT_CREATION_EXECUTION_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CART_PAYMENT_CREATION_EXECUTION_REHEARSAL ' in line]
            if tested.returncode == 0 and len(payment_creation_execution_markers) != 1:
                raise RuntimeError('Cart payment creation executor evidence missing')
            payment_compensation_markers = [line.split('CART_PAYMENT_COMPENSATION_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CART_PAYMENT_COMPENSATION_REHEARSAL ' in line]
            if tested.returncode == 0 and len(payment_compensation_markers) != 1:
                raise RuntimeError('Cart payment compensation evidence missing')
            payment_execution_markers = [line.split('CART_PAYMENT_EXECUTION_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CART_PAYMENT_EXECUTION_REHEARSAL ' in line]
            if tested.returncode == 0 and len(payment_execution_markers) != 1:
                raise RuntimeError('Cart payment execution evidence missing')
            payment_operation_markers = [line.split('CART_PAYMENT_OPERATIONS_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CART_PAYMENT_OPERATIONS_REHEARSAL ' in line]
            if tested.returncode == 0 and len(payment_operation_markers) != 1:
                raise RuntimeError('Cart payment operation evidence missing')
            payment_request_markers = [line.split('PAYMENT_REQUESTS_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'PAYMENT_REQUESTS_REHEARSAL ' in line]
            if tested.returncode == 0 and len(payment_request_markers) != 1:
                raise RuntimeError('Payment request recovery evidence missing')
            payment_effect_markers = [line.split('PAYMENT_EFFECTS_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'PAYMENT_EFFECTS_REHEARSAL ' in line]
            if tested.returncode == 0 and len(payment_effect_markers) != 1:
                raise RuntimeError('Payment effect journal evidence missing')
            promotion_refresh_markers = [line.split('CART_PROMOTION_REFRESH_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CART_PROMOTION_REFRESH_REHEARSAL ' in line]
            if tested.returncode == 0 and len(promotion_refresh_markers) != 1:
                raise RuntimeError('Cart promotion refresh composition evidence missing')
            if tested.returncode == 0 and len(promotion_amount_markers) != 1:
                raise RuntimeError('Cart promotion monetary preparation evidence missing')
            amount_markers = [line.split('CART_AMOUNTS_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CART_AMOUNTS_REHEARSAL ' in line]
            if tested.returncode == 0 and len(amount_markers) != 1:
                raise RuntimeError('Cart amount evidence missing')
            item_markers = [line.split('CART_ITEMS_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CART_ITEMS_REHEARSAL ' in line]
            item_http_markers = [line.split('CART_ITEMS_HTTP_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CART_ITEMS_HTTP_REHEARSAL ' in line]
            if tested.returncode == 0 and len(item_http_markers) != 1:
                raise RuntimeError('Cart item HTTP evidence missing')
            if tested.returncode == 0 and len(item_markers) != 1:
                raise RuntimeError('Cart item mutation evidence missing')
            if cart_only:
                markers = [line.split('CART_RUNTIME_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CART_RUNTIME_REHEARSAL ' in line]
                if tested.returncode or len(markers) != 1:
                    print(tested.stdout.decode(errors='replace'))
                    raise RuntimeError('Owned cart runtime checks failed')
                counts = json.loads(markers[0])
                http_markers = [line.split('CART_HTTP_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CART_HTTP_REHEARSAL ' in line]
                if len(http_markers) != 1:
                    raise RuntimeError('Cart HTTP evidence missing')
                files = ['retail/scripts/test-admin-auth.py','retail/backend/internal/identity/cart_runtime_test.go','retail/backend/internal/identity/cart_http_test.go','retail/backend/internal/identity/cart_amounts_test.go','retail/backend/internal/identity/promotion_products_test.go','retail/backend/internal/identity/cart_promotion_adjustments_test.go','retail/backend/internal/identity/cart_promotion_amounts_test.go','retail/backend/internal/identity/cart_items_test.go','retail/backend/internal/identity/cart_items_http_test.go',
                         'retail/backend/internal/server/router.go','retail/backend/internal/platform/request.go','retail/backend/internal/platform/http.go',
                         'retail/backend/internal/catalog/schema.go','retail/backend/internal/catalog/schema_plan.json']
                files += [str(p.relative_to(ROOT)).replace('\\','/') for folder in ('cart','schema','sessionstore','totals','catalog','pricing','inventory','promotion','payments','protected') for p in (ROOT/'retail/backend/internal'/folder).glob('*') if p.is_file()]
                files += ['retail/scripts/capture-cart-promotion-context.cjs','docs/migration/fixtures/cart-promotion-context.json']
                files += ['retail/scripts/capture-cart-payment-refresh.cjs','docs/migration/fixtures/cart-payment-refresh.json']
                files += ['retail/backend/internal/identity/cart_promotion_refresh_test.go','retail/backend/internal/identity/payment_effects_test.go','retail/backend/internal/payments/effects.go','retail/backend/internal/payments/requests.go','retail/backend/internal/identity/payment_requests_test.go','retail/backend/internal/identity/cart_payment_operations_test.go','retail/backend/internal/identity/cart_payment_execution_test.go','retail/backend/internal/identity/cart_payment_compensation_test.go','retail/backend/internal/identity/cart_payment_creation_execution_test.go']
                files += [str(p.relative_to(ROOT)).replace('\\','/') for p in (ROOT/'retail/backend/migrations').glob('*.sql')]
                report = {'format_version':1,'scope':'Owned artificial MySQL/Redis cart runtime and HTTP dispatch; browser and checkout not accepted',
                          'cart_runtime':counts,'cart_items':json.loads(item_markers[0]),'cart_items_http':json.loads(item_http_markers[0]),'cart_amounts':json.loads(amount_markers[0]),'cart_payment_creation_execution':json.loads(payment_creation_execution_markers[0]),'cart_payment_compensation':json.loads(payment_compensation_markers[0]),'cart_payment_execution':json.loads(payment_execution_markers[0]),'cart_payment_operations':json.loads(payment_operation_markers[0]),'payment_requests':json.loads(payment_request_markers[0]),'payment_effects':json.loads(payment_effect_markers[0]),'cart_promotion_refresh':json.loads(promotion_refresh_markers[0]),'cart_promotion_amounts':json.loads(promotion_amount_markers[0]),'cart_promotion_adjustments':json.loads(promotion_adjustment_markers[0]),'promotion_products':json.loads(promotion_product_markers[0]),'cart_http':json.loads(http_markers[0]),'application_resources_used':False,'real_accounts_used':False,
                          'implementation_sha256':{name:hashlib.sha256((ROOT/name).read_bytes().replace(b'\r\n',b'\n')).hexdigest() for name in files}}
            else:
                markers = [line.split('IDENTITY_REHEARSAL ', 1)[1] for line in tested.stdout.decode().splitlines() if 'IDENTITY_REHEARSAL ' in line]
                if tested.returncode or len(markers) != 1:
                    # All inputs and failure messages in this binary are artificial.
                    print(tested.stdout.decode(errors='replace'))
                    print(tested.stderr.decode(errors='replace'))
                    state = docker('inspect', mysql_id, '--format', '{{.State.OOMKilled}} {{.State.ExitCode}}')
                    print(json.dumps({'test_exit_code': tested.returncode, 'mysql_oom_and_exit': state.stdout.decode().strip()}))
                    raise RuntimeError('Owned MySQL/Redis authentication checks failed')
                counts = json.loads(markers[0])
                customer_account_markers = [line.split('CUSTOMER_ACCOUNTS_REHEARSAL ', 1)[1] for line in tested.stdout.decode().splitlines() if 'CUSTOMER_ACCOUNTS_REHEARSAL ' in line]
                if len(customer_account_markers) != 1:
                    raise RuntimeError('New customer account verification missing')
                cart_markers=[line.split('CART_SCHEMA_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CART_SCHEMA_REHEARSAL ' in line]
                if len(cart_markers)!=1:
                    raise RuntimeError('Cart schema verification missing')
                cart_runtime_markers=[line.split('CART_RUNTIME_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CART_RUNTIME_REHEARSAL ' in line]
                if len(cart_runtime_markers)!=1:
                    raise RuntimeError('Cart runtime verification missing')
                policy_markers=[line.split('STORE_POLICIES_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'STORE_POLICIES_REHEARSAL ' in line]
                if len(policy_markers)!=1:
                    raise RuntimeError('Store policy verification missing')
                search_markers=[line.split('STORE_SEARCH_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'STORE_SEARCH_REHEARSAL ' in line]
                if len(search_markers)!=1:
                    raise RuntimeError('Store search verification missing')
                product_markers=[line.split('STORE_PRODUCT_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'STORE_PRODUCT_REHEARSAL ' in line]
                availability_markers=[line.split('STORE_AVAILABILITY_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'STORE_AVAILABILITY_REHEARSAL ' in line]
                cart_stock_markers=[line.split('CART_STOCK_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CART_STOCK_REHEARSAL ' in line]
                if len(cart_stock_markers)!=1:
                    raise RuntimeError('Whole-cart stock evidence missing')
                display_markers=[line.split('STORE_DISPLAY_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'STORE_DISPLAY_REHEARSAL ' in line]
                if len(display_markers)!=1:
                    raise RuntimeError('Store display verification missing')
                if len(availability_markers)!=1:
                    raise RuntimeError('Store availability verification missing')
                pricing_markers=[line.split('STORE_PRICING_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'STORE_PRICING_REHEARSAL ' in line]
                if len(pricing_markers)!=1:
                    raise RuntimeError('Store pricing verification missing')
                if len(product_markers)!=1:
                    raise RuntimeError('Store product verification missing')
                taxonomy_markers=[line.split('STORE_TAXONOMY_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'STORE_TAXONOMY_REHEARSAL ' in line]
                if len(taxonomy_markers)!=1:
                    raise RuntimeError('Store taxonomy verification missing')
                locale_markers=[line.split('STORE_LOCALE_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'STORE_LOCALE_REHEARSAL ' in line]
                if len(locale_markers)!=1:
                    raise RuntimeError('Store locale verification missing')
                region_markers=[line.split('STORE_REGION_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'STORE_REGION_REHEARSAL ' in line]
                if len(region_markers)!=1:
                    raise RuntimeError('Store region verification missing')
                manage_markers=[line.split('AUTHORIZATION_MANAGEMENT_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'AUTHORIZATION_MANAGEMENT_REHEARSAL ' in line]
                if len(manage_markers)!=1:
                    raise RuntimeError('Authorization management verification missing')
                authorization_markers=[line.split('AUTHORIZATION_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'AUTHORIZATION_REHEARSAL ' in line]
                if len(authorization_markers)!=1:
                    raise RuntimeError('Authorization verification missing')
                fulfillment_markers = [line.split('FULFILLMENT_HISTORY_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'FULFILLMENT_HISTORY_REHEARSAL ' in line]
                if len(fulfillment_markers)!=1:
                    raise RuntimeError('Fulfillment history verification missing')
                fulfillment_read_markers=[line.split('ORDER_FULFILLMENT_READ_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'ORDER_FULFILLMENT_READ_REHEARSAL ' in line]
                if len(fulfillment_read_markers)!=1:
                    raise RuntimeError('Order fulfillment read verification missing')
                payment_read_markers=[line.split('ORDER_PAYMENT_READ_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'ORDER_PAYMENT_READ_REHEARSAL ' in line]
                if len(payment_read_markers)!=1:
                    raise RuntimeError('Order payment read verification missing')
                payment_import_markers = [line.split('PAYMENT_IMPORT_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'PAYMENT_IMPORT_REHEARSAL ' in line]
                if len(payment_import_markers)!=1:
                    raise RuntimeError('Payment import verification missing')
                payment_markers = [line.split('PAYMENT_SCHEMA_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'PAYMENT_SCHEMA_REHEARSAL ' in line]
                if len(payment_markers)!=1:
                    raise RuntimeError('Payment schema verification missing')
                schema_markers = [line.split('CATALOG_SCHEMA_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CATALOG_SCHEMA_REHEARSAL ' in line]
                if len(schema_markers)!=1:
                    raise RuntimeError('Catalog schema verification missing')
                edit_markers = [line.split('PRODUCT_EDIT_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'PRODUCT_EDIT_REHEARSAL ' in line]
                if len(edit_markers)!=1:
                    raise RuntimeError('Product edit verification missing')
                price_markers = [line.split('PRODUCT_PRICE_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'PRODUCT_PRICE_REHEARSAL ' in line]
                if len(price_markers)!=1:
                    raise RuntimeError('Product price verification missing')
                inventory_markers = [line.split('PRODUCT_INVENTORY_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'PRODUCT_INVENTORY_REHEARSAL ' in line]
                if len(inventory_markers)!=1:
                    raise RuntimeError('Inventory verification missing')
                import_markers = [line.split('INVENTORY_IMPORT_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'INVENTORY_IMPORT_REHEARSAL ' in line]
                if len(import_markers)!=1:
                    raise RuntimeError('Inventory import verification missing')
                stock_markers = [line.split('STOCK_EDIT_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'STOCK_EDIT_REHEARSAL ' in line]
                if len(stock_markers)!=1:
                    raise RuntimeError('Stock edit verification missing')
                reservation_markers = [line.split('RESERVATION_TRANSACTION_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'RESERVATION_TRANSACTION_REHEARSAL ' in line]
                if len(reservation_markers)!=1:
                    raise RuntimeError('Reservation transaction verification missing')
                order_markers = [line.split('ORDER_SCHEMA_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'ORDER_SCHEMA_REHEARSAL ' in line]
                if len(order_markers)!=1:
                    raise RuntimeError('Order schema verification missing')
                order_import_markers = [line.split('ORDER_IMPORT_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'ORDER_IMPORT_REHEARSAL ' in line]
                if len(order_import_markers)!=1:
                    raise RuntimeError('Order import verification missing')
                customer_markers = [line.split('CUSTOMER_HISTORY_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CUSTOMER_HISTORY_REHEARSAL ' in line]
                if len(customer_markers)!=1:
                    raise RuntimeError('Customer history verification missing')
                order_read_markers = [line.split('ORDER_READ_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'ORDER_READ_REHEARSAL ' in line]
                if len(order_read_markers)!=1:
                    raise RuntimeError('Order read verification missing')
                files = ['retail/scripts/test-admin-auth.py', 'retail/backend/internal/identity/admin.go',
                         'retail/backend/internal/identity/admin_test.go', 'retail/backend/internal/identity/integration_test.go',
                         'retail/backend/internal/server/router.go', 'retail/backend/internal/platform/http.go',
                         'retail/backend/internal/platform/request.go', 'retail/backend/cmd/api/main.go',
                         'retail/backend/cmd/create-admin/main.go', 'retail/backend/migrations/0009_admin_auth_version.sql',
                         'retail/backend/internal/sessionstore/store.go', 'retail/backend/internal/sessionstore/config.go']
                tax_markers = [line.split('TAX_SCHEMA_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'TAX_SCHEMA_REHEARSAL ' in line]
                if len(tax_markers) != 1:
                    raise RuntimeError('Tax schema and actual selection evidence missing')
                tax_import_markers = [line.split('TAX_IMPORT_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'TAX_IMPORT_REHEARSAL ' in line]
                if len(tax_import_markers) != 1:
                    raise RuntimeError('Tax import evidence missing')
                flat_markers = [line.split('FLAT_SHIPPING_PRICE_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'FLAT_SHIPPING_PRICE_REHEARSAL ' in line]
                if len(flat_markers) != 1:
                    raise RuntimeError('Flat shipping price evidence missing')
                files += ['retail/backend/internal/identity/flat_shipping_prices_test.go','retail/backend/internal/identity/geo_schema_test.go','retail/backend/migrations/0030_fulfillment_geo_relational.sql','retail/scripts/generate-geo-schema.py']
                profile_markers = [line.split('CART_SHIPPING_PROFILES_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CART_SHIPPING_PROFILES_REHEARSAL ' in line]
                if len(profile_markers) != 1:
                    raise RuntimeError('Cart shipping profile evidence missing')
                quote_markers = [line.split('CART_SHIPPING_QUOTES_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CART_SHIPPING_QUOTES_REHEARSAL ' in line]
                if len(quote_markers) != 1:
                    raise RuntimeError('Cart shipping quote evidence missing')
                selection_markers = [line.split('CART_SHIPPING_SELECTION_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CART_SHIPPING_SELECTION_REHEARSAL ' in line]
                if len(selection_markers) != 1:
                    raise RuntimeError('Cart shipping selection evidence missing')
                method_markers = [line.split('CART_SHIPPING_METHODS_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CART_SHIPPING_METHODS_REHEARSAL ' in line]
                if len(method_markers) != 1:
                    raise RuntimeError('Cart shipping persistence evidence missing')
                refresh_markers = [line.split('CART_SHIPPING_REFRESH_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CART_SHIPPING_REFRESH_REHEARSAL ' in line]
                if len(refresh_markers) != 1:
                    raise RuntimeError('Cart shipping refresh evidence missing')
                files += ['retail/backend/internal/identity/cart_shipping_profiles_test.go','retail/backend/internal/identity/cart_shipping_quotes_test.go','retail/backend/internal/identity/cart_shipping_selection_test.go','retail/backend/internal/identity/cart_shipping_methods_test.go','retail/backend/internal/identity/cart_shipping_refresh_test.go']
                manual_markers = [line.split('MANUAL_SHIPPING_QUOTES_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'MANUAL_SHIPPING_QUOTES_REHEARSAL ' in line]
                if len(manual_markers) != 1:
                    raise RuntimeError('Manual shipping quote evidence missing')
                files += ['retail/backend/internal/identity/manual_shipping_quotes_test.go']
                candidate_markers = [line.split('SHIPPING_CANDIDATES_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'SHIPPING_CANDIDATES_REHEARSAL ' in line]
                if len(candidate_markers) != 1:
                    raise RuntimeError('Shipping candidate evidence missing')
                files += ['retail/backend/internal/identity/shipping_candidates_test.go']
                snapshot_markers = [line.split('CART_ITEM_SNAPSHOT_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CART_ITEM_SNAPSHOT_REHEARSAL ' in line]
                if len(snapshot_markers) != 1:
                    raise RuntimeError('Cart item snapshot evidence missing')
                files += ['retail/backend/internal/identity/cart_item_snapshots_test.go']
                price_refresh_markers = [line.split('CART_PRICE_REFRESH_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CART_PRICE_REFRESH_REHEARSAL ' in line]
                if len(price_refresh_markers) != 1:
                    raise RuntimeError('Cart price refresh evidence missing')
                files += ['retail/backend/internal/identity/cart_price_refresh_test.go']
                tax_refresh_markers = [line.split('CART_TAX_REFRESH_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CART_TAX_REFRESH_REHEARSAL ' in line]
                if len(tax_refresh_markers) != 1:
                    raise RuntimeError('Cart tax refresh evidence missing')
                files += ['retail/backend/internal/identity/cart_tax_refresh_test.go']
                files += ['retail/backend/migrations/0029_tax_import_evidence.sql','retail/scripts/generate-tax-import-plan.py','retail/backend/internal/identity/tax_import_test.go']
                files += [p.relative_to(ROOT).as_posix() for p in (ROOT/'retail/backend/internal/taximport').glob('*') if p.is_file()]
                files += ['retail/backend/migrations/0032_promotion_usage_receipts.sql','retail/backend/migrations/0031_promotion_relational.sql','retail/scripts/generate-promotion-schema.py','retail/backend/internal/identity/promotion_schema_test.go','retail/backend/internal/identity/promotion_read_test.go','retail/backend/internal/identity/promotion_attribute_read_test.go','retail/backend/internal/identity/promotion_usage_test.go']
                files += [p.relative_to(ROOT).as_posix() for p in (ROOT/'retail/backend/internal/promotion').glob('*') if p.is_file()]
                files += ['retail/backend/migrations/0028_tax_relational.sql','retail/scripts/generate-tax-schema.py','retail/backend/internal/identity/tax_schema_test.go']
                files += [p.relative_to(ROOT).as_posix() for p in (ROOT/'retail/backend/internal/tax').glob('*') if p.is_file()]
                files += ['retail/backend/internal/identity/catalog_schema_test.go','retail/backend/internal/schema/ready.go',
                          'retail/backend/internal/catalog/schema.go','retail/backend/internal/catalog/schema_plan.json',
                          'retail/backend/migrations/0010_catalog_relational.sql', 'retail/backend/migrations/0011_catalog_product_edits.sql',
                          'retail/backend/internal/catalog/edit.go','retail/backend/internal/catalog/edit_test.go',
                          'retail/backend/internal/identity/product_edit_test.go','retail/backend/internal/identity/product_prices_test.go',
                          'retail/backend/internal/identity/browser_fixture_test.go','retail/backend/internal/catalog/admin_prices.go',
                          'retail/backend/internal/catalog/admin_prices_test.go']
                files += [str(p.relative_to(ROOT)).replace('\\','/') for p in (ROOT/'retail/backend/internal/inventory').glob('*') if p.is_file()]
                files += ['retail/backend/migrations/0012_inventory_relational.sql','retail/backend/internal/identity/product_inventory_test.go','retail/scripts/generate-inventory-schema.py']
                files += [str(p.relative_to(ROOT)).replace('\\','/') for p in (ROOT/'retail/backend/internal/inventoryimport').glob('*') if p.is_file()]
                files += ['retail/backend/migrations/0013_inventory_import_evidence.sql','retail/backend/internal/identity/inventory_import_test.go','retail/scripts/generate-inventory-import-plan.py']
                files += ['retail/backend/migrations/0014_inventory_stock_edits.sql','retail/backend/internal/identity/stock_edit_test.go']
                files += ['retail/backend/migrations/0015_inventory_reservation_operations.sql','retail/backend/internal/identity/reservation_transactions_test.go']
                files += ['retail/backend/migrations/0016_order_relational.sql','retail/backend/internal/identity/order_schema_test.go','retail/backend/internal/orders/schema.go','retail/backend/internal/orders/schema_plan.json','retail/scripts/generate-order-schema.py']
                files += ['retail/backend/migrations/0017_order_import_evidence.sql','retail/backend/internal/identity/order_import_test.go','retail/scripts/generate-order-import-plan.py']
                files += [str(p.relative_to(ROOT)).replace('\\','/') for p in (ROOT/'retail/backend/internal/orderimport').glob('*') if p.is_file()]
                files += ['retail/backend/migrations/0018_customer_history_dispositions.sql','retail/backend/internal/customerimport/import.go','retail/backend/internal/rowimport/history_customer.go','retail/backend/internal/rowimport/history_customer_test.go','retail/backend/internal/identity/customer_history_test.go','retail/backend/internal/rowimport/convert.go']
                files += ['retail/backend/internal/orders/admin.go','retail/backend/internal/identity/order_read_test.go']
                files += ['retail/backend/internal/orders/amounts.go','retail/backend/internal/orders/amount_json.go','retail/backend/internal/identity/order_amounts_test.go']
                files += [str(p.relative_to(ROOT)).replace('\\','/') for p in (ROOT/'retail/backend/internal/totals').glob('*.go')]
                files += ['retail/backend/internal/identity/payment_schema_test.go','retail/backend/migrations/0019_payment_relational.sql','retail/scripts/generate-payment-schema.py','retail/backend/internal/payments/schema.go','retail/backend/internal/payments/schema_plan.json']
                files += ['retail/backend/internal/identity/payment_import_test.go','retail/backend/migrations/0020_payment_import_evidence.sql','retail/scripts/generate-payment-import-plan.py','retail/backend/internal/holderimport/import.go','retail/backend/internal/rowimport/history_payment_sql.go','retail/backend/internal/rowimport/history_account_holder.go']
                files += [str(p.relative_to(ROOT)).replace('\\','/') for p in (ROOT/'retail/backend/internal/paymentimport').glob('*') if p.is_file()]
                files += ['retail/backend/internal/orders/payments.go','retail/backend/internal/identity/order_payments_test.go']
                files += ['retail/backend/migrations/0021_fulfillment_relational.sql','retail/backend/migrations/0022_fulfillment_import_evidence.sql','retail/scripts/generate-fulfillment-schema.py','retail/scripts/generate-fulfillment-import-plan.py','retail/backend/internal/identity/fulfillment_history_test.go']
                files += [str(p.relative_to(ROOT)).replace('\\','/') for folder in ('fulfillment','fulfillmentimport') for p in (ROOT/'retail/backend/internal'/folder).glob('*') if p.is_file()]
                files += ['retail/backend/internal/orders/fulfillments.go','retail/backend/internal/identity/order_fulfillments_test.go']
                files += ['retail/backend/internal/identity/authorization_test.go','retail/backend/migrations/0023_authorization_actions.sql']
                files += [str(p.relative_to(ROOT)).replace('\\','/') for p in (ROOT/'retail/backend/internal/authorization').glob('*.go')]
                files += ['retail/backend/internal/identity/authorization_manage_test.go','retail/backend/migrations/0024_authorization_management.sql']
                files += ['retail/backend/internal/catalog/promotion_variants.go']
                files += ['retail/backend/internal/catalog/store_locales.go','retail/backend/internal/catalog/default_locales.json','retail/backend/internal/identity/store_locales_test.go']
                files += ['retail/backend/internal/catalog/store_regions.go','retail/backend/internal/catalog/http.go','retail/backend/internal/identity/store_regions_test.go']
                files += ['retail/backend/internal/catalog/store_taxonomy.go','retail/backend/internal/catalog/store_taxonomy_test.go','retail/backend/internal/identity/store_taxonomy_test.go']
                files += ['retail/backend/internal/catalog/store_policies.go','retail/backend/internal/catalog/store_policies_test.go','retail/backend/internal/identity/store_policies_test.go']
                files += ['retail/backend/internal/catalog/store_search.go','retail/backend/internal/catalog/store_search_colors.go','retail/backend/internal/catalog/store_search_test.go','retail/backend/internal/identity/store_search_test.go']
                files += ['retail/backend/internal/catalog/store_products.go','retail/backend/internal/catalog/store_products_test.go','retail/backend/internal/identity/store_products_test.go']
                files += ['retail/backend/internal/identity/store_pricing_test.go']
                shipping_stock_markers = [line.split('CART_SHIPPING_STOCK_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CART_SHIPPING_STOCK_REHEARSAL ' in line]
                if len(shipping_stock_markers) != 1:
                    raise RuntimeError('Cart shipping stock evidence missing')
                files += ['retail/backend/internal/identity/store_availability_test.go','retail/backend/internal/identity/cart_stock_test.go','retail/backend/internal/identity/cart_shipping_stock_test.go']
                files += ['retail/backend/internal/catalog/store_display.go','retail/backend/internal/catalog/store_display_test.go','retail/backend/internal/identity/store_display_test.go']
                files += [str(p.relative_to(ROOT)).replace('\\','/') for p in (ROOT/'retail/backend/internal/pricing').glob('*.go')]
                files += ['retail/scripts/generate-cart-schema.py','retail/backend/migrations/0025_cart_relational.sql','retail/backend/internal/identity/cart_schema_test.go']
                files += ['retail/backend/migrations/0026_cart_runtime_authority.sql','retail/backend/internal/identity/cart_runtime_test.go','retail/backend/internal/identity/cart_http_test.go','retail/backend/internal/identity/cart_amounts_test.go','retail/backend/internal/identity/promotion_products_test.go','retail/backend/internal/identity/cart_promotion_adjustments_test.go','retail/backend/internal/identity/cart_promotion_amounts_test.go','retail/backend/internal/identity/cart_items_test.go','retail/backend/internal/identity/cart_items_http_test.go']
                files += [str(p.relative_to(ROOT)).replace('\\','/') for p in (ROOT/'retail/backend/internal/cart').glob('*') if p.is_file()]
                files += ['retail/scripts/capture-cart-promotion-context.cjs','docs/migration/fixtures/cart-promotion-context.json']
                files += ['retail/scripts/capture-cart-payment-refresh.cjs','docs/migration/fixtures/cart-payment-refresh.json']
                files += ['retail/backend/internal/identity/cart_promotion_refresh_test.go','retail/backend/internal/identity/payment_effects_test.go','retail/backend/internal/payments/effects.go','retail/backend/internal/payments/requests.go','retail/backend/internal/identity/payment_requests_test.go','retail/backend/internal/identity/cart_payment_operations_test.go','retail/backend/internal/identity/cart_payment_execution_test.go','retail/backend/internal/identity/cart_payment_compensation_test.go','retail/backend/internal/identity/cart_payment_creation_execution_test.go']
                files += [str(p.relative_to(ROOT)).replace('\\','/') for p in (ROOT/'retail/backend/internal/sessionstore').glob('*.go')]
                http_markers = [line.split('CART_HTTP_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if 'CART_HTTP_REHEARSAL ' in line]
                if len(http_markers) != 1:
                    raise RuntimeError('Cart HTTP evidence missing')
                files += [p.relative_to(ROOT).as_posix() for p in (ROOT/'retail/backend/migrations').glob('*.sql')]
                files += [p.relative_to(ROOT).as_posix() for p in (ROOT/'retail/backend/internal/schema').glob('*.go')]
                files += [p.relative_to(ROOT).as_posix() for folder in ('payments','protected') for p in (ROOT/'retail/backend/internal'/folder).glob('*') if p.is_file()]
                files += ['retail/backend/migrations/0038_customer_accounts.sql', 'retail/backend/internal/customer/accounts.go', 'retail/backend/internal/customer/accounts_test.go', 'retail/backend/internal/identity/customer_accounts_test.go']
                report = {'format_version': 1, 'scope': 'New artificial admin with real Gin dispatch/MySQL/Redis; not customer/authz/UI acceptance',
                          'cart_schema':json.loads(cart_markers[0]),
                          'cart_amounts':json.loads(amount_markers[0]),'cart_payment_creation_execution':json.loads(payment_creation_execution_markers[0]),'cart_payment_compensation':json.loads(payment_compensation_markers[0]),'cart_payment_execution':json.loads(payment_execution_markers[0]),'cart_payment_operations':json.loads(payment_operation_markers[0]),'payment_requests':json.loads(payment_request_markers[0]),'payment_effects':json.loads(payment_effect_markers[0]),'cart_promotion_refresh':json.loads(promotion_refresh_markers[0]),'cart_promotion_amounts':json.loads(promotion_amount_markers[0]),'cart_promotion_adjustments':json.loads(promotion_adjustment_markers[0]),'promotion_products':json.loads(promotion_product_markers[0]),
                          'cart_items':json.loads(item_markers[0]),'cart_items_http':json.loads(item_http_markers[0]),
                          'cart_runtime':json.loads(cart_runtime_markers[0]),
                          'cart_http':json.loads(http_markers[0]),
                          'store_policies': json.loads(policy_markers[0]),
                          'store_search': json.loads(search_markers[0]),
                          'store_products': json.loads(product_markers[0]),
                          'store_pricing': json.loads(pricing_markers[0]),
                          'store_availability': json.loads(availability_markers[0]),
                          'cart_stock': json.loads(cart_stock_markers[0]),
                          'cart_shipping_stock': json.loads(shipping_stock_markers[0]),
                          'store_display': json.loads(display_markers[0]),
                          'store_taxonomy': json.loads(taxonomy_markers[0]),
                          'store_regions': json.loads(region_markers[0]),
                          'store_locales': json.loads(locale_markers[0]),
                          'counts': counts, 'limits': {'mysql_cpus': 1, 'mysql_memory_mib': 768, 'redis_cpus': 1, 'redis_memory_mib': 128,
                                                      'network': 'owned shared namespace with no external network', 'published_ports': 0},
                          'real_accounts_used': False, 'application_resources_used': False, 'private_values_in_report': False, 'temporary_databases_cleared_between_suites': True,
                          'full_authentication_accepted': False,
                          'catalog_schema_transition': json.loads(schema_markers[0]),
                          'product_edit_transaction': json.loads(edit_markers[0]),
                          'product_configured_prices': json.loads(price_markers[0]),
                          'product_inventory': json.loads(inventory_markers[0]),
                          'inventory_import': json.loads(import_markers[0]),
                          'inventory_stock_edit': json.loads(stock_markers[0]),
                          'reservation_transactions': json.loads(reservation_markers[0]),
                          'authorization_management': json.loads(manage_markers[0]),
                          'authorization': json.loads(authorization_markers[0]),
                          'fulfillment_history': json.loads(fulfillment_markers[0]),
                          'payment_import': json.loads(payment_import_markers[0]),
                          'payment_schema': json.loads(payment_markers[0]),
                          'order_schema': json.loads(order_markers[0]),
                          'order_import': json.loads(order_import_markers[0]),
                          'customer_history': json.loads(customer_markers[0]),
                          'new_customer_accounts': json.loads(customer_account_markers[0]),
                          'order_fulfillments': json.loads(fulfillment_read_markers[0]),
                          'order_payments': json.loads(payment_read_markers[0]),
                          'order_read': json.loads(order_read_markers[0]),
                          'tax_schema_and_read': json.loads(tax_markers[0]),
                          'tax_import': json.loads(tax_import_markers[0]),
                          'cart_tax_refresh': json.loads(tax_refresh_markers[0]),
                          'cart_price_refresh': json.loads(price_refresh_markers[0]),
                          'cart_item_snapshots': json.loads(snapshot_markers[0]),
                          'flat_shipping_prices': json.loads(flat_markers[0]),
                          'promotion_usage': json.loads(usage_markers[0]),
                          'promotion_attribute_read': json.loads(attribute_read_markers[0]),
                          'promotion_read': json.loads(read_markers[0]),
                          'promotion_schema': json.loads(promotion_markers[0]),
                          'geo_schema': json.loads(geo_markers[0]),
                          'shipping_candidates': json.loads(candidate_markers[0]),
                          'manual_shipping_quotes': json.loads(manual_markers[0]),
                          'cart_shipping_profiles': json.loads(profile_markers[0]),
                          'cart_shipping_quotes': json.loads(quote_markers[0]),
                          'cart_shipping_selection': json.loads(selection_markers[0]),
                          'cart_shipping_methods': json.loads(method_markers[0]),
                          'cart_shipping_refresh': json.loads(refresh_markers[0]),
                          'implementation_sha256': {name: hashlib.sha256((ROOT / name).read_bytes().replace(b'\r\n', b'\n')).hexdigest() for name in files}}
    finally:
        password = ''
        for container in reversed(containers):
            inspected = docker('inspect', container, '--format', '{{index .Config.Labels "' + LABEL + '"}}')
            if inspected.returncode or inspected.stdout.decode().strip() != owner:
                raise RuntimeError('Identity fixture ownership mismatch; cleanup refused')
            if docker('rm', '-f', '-v', container).returncode:
                raise RuntimeError('Owned identity fixture cleanup failed')
        if owned_binary and binary.exists():
            binary.unlink()
    report['cleanup'] = 'both owned containers and temporary runner removed'
    report_name = 'cart-order-snapshot-runtime-report.json' if snapshot_only else 'order-numbering-runtime-report.json' if numbering_only else 'cart-payment-status-runtime-report.json' if status_only else 'cart-initial-payment-runtime-report.json' if initial_only else 'cart-delivery-runtime-report.json' if delivery_only else 'cart-contacts-runtime-report.json' if contacts_only else 'store-collections-runtime-report.json' if collection_only else 'gin-customer-accounts-report.json' if customer_only else 'promotion-usage-report.json' if usage_only else 'cart-runtime-report.json' if cart_only else 'admin-auth-report.json'
    (ROOT / 'docs/migration' / report_name).write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'counts': counts, 'cleanup': 'complete'}))


if __name__ == '__main__':
    main()
