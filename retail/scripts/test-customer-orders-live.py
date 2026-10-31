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
import signal
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



def run_owned_browser(command, cwd, env, timeout):
    # The Astro preview is a child of the browser script. An unhandled browser
    # failure must not orphan it and poison the next run on the fixture port.
    process = subprocess.Popen(command, cwd=cwd, env=env, stdout=subprocess.PIPE,
                               stderr=subprocess.PIPE, start_new_session=os.name == 'posix')
    try:
        stdout, stderr = process.communicate(timeout=timeout)
        return subprocess.CompletedProcess(command, process.returncode, stdout, stderr)
    finally:
        if os.name == 'posix':
            try:
                os.killpg(process.pid, signal.SIGTERM)
                time.sleep(.1)
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
        elif process.poll() is None:
            process.terminate()
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait(timeout=5)


def payment_joint_manifest(backend):
    if backend != ROOT / 'retail/backend':
        raise RuntimeError('Payment joint requires one local source snapshot')
    paths = [Path(__file__)]
    paths += [p for p in backend.rglob('*') if p.is_file() and (p.suffix in ('.go','.sql','.json') or p.name in ('go.mod','go.sum'))]
    for folder in ['retail/storefront/src','retail/storefront/tests','retail/admin/src','retail/admin/tests']:
        paths += [p for p in (ROOT/folder).rglob('*') if p.is_file() and p.suffix in ('.ts','.tsx','.mjs','.json','.css') and not any(x in p.parts for x in ['node_modules','.astro','dist'])]
    paths += [ROOT/p for p in ['retail/admin/package.json','retail/admin/pnpm-lock.yaml','retail/admin/vite.config.ts','retail/storefront/package.json','retail/storefront/pnpm-lock.yaml','retail/storefront/astro.config.mjs','retail/storefront/scripts/start.mjs'] if (ROOT/p).is_file()]
    return {str(p.relative_to(ROOT)): hashlib.sha256(p.read_bytes().replace(b'\r\n',b'\n')).hexdigest() for p in sorted(set(paths))}


