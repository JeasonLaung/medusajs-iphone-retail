"""New artificial admin over real Gin dispatch, owned MySQL and Redis only."""
import hashlib
import json
import os
import re
from pathlib import Path
import secrets
import subprocess
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
                       '--performance-schema=OFF', '--mysqlx=OFF', '--max-connections=16')
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
        # Each suite owns artificial databases in this uniquely labelled,
        # disconnected server. Release their tmpfs tables before the next suite;
        # otherwise cumulative DDL consumes the bounded fixture memory.
        suites = ['AdminAuthentication','CatalogSchemaTransition','ProductEdits','ProductPrices',
                  'ProductInventory','InventoryStockEdits','ReservationTransactions','OrderSchema',
                  'OrderImport','CustomerHistory','OrderRead','PaymentSchema']
        output, errors = b'', b''
        for suite in suites:
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
        markers = [line.split('IDENTITY_REHEARSAL ', 1)[1] for line in tested.stdout.decode().splitlines() if 'IDENTITY_REHEARSAL ' in line]
        if tested.returncode or len(markers) != 1:
            # All inputs and failure messages in this binary are artificial.
            print(tested.stdout.decode(errors='replace'))
            state = docker('inspect', mysql_id, '--format', '{{.State.OOMKilled}} {{.State.ExitCode}}')
            print(json.dumps({'test_exit_code': tested.returncode, 'mysql_oom_and_exit': state.stdout.decode().strip()}))
            raise RuntimeError('Owned MySQL/Redis authentication checks failed')
        counts = json.loads(markers[0])
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
        report = {'format_version': 1, 'scope': 'New artificial admin with real Gin dispatch/MySQL/Redis; not customer/authz/UI acceptance',
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
                  'payment_schema': json.loads(payment_markers[0]),
                  'order_schema': json.loads(order_markers[0]),
                  'order_import': json.loads(order_import_markers[0]),
                  'customer_history': json.loads(customer_markers[0]),
                  'order_read': json.loads(order_read_markers[0]),
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
    (ROOT / 'docs/migration/admin-auth-report.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'counts': counts, 'cleanup': 'complete'}))


if __name__ == '__main__':
    main()
