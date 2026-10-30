"""Actual Astro/browser -> Gin handlers -> bounded, disposable MySQL/Redis.

Default: account/order attachment, with an isolated adapter-produced order.
--promotions: checkout discount mutations. Neither accepts real payments/mail.
--checkout: addresses through payment/confirmation with an isolated provider.
"""
import json
import hashlib
import os
from pathlib import Path
import re
import secrets
import shutil
import subprocess
import time
import uuid
import sys

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
    if sys.argv[1:] not in ([], ['--promotions'], ['--checkout']):
        raise RuntimeError('Unknown joint rehearsal option')
    promotion = sys.argv[1:] == ['--promotions']
    checkout = sys.argv[1:] == ['--checkout']
    log_prefix = 'promotion-joint' if promotion else 'customer-orders-joint'
    fixture_switch = 'PROMOTION_BROWSER_FIXTURE' if promotion else 'CUSTOMER_ORDERS_BROWSER_FIXTURE'
    fixture_key = 'PROMOTION_FIXTURE_KEY' if promotion else 'CUSTOMER_ORDERS_FIXTURE_KEY'
    test_script = 'tests/promotions.live.mjs' if promotion else 'tests/customer-orders.live.mjs'
    browser_marker = 'PROMOTION_JOINT_BROWSER ' if promotion else 'CUSTOMER_ORDERS_JOINT_BROWSER '
    sql_marker = 'PROMOTION_JOINT_SQL ' if promotion else 'CUSTOMER_ORDERS_JOINT_SQL '
    regression_marker = 'CART_PROMOTION_COMMAND_REHEARSAL ' if promotion else 'CUSTOMER_ORDER_OWNERSHIP_REHEARSAL '
    if checkout:
        log_prefix, fixture_switch, fixture_key = 'checkout-joint', 'CHECKOUT_BROWSER_FIXTURE', 'CHECKOUT_FIXTURE_KEY'
        test_script = 'tests/checkout.live.mjs'
        browser_marker, sql_marker, regression_marker = 'CHECKOUT_JOINT_BROWSER ', 'CHECKOUT_JOINT_SQL ', 'CART_COMPLETION_HTTP_REHEARSAL '
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
        backend = Path(os.environ.get('CUSTOMER_ORDERS_TEST_BACKEND', str(ROOT / 'retail/backend'))).resolve()
        build = subprocess.run(['go', 'test', '-c', '-o', str(binary), './internal/identity'], cwd=backend, env=env, capture_output=True, timeout=120)
        if build.returncode:
            (logs / (log_prefix + '-build.log')).write_bytes(build.stdout + build.stderr)
            raise RuntimeError('Identity test binary compilation failed; inspect ignored build log')
        built = subprocess.run([str(node), 'node_modules/astro/bin/astro.mjs', 'build'], cwd=ROOT / 'retail/storefront', capture_output=True, timeout=120)
        (logs / (log_prefix + '-astro-build.log')).write_bytes(built.stdout + built.stderr)
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
        stream = (logs / (log_prefix + '-sql.log')).open('wb')
        process = subprocess.Popen(['docker', 'exec', '-e', 'MYSQL_PWD=' + password, '-e', 'IDENTITY_REHEARSAL=1',
                                    '-e', fixture_switch + '=1', '-e', fixture_key + '=' + key,
                                    mysql, '/opt/identity.test', '-test.v', '-test.run', '^TestOwnedCartRuntime$', '-test.timeout', '150s'], stdout=stream, stderr=subprocess.STDOUT)
        browser_env = os.environ.copy()
        browser_env.update(CUSTOMER_ORDERS_TEST_GIN='http://127.0.0.1:' + match[1])
        browser_env[fixture_key] = key
        browser = subprocess.run([str(node), test_script], cwd=ROOT / 'retail/storefront', env=browser_env, capture_output=True, timeout=100)
        (logs / (log_prefix + '-browser.log')).write_bytes(browser.stdout + browser.stderr)
        require(browser, 'Actual Gin/Astro browser check failed; inspect ignored browser and SQL logs')
        if wait_owned(process, 45):
            raise RuntimeError('SQL regressions after browser failed; inspect ignored SQL log')
        stream.close()
        stream = None
        sql_output = (logs / (log_prefix + '-sql.log')).read_text(encoding='utf-8')
        markers = [line.split(browser_marker, 1)[1] for line in browser.stdout.decode().splitlines() if line.startswith(browser_marker)]
        if len(markers) != 1 or sql_marker not in sql_output or regression_marker not in sql_output or '\nPASS\n' not in sql_output:
            raise RuntimeError('Joint evidence incomplete')
        report = {'format_version': 1, 'node_version': version, 'browser': json.loads(markers[0]), 'post_browser_sql_regression_passed': True,
                  'sql_receipts_checked': True, 'scope': 'checkout pipeline' if checkout else 'checkout promotions' if promotion else 'customer order attachment', 'production_services_or_data_used': False, 'full_storefront_accepted': False,
                  'resources': {'mysql_cpus': 1, 'mysql_memory_mib': 768, 'redis_cpus': 1, 'redis_memory_mib': 128,
                                'database_ports_published': False, 'api_port_loopback_only': True, 'network_egress_restricted': False}}
        if not promotion and not checkout:
            report['sql_attachment_and_unique_audit_checked'] = True
        if checkout:
            mail_marker = 'CHECKOUT_MAIL_JOINT_SQL '
            mail = [line.split(mail_marker, 1)[1] for line in sql_output.splitlines() if mail_marker in line]
            if len(mail) != 1:
                raise RuntimeError('Paid checkout mail evidence incomplete')
            report['order_mail'] = json.loads(mail[0])
            sql = [line.split(sql_marker, 1)[1] for line in sql_output.splitlines() if sql_marker in line]
            if len(sql) != 1:
                raise RuntimeError('Checkout receipt evidence incomplete')
            report['sql'] = json.loads(sql[0])
            report['source_sha256_lf'] = {}
            sources = [(backend / 'internal/identity' / name, 'retail/backend/internal/identity/' + name)
                       for name in ['checkout_browser_test.go', 'checkout_mail_joint_test.go', 'mail_joint_test.go']]
            sources += [(Path(__file__), 'retail/scripts/test-customer-orders-live.py'),
                        (ROOT / 'retail/storefront/tests/checkout.live.mjs', 'retail/storefront/tests/checkout.live.mjs')]
            for source, name in sources:
                report['source_sha256_lf'][name] = hashlib.sha256(source.read_bytes().replace(b'\r\n', b'\n')).hexdigest()
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
    (ROOT / ('docs/migration/checkout-mail-joint-report.json' if checkout else 'docs/migration/promotion-joint-report.json' if promotion else 'docs/migration/customer-orders-joint-report.json')).write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'joint_browser': 'passed', 'scope': 'checkout' if checkout else 'promotions' if promotion else 'customer_orders', 'sql_regressions': 'passed', 'cleanup': 'complete'}))


if __name__ == '__main__':
    main()