def main():
    if sys.argv[1:] not in ([], ['--promotions'], ['--checkout']):
        raise RuntimeError('Unknown joint rehearsal option')
    promotion = sys.argv[1:] == ['--promotions']
    checkout = sys.argv[1:] == ['--checkout']
    fulfillment = os.environ.get('CHECKOUT_FULFILLMENT') == '1'
    repair = os.environ.get('CHECKOUT_REPAIR') == '1'
    exchange = os.environ.get('CHECKOUT_EXCHANGE') == '1'
    refund = os.environ.get('CHECKOUT_REFUND') == '1'
    exchange_refund = os.environ.get('CHECKOUT_EXCHANGE_REFUND') == '1'
    exchange_payment = os.environ.get('CHECKOUT_EXCHANGE_PAYMENT') == '1'
    if exchange_payment and (not exchange or exchange_refund or refund):
        raise RuntimeError('Supplemental payment requires exchange without refund mode')
    if exchange_refund and (not exchange or refund):
        raise RuntimeError('Exchange credit refund requires exchange mode without return mode')
    if refund and (not fulfillment or repair or exchange):
        raise RuntimeError('Refund requires fulfillment and is separate from repair/exchange modes')
    if exchange and (not fulfillment or repair):
        raise RuntimeError('Exchange requires fulfillment and is separate from repair mode')
    if repair and not fulfillment:
        raise RuntimeError('Repair requires the new order fulfillment mode')
    if fulfillment and (not checkout or any(os.environ.get(k) != '1' for k in ['CHECKOUT_ACCOUNTS', 'CHECKOUT_FRESH_CART', 'CHECKOUT_PRODUCT_BROWSE'])):
        raise RuntimeError('Fulfillment requires checkout with new accounts and fresh product carts')
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
        if fulfillment:
            log_prefix = 'shopping-exchange-payment-joint' if exchange_payment else 'shopping-exchange-refund-joint' if exchange_refund else 'shopping-refund-joint' if refund else 'shopping-exchange-joint' if exchange else 'shopping-repair-joint' if repair else 'shopping-fulfillment-joint'
        browser_marker, sql_marker, regression_marker = 'CHECKOUT_JOINT_BROWSER ', 'CHECKOUT_JOINT_SQL ', 'CART_COMPLETION_HTTP_REHEARSAL '
    if shutil.disk_usage(ROOT).free < 2 * (1 << 30):
        raise RuntimeError('At least 2 GiB free disk required')
    for image in ['mysql:8.4', 'redis:7-alpine']:
        require(docker('image', 'inspect', image), 'Existing fixture images required')
    owner = uuid.uuid4().hex
    logs = ROOT / 'logs/gin-migration'
    binary = logs / ('customer-orders-' + owner + '.test')
    refund_binary = logs / ('refund-worker-' + owner)
    payment_binary = logs / ('order-payment-worker-' + owner)
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
        frozen_payment_sources = payment_joint_manifest(backend) if exchange_payment else None
        build = subprocess.run(['go', 'test', '-c', '-o', str(binary), './internal/identity'], cwd=backend, env=env, capture_output=True, timeout=120)
        if build.returncode:
            (logs / (log_prefix + '-build.log')).write_bytes(build.stdout + build.stderr)
            raise RuntimeError('Identity test binary compilation failed; inspect ignored build log')
        if refund or exchange_refund:
            require(subprocess.run(['go','build','-o',str(refund_binary),'./cmd/process-order-refunds'],cwd=backend,env=env,capture_output=True,timeout=120),'Refund CLI build failed')
        if exchange_payment:
            require(subprocess.run(['go','build','-o',str(payment_binary),'./cmd/process-order-payments'],cwd=backend,env=env,capture_output=True,timeout=120),'Payment CLI build failed')
            payment_binary_sha256 = hashlib.sha256(payment_binary.read_bytes()).hexdigest()
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
            ready = docker('exec', '-e', 'MYSQL_PWD=' + password, mysql, 'mysql', '-uroot', '--protocol=socket', '--batch', '--skip-column-names', '-e', 'SELECT @@GLOBAL.skip_networking', timeout=10)
            if ready.returncode == 0 and ready.stdout.strip() == b'0':
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
        if refund or exchange_refund:
            require(docker('cp',str(refund_binary),mysql+':/opt/refund-worker'),'Owned refund worker copy failed')
        if exchange_payment:
            require(docker('cp',str(payment_binary),mysql+':/opt/order-payment-worker'),'Owned payment worker copy failed')
        process = subprocess.Popen(['docker', 'exec', '-e', 'MYSQL_PWD=' + password, '-e', 'IDENTITY_REHEARSAL=1',
                                    '-e', fixture_switch + '=1', '-e', fixture_key + '=' + key,
                                    '-e', 'CHECKOUT_PRODUCT_BROWSE=' + os.environ.get('CHECKOUT_PRODUCT_BROWSE', '0'),
                                    '-e', 'CHECKOUT_FRESH_CART=' + os.environ.get('CHECKOUT_FRESH_CART', '0'),
                                    '-e', 'CHECKOUT_ACCOUNTS=' + os.environ.get('CHECKOUT_ACCOUNTS', '0'),
                                    '-e', 'CHECKOUT_FULFILLMENT=' + ('1' if fulfillment else '0'),
                                    '-e', 'CHECKOUT_REPAIR=' + ('1' if repair else '0'),
                                    '-e', 'CHECKOUT_EXCHANGE=' + ('1' if exchange else '0'),
                                    '-e', 'CHECKOUT_REFUND=' + ('1' if refund else '0'),
                                    '-e', 'CHECKOUT_EXCHANGE_REFUND=' + ('1' if exchange_refund else '0'),
                                    '-e', 'CHECKOUT_REFUND_WORKER_BINARY=/opt/refund-worker',
                                    '-e', 'CHECKOUT_EXCHANGE_PAYMENT=' + ('1' if exchange_payment else '0'),
                                    '-e', 'CHECKOUT_PAYMENT_WORKER_BINARY=/opt/order-payment-worker',
                                    mysql, '/opt/identity.test', '-test.v', '-test.run', '^TestOwnedCartRuntime$', '-test.timeout', '540s' if repair or exchange or refund else '420s' if fulfillment else '150s'], stdout=stream, stderr=subprocess.STDOUT)
        browser_env = os.environ.copy()
        browser_env.update(CUSTOMER_ORDERS_TEST_GIN='http://127.0.0.1:' + match[1])
        browser_env[fixture_key] = key
        browser = run_owned_browser([str(node), test_script], cwd=ROOT / 'retail/storefront', env=browser_env, timeout=420 if repair or exchange or refund else 340 if fulfillment else 100)
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
                  'sql_receipts_checked': True, 'scope': ('new account registration, verification, anonymous cart, payment and account order ownership' if os.environ.get('CHECKOUT_ACCOUNTS') == '1' else 'anonymous cart creation, product add, checkout, payment and confirmation' if os.environ.get('CHECKOUT_FRESH_CART') == '1' else 'product page add to empty cart through checkout, payment and confirmation' if os.environ.get('CHECKOUT_PRODUCT_BROWSE') == '1' else 'checkout pipeline') if checkout else 'checkout promotions' if promotion else 'customer order attachment', 'production_services_or_data_used': False, 'full_storefront_accepted': False,
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
            if fulfillment:
                evidence = [line.split('CHECKOUT_FULFILLMENT_JOINT_SQL ', 1)[1] for line in sql_output.splitlines() if 'CHECKOUT_FULFILLMENT_JOINT_SQL ' in line]
                if len(evidence) != 1:
                    raise RuntimeError('New order fulfillment evidence incomplete')
                report['fulfillment'] = json.loads(evidence[0])
                if refund or exchange_refund:
                    refund_marker='CHECKOUT_EXCHANGE_REFUND_JOINT_SQL ' if exchange_refund else 'CHECKOUT_REFUND_JOINT_SQL '
                    evidence=[line.split(refund_marker,1)[1] for line in sql_output.splitlines() if refund_marker in line]
                    if len(evidence)!=1: raise RuntimeError('New shopper refund evidence incomplete')
                    report['refund']=json.loads(evidence[0])
                if exchange_payment:
                    evidence=[line.split('CHECKOUT_EXCHANGE_PAYMENT_JOINT_SQL ',1)[1] for line in sql_output.splitlines() if 'CHECKOUT_EXCHANGE_PAYMENT_JOINT_SQL ' in line]
                    if len(evidence)!=1: raise RuntimeError('New customer supplemental payment evidence incomplete')
                    report['supplemental_payment']=json.loads(evidence[0])
                if exchange and not exchange_refund and not exchange_payment:
                    evidence = [line.split('CHECKOUT_EXCHANGE_JOINT_SQL ', 1)[1] for line in sql_output.splitlines() if 'CHECKOUT_EXCHANGE_JOINT_SQL ' in line]
                    if len(evidence) != 1:
                        raise RuntimeError('New order exchange evidence incomplete')
                    report['exchange'] = json.loads(evidence[0])
                if repair:
                    evidence = [line.split('CHECKOUT_REPAIR_JOINT_SQL ', 1)[1] for line in sql_output.splitlines() if 'CHECKOUT_REPAIR_JOINT_SQL ' in line]
                    if len(evidence) != 1:
                        raise RuntimeError('New order repair evidence incomplete')
                    report['repair'] = json.loads(evidence[0])
                report['scope'] = 'new customer registration and shopping through the same paid order, cancellation, warehouse reallocation, repacking, shipment and own customer tracking and notice' + ('; own repair request, physical receipt, inspection with photo, original-device repair dispatch, completion and own return tracking/notices' if repair else '; own exchange request, scanned receipt, native replacement, warehouse shipment, original inspection, completion and own tracking/notices' if exchange else '')
            if os.environ.get('CHECKOUT_ACCOUNTS') == '1':
                accounts = [line.split('CHECKOUT_ACCOUNT_JOINT_SQL ', 1)[1] for line in sql_output.splitlines() if 'CHECKOUT_ACCOUNT_JOINT_SQL ' in line]
                if len(accounts) != 1:
                    raise RuntimeError('Checkout account evidence incomplete')
                report['account_ownership'] = json.loads(accounts[0])
            sql = [line.split(sql_marker, 1)[1] for line in sql_output.splitlines() if sql_marker in line]
            if len(sql) != 1:
                raise RuntimeError('Checkout receipt evidence incomplete')
            report['sql'] = json.loads(sql[0])
            report['source_sha256_lf'] = {}
            sources = [(backend / 'internal/identity' / name, 'retail/backend/internal/identity/' + name)
                       for name in ['checkout_browser_test.go', 'checkout_accounts_browser_test.go', 'checkout_mail_joint_test.go', 'mail_joint_test.go']]
            sources += [(Path(__file__), 'retail/scripts/test-customer-orders-live.py'),
                        (ROOT / 'retail/storefront/tests/checkout.live.mjs', 'retail/storefront/tests/checkout.live.mjs'),
                        (ROOT / 'retail/storefront/src/components/cart/gin-cart.tsx', 'retail/storefront/src/components/cart/gin-cart.tsx'),
                        (ROOT / 'retail/storefront/src/lib/cart-commerce.ts', 'retail/storefront/src/lib/cart-commerce.ts'),
                        (ROOT / 'retail/storefront/tests/checkout-accounts.joint.mjs', 'retail/storefront/tests/checkout-accounts.joint.mjs')]
            if fulfillment:
                sources += [(backend / 'internal/identity' / name, 'retail/backend/internal/identity/' + name) for name in ['checkout_fulfillment_browser_test.go', 'checkout_repair_browser_test.go', 'checkout_exchange_browser_test.go', 'cart_runtime_test.go']]
                sources += [(ROOT / 'retail/admin/tests' / name, 'retail/admin/tests/' + name) for name in ['checkout-fulfillment.joint.mjs', 'checkout-repair.joint.mjs', 'checkout-exchange.joint.mjs']]
            if repair or exchange or refund:
                sources += [(ROOT / name, name) for name in ['retail/backend/internal/customer/after_sales.go', 'retail/backend/internal/customer/repair_shipments.go', 'retail/backend/internal/devices/service.go', 'retail/storefront/src/lib/customer-after-sales.ts', 'retail/storefront/src/components/order/after-sales.tsx']]
            if refund or exchange_refund:
                sources += [(ROOT / name,name) for name in ['retail/admin/tests/checkout-refund.joint.mjs','retail/backend/internal/identity/checkout_refund_worker_test.go','retail/backend/internal/identity/checkout_refund_browser_test.go','retail/backend/cmd/process-order-refunds/main.go','retail/backend/internal/customernotices/events.go','retail/storefront/src/lib/customer-notices.ts','retail/storefront/src/components/notifications/i18n.ts','retail/storefront/src/components/notifications/notification-bell.tsx','retail/admin/src/OrderRefund.tsx','retail/admin/src/refundApi.ts','retail/backend/internal/orders/current_balance.go','retail/backend/internal/orders/supplemental_payment.go','retail/backend/internal/orders/schema.go','retail/backend/internal/orders/version.go','retail/backend/internal/orders/change_summary.go','retail/backend/internal/customer/after_sales_review.go','retail/backend/internal/identity/order_payment_commands_test.go','retail/backend/internal/identity/order_payment_execution_test.go','retail/backend/internal/identity/order_payment_consumption_test.go','retail/backend/internal/orders/change_summary.go','retail/backend/internal/checkoutconfig/order_payments.go','retail/backend/internal/checkoutconfig/order_payments_test.go','retail/backend/cmd/process-order-payments/main.go','retail/backend/.env.example','retail/backend/internal/identity/exchange_payment_balance_test.go']]
                sources += [(p,str(p.relative_to(ROOT))) for p in (backend/'internal/refunds').glob('*.go')]
                sources += [(p,str(p.relative_to(ROOT))) for p in (backend/'internal/orderpayments').glob('*.go')]
                sources += [(p,str(p.relative_to(ROOT))) for p in (backend/'migrations').glob('*.sql')]
                report['scope'] += '; ' + ('own exchange credit 12 EUR settled to zero difference' if exchange_refund else 'own return') + ', receipt/photo inspection, Admin refund drawer, separate production CLI via owned HTTPS provider, lost POST restart GET recovery, exact refund ledger and own notice'
            if exchange_payment:
                sources += [(p,str(p.relative_to(ROOT))) for p in backend.rglob('*') if p.is_file() and (p.suffix in ('.go','.sql','.json') or p.name in ('go.mod','go.sum'))]
                sources += [(p,str(p.relative_to(ROOT))) for folder in ['retail/storefront/src','retail/storefront/tests','retail/admin/src','retail/admin/tests'] for p in (ROOT/folder).rglob('*') if p.is_file() and p.suffix in ('.ts','.tsx','.mjs','.json','.css') and not any(x in p.parts for x in ['node_modules','.astro','dist'])]
                report['scope'] += '; own exact 6 EUR supplemental payment through actual customer HTTP/UI, separately compiled production CLI, owned HTTPS provider, private notice and replacement shipment'
            for source, name in sources:
                report['source_sha256_lf'][name] = hashlib.sha256(source.read_bytes().replace(b'\r\n', b'\n')).hexdigest()
            if exchange_payment:
                if payment_joint_manifest(backend) != frozen_payment_sources:
                    raise RuntimeError('Payment joint source changed after compilation; evidence refused')
                report['source_sha256_lf'] = frozen_payment_sources
                report['compiled_production_cli_sha256'] = payment_binary_sha256
                report['source_frozen_before_build_and_verified_after'] = True
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
        refund_binary.unlink(missing_ok=True)
        payment_binary.unlink(missing_ok=True)
    report['cleanup_complete'] = True
    (ROOT / (('docs/migration/shopping-exchange-payment-joint-report.json' if exchange_payment else 'docs/migration/shopping-exchange-refund-joint-report.json' if exchange_refund else 'docs/migration/shopping-refund-joint-report.json' if refund else 'docs/migration/shopping-exchange-joint-report.json' if exchange else 'docs/migration/shopping-repair-joint-report.json' if repair else 'docs/migration/shopping-fulfillment-joint-report.json' if fulfillment else 'docs/migration/shopping-account-joint-report.json' if os.environ.get('CHECKOUT_ACCOUNTS') == '1' else 'docs/migration/shopping-fresh-cart-joint-report.json' if os.environ.get('CHECKOUT_FRESH_CART') == '1' else 'docs/migration/shopping-joint-report.json' if os.environ.get('CHECKOUT_PRODUCT_BROWSE') == '1' else 'docs/migration/checkout-mail-joint-report.json') if checkout else 'docs/migration/promotion-joint-report.json' if promotion else 'docs/migration/customer-orders-joint-report.json')).write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'joint_browser': 'passed', 'scope': 'checkout' if checkout else 'promotions' if promotion else 'customer_orders', 'sql_regressions': 'passed', 'cleanup': 'complete'}))


if __name__ == '__main__':
    main()
