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
LABEL = 'eurorefurb.warehouse-browser-rehearsal'


def docker(*args, timeout=90):
    return subprocess.run(['docker', *args], capture_output=True, timeout=timeout)


def main():
    if shutil.disk_usage(ROOT).free < 2 * (1 << 30):
        raise RuntimeError('At least 2 GiB free workspace disk required to preserve development services')
    for image in ['mysql:8.4', 'redis:7-alpine']:
        if docker('image', 'inspect', image).returncode:
            raise RuntimeError('Existing fixture images required; no pull performed')
    order_packing = sys.argv[1:] == ['--order-packing']
    order_allocation = order_packing or sys.argv[1:] == ['--order-allocation']
    level_create = sys.argv[1:] == ['--level-create']
    variant_create = sys.argv[1:] == ['--variant-create']
    price_create = sys.argv[1:] == ['--price-create']
    price_edit = sys.argv[1:] == ['--price-edit']
    warehouse_intake = sys.argv[1:] == ['--warehouse-intake']
    warehouse_devices = sys.argv[1:] == ['--warehouse-devices']
    device_tracking = sys.argv[1:] == ['--device-tracking']
    device_sources = sys.argv[1:] == ['--device-sources']
    device_intake = sys.argv[1:] == ['--device-intake']
    if sys.argv[1:] and not (order_allocation or level_create or variant_create or price_create or price_edit or device_tracking or device_sources or device_intake or warehouse_devices or warehouse_intake):
        raise RuntimeError('Unknown joint rehearsal option')
    owner = uuid.uuid4().hex
    binaries = [ROOT / 'logs/gin-migration' / ('browser-' + owner + suffix) for suffix in ['-api', '-prep.test']]
    for binary in binaries:
        if binary.exists() or subprocess.run(['git', 'check-ignore', '--quiet', str(binary)], cwd=ROOT, capture_output=True).returncode:
            raise RuntimeError('New ignored runner paths required')
    env = os.environ.copy()
    env.update(GOOS='linux', GOARCH='amd64', CGO_ENABLED='0')
    containers, network, owned_binaries = [], None, []
    password = secrets.token_urlsafe(32)
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
                          '/opt/prep.test','-test.v','-test.run','^TestPrepareWarehouseBrowserFixture$','-test.timeout','40s')
        if prepared.returncode or b'OWNED_WAREHOUSE_BROWSER_READY' not in prepared.stdout:
            raise RuntimeError('Reviewed artificial catalog preparation failed')
        if device_tracking or device_intake:
            prepared = docker('exec','-e','MYSQL_PWD=' + password,'-e','BROWSER_IDENTITY_FIXTURE=1',mysql_id,
                              '/opt/prep.test','-test.v','-test.run','^TestPrepareDeviceTrackingBrowserFixture$','-test.timeout','40s')
            if prepared.returncode or b'OWNED_DEVICE_BROWSER_READY' not in prepared.stdout:
                raise RuntimeError('Device tracking browser fixture failed')
        if device_sources or device_intake:
            prepared = docker('exec','-e','MYSQL_PWD=' + password,'-e','BROWSER_IDENTITY_FIXTURE=1',mysql_id,
                              '/opt/prep.test','-test.v','-test.run','^TestPrepareDeviceSourcesBrowserFixture$','-test.timeout','40s')
            if prepared.returncode or b'OWNED_DEVICE_SOURCES_BROWSER_READY' not in prepared.stdout:
                raise RuntimeError('Device sources browser fixture failed')
        if warehouse_devices or warehouse_intake:
            prepared=docker('exec','-e','MYSQL_PWD='+password,'-e','BROWSER_IDENTITY_FIXTURE=1',mysql_id,'/opt/prep.test','-test.v','-test.run','^TestPrepareWarehouseDevicesBrowserFixture$','-test.timeout','40s')
            if prepared.returncode or b'OWNED_WAREHOUSE_DEVICES_READY' not in prepared.stdout: raise RuntimeError('Warehouse devices fixture failed')
        if price_edit or price_create or variant_create or level_create or order_allocation:
            prepared=docker('exec','-e','MYSQL_PWD='+password,'-e','BROWSER_IDENTITY_FIXTURE=1','-e','ORDER_PACKING_BROWSER='+('1' if order_packing else '0'),'-e','ORDER_ALLOCATION_BROWSER='+('1' if order_allocation else '0'),'-e','LEVEL_CREATE_BROWSER='+('1' if level_create else '0'),'-e','VARIANT_CREATE_BROWSER='+('1' if (variant_create or level_create) else '0'),'-e','PRICE_CREATE_BROWSER='+('1' if price_create else '0'),mysql_id,'/opt/prep.test','-test.v','-test.run','^TestPreparePriceEditBrowserFixture$','-test.timeout','40s')
            if prepared.returncode or b'OWNED_PRICE_EDIT_READY' not in prepared.stdout: raise RuntimeError('Price edit fixture failed')
        app_env = {'STOREFRONT_STORE_ID':'price-edit-store' if (price_edit or price_create or variant_create or level_create or order_allocation) else 'fixture-store','GIN_MODE': 'release', 'APP_BIND': '0.0.0.0', 'APP_PORT': '9100', 'APP_ENV': 'development',
                   'MYSQL_DSN': 'root:' + password + '@unix(/var/run/mysqld/mysqld.sock)/browser_identity?parseTime=true&loc=UTC',
                   'SESSION_REDIS_URL': 'redis://127.0.0.1:6379/0', 'SESSION_REDIS_PREFIX': 'eurorefurb:browser_fixture:v1:',
                   'ADMIN_ORIGIN': 'http://127.0.0.1:19383', 'STOREFRONT_ORIGIN': 'http://127.0.0.1:19383', 'SESSION_SECURE': 'false'}
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
        allocation_stock_before = None
        if order_allocation:
            allocation_stock_before = docker('exec','-e','MYSQL_PWD='+password,mysql_id,'mysql','-uroot','--protocol=socket','--database=browser_identity','--batch','--skip-column-names','-e','SELECT * FROM inventory_levels ORDER BY id; SELECT * FROM inventory_reservations ORDER BY id')
            if allocation_stock_before.returncode: raise RuntimeError('Allocation stock baseline failed')
        browser_env = os.environ.copy()
        browser_env['ADMIN_TEST_API_TARGET'] = target
        node = browser_env.get('ADMIN_TEST_NODE') or shutil.which('node')
        tested = subprocess.run([node,'tests/order-packing.joint.mjs' if order_packing else 'tests/order-allocation.joint.mjs' if order_allocation else 'tests/level-create.joint.mjs' if level_create else 'tests/variant-create.joint.mjs' if variant_create else 'tests/price-create.joint.mjs' if price_create else 'tests/price-edit.joint.mjs' if price_edit else 'tests/warehouse-intake.joint.mjs' if warehouse_intake else 'tests/warehouse-devices.joint.mjs' if warehouse_devices else 'tests/device-intake.joint.mjs' if device_intake else 'tests/device-sources.joint.mjs' if device_sources else 'tests/device-tracking.joint.mjs' if device_tracking else 'tests/warehouse-inventory.joint.mjs'],cwd=ROOT/'retail/admin',env=browser_env,capture_output=True,timeout=180)
        marker = 'ORDER_PACKING_JOINT ' if order_packing else 'ORDER_ALLOCATION_JOINT ' if order_allocation else 'LEVEL_CREATE_JOINT ' if level_create else 'VARIANT_CREATE_JOINT ' if variant_create else 'PRICE_CREATE_JOINT ' if price_create else 'PRICE_EDIT_JOINT ' if price_edit else 'WAREHOUSE_INTAKE_JOINT ' if warehouse_intake else 'WAREHOUSE_DEVICES_JOINT ' if warehouse_devices else 'DEVICE_INTAKE_JOINT ' if device_intake else 'DEVICE_SOURCES_JOINT ' if device_sources else 'DEVICE_TRACKING_JOINT ' if device_tracking else 'WAREHOUSE_INVENTORY_JOINT '
        markers = [line.split(marker,1)[1] for line in tested.stdout.decode('utf-8').splitlines() if line.startswith(marker)]
        if tested.returncode or len(markers)!=1:
            (ROOT/('logs/gin-migration/order-packing-joint.log' if order_packing else 'logs/gin-migration/order-allocation-joint.log' if order_allocation else 'logs/gin-migration/level-create-joint.log' if level_create else 'logs/gin-migration/variant-create-joint.log' if variant_create else 'logs/gin-migration/price-create-joint.log' if price_create else 'logs/gin-migration/price-edit-joint.log' if price_edit else 'logs/gin-migration/warehouse-intake-joint.log' if warehouse_intake else 'logs/gin-migration/warehouse-devices-joint.log' if warehouse_devices else 'logs/gin-migration/device-intake-joint.log' if device_intake else 'logs/gin-migration/device-sources-joint.log' if device_sources else 'logs/gin-migration/device-tracking-joint.log' if device_tracking else 'logs/gin-migration/warehouse-inventory-joint.log')).write_bytes(tested.stdout+tested.stderr)
            raise RuntimeError('Warehouse browser joint failed; inspect ignored log')
        if order_packing:
            durable=docker('exec','-e','MYSQL_PWD='+password,mysql_id,'mysql','-uroot','--protocol=socket','--database=browser_identity','--batch','--skip-column-names','-e',"SELECT (SELECT COUNT(*) FROM order_device_packings),l.stocked_quantity,JSON_UNQUOTE(JSON_EXTRACT(l.raw_stocked_quantity,'$.value')),(SELECT reserved_quantity FROM inventory_levels WHERE id='packing-original'),(SELECT quantity FROM inventory_reservations WHERE id='packing-reservation' AND deleted_at IS NULL),(SELECT fulfilled_quantity FROM order_items WHERE id='allocation-order-item'),(SELECT COUNT(*) FROM devices WHERE state='packed'),(SELECT COUNT(*) FROM device_assignments WHERE status='packed'),(SELECT barcode FROM fulfillment_items WHERE line_item_id='allocation-line' AND deleted_at IS NULL LIMIT 1) FROM inventory_levels l WHERE l.id='packing-target'")
            if durable.returncode or durable.stdout.decode().strip()!='1\t4.000000000000000001\t4.000000000000000001\t1.000000000000000000\t1.000000000000000000\t1.000000000000000000\t1\t1\tPACK-BC': raise RuntimeError('Durable packing differs: '+durable.stdout.decode().strip())
            report={'scope':'Employee manual-provider device parcel, exact warehouse stock and reservation remainder; shipment, cancellation and warehouse employee fulfillment remain incomplete','counts':json.loads(markers[0]),'durable_mysql_verified':True,'real_accounts_used':False,'tls_secure_cookie_verified':False,'limits':{'mysql_cpus':1,'mysql_memory_mib':768,'redis_cpus':1,'redis_memory_mib':128,'published_ports':'one random loopback API port','network':'owned bridge; egress unrestricted'}}
        elif order_allocation:
            durable=docker('exec','-e','MYSQL_PWD='+password,mysql_id,'mysql','-uroot','--protocol=socket','--database=browser_identity','--batch','--skip-column-names','-e',"SELECT (SELECT COUNT(*) FROM order_device_allocations),(SELECT COUNT(*) FROM device_assignments WHERE status='reserved'),(SELECT COUNT(*) FROM device_assignments WHERE status='canceled'),(SELECT COUNT(*) FROM devices WHERE state='reserved'),(SELECT COUNT(*) FROM device_events WHERE action='assign'),(SELECT COUNT(*) FROM device_events WHERE action='release')")
            if durable.returncode or durable.stdout.decode().strip()!='2\t1\t1\t1\t2\t1':raise RuntimeError('Durable order allocation differs: '+durable.stdout.decode().strip())
            allocation_stock_after = docker('exec','-e','MYSQL_PWD='+password,mysql_id,'mysql','-uroot','--protocol=socket','--database=browser_identity','--batch','--skip-column-names','-e','SELECT * FROM inventory_levels ORDER BY id; SELECT * FROM inventory_reservations ORDER BY id')
            if allocation_stock_after.returncode or allocation_stock_after.stdout != allocation_stock_before.stdout: raise RuntimeError('Allocation changed quantity inventory or reservations')
            report={'scope':'Employee order device reservation and release; packing, shipping and warehouse employee fulfillment remain incomplete','counts':json.loads(markers[0]),'durable_mysql_verified':True,'real_accounts_used':False,'tls_secure_cookie_verified':False,'limits':{'mysql_cpus':1,'mysql_memory_mib':768,'redis_cpus':1,'redis_memory_mib':128,'published_ports':'one random loopback API port','network':'owned bridge; egress unrestricted'}}
        elif level_create:
            durable=docker('exec','-e','MYSQL_PWD='+password,mysql_id,'mysql','-uroot','--protocol=socket','--database=browser_identity','--batch','--skip-column-names','-e',"SELECT (SELECT COUNT(*) FROM inventory_level_creations),l.stocked_quantity,l.reserved_quantity,l.incoming_quantity,JSON_UNQUOTE(JSON_EXTRACT(l.raw_stocked_quantity,'$.value')),(SELECT COUNT(*) FROM inventory_stock_edits),(SELECT COUNT(*) FROM devices),(SELECT COUNT(*) FROM inventory_reservations) FROM inventory_levels l JOIN catalog_variant_inventory_items vi ON vi.inventory_item_id=l.inventory_item_id JOIN catalog_variants v ON v.id=vi.variant_id WHERE v.product_id='variant-create-product' AND l.deleted_at IS NULL")
            if durable.returncode or durable.stdout.decode().strip()!='1\t7.000000000000000001\t0.000000000000000000\t3.000000000000000000\t7.000000000000000001\t1\t0\t0':raise RuntimeError('Durable new SKU stock differs')
            report={'scope':'New SKU, exact initial warehouse stock, public availability and existing stock edit; full checkout and device fulfillment remain incomplete','counts':json.loads(markers[0]),'durable_mysql_verified':True,'real_accounts_used':False,'tls_secure_cookie_verified':False,'limits':{'mysql_cpus':1,'mysql_memory_mib':768,'redis_cpus':1,'redis_memory_mib':128,'published_ports':'one random loopback API port','network':'owned bridge; egress unrestricted'}}
        elif variant_create:
            durable=docker('exec','-e','MYSQL_PWD='+password,mysql_id,'mysql','-uroot','--protocol=socket','--database=browser_identity','--batch','--skip-column-names','-e',"SELECT (SELECT COUNT(*) FROM catalog_variant_creations),v.sku,i.sku,v.manage_inventory,v.allow_backorder,vi.required_quantity,(SELECT COUNT(*) FROM catalog_variant_options WHERE variant_id=v.id),(SELECT COUNT(*) FROM inventory_levels WHERE inventory_item_id=i.id),(SELECT COUNT(*) FROM catalog_price_edits) FROM catalog_variants v JOIN catalog_variant_inventory_items vi ON vi.variant_id=v.id AND vi.deleted_at IS NULL JOIN inventory_items i ON i.id=vi.inventory_item_id AND i.deleted_at IS NULL WHERE v.product_id='variant-create-product' AND v.deleted_at IS NULL")
            parts=durable.stdout.decode().strip().split('\t')
            if durable.returncode or len(parts)!=9 or parts[0]!='1' or not re.fullmatch(r'ERP[1-9][0-9]{4,}',parts[1]) or parts[1]!=parts[2] or parts[3:]!=['1','0','1.000000000000000000','2','0','1']:raise RuntimeError('Durable SKU creation differs')
            report={'scope':'SKU creation from existing product options and independent inventory, followed by base pricing and public product read; full shopping and fulfillment remain incomplete','counts':json.loads(markers[0]),'durable_mysql_verified':True,'real_accounts_used':False,'tls_secure_cookie_verified':False,'limits':{'mysql_cpus':1,'mysql_memory_mib':768,'redis_cpus':1,'redis_memory_mib':128,'published_ports':'one random loopback API port','network':'owned bridge; egress unrestricted'}}
        elif price_create:
            durable=docker('exec','-e','MYSQL_PWD='+password,mysql_id,'mysql','-uroot','--protocol=socket','--database=browser_identity','--batch','--skip-column-names','-e',"SELECT p.amount,JSON_UNQUOTE(JSON_EXTRACT(p.raw_amount,'$.value')),(SELECT COUNT(*) FROM catalog_price_edits),(SELECT COUNT(*) FROM catalog_variant_price_sets WHERE variant_id='price-create-v' AND deleted_at IS NULL) FROM pricing_prices p JOIN catalog_variant_price_sets vp ON vp.price_set_id=p.price_set_id AND vp.deleted_at IS NULL WHERE vp.variant_id='price-create-v' AND p.deleted_at IS NULL")
            if durable.returncode or durable.stdout.decode().strip()!='99.123456789012345678\t99.123456789012345678\t1\t1': raise RuntimeError('Durable new base price differs')
            report={'scope':'Base price creation on unpriced SKU and public quote propagation; product/SKU creation and complete storefront remain incomplete','counts':json.loads(markers[0]),'durable_mysql_verified':True,'real_accounts_used':False,'tls_secure_cookie_verified':False,'limits':{'mysql_cpus':1,'mysql_memory_mib':768,'redis_cpus':1,'redis_memory_mib':128,'published_ports':'one random loopback API port','network':'owned bridge; egress unrestricted'}}
        elif price_edit:
            durable=docker('exec','-e','MYSQL_PWD='+password,mysql_id,'mysql','-uroot','--protocol=socket','--database=browser_identity','--batch','--skip-column-names','-e',"SELECT amount,JSON_UNQUOTE(JSON_EXTRACT(raw_amount,'$.value')),(SELECT COUNT(*) FROM catalog_price_edits),(SELECT COUNT(*) FROM catalog_variant_price_sets WHERE price_set_id='price-edit-set' AND deleted_at IS NULL) FROM pricing_prices WHERE id='price-edit-base'")
            if durable.returncode or durable.stdout.decode().strip()!='101.000000000000000001\t101.000000000000000001\t1\t2': raise RuntimeError('Durable price amount differs')
            report={'scope':'Existing configured price amount edits, shared SKU public quote propagation; price creation and full storefront acceptance incomplete','counts':json.loads(markers[0]),'durable_mysql_verified':True,'real_accounts_used':False,'tls_secure_cookie_verified':False,'limits':{'mysql_cpus':1,'mysql_memory_mib':768,'redis_cpus':1,'redis_memory_mib':128,'published_ports':'one random loopback API port','network':'owned bridge; egress unrestricted'}}
        elif warehouse_intake:
            durable=docker('exec','-e','MYSQL_PWD='+password,mysql_id,'mysql','-uroot','--protocol=socket','--database=browser_identity','--batch','--skip-column-names','-e',"SELECT (SELECT COUNT(*) FROM devices WHERE serial_number IN ('WORKER-NEW-SALE','WORKER-NEW-RETURN') AND location_id='joint-warehouse-a'),(SELECT state FROM devices WHERE serial_number='WORKER-NEW-SALE'),(SELECT state FROM devices WHERE serial_number='WORKER-NEW-RETURN'),(SELECT COUNT(*) FROM device_intake_changes WHERE context_key<>''),(SELECT COUNT(*) FROM device_photos WHERE context_key<>''),(SELECT COUNT(*) FROM device_events WHERE action='intake'),(SELECT stocked_quantity FROM inventory_levels WHERE id='joint-warehouse-level'),(SELECT COUNT(*) FROM inventory_stock_edits)")
            if durable.returncode or durable.stdout.decode().strip()!='2\tavailable\tinspection\t2\t1\t2\t100.000000000000000001\t0': raise RuntimeError('Durable warehouse intake differs')
            report={'scope':'Warehouse employee scoped sale/returned intake with photos; editing, inspection and fulfillment incomplete','counts':json.loads(markers[0]),'durable_mysql_verified':True,'real_accounts_used':False,'tls_secure_cookie_verified':False,'fulfillment_accepted':False,'limits':{'mysql_cpus':1,'mysql_memory_mib':768,'redis_cpus':1,'redis_memory_mib':128,'published_ports':'one random loopback API port','network':'owned bridge; egress unrestricted'}}
        elif warehouse_devices:
            durable=docker('exec','-e','MYSQL_PWD='+password,mysql_id,'mysql','-uroot','--protocol=socket','--database=browser_identity','--batch','--skip-column-names','-e',"SELECT (SELECT CONCAT(category,':',state) FROM devices WHERE id='joint-device-1'),(SELECT state FROM devices WHERE id='joint-device-2'),(SELECT deleted_at IS NOT NULL FROM devices WHERE id='joint-device-3'),(SELECT COUNT(*) FROM warehouse_device_changes),(SELECT COUNT(*) FROM device_events WHERE action IN ('activate','make_available','retire','delete','transfer_return')),(SELECT stocked_quantity FROM inventory_levels WHERE id='joint-warehouse-level'),(SELECT COUNT(*) FROM inventory_stock_edits)")
            if durable.returncode or durable.stdout.decode().strip()!='returned:inspection\tretired\t1\t5\t5\t100.000000000000000001\t0': raise RuntimeError('Durable warehouse devices differs')
            report={'scope':'Scoped sale/returned warehouse devices, lifecycle and audited transfer to returned; inspection and fulfillment incomplete','counts':json.loads(markers[0]),'durable_mysql_verified':True,'real_accounts_used':False,'tls_secure_cookie_verified':False,'warehouse_intake_accepted':False,'fulfillment_accepted':False,'limits':{'mysql_cpus':1,'mysql_memory_mib':768,'redis_cpus':1,'redis_memory_mib':128,'published_ports':'one random loopback API port','network':'owned bridge; egress unrestricted'}}
        elif device_intake:
            durable = docker('exec','-e','MYSQL_PWD=' + password,mysql_id,'mysql','-uroot','--protocol=socket','--database=browser_identity','--batch','--skip-column-names','-e',
                             "SELECT (SELECT COUNT(*) FROM devices WHERE variant_id='joint-device-variant' AND deleted_at IS NULL),(SELECT COUNT(*) FROM device_events WHERE action='intake'),(SELECT COUNT(*) FROM device_intake_changes),(SELECT COUNT(*) FROM device_identity_claims),(SELECT stocked_quantity FROM inventory_levels WHERE id='joint-warehouse-level'),(SELECT COUNT(*) FROM inventory_stock_edits),(SELECT COUNT(*) FROM device_sources WHERE type='self'),(SELECT COUNT(*) FROM devices d JOIN device_sources s ON s.id=d.source_id WHERE d.location_id=s.location_id AND d.variant_id='joint-device-variant'),(SELECT COUNT(*) FROM device_photos),(SELECT SUM(JSON_LENGTH(photos)) FROM devices WHERE variant_id='joint-device-variant')")
            if durable.returncode or durable.stdout.decode().strip()!='2\t2\t1\t4\t100.000000000000000001\t0\t1\t2\t1\t1':
                raise RuntimeError('Durable device intake differs')
            report={'scope':'Real browser creates own source, enrolls SKU and uploads image and registers sale/returned devices with photos surviving reload; fulfillment incomplete',
                    'counts':json.loads(markers[0]),'durable_mysql_verified':True,'real_accounts_used':False,
                    'limits':{'mysql_cpus':1,'mysql_memory_mib':768,'redis_cpus':1,'redis_memory_mib':128,'network':'owned bridge; egress unrestricted','published_ports':'one random API port on 127.0.0.1 only'},
                    'tls_secure_cookie_verified':False,'photo_upload_accepted':True,'fulfillment_accepted':False}
        elif device_sources:
            durable = docker('exec','-e','MYSQL_PWD=' + password,mysql_id,'mysql','-uroot','--protocol=socket','--database=browser_identity','--batch','--skip-column-names','-e',
                             "SELECT (SELECT COUNT(*) FROM device_sources WHERE deleted_at IS NULL),(SELECT COUNT(*) FROM device_source_changes),(SELECT COUNT(*) FROM commerce_channel_locations WHERE sales_channel_id='source-joint-channel' AND deleted_at IS NULL),(SELECT COUNT(*) FROM fulfillment_location_providers WHERE fulfillment_provider_id='manual_manual' AND deleted_at IS NULL),(SELECT stocked_quantity FROM inventory_levels WHERE id='joint-warehouse-level'),(SELECT COUNT(*) FROM inventory_stock_edits)")
            if durable.returncode or durable.stdout.decode().strip()!='2\t2\t2\t2\t100.000000000000000001\t0':
                raise RuntimeError('Durable source creation differs')
            report={'scope':'Real browser creates self and supplier device sources, linked warehouses, channel and provider; no intake acceptance',
                    'counts':json.loads(markers[0]),'durable_mysql_verified':True,'real_accounts_used':False,
                    'limits':{'mysql_cpus':1,'mysql_memory_mib':768,'redis_cpus':1,'redis_memory_mib':128,'network':'owned bridge; egress unrestricted','published_ports':'one random API port on 127.0.0.1 only'},
                    'tls_secure_cookie_verified':False,'intake_accepted':False}
        elif device_tracking:
            durable = docker('exec','-e','MYSQL_PWD=' + password,mysql_id,'mysql','-uroot','--protocol=socket','--database=browser_identity','--batch','--skip-column-names','-e',
                             "SELECT (SELECT COUNT(*) FROM device_variant_tracking WHERE variant_id='joint-device-variant' AND deleted_at IS NULL),(SELECT COUNT(*) FROM device_events WHERE action='enroll'),(SELECT COUNT(*) FROM device_tracking_changes),(SELECT stocked_quantity FROM inventory_levels WHERE id='joint-warehouse-level'),(SELECT COUNT(*) FROM inventory_stock_edits)")
            if durable.returncode or durable.stdout.decode().strip()!='1\t1\t1\t100.000000000000000001\t0':
                raise RuntimeError('Durable device enrollment differs')
            report={'scope':'Real browser enables SKU device tracking and reloads durable state; no intake or fulfillment acceptance',
                    'counts':json.loads(markers[0]),'durable_mysql_verified':True,'real_accounts_used':False,
                    'limits':{'mysql_cpus':1,'mysql_memory_mib':768,'redis_cpus':1,'redis_memory_mib':128,'network':'owned bridge; egress unrestricted','published_ports':'one random API port on 127.0.0.1 only'},
                    'tls_secure_cookie_verified':False,'intake_accepted':False}
        else:
            durable = docker('exec','-e','MYSQL_PWD=' + password,mysql_id,'mysql','-uroot','--protocol=socket','--database=browser_identity','--batch','--skip-column-names','-e',
                             "SELECT (SELECT stocked_quantity FROM inventory_levels WHERE id='joint-warehouse-level'),(SELECT stocked_quantity FROM inventory_levels WHERE id='joint-other-level'),(SELECT COUNT(*) FROM inventory_stock_edits),(SELECT COUNT(*) FROM warehouse_user_accounts),(SELECT COUNT(*) FROM warehouse_user_changes),(SELECT enabled FROM admin_users WHERE email='warehouse-joint@example.invalid')")
            if durable.returncode or durable.stdout.decode().strip()!='99.000000000000000001\t999.000000000000000000\t1\t1\t2\t0':
                raise RuntimeError('Durable warehouse joint state differs')
            report={'scope':'Warehouse account create, scoped inventory edit and account disable with real browser and Gin',
                    'counts':json.loads(markers[0]),'durable_mysql_verified':True,'real_accounts_used':False,
                    'limits':{'mysql_cpus':1,'mysql_memory_mib':768,'redis_cpus':1,'redis_memory_mib':128,'network':'owned bridge; egress unrestricted','published_ports':'one random API port on 127.0.0.1 only'},
                    'tls_secure_cookie_verified':False,'full_warehouse_fulfillment_accepted':False}
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
    report['cleanup']='both owned containers, bridge and temporary runners removed'
    (ROOT/('docs/migration/order-packing-joint-report.json' if order_packing else 'docs/migration/order-allocation-joint-report.json' if order_allocation else 'docs/migration/level-create-joint-report.json' if level_create else 'docs/migration/variant-create-joint-report.json' if variant_create else 'docs/migration/price-create-joint-report.json' if price_create else 'docs/migration/price-edit-joint-report.json' if price_edit else 'docs/migration/warehouse-intake-joint-report.json' if warehouse_intake else 'docs/migration/warehouse-devices-joint-report.json' if warehouse_devices else 'docs/migration/device-intake-joint-report.json' if device_intake else 'docs/migration/device-sources-joint-report.json' if device_sources else 'docs/migration/device-tracking-joint-report.json' if device_tracking else 'docs/migration/warehouse-inventory-joint-report.json')).write_text(json.dumps(report,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({'real_browser_joint':True,'durable_mysql_verified':True,'cleanup':'complete'}))

if __name__=='__main__':
    main()
