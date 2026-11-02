"""Owned native-MySQL/Redis checkout -> review -> Admin -> public browser joint.

prepare creates a private manifest and bounded root setup/cleanup SQL, without
executing it. Apply setup.sql with mysql's hidden password prompt, then run.
No Docker, root password arguments, production database, or real provider/mail.
"""
import argparse
import getpass
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
import sys
import time
from urllib.request import Request, urlopen

ROOT = Path(__file__).resolve().parents[2]
BASE = ROOT / 'logs/gin-migration'
STATE = BASE / 'product-reviews-local-state.json'
NODE = '/opt/homebrew/opt/node/bin/node'
GO = '/opt/homebrew/opt/go/bin/go'
MYSQL = '/opt/homebrew/bin/mysql'


def private_json(path, data):
    path.write_text(json.dumps(data, indent=2) + '\n')
    path.chmod(0o600)


def state():
    c = json.loads(STATE.read_text())
    if not re.fullmatch('[0-9a-f]{12}', c['owner']):
        raise RuntimeError('Unexpected ownership marker')
    owner = c['owner']
    if c['database'] != 'eurorefurb_review_joint_' + owner or c['user'] != 'retail_review_' + owner or c['redis_prefix'] != 'eurorefurb:r' + owner + '-':
        raise RuntimeError('Unexpected owned resource identity')
    if c['snapshot'] != str(BASE / ('product-reviews-local-' + owner)):
        raise RuntimeError('Unexpected snapshot path')
    return c


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def replace(path, before, after):
    text = path.read_text()
    if text.count(before) != 1:
        raise RuntimeError('Source anchor changed: ' + path.name)
    path.write_text(text.replace(before, after))


def adapt_admin_fixture(path):
    # Linked node_modules must not make independent Vite servers share their
    # optimizer output with each other or the user's running Admin.
    replace(path, 'server=await createServer({root:',
            'server=await createServer({cacheDir:fileURLToPath(new URL("../.vite-review-joint",import.meta.url)),root:')
    anchor = "const p=await context.newPage();p.on('pageerror',e=>errors.push(e.message))"
    replace(path, anchor, anchor + "\n   p.on('response',async r=>{if(r.status()>=500){try{const body=await r.json();console.log('REVIEW_NATIVE_ERROR',JSON.stringify({path:new URL(r.url()).pathname,code:body.error?.code}))}catch{}}})")
    anchor = "const d=page.getByRole('dialog');await d.getByRole('row').filter({has:page.getByText(sn,{exact:true})}).getByRole('checkbox').check()"
    replace(path, anchor, "const d=page.getByRole('dialog');try{await d.getByRole('row').filter({has:page.getByText(sn,{exact:true})}).getByRole('checkbox').check()}catch(e){console.error('REVIEW_ALLOCATION_DIAGNOSTIC',JSON.stringify({url:page.url(),dialogs:await d.allTextContents()}));await page.screenshot({path:fileURLToPath(new URL('../../../logs/gin-migration/allocation-failure.png',import.meta.url)),fullPage:true});throw e}")


