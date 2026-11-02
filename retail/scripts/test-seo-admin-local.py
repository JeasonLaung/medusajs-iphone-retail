"""Actual Ant/Gin/MySQL/Redis/Astro SEO joint on owned local resources.

Never inherits application configuration, uses shared MySQL, real providers,
legacy credentials, or the main preview. Builds only a frozen separate copy.
"""
import hashlib
import json
import os
from pathlib import Path
import secrets
import shutil
import signal
import socket
import subprocess
import tempfile
import time
import urllib.request

ROOT = Path(__file__).resolve().parents[2]
GO = '/opt/homebrew/opt/go/bin/go'
NODE = '/opt/homebrew/opt/node/bin/node'
MYSQL = Path('/opt/homebrew/bin/mysql').resolve()
REDIS = '/opt/homebrew/bin/redis-cli'


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def write(path, value):
    path.write_text(json.dumps(value, indent=2) + '\n')
    path.chmod(0o600)


def main():
    if shutil.disk_usage(ROOT).free < 2 * 1024**3:
        raise RuntimeError('2 GiB free required')
    owner = secrets.token_hex(6)
    base = ROOT / 'logs/gin-migration' / ('seo-admin-native-' + owner)
    base.mkdir(mode=0o700)
    env = {k: os.environ[k] for k in ['PATH', 'HOME', 'TMPDIR'] if k in os.environ}
    inventories = {}
    for app in ['backend', 'admin', 'storefront']:
        dest = base / 'retail' / app
        shutil.copytree(ROOT / 'retail' / app, dest, ignore=shutil.ignore_patterns(
            '.env*', '*.log', '__pycache__', 'node_modules', 'dist', '.astro', 'test-results', 'playwright-report'))
        inventories[app] = {str(p.relative_to(dest)): sha(p) for p in dest.rglob('*') if p.is_file()}
        if any(sha(ROOT / 'retail' / app / n) != h for n, h in inventories[app].items()):
            raise RuntimeError('Source changed during copy')
        if app != 'backend':
            (dest / 'node_modules').symlink_to(ROOT / 'retail' / app / 'node_modules', target_is_directory=True)
    backend, admin, storefront = [base / 'retail' / app for app in ['backend', 'admin', 'storefront']]
    sockets = Path(tempfile.mkdtemp(prefix='erseo-' + owner + '-', dir='/private/tmp'))
    address, database = sockets / 'mysql.sock', 'eurorefurb_seo_' + owner
    prefix = 'eurorefurb:s' + owner + ':v1:'
    # Only the existing browser fixture's fixed test connection/name changes.
    fixture = backend / 'internal/identity/browser_fixture_test.go'
    fixture.write_text(fixture.read_text().replace('/var/run/mysqld/mysqld.sock', str(address)).replace('"browser_identity"', json.dumps(database)))
    subprocess.run([str(Path(GO).with_name('gofmt')), '-w', str(fixture)], check=True)
    adapted = {'internal/identity/browser_fixture_test.go': sha(fixture)}
    write(base / 'manifest.json', dict(owner=owner, source_sha256=inventories,
          adapted_tests=adapted, socket=str(address), database=database, redis_prefix=prefix,
          migrations=101, production_source_adapted=False))

    def frozen():
        for app, files in inventories.items():
            dest = base / 'retail' / app
            for name, digest in files.items():
                expected = adapted.get(name, digest) if app == 'backend' else digest
                if sha(dest / name) != expected:
                    raise RuntimeError('Frozen source changed: ' + app + '/' + name)

    def run(cmd, log, cwd=None, runenv=None, timeout=120):
        with (base / log).open('w') as out:
            subprocess.run(cmd, cwd=cwd, env=runenv or env, stdout=out, stderr=subprocess.STDOUT, check=True, timeout=timeout)

    data = base / 'mysql-data'
    server = api = astro = browser = None
    cleanup_error = None

    def mysql(query):
        p = subprocess.run([str(MYSQL), '--no-defaults', '--protocol=SOCKET', '--socket=' + str(address),
                            '-uroot', '--batch', '--skip-column-names'], input=query, text=True, env=env,
                           capture_output=True, timeout=30)
        if p.returncode:
            raise RuntimeError('Owned SEO SQL command failed')
        return p.stdout.strip()

    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))

    def ready(proc, url):
        deadline = time.monotonic() + 40
        while time.monotonic() < deadline:
            if proc.poll() is not None:
                raise RuntimeError('Owned process exited; inspect log')
            try:
                with opener.open(url, timeout=3) as r:
                    if r.status == 200:
                        return
            except OSError:
                pass
            time.sleep(0.2)
        raise RuntimeError('Owned process readiness timed out')

    try:
        print(json.dumps(dict(owner=owner, phase='build')), flush=True)
        run([GO, 'test', '-c', '-o', str(base / 'identity.test'), './internal/identity'], 'compile.log', backend)
        run([GO, 'build', '-o', str(base / 'api'), './cmd/api'], 'api-compile.log', backend)
        run([NODE, 'node_modules/astro/bin/astro.mjs', 'build'], 'astro-build.log', storefront, dict(env, ASTRO_TELEMETRY_DISABLED='1'))
        data.mkdir(mode=0o700)
        run([str(MYSQL.with_name('mysqld')), '--no-defaults', '--initialize-insecure', '--datadir=' + str(data),
             '--innodb-buffer-pool-size=64M', '--innodb-redo-log-capacity=64M', '--log-error=' + str(base / 'mysql-initialize.log')], 'initialize.log')
        with (base / 'mysql-launch.log').open('w') as out:
            server = subprocess.Popen([str(MYSQL.with_name('mysqld')), '--no-defaults', '--datadir=' + str(data),
                '--socket=' + str(address), '--pid-file=' + str(base / 'mysql.pid'), '--log-error=' + str(base / 'mysql.log'),
                '--skip-networking', '--mysqlx=OFF', '--skip-log-bin', '--event-scheduler=OFF',
                '--innodb-buffer-pool-size=64M', '--innodb-redo-log-capacity=64M', '--innodb-native-foreign-keys=ON'],
                env=env, stdout=out, stderr=subprocess.STDOUT)
        deadline = time.monotonic() + 45
        while time.monotonic() < deadline:
            if server.poll() is not None:
                raise RuntimeError('Owned MySQL exited')
            if address.exists():
                try:
                    facts = mysql('SELECT VERSION(),@@innodb_native_foreign_keys,@@foreign_key_checks,@@skip_networking')
                    if facts.split('\t')[1:] == ['1', '1', '1']:
                        write(base / 'mysql-runtime.json', dict(version=facts.split('\t')[0], native_foreign_keys=True, foreign_key_checks=True, networking_disabled=True, pid=server.pid))
                        break
                except RuntimeError:
                    pass
            time.sleep(0.2)
        else:
            raise RuntimeError('Owned MySQL readiness timed out')
        mysql('CREATE DATABASE `' + database + '`')
        fixture_env = dict(env, BROWSER_IDENTITY_FIXTURE='1', MYSQL_PWD='')
        for test in ['TestPrepareOwnedBrowserAPI', 'TestPrepareReviewedCatalogFixture']:
            run([str(base / 'identity.test'), '-test.run', '^' + test + '$', '-test.v', '-test.timeout', '90s'], test + '.log', backend, fixture_env)
            log = (base / (test + '.log')).read_text()
            if '--- PASS: ' + test not in log or '--- SKIP' in log:
                raise RuntimeError('Owned browser fixture not executed')
        mysql('USE `' + database + '`; UPDATE cart_command_capabilities SET enabled=FALSE; UPDATE seo_command_state SET enabled=TRUE WHERE id=1;')
        if subprocess.check_output([REDIS, '-p', '16379', 'PING'], text=True).strip() != 'PONG' or subprocess.check_output([REDIS, '-p', '16379', '--scan', '--pattern', prefix + '*'], text=True).strip():
            raise RuntimeError('Fresh owned Redis namespace required')
        ports = []
        for _ in range(3):
            with socket.socket() as sock:
                sock.bind(('127.0.0.1', 0)); ports.append(sock.getsockname()[1])
        if len(set(ports)) != 3:
            raise RuntimeError('Distinct owned ports required')
        api_origin, admin_origin, astro_origin = ['http://127.0.0.1:' + str(p) for p in ports]
        api_env = dict(env, APP_BIND='127.0.0.1', APP_PORT=str(ports[0]), APP_ENV='development', GIN_MODE='release',
            MYSQL_DSN='root@unix(' + str(address) + ')/' + database + '?parseTime=true&loc=UTC',
            SESSION_SECURE='false', SESSION_REDIS_URL='redis://127.0.0.1:16379/0', SESSION_REDIS_PREFIX=prefix,
            ADMIN_ORIGIN=admin_origin, STOREFRONT_STORE_ID='fixture-store',
            MERCHANT_KEY_ACTIVE='artificial', MERCHANT_KEYRING_JSON=json.dumps({'artificial': '71' * 32}))
        # No commerce origin or provider/worker gates; site URL is saved using
        # the actual Admin form, then both metadata and discovery must use it.
        with (base / 'api.log').open('w') as out:
            api = subprocess.Popen([str(base / 'api')], cwd=backend, env=api_env, stdout=out, stderr=subprocess.STDOUT)
        ready(api, api_origin + '/health')
        with (base / 'astro.log').open('w') as out:
            astro = subprocess.Popen([NODE, 'scripts/start.mjs'], cwd=storefront,
                env=dict(env, HOST='127.0.0.1', PORT=str(ports[2]), GIN_API_ORIGIN=api_origin, ASTRO_TELEMETRY_DISABLED='1'), stdout=out, stderr=subprocess.STDOUT)
        ready(astro, astro_origin + '/dk-en/about')
        print(json.dumps(dict(owner=owner, phase='browser')), flush=True)
        browser_env = dict(env, ADMIN_TEST_PORT=str(ports[1]), ADMIN_TEST_API_TARGET=api_origin, SEO_ASTRO_ORIGIN=astro_origin, SEO_BROWSER_OUTPUT=str(base))
        with (base / 'browser.log').open('w') as out:
            browser = subprocess.Popen([NODE, 'tests/seo-admin.native.mjs'], cwd=admin, env=browser_env, stdout=out, stderr=subprocess.STDOUT, start_new_session=True)
            browser.wait(timeout=240)
        if browser.returncode:
            raise RuntimeError('Actual SEO browser joint failed; inspect browser.log')
        proof = json.loads((base / 'browser-result.json').read_text())
        if not proof.get('actual_admin_gin_mysql_redis_astro') or proof.get('api_mocking') or proof.get('errors'):
            raise RuntimeError('Actual joint proof missing')
        receipts = mysql('SELECT COUNT(*) FROM `' + database + '`.seo_command_receipts;')
        if int(receipts) != proof['unique_successful_operations']:
            raise RuntimeError('Browser operations and SQL receipt count differ')
        # Reconcile raw JSON-LD and envelope storage independently of the UI.
        exact = mysql("SELECT COUNT(*) FROM `" + database + "`.seo_pages WHERE structured_data LIKE '%900719925474099312345%' AND deleted_at IS NULL;")
        plaintext = mysql("SELECT COUNT(*) FROM `" + database + "`.seo_settings WHERE CAST(value AS CHAR) LIKE '%Native SEO Store%';")
        if exact != '2' or plaintext != '0':
            raise RuntimeError('Exact JSON or encrypted storage reconciliation failed')
        frozen()
        write(base / 'result.json', dict(owner=owner, migrations=101, actual_browser_joint=True, browser=proof,
              sql_receipts=int(receipts), exact_jsonld_integer_verified=True, settings_encrypted=True,
              source_verified=True, api_binary_sha256=sha(base / 'api'), test_binary_sha256=sha(base / 'identity.test'),
              real_provider_called=False, main_mysql_modified=False, goal_complete=False))
        print(json.dumps(dict(owner=owner, passed=True, sql_receipts=int(receipts))), flush=True)
    finally:
        for proc in [browser, astro, api]:
            if proc is not None and proc.poll() is None:
                if proc is browser:
                    os.killpg(proc.pid, signal.SIGTERM)
                else:
                    proc.terminate()
                try:
                    proc.wait(timeout=15)
                except subprocess.TimeoutExpired:
                    if proc is browser:
                        os.killpg(proc.pid, signal.SIGKILL)
                    else:
                        proc.kill()
                    proc.wait(timeout=10)
        try:
            keys = subprocess.check_output([REDIS, '-p', '16379', '--scan', '--pattern', prefix + '*'], text=True, timeout=10).splitlines()
            if any(not k.startswith(prefix) for k in keys):
                raise RuntimeError('Unowned Redis key rejected')
            for i in range(0, len(keys), 100):
                subprocess.run([REDIS, '-p', '16379', 'DEL', *keys[i:i + 100]], check=True, capture_output=True, timeout=10)
            if subprocess.check_output([REDIS, '-p', '16379', '--scan', '--pattern', prefix + '*'], text=True, timeout=10).strip():
                raise RuntimeError('Owned Redis cleanup incomplete')
        except (OSError, subprocess.SubprocessError, RuntimeError) as e:
            cleanup_error = type(e).__name__
        if server is not None and server.poll() is None:
            try:
                subprocess.run([str(MYSQL.with_name('mysqladmin')), '--no-defaults', '--protocol=SOCKET', '--socket=' + str(address), '-uroot', 'shutdown'], env=env, capture_output=True, timeout=15)
            except subprocess.TimeoutExpired:
                pass
            try:
                server.wait(timeout=30)
            except subprocess.TimeoutExpired:
                server.terminate(); server.wait(timeout=15)
        terminal = server is None or server.poll() is not None
        if terminal:
            if data.exists(): shutil.rmtree(data)
            if sockets.exists() and not list(sockets.iterdir()): sockets.rmdir()
        write(base / 'cleanup.json', dict(server_terminal=terminal, data_removed=not data.exists(), socket_removed=not address.exists(),
            api_terminal=api is None or api.poll() is not None, astro_terminal=astro is None or astro.poll() is not None,
            browser_terminal=browser is None or browser.poll() is not None, redis_cleanup_error=cleanup_error, main_mysql_unchanged=True))
        if cleanup_error:
            raise RuntimeError('Owned Redis cleanup failed; MySQL cleanup still performed')


if __name__ == '__main__':
    main()
