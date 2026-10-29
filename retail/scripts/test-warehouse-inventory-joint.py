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
    device_tracking = sys.argv[1:] == ['--device-tracking']
    device_sources = sys.argv[1:] == ['--device-sources']
    device_intake = sys.argv[1:] == ['--device-intake']
    if sys.argv[1:] and not (device_tracking or device_sources or device_intake):
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
        app_env = {'STOREFRONT_STORE_ID':'fixture-store','GIN_MODE': 'release', 'APP_BIND': '0.0.0.0', 'APP_PORT': '9100', 'APP_ENV': 'development',
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
        browser_env = os.environ.copy()
        browser_env['ADMIN_TEST_API_TARGET'] = target
        node = browser_env.get('ADMIN_TEST_NODE') or shutil.which('node')
        tested = subprocess.run([node,'tests/device-intake.joint.mjs' if device_intake else 'tests/device-sources.joint.mjs' if device_sources else 'tests/device-tracking.joint.mjs' if device_tracking else 'tests/warehouse-inventory.joint.mjs'],cwd=ROOT/'retail/admin',env=browser_env,capture_output=True,timeout=180)
        marker = 'DEVICE_INTAKE_JOINT ' if device_intake else 'DEVICE_SOURCES_JOINT ' if device_sources else 'DEVICE_TRACKING_JOINT ' if device_tracking else 'WAREHOUSE_INVENTORY_JOINT '
        markers = [line.split(marker,1)[1] for line in tested.stdout.decode('utf-8').splitlines() if line.startswith(marker)]
        if tested.returncode or len(markers)!=1:
            (ROOT/('logs/gin-migration/device-intake-joint.log' if device_intake else 'logs/gin-migration/device-sources-joint.log' if device_sources else 'logs/gin-migration/device-tracking-joint.log' if device_tracking else 'logs/gin-migration/warehouse-inventory-joint.log')).write_bytes(tested.stdout+tested.stderr)
            raise RuntimeError('Warehouse browser joint failed; inspect ignored log')
        if device_intake:
            durable = docker('exec','-e','MYSQL_PWD=' + password,mysql_id,'mysql','-uroot','--protocol=socket','--database=browser_identity','--batch','--skip-column-names','-e',
                             "SELECT (SELECT COUNT(*) FROM devices WHERE variant_id='joint-device-variant' AND deleted_at IS NULL),(SELECT COUNT(*) FROM device_events WHERE action='intake'),(SELECT COUNT(*) FROM device_intake_changes),(SELECT COUNT(*) FROM device_identity_claims),(SELECT stocked_quantity FROM inventory_levels WHERE id='joint-warehouse-level'),(SELECT COUNT(*) FROM inventory_stock_edits),(SELECT COUNT(*) FROM device_sources WHERE type='self'),(SELECT COUNT(*) FROM devices d JOIN device_sources s ON s.id=d.source_id WHERE d.location_id=s.location_id AND d.variant_id='joint-device-variant')")
            if durable.returncode or durable.stdout.decode().strip()!='2\t2\t1\t4\t100.000000000000000001\t0\t1\t2':
                raise RuntimeError('Durable device intake differs')
            report={'scope':'Real browser creates own source, enrolls SKU and registers sale/returned devices; photo upload and fulfillment incomplete',
                    'counts':json.loads(markers[0]),'durable_mysql_verified':True,'real_accounts_used':False,
                    'limits':{'mysql_cpus':1,'mysql_memory_mib':768,'redis_cpus':1,'redis_memory_mib':128,'network':'owned bridge; egress unrestricted','published_ports':'one random API port on 127.0.0.1 only'},
                    'tls_secure_cookie_verified':False,'photo_upload_accepted':False,'fulfillment_accepted':False}
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
    (ROOT/('docs/migration/device-intake-joint-report.json' if device_intake else 'docs/migration/device-sources-joint-report.json' if device_sources else 'docs/migration/device-tracking-joint-report.json' if device_tracking else 'docs/migration/warehouse-inventory-joint-report.json')).write_text(json.dumps(report,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({'real_browser_joint':True,'durable_mysql_verified':True,'cleanup':'complete'}))

if __name__=='__main__':
    main()