def prepare():
    if STATE.exists():
        raise RuntimeError('Existing state must be inspected and cleaned before preparing another run')
    if shutil.disk_usage(ROOT).free < 512 * 1024 * 1024:
        raise RuntimeError('Native isolated rehearsal needs at least 512 MiB free')
    BASE.mkdir(parents=True, exist_ok=True)
    owner = secrets.token_hex(6)
    sockets = [socket.socket() for _ in range(3)]
    try:
        for sock in sockets:
            sock.bind(('127.0.0.1', 0))
        ports = [sock.getsockname()[1] for sock in sockets]
    finally:
        for sock in sockets:
            sock.close()
    c = dict(owner=owner, database='eurorefurb_review_joint_' + owner,
             user='retail_review_' + owner, password=secrets.token_urlsafe(30),
             redis_prefix='eurorefurb:r' + owner + '-',
             fixture_key=secrets.token_hex(32), api_port=ports[0],
             storefront_port=ports[1], admin_port=ports[2],
             snapshot=str(BASE / ('product-reviews-local-' + owner)))
    snapshot = Path(c['snapshot'])
    snapshot.mkdir()
    original = {}
    for folder in ['backend', 'admin', 'storefront']:
        source = ROOT / 'retail' / folder
        target = snapshot / 'retail' / folder
        shutil.copytree(source, target, ignore=shutil.ignore_patterns(
            'node_modules', '.astro', 'dist', '.env*', '*.log', 'public',
            '.admin-credentials', 'playwright-report', 'test-results'))
        for copied in target.rglob('*'):
            if copied.is_file():
                relative = copied.relative_to(snapshot)
                if sha(copied) != sha(ROOT / relative):
                    raise RuntimeError('Source changed during snapshot copy')
                original[str(relative)] = sha(copied)
        if folder != 'backend':
            (target / 'node_modules').symlink_to(source / 'node_modules', target_is_directory=True)
        if (source / 'public').exists():
            (target / 'public').symlink_to(source / 'public', target_is_directory=True)
    (snapshot / 'logs/gin-migration').mkdir(parents=True)
    oracle = ROOT / 'docs/migration/fixtures/cart-promotion-context.json'
    oracle_target = snapshot / oracle.relative_to(ROOT)
    oracle_target.parent.mkdir(parents=True)
    shutil.copy2(oracle, oracle_target)
    original[str(oracle.relative_to(ROOT))] = sha(oracle)
    # Only owned fixture sources are adapted. Every production .go/TS/CSS file
    # remains byte-identical to the recorded worktree source.
    for path in (snapshot / 'retail/backend').rglob('*_test.go'):
        text = path.read_text().replace('127.0.0.1:6379', '127.0.0.1:16379')
        text = text.replace('eurorefurb:', c['redis_prefix'])
        text = text.replace('cart_runtime_rehearsal', c['database'])
        text = text.replace('19379', str(c['storefront_port'])).replace('19383', str(c['admin_port']))
        text = text.replace('0.0.0.0:9100', '127.0.0.1:' + str(c['api_port']))
        path.write_text(text)
    runtime = snapshot / 'retail/backend/internal/identity/cart_runtime_test.go'
    replace(runtime, 'cfg.User, cfg.Passwd, cfg.Net, cfg.Addr = "root", os.Getenv("MYSQL_PWD"), "unix", "/var/run/mysqld/mysqld.sock"',
            'cfg.User, cfg.Passwd, cfg.Net, cfg.Addr = os.Getenv("REVIEW_TEST_USER"), os.Getenv("MYSQL_PWD"), "tcp", "127.0.0.1:3306"')
    replace(runtime, 'duration = 6 * time.Minute', 'duration = 12 * time.Minute')
    fixture = snapshot / 'retail/backend/internal/identity/checkout_browser_test.go'
    replace(fixture, 'public.POST("/search", catalogue.Search)',
            'public.POST("/search", catalogue.Search)\n\tcleanupReviews := mountCheckoutReviewJoint(t, ctx, db, router)\n\tdefer cleanupReviews()')
    replace(fixture, 'checkCheckoutAccountReceipts(t, db, orderIDs)',
            'checkCheckoutAccountReceipts(t, db, orderIDs)\n\t\tcheckCheckoutReviewJoint(t, db, orderIDs)')
    for path in (snapshot / 'retail').rglob('*.mjs'):
        if 'node_modules' in path.parts:
            continue
        text = path.read_text().replace('19379', str(c['storefront_port'])).replace('19383', str(c['admin_port']))
        path.write_text(text)
    browser = snapshot / 'retail/storefront/tests/checkout.live.mjs'
    text = browser.read_text()
    text = "import {startReviewModeration,verifyCheckoutReviews} from './product-reviews.joint.mjs'\n" + text
    text = text.replace('let app,browser,fulfiller', 'let app,browser,fulfiller,reviewer')
    anchor = 'if(fulfillment)fulfiller=await startCheckoutFulfillment({browser,target:target.origin,errors})'
    if text.count(anchor) != 1:
        raise RuntimeError('Checkout browser startup anchor changed')
    text = text.replace(anchor, anchor + '\n reviewer=await startReviewModeration({browser,origin:"http://127.0.0.1:' + str(c['admin_port']) + '",errors,expect})')
    anchor = 'afterDetail:fulfillment?async()=>{const result='
    if text.count(anchor) != 1:
        raise RuntimeError('Checkout order hook changed')
    text = text.replace(anchor, 'afterDetail:fulfillment?async()=>{await verifyCheckoutReviews({page,context,shop,segment,orderID,index,admin:reviewer,control,expect});const result=')
    text = text.replace('finally{await fulfiller?.close();', 'finally{await reviewer?.close();await fulfiller?.close();')
    browser.write_text(text)
    adapt_admin_fixture(snapshot / 'retail/admin/tests/checkout-fulfillment.joint.mjs')
    for name, digest in original.items():
        if name.endswith('_test.go') or name.endswith('.mjs'):
            continue
        if sha(snapshot / name) != digest:
            raise RuntimeError('Production source changed during fixture adaptation: ' + name)
    c['source_sha256'] = original
    c['adapted_source_sha256'] = {name: sha(snapshot / name) for name in original if sha(snapshot / name) != original[name]}
    setup = snapshot / 'setup.sql'
    setup.write_text('\n'.join(f"CREATE USER '{c['user']}'@'{host}' IDENTIFIED BY '{c['password']}';\nGRANT ALL PRIVILEGES ON `{c['database']}`.* TO '{c['user']}'@'{host}';" for host in ['127.0.0.1', 'localhost']) + '\n')
    setup.chmod(0o600)
    cleanup = snapshot / 'cleanup.sql'
    cleanup.write_text(f"DROP DATABASE IF EXISTS `{c['database']}`;\n" + '\n'.join(f"DROP USER IF EXISTS '{c['user']}'@'{host}';" for host in ['127.0.0.1', 'localhost']) + '\n' + f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{c['database']}';\nSELECT COUNT(*) FROM mysql.user WHERE user='{c['user']}';\n")
    cleanup.chmod(0o600)
    private_json(STATE, c)
    print(json.dumps({k: c[k] for k in ['owner', 'database', 'snapshot', 'api_port', 'storefront_port', 'admin_port']}))


def run(root_fixture=False, reset_owned_db=False):
    c = state()
    snapshot = Path(c['snapshot'])
    runner_sha = sha(Path(__file__))
    expected = {name: c['adapted_source_sha256'].get(name, digest) for name, digest in c['source_sha256'].items()}
    def frozen():
        if sha(Path(__file__)) != runner_sha:
            raise RuntimeError('Running controller source changed')
        for name, digest in expected.items():
            if sha(snapshot / name) != digest:
                raise RuntimeError('Frozen source changed: ' + name)
    frozen()
    env = os.environ.copy()
    env.update(PATH='/opt/homebrew/opt/node/bin:/opt/homebrew/opt/go/bin:' + env['PATH'],
               MYSQL_PWD=c['password'], REVIEW_TEST_USER=c['user'],
               IDENTITY_REHEARSAL='1', CHECKOUT_BROWSER_FIXTURE='1',
               CHECKOUT_PRODUCT_BROWSE='1', CHECKOUT_FRESH_CART='1',
               CHECKOUT_ACCOUNTS='1', CHECKOUT_FULFILLMENT='1', CHECKOUT_REVIEWS='1',
               CHECKOUT_FIXTURE_KEY=c['fixture_key'],
               CUSTOMER_ORDERS_TEST_GIN='http://127.0.0.1:' + str(c['api_port']),
               ADMIN_TEST_BROWSER='/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
               ASTRO_TELEMETRY_DISABLED='1')
    for key in ['CHECKOUT_EXCHANGE', 'CHECKOUT_EXCHANGE_PAYMENT', 'CHECKOUT_EXCHANGE_REFUND', 'CHECKOUT_REFUND', 'CHECKOUT_REPAIR']:
        env[key] = '0'
    for port in [c['api_port'], c['storefront_port'], c['admin_port']]:
        if port in [8000, 9001, 9100, 9200, 9300, 19379, 19383]:
            raise RuntimeError('Reserved service port rejected')
        with socket.socket() as sock:
            sock.bind(('127.0.0.1', port))
    absent = subprocess.run([MYSQL, '--host=127.0.0.1', '--port=3306', '--user=' + c['user'], '--batch', '--skip-column-names', '--execute', "SELECT schema_name FROM information_schema.schemata WHERE schema_name='" + c['database'] + "'"], env=env, capture_output=True, text=True)
    if reset_owned_db and not root_fixture:
        raise RuntimeError('Owned database reset requires the hidden root fixture mode')
    if absent.returncode or absent.stdout.strip() and not reset_owned_db:
        raise RuntimeError('Owned user setup missing or database already exists; never reuse test facts')
    log = ROOT / 'log'
    log.mkdir(exist_ok=True)
    processes = []
    def terminate(process):
        if process.poll() is None:
            os.killpg(process.pid, signal.SIGTERM)
            try: process.wait(timeout=5)
            except subprocess.TimeoutExpired: os.killpg(process.pid, signal.SIGKILL); process.wait(timeout=5)
    try:
        with (log / 'product-reviews-joint-build.log').open('w') as output:
            built = subprocess.run([NODE, 'node_modules/astro/bin/astro.mjs', 'build'], cwd=snapshot / 'retail/storefront', env=env, stdout=output, stderr=subprocess.STDOUT, timeout=120)
        if built.returncode:
            raise RuntimeError('Owned Astro build failed; see joint build log')
        backend_env = env.copy()
        if root_fixture:
            if not sys.stdin.isatty():
                raise RuntimeError('Root fixture credentials require a hidden terminal prompt')
            # Existing SQL rollback tests create fault-injection triggers. On
            # binlogged local MySQL they require SUPER; never grant it to the
            # disposable user or change a server-global setting. The audited
            # snapshot creates/uses only the manifest's unpredictable database.
            backend_env['MYSQL_PWD'] = getpass.getpass('Owned fixture root password (hidden): ')
            backend_env['REVIEW_TEST_USER'] = 'root'
            if reset_owned_db:
                reset = subprocess.run([MYSQL, '--user=root', '--execute', 'DROP DATABASE IF EXISTS `' + c['database'] + '`'], env=backend_env, capture_output=True)
                if reset.returncode:
                    raise RuntimeError('Owned database reset failed; credentials suppressed')
        with (log / 'product-reviews-joint-sql.log').open('w') as output:
            backend = subprocess.Popen([GO, 'test', './internal/identity', '-run', '^TestOwnedCartRuntime$', '-count=1', '-timeout', '15m', '-v'], cwd=snapshot / 'retail/backend', env=backend_env, stdout=output, stderr=subprocess.STDOUT, start_new_session=True)
            processes.append(backend)
            backend_env.pop('MYSQL_PWD', None)
            ready = False
            for _ in range(300):
                if backend.poll() is not None:
                    raise RuntimeError('Owned checkout fixture stopped before readiness; see SQL log')
                try:
                    request = Request(env['CUSTOMER_ORDERS_TEST_GIN'] + '/__fixture/ready', headers={'X-Fixture-Key': c['fixture_key']})
                    with urlopen(request, timeout=1) as response:
                        ready = response.status == 200
                    if ready: break
                except OSError: pass
                time.sleep(.2)
            if not ready:
                raise RuntimeError('Owned checkout fixture readiness deadline')
            with (log / 'product-reviews-joint-browser.log').open('w') as browser_output:
                browser_env = {key: value for key, value in env.items() if key not in ['MYSQL_PWD', 'REVIEW_TEST_USER', 'MYSQL_DSN']}
                browser = subprocess.Popen([NODE, 'tests/checkout.live.mjs'], cwd=snapshot / 'retail/storefront', env=browser_env, stdout=browser_output, stderr=subprocess.STDOUT, start_new_session=True)
                processes.append(browser)
                if browser.wait(timeout=660):
                    raise RuntimeError('Owned review browser joint failed; see browser log')
            if backend.wait(timeout=90):
                raise RuntimeError('Owned checkout/review SQL postconditions failed; see SQL log')
        frozen()
        browser_text = (log / 'product-reviews-joint-browser.log').read_text()
        sql_text = (log / 'product-reviews-joint-sql.log').read_text()
        cases = [json.loads(line.split('PRODUCT_REVIEWS_JOINT_CASE ', 1)[1]) for line in browser_text.splitlines() if line.startswith('PRODUCT_REVIEWS_JOINT_CASE ')]
        browser_results = [json.loads(line.split('CHECKOUT_JOINT_BROWSER ', 1)[1]) for line in browser_text.splitlines() if line.startswith('CHECKOUT_JOINT_BROWSER ')]
        sql_results = [json.loads(line.split('PRODUCT_REVIEWS_JOINT_SQL ', 1)[1]) for line in sql_text.splitlines() if 'PRODUCT_REVIEWS_JOINT_SQL ' in line]
        if len(cases) != 4 or len(browser_results) != 1 or len(sql_results) != 1 or browser_results[0].get('browser_errors') != 0 or '\nPASS\n' not in sql_text:
            raise RuntimeError('Native review joint evidence incomplete')
        private_json(snapshot / 'joint-result.json', dict(cases=cases, checkout_browser=browser_results[0], review_sql=sql_results[0], runner_sha256=runner_sha, frozen_source_before_build_and_after_run=True))
        print(json.dumps({'browser': 'passed', 'native_sql': 'passed', 'database_cleanup': 'root cleanup.sql still required'}))
    finally:
        for process in reversed(processes): terminate(process)
        # Delete only this unpredictable owned prefix, never FLUSHDB or KEYS *.
        redis = '/opt/homebrew/bin/redis-cli'
        found = subprocess.run([redis, '-h', '127.0.0.1', '-p', '16379', '--scan', '--pattern', c['redis_prefix'] + '*'], capture_output=True, text=True)
        if found.returncode:
            raise RuntimeError('Owned Redis cleanup scan failed')
        keys = found.stdout.splitlines()
        if any(not key.startswith(c['redis_prefix']) for key in keys):
            raise RuntimeError('Unowned Redis key rejected')
        for start in range(0, len(keys), 100):
            deleted = subprocess.run([redis, '-h', '127.0.0.1', '-p', '16379', 'DEL', *keys[start:start + 100]], capture_output=True)
            if deleted.returncode: raise RuntimeError('Owned Redis cleanup failed')
        private_json(snapshot / 'process-cleanup.json', {'owned_processes_terminal': all(p.poll() is not None for p in processes), 'owned_redis_keys_deleted': len(keys), 'main_services_touched': False})


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('mode', choices=['prepare', 'run'])
    parser.add_argument('--root-fixture', action='store_true', help='Hidden prompt for existing trigger-based SQL rollback tests only')
    parser.add_argument('--reset-owned-db', action='store_true', help='Reset only the manifest database after a terminal failed run')
    args = parser.parse_args()
    prepare() if args.mode == 'prepare' else run(args.root_fixture, args.reset_owned_db)
