"""Actual Astro/browser -> Gin handlers -> bounded, disposable MySQL/Redis.

The paid order is produced by the existing isolated payment-adapter rehearsal.
This accepts account/order integration only, not real provider payments or mail.
"""
import json
import os
from pathlib import Path
import re
import secrets
import shutil
import subprocess
import time
import uuid

ROOT = Path(__file__).resolve().parents[2]
LABEL = 'eurorefurb.customer-orders-rehearsal'


def docker(*args, timeout=60):
    try:
        return subprocess.run(['docker', *args], capture_output=True, timeout=timeout)
    except subprocess.TimeoutExpired:
        raise RuntimeError('Owned Docker operation timed out; arguments suppressed') from None


def require(result, description):
    if result.returncode:
        raise RuntimeError(description)
    return result.stdout.decode().strip()


def wait_owned(process, timeout):
    try:
        return process.wait(timeout=timeout)
    except subprocess.TimeoutExpired:
        # docker exec arguments include artificial fixture secrets.
        raise RuntimeError('Owned SQL process timed out; arguments suppressed') from None


def main():
    if shutil.disk_usage(ROOT).free < 2 * (1 << 30):
        raise RuntimeError('At least 2 GiB free disk required')
    for image in ['mysql:8.4', 'redis:7-alpine']:
        require(docker('image', 'inspect', image), 'Existing fixture images required')
    owner = uuid.uuid4().hex
    logs = ROOT / 'logs/gin-migration'
    binary = logs / ('customer-orders-' + owner + '.test')
    if binary.exists() or subprocess.run(['git', 'check-ignore', '--quiet', str(binary)], cwd=ROOT, capture_output=True).returncode:
        raise RuntimeError('New ignored binary path required')
    node = Path(os.environ.get('STOREFRONT_TEST_NODE') or shutil.which('node') or '').resolve()
    version = subprocess.run([str(node), '--version'], capture_output=True, check=True).stdout.decode().strip()
    if tuple(int(x) for x in version.lstrip('v').split('.')) < (22, 23, 4):
        raise RuntimeError('Set STOREFRONT_TEST_NODE to Node >=22.23.4')
    env = os.environ.copy()
    env.update(GOOS='linux', GOARCH='amd64', CGO_ENABLED='0')
    containers, network, process, stream = [], None, None, None
    password, key = secrets.token_urlsafe(32), secrets.token_hex(32)
    try:
        build = subprocess.run(['go', 'test', '-c', '-o', str(binary), './internal/identity'], cwd=ROOT / 'retail/backend', env=env, capture_output=True, timeout=120)
        if build.returncode:
            (logs / 'customer-orders-joint-build.log').write_bytes(build.stdout + build.stderr)
            raise RuntimeError('Identity test binary compilation failed; inspect ignored build log')
        built = subprocess.run([str(node), 'node_modules/astro/bin/astro.mjs', 'build'], cwd=ROOT / 'retail/storefront', capture_output=True, timeout=120)
        (logs / 'customer-orders-joint-astro-build.log').write_bytes(built.stdout + built.stderr)
        require(built, 'Astro build failed; inspect ignored build log')
        network = require(docker('network', 'create', '--label', LABEL + '=' + owner, 'customer-orders-' + owner[:12]), 'Owned network creation failed')
        mysql = require(docker('run', '-d', '--name', 'customer-orders-mysql-' + owner[:12], '--label', LABEL + '=' + owner,
                              '--network', network, '--cpus', '1', '--memory', '768m', '--memory-swap', '768m', '--pids-limit', '192',
                              '--tmpfs', '/var/lib/mysql:rw,size=512m', '--tmpfs', '/tmp:rw,size=32m,mode=1777',
                              '-p', '127.0.0.1::9100', '-e', 'MYSQL_ROOT_PASSWORD=' + password, 'mysql:8.4',
                              '--skip-log-bin', '--innodb-buffer-pool-size=64M', '--performance-schema=OFF', '--mysqlx=OFF',
                              '--max-connections=16', '--table-open-cache=128', '--table-definition-cache=400', '--table-open-cache-instances=1'), 'Owned MySQL creation failed')
        containers.append(mysql)
        redis = require(docker('run', '-d', '--label', LABEL + '=' + owner, '--network', 'container:' + mysql,
                              '--cpus', '1', '--memory', '128m', '--memory-swap', '128m', '--pids-limit', '64', '--tmpfs', '/data:rw,size=16m',
                              'redis:7-alpine', 'redis-server', '--bind', '127.0.0.1', '--save', '', '--appendonly', 'no',
                              '--maxmemory', '16mb', '--maxmemory-policy', 'noeviction'), 'Owned Redis creation failed')
        containers.append(redis)
        for _ in range(100):
            if not docker('exec', '-e', 'MYSQL_PWD=' + password, mysql, 'mysql', '-uroot', '--protocol=socket', '-e', 'SELECT 1', timeout=10).returncode:
                break
            time.sleep(.5)
        else:
            raise RuntimeError('Owned MySQL not ready')
        if require(docker('exec', redis, 'redis-cli', 'PING'), 'Owned Redis not ready') != 'PONG':
            raise RuntimeError('Owned Redis not ready')
        require(docker('cp', str(binary), mysql + ':/opt/identity.test'), 'Fixture transfer failed')
        require(docker('exec', mysql, 'chmod', '700', '/opt/identity.test'), 'Fixture chmod failed')
        require(docker('exec', mysql, 'mkdir', '-p', '/docs/migration/fixtures'), 'Fixture directory failed')
        require(docker('cp', str(ROOT / 'docs/migration/fixtures/cart-promotion-context.json'), mysql + ':/docs/migration/fixtures/cart-promotion-context.json'), 'Fixture oracle transfer failed')
        published = require(docker('port', mysql, '9100/tcp'), 'Owned API port unavailable')
        match = re.fullmatch(r'127\.0\.0\.1:(\d+)', published)
        if not match or not 1 <= int(match[1]) <= 65535:
            raise RuntimeError('Expected one loopback-only API port')
        stream = (logs / 'customer-orders-joint-sql.log').open('wb')
        process = subprocess.Popen(['docker', 'exec', '-e', 'MYSQL_PWD=' + password, '-e', 'IDENTITY_REHEARSAL=1',
                                    '-e', 'CUSTOMER_ORDERS_BROWSER_FIXTURE=1', '-e', 'CUSTOMER_ORDERS_FIXTURE_KEY=' + key,
                                    mysql, '/opt/identity.test', '-test.v', '-test.run', '^TestOwnedCartRuntime$', '-test.timeout', '150s'], stdout=stream, stderr=subprocess.STDOUT)
        browser_env = os.environ.copy()
        browser_env.update(CUSTOMER_ORDERS_TEST_GIN='http://127.0.0.1:' + match[1], CUSTOMER_ORDERS_FIXTURE_KEY=key)
        browser = subprocess.run([str(node), 'tests/customer-orders.live.mjs'], cwd=ROOT / 'retail/storefront', env=browser_env, capture_output=True, timeout=100)
        (logs / 'customer-orders-joint-browser.log').write_bytes(browser.stdout + browser.stderr)
        require(browser, 'Actual Gin/Astro browser check failed; inspect ignored browser and SQL logs')
        if wait_owned(process, 45):
            raise RuntimeError('SQL regressions after browser failed; inspect ignored SQL log')
        stream.close()
        stream = None
        sql_output = (logs / 'customer-orders-joint-sql.log').read_text(encoding='utf-8')
        markers = [line.split('CUSTOMER_ORDERS_JOINT_BROWSER ', 1)[1] for line in browser.stdout.decode().splitlines() if line.startswith('CUSTOMER_ORDERS_JOINT_BROWSER ')]
        if len(markers) != 1 or 'CUSTOMER_ORDERS_JOINT_SQL ' not in sql_output or 'CUSTOMER_ORDER_OWNERSHIP_REHEARSAL ' not in sql_output or '\nPASS\n' not in sql_output:
            raise RuntimeError('Joint evidence incomplete')
        report = {'format_version': 1, 'node_version': version, 'browser': json.loads(markers[0]), 'post_browser_sql_regression_passed': True,
                  'sql_attachment_and_unique_audit_checked': True, 'production_services_or_data_used': False, 'full_storefront_accepted': False,
                  'resources': {'mysql_cpus': 1, 'mysql_memory_mib': 768, 'redis_cpus': 1, 'redis_memory_mib': 128,
                                'database_ports_published': False, 'api_port_loopback_only': True, 'network_egress_restricted': False}}
    finally:
        if process is not None and process.poll() is None:
            # Stop only this runner; container removal below stops its test child.
            process.terminate()
            wait_owned(process, 10)
        if stream is not None:
            stream.close()
        for container in reversed(containers):
            actual = require(docker('inspect', '--format', '{{ index .Config.Labels "' + LABEL + '" }}', container), 'Ownership inspection failed')
            if actual != owner:
                raise RuntimeError('Container ownership differs; cleanup refused')
            require(docker('rm', '-f', container), 'Owned container cleanup failed')
        if network:
            actual = require(docker('network', 'inspect', '--format', '{{ index .Labels "' + LABEL + '" }}', network), 'Network ownership inspection failed')
            if actual != owner:
                raise RuntimeError('Network ownership differs; cleanup refused')
            require(docker('network', 'rm', network), 'Owned network cleanup failed')
        binary.unlink(missing_ok=True)
    report['cleanup_complete'] = True
    (ROOT / 'docs/migration/customer-orders-joint-report.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'customer_orders_joint_browser': 'passed', 'sql_regressions': 'passed', 'cleanup': 'complete'}))


if __name__ == '__main__':
    main()
