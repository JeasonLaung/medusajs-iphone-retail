"""Production Astro browser -> signed bridge -> production Gin binary, with owned artificial SQL/Redis."""
import json
import os
from pathlib import Path
import re
import secrets
import shutil
import subprocess
import time
import urllib.request
import uuid

ROOT = Path(__file__).resolve().parents[2]
LABEL = 'eurorefurb.buyback-browser-rehearsal'


def docker(*args, timeout=90):
    return subprocess.run(['docker', *args], capture_output=True, timeout=timeout)


def main():
    if shutil.disk_usage(ROOT).free < 2 * (1 << 30):
        raise RuntimeError('At least 2 GiB free workspace disk required to preserve development services')
    for image in ['mysql:8.4', 'redis:7-alpine']:
        if docker('image', 'inspect', image).returncode:
            raise RuntimeError('Existing fixture images required; no pull performed')
    backend = Path(os.environ.get("BUYBACK_JOINT_BACKEND", str(ROOT / "retail/backend"))).resolve()
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
            if subprocess.run(command, cwd=backend, env=env, capture_output=True, timeout=120).returncode:
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
        for fixture, marker in [('TestPrepareReviewedCatalogFixture',b''),('TestPrepareBuybackBrowserFixture',b'OWNED_BUYBACK_BROWSER_READY')]:
            prepared=docker('exec','-e','MYSQL_PWD='+password,'-e','BROWSER_IDENTITY_FIXTURE=1',mysql_id,'/opt/prep.test','-test.v','-test.run','^'+fixture+'$','-test.timeout','40s')
            if prepared.returncode or marker not in prepared.stdout:
                raise RuntimeError('Owned buyback preparation failed: '+prepared.stdout.decode(errors='replace')+prepared.stderr.decode(errors='replace'))
        app_env={'STOREFRONT_STORE_ID':'fixture-store','GIN_MODE':'release','APP_BIND':'0.0.0.0','APP_PORT':'9100','APP_ENV':'development',
                 'MYSQL_DSN':'root:'+password+'@unix(/var/run/mysqld/mysqld.sock)/browser_identity?parseTime=true&loc=UTC',
                 'SESSION_REDIS_URL':'redis://127.0.0.1:6379/0','SESSION_REDIS_PREFIX':'eurorefurb:buyback_browser:v1:',
                 'ADMIN_ORIGIN':'http://127.0.0.1:19383','STOREFRONT_ORIGIN':'http://127.0.0.1:19383','SESSION_SECURE':'false',
                 'CUSTOMER_AUTH_ENABLED':'true','CUSTOMER_BRIDGE_KEY':secrets.token_hex(32),'CUSTOMER_KEY_ACTIVE':'fixture','CUSTOMER_KEYRING_JSON':json.dumps({'fixture':secrets.token_hex(32)})}
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
        browser_env=os.environ.copy()
        browser_env.update(ADMIN_TEST_API_TARGET=target,CUSTOMER_BRIDGE_KEY=app_env['CUSTOMER_BRIDGE_KEY'])
        node=browser_env.get('ADMIN_TEST_NODE') or shutil.which('node')
        tested=subprocess.run([node,'tests/buyback.joint.mjs'],cwd=ROOT/'retail/storefront',env=browser_env,capture_output=True,timeout=300)
        (ROOT/'logs/gin-migration/buyback-joint-browser.log').write_bytes(tested.stdout+tested.stderr)
        marker='BUYBACK_JOINT '
        outcomes=[line.split(marker,1)[1] for line in tested.stdout.decode('utf-8').splitlines() if line.startswith(marker)]
        if tested.returncode or len(outcomes)!=1:raise RuntimeError('Buyback browser joint failed; inspect ignored browser log')
        durable=docker('exec','-e','MYSQL_PWD='+password,mysql_id,'mysql','-uroot','--protocol=socket','--database=browser_identity','--batch','--skip-column-names','-e',"SELECT (SELECT COUNT(*) FROM buyback_requests),(SELECT COUNT(*) FROM customer_buyback_submissions),(SELECT COUNT(*) FROM buyback_photos),(SELECT COUNT(*) FROM buyback_photo_bytes),(SELECT COUNT(*) FROM buyback_requests WHERE status='photo_review'),(SELECT COUNT(*) FROM buyback_requests WHERE status='canceled' AND closed_at IS NOT NULL),(SELECT COUNT(*) FROM buyback_requests WHERE JSON_UNQUOTE(JSON_EXTRACT(items,'$[0].serial_number'))='SN-JOINT'),(SELECT COUNT(*) FROM customer_buyback_submissions WHERE customer_id='cust_buyback-other')")
        if durable.returncode or durable.stdout.decode().strip()!='2\t2\t2\t2\t1\t1\t1\t0':raise RuntimeError('Durable buyback state differs: '+durable.stdout.decode().strip())
        report={'browser':json.loads(outcomes[0]),'durable_mysql_verified':True,'artificial_accounts_only':True,'tls_secure_cookie_verified':False,'full_buyback_accepted':False,'limits':{'mysql_cpus':1,'mysql_memory_mib':768,'redis_memory_mib':128}}
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
    (ROOT/'docs/migration/buyback-joint-report.json').write_text(json.dumps(report,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({'real_browser_joint':True,'durable_mysql_verified':True,'cleanup':'complete'}))

if __name__=='__main__':main()
