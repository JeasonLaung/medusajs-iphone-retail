"""Windows native account subflow against a disposable copy of the preview DB.

Requires the running dedicated MySQL container, native Redis, Go-built preview
API, frontend dependencies, Node >=22 and Python cryptography. No mail is sent.
--scenario=buyback and --scenario=buyback-return independently rehearse customer
applications through Admin closing with explicit prices/devices in the copy.
Verification links are read from authenticated encrypted intents in the owned
copy, never returned by an application endpoint or marked verified with SQL.
"""
import json
import hashlib
import os
from pathlib import Path
import re
import secrets
import shutil
import socket
import struct
import subprocess
import sys
import time
import urllib.request

ROOT = Path(__file__).resolve().parents[2]
CONTAINER = 'eurorefurb-retail-local-mysql'
DB_PATTERN = r'retail_account_[0-9a-f]{12}'


def config():
    values = {}
    for line in (ROOT / 'retail/backend/.env').read_text(encoding='utf-8-sig').splitlines():
        if re.match(r'^[A-Z_]+=', line):
            name, value = line.split('=', 1)
            value = value.strip()
            if len(value) >= 2 and value[0] == value[-1] and value[0] in "\"'":
                value = value[1:-1]
            values[name] = value
    if values.get('APP_ENV') != 'development' or not re.search(
        r'@tcp\(127\.0\.0\.1:13307\)/eurorefurb_local_trial\?', values.get('MYSQL_DSN', '')
    ):
        raise RuntimeError('Dedicated Windows development configuration required')
    return values


def docker(*args, data=None):
    result = subprocess.run(['docker', *args], input=data, capture_output=True, timeout=120)
    if result.returncode:
        raise RuntimeError('Owned database operation failed; credentials/output suppressed')
    return result.stdout


def sql(database, statement):
    if not re.fullmatch(DB_PATTERN, database):
        raise RuntimeError('Owned account database required')
    return docker('exec', CONTAINER, 'mysql', '--defaults-extra-file=/tmp/local-root.cnf',
                  '--batch', '--skip-column-names', database, '-e', statement).decode().strip()


def source_account_counts():
    tables = ('customer_accounts', 'customer_verifications', 'customer_verification_mail',
              'customer_account_addresses', 'customer_order_ownership', 'buyback_requests',
              'buyback_prices', 'buyback_photo_bytes', 'buyback_device_links')
    statement = 'SELECT ' + ','.join('(SELECT COUNT(*) FROM ' + table + ')' for table in tables)
    values = docker('exec', CONTAINER, 'mysql', '--defaults-extra-file=/tmp/local-root.cnf',
                    '--batch', '--skip-column-names', 'eurorefurb_local_trial', '-e', statement)
    return dict(zip(tables, map(int, values.decode().strip().split('\t'))))


def verification_link(database, email, output):
    from cryptography.hazmat.primitives.ciphers.aead import AESGCM
    if not re.fullmatch(r'local-account-[0-9a-f]{12}-[0-9]+@example\.invalid', email):
        raise RuntimeError('Isolated fixture email required')
    row = sql(database, "SELECT m.id,HEX(m.payload) FROM customer_verification_mail m "
              "JOIN customer_accounts c ON c.id=m.customer_id WHERE c.email='" + email + "'")
    identifier, encoded = row.split('\t')
    envelope = bytes.fromhex(encoded)
    if envelope[:4] != b'ERS\x01':
        raise RuntimeError('Authenticated envelope required')
    end = 5 + envelope[4]
    key_id = envelope[5:end].decode()
    keys = json.loads(config()['CUSTOMER_KEYRING_JSON'])
    parts = [b'eurorefurb.business-secret.v1', envelope[:end], b'customer-verification',
             b'customer_verification_mail', b'payload', identifier.encode()]
    aad = b''.join(struct.pack('>I', len(part)) + part for part in parts)
    plain = AESGCM(bytes.fromhex(keys[key_id])).decrypt(envelope[end:end+12], envelope[end+12:], aad)
    intent = json.loads(plain)
    link = intent.get('URL', intent.get('url'))
    if not link or not link.startswith('http://127.0.0.1:19300/'):
        raise RuntimeError('Owned storefront link required')
    Path(output).write_text(link, encoding='utf-8')


def redis_cleanup(prefix):
    # Exact owned namespace only, no FLUSH or cross-namespace operations.
    if not re.fullmatch(r'eurorefurb:account-[0-9a-f]{12}:v1:', prefix):
        raise RuntimeError('Owned Redis prefix required')
    with socket.create_connection(('127.0.0.1', 6379), timeout=5) as connection:
        stream = connection.makefile('rb')
        def read():
            kind, line = stream.read(1), stream.readline().rstrip(b'\r\n')
            if kind == b'*':
                return [read() for _ in range(int(line))]
            if kind == b'$':
                length = int(line)
                if length < 0:
                    return None
                result = stream.read(length)
                stream.read(2)
                return result
            if kind in (b':', b'+'):
                return line
            raise RuntimeError('Redis cleanup failed')
        def command(*parts):
            values = [p.encode() if isinstance(p, str) else p for p in parts]
            connection.sendall(b'*' + str(len(values)).encode() + b'\r\n' + b''.join(
                b'$' + str(len(p)).encode() + b'\r\n' + p + b'\r\n' for p in values))
            return read()
        cursor, keys = b'0', []
        while True:
            cursor, batch = command('SCAN', cursor, 'MATCH', prefix + '*', 'COUNT', '100')
            keys.extend(batch)
            if cursor == b'0':
                break
        for key in keys:
            if not key.startswith(prefix.encode()):
                raise RuntimeError('Unexpected Redis key')
            command('DEL', key)
        return len(keys)


def main(language, scenario='account'):
    buyback = scenario in ('buyback', 'buyback-return')
    values = config()
    owner = secrets.token_hex(6)
    database = 'retail_account_' + owner
    prefix = 'eurorefurb:account-' + owner + ':v1:'
    logs = ROOT / 'logs/retail-native' / ('account-' + owner)
    logs.mkdir()
    storefront = logs / 'storefront'
    storefront.mkdir()
    source = ROOT / 'retail/storefront'
    shutil.copytree(source / 'src', storefront / 'src')
    for name in ('package.json', 'tsconfig.json'):
        shutil.copy2(source / name, storefront / name)
    astro_config = (source / 'astro.config.mjs').read_text(encoding='utf-8')
    allowed = json.dumps([str(storefront).replace('\\', '/'), str(source / 'node_modules').replace('\\', '/')])
    astro_config = astro_config.replace("output:'server',", "output:'server',vite:{cacheDir:'.cache/vite',server:{fs:{allow:" + allowed + "}}},")
    (storefront / 'astro.config.mjs').write_text(astro_config, encoding='utf-8')
    # Shared dependencies/assets are read-only; generated Astro/Vite state is
    # inside the owned source copy. Never replace the user's running dev server.
    for name in ('node_modules', 'public'):
        junction = subprocess.run(['powershell', '-NoProfile', '-Command',
            'New-Item -ItemType Junction -Path $env:ACCOUNT_JUNCTION_PATH -Target $env:ACCOUNT_JUNCTION_TARGET | Out-Null'],
            env=dict(os.environ, ACCOUNT_JUNCTION_PATH=str(storefront / name),
                     ACCOUNT_JUNCTION_TARGET=str(source / name)), capture_output=True)
        if junction.returncode:
            raise RuntimeError('Owned read-only dependency junction failed')
    node = Path(os.environ.get('STOREFRONT_TEST_NODE') or
                Path.home() / '.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node.exe')
    processes, streams = [], []
    created = False
    source_counts = source_account_counts()
    report = {'owner': owner, 'database': database, 'real_email_delivery': False,
              'business_api_mocking': False, 'main_database_modified': False,
              'api_sha256': hashlib.sha256((ROOT / 'logs/retail-native/retail-api.exe').read_bytes()).hexdigest()}
    try:
        # Fail before creating resources when another rehearsal owns these ports.
        for port in ((19100, 19200, 19300) if buyback else (19100, 19300)):
            with socket.socket() as check:
                check.bind(('127.0.0.1', port))
        snapshot = '/tmp/account-' + owner + '.sql'
        docker('exec', CONTAINER, 'mysqldump', '--defaults-extra-file=/tmp/local-root.cnf',
               '--single-transaction', '--set-gtid-purged=OFF', '--skip-triggers',
               '--result-file=' + snapshot, 'eurorefurb_local_trial')
        docker('cp', CONTAINER + ':' + snapshot, str(logs / 'snapshot.sql'))
        docker('exec', CONTAINER, 'mysql', '--defaults-extra-file=/tmp/local-root.cnf', '-e',
               'CREATE DATABASE ' + database + "; GRANT ALL ON " + database + ".* TO 'retail_local'@'%';")
        created = True
        docker('exec', CONTAINER, 'sh', '-c',
               'mysql --defaults-extra-file=/tmp/local-root.cnf ' + database + ' < ' + snapshot)
        if buyback:
            sql(database, "UPDATE buyback_request_runtime SET enabled=TRUE WHERE id=1 AND version=1; "
                "UPDATE buyback_admin_runtime SET enabled=TRUE WHERE id=1 AND version=1; "
                "INSERT INTO buyback_prices(id,product_id,capacity,amount,raw_amount,currency_code,created_at,updated_at) "
                "SELECT 'isolated-account-buyback-price',id,'256GB',294,'{\"value\":\"294\",\"precision\":100}','eur',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6) "
                "FROM catalog_products WHERE handle='swappie-iphone-13' AND deleted_at IS NULL; "
                "INSERT INTO device_sources(id,name,type,location_id,created_at,updated_at) VALUES"
                "('isolated-buyback-source','Isolated buyback fixture','self','sloc_01M4FBP3NDPZHJKVV3VY682QTN',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)); "
                "INSERT INTO devices(id,serial_number,imei,variant_id,source_id,location_id,cost_cents,currency_code,bin,notes,state,category,photos,created_at,updated_at) "
                "SELECT 'isolated-buyback-device','SN-JOINT','000000000000001',v.id,'isolated-buyback-source','sloc_01M4FBP3NDPZHJKVV3VY682QTN',0,'eur','','','inspection','recycled','[]',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6) "
                "FROM catalog_variants v JOIN catalog_products p ON p.id=v.product_id JOIN catalog_variant_options vo ON vo.variant_id=v.id "
                "JOIN catalog_option_values ov ON ov.id=vo.option_value_id WHERE p.handle='swappie-iphone-13' "
                "AND REPLACE(ov.value,' ','')='256GB' AND v.deleted_at IS NULL ORDER BY v.id LIMIT 1;")
        env = dict(os.environ, **values)
        env.update(MYSQL_DSN=values['MYSQL_DSN'].replace('/eurorefurb_local_trial?', '/' + database + '?'),
                   APP_PORT='19100', ADMIN_ORIGIN='http://127.0.0.1:19200',
                   STOREFRONT_ORIGIN='http://127.0.0.1:19300', SESSION_REDIS_PREFIX=prefix,
                   MOLLIE_BACKEND_ORIGIN='http://127.0.0.1:19100', CHECKOUT_PAYMENT_ENABLED='false',
                   CUSTOMER_AUTH_ENABLED='true', CUSTOMER_VERIFICATION_MAIL_ENABLED='false',
                   NOTIFICATION_MAIL_ENABLED='false', ORDER_MAIL_ENABLED='false',
                   GIN_API_ORIGIN='http://127.0.0.1:19100')
        services = [
            ('api', [str(ROOT / 'logs/retail-native/retail-api.exe')], ROOT / 'retail/backend'),
            ('storefront', [str(node), 'node_modules/astro/bin/astro.mjs', 'dev', '--host', '127.0.0.1',
                            '--port', '19300'], storefront)]
        if buyback:
            env['ADMIN_TEST_API_TARGET'] = 'http://127.0.0.1:19100'
            services.append(('admin', [str(node), 'node_modules/vite/bin/vite.js', '--host', '127.0.0.1',
                                      '--port', '19200', '--strictPort'], ROOT / 'retail/admin'))
        for label, command, cwd in services:
            stream = open(logs / (label + '.log'), 'wb')
            streams.append(stream)
            processes.append(subprocess.Popen(command, cwd=cwd, env=env, stdout=stream, stderr=stream,
                                               creationflags=subprocess.CREATE_NO_WINDOW))
        urls = ['http://127.0.0.1:19100/health', 'http://127.0.0.1:19300/gb-en/account']
        if buyback:
            urls.append('http://127.0.0.1:19200')
        for url in urls:
            for _ in range(90):
                if any(p.poll() is not None for p in processes):
                    raise RuntimeError('Owned service exited; inspect ignored service logs')
                try:
                    with urllib.request.urlopen(url, timeout=3) as response:
                        if response.status == 200:
                            break
                except Exception:
                    time.sleep(.5)
            else:
                raise RuntimeError('Owned service startup timeout')
        browser_env = dict(env, ACCOUNT_TEST_OWNER=owner, ACCOUNT_TEST_DB=database,
                           ACCOUNT_TEST_LOGS=str(logs), ACCOUNT_TEST_PYTHON=sys.executable,
                           ACCOUNT_TEST_LANGUAGE=language, ACCOUNT_TEST_SCENARIO=scenario)
        script = 'tests/local-buyback.live.mjs' if buyback else 'tests/local-customer-account.live.mjs'
        result = subprocess.run([str(node), script],
                                cwd=ROOT / 'retail/storefront', env=browser_env,
                                stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=240)
        (logs / 'browser.log').write_bytes(result.stdout)
        if result.returncode:
            raise RuntimeError('Account browser check failed; inspect ignored browser.log')
        report.update(json.loads((logs / 'browser-result.json').read_text()))
        report['verified_accounts'] = int(sql(database, "SELECT COUNT(*) FROM customer_accounts "
             "WHERE email LIKE 'local-account-" + owner + "-%@example.invalid' AND verified_at IS NOT NULL"))
        if report['verified_accounts'] != 1:
            raise RuntimeError('Expected exactly one newly verified fixture account')
        if buyback:
            request = report['request_id']
            if not re.fullmatch(r'bbr_[0-9a-f-]{36}', request):
                raise RuntimeError('Unexpected owned request ID')
            rows = sql(database, "SELECT status,CAST(final_amount AS CHAR),payment_reference,return_tracking,closed_at IS NOT NULL "
                       "FROM buyback_requests WHERE id='" + request + "'").split('\t')
            expected = 'returned' if scenario == 'buyback-return' else 'paid'
            if rows[0] != expected or rows[-1] != '1' or float(rows[1]) != 270.01:
                raise RuntimeError('Buyback SQL terminal state differs')
            report['sql_terminal_status'] = rows[0]
            report['sql_final_amount'] = rows[1]
            report['sql_closed'] = True
            report['sql_private_photos'] = int(sql(database, 'SELECT COUNT(*) FROM buyback_photo_bytes'))
            report['sql_device_links'] = int(sql(database, "SELECT COUNT(*) FROM buyback_device_links WHERE request_id='" + request + "'"))
            if report['sql_private_photos'] != 3 or report['sql_device_links'] != 1:
                raise RuntimeError('Buyback photo/device facts differ')
    finally:
        for process in reversed(processes):
            if process.poll() is None:
                subprocess.run(['taskkill', '/PID', str(process.pid), '/T', '/F'],
                               capture_output=True, timeout=15)
                process.wait(timeout=10)
        for stream in streams:
            stream.close()
        cleanup_errors = []
        try:
            report['redis_keys_removed'] = redis_cleanup(prefix)
        except Exception:
            cleanup_errors.append('Redis cleanup')
        try:
            if created:
                docker('exec', CONTAINER, 'mysql', '--defaults-extra-file=/tmp/local-root.cnf', '-e',
                       'DROP DATABASE ' + database + "; REVOKE ALL ON " + database + ".* FROM 'retail_local'@'%';")
            docker('exec', CONTAINER, 'rm', '-f', '/tmp/account-' + owner + '.sql')
        except Exception:
            cleanup_errors.append('database cleanup')
        report['source_account_counts_before'] = source_counts
        report['source_account_counts_after'] = source_account_counts()
        report['resources_removed'] = not cleanup_errors
        report['cleanup_errors'] = cleanup_errors
        (logs / 'report.json').write_text(json.dumps(report, indent=2), encoding='utf-8')
        print('Account rehearsal evidence: ' + str(logs / 'report.json'))
        if cleanup_errors:
            raise RuntimeError('Owned cleanup incomplete: ' + ', '.join(cleanup_errors))
        if report['source_account_counts_after'] != source_counts:
            raise RuntimeError('Source account counts changed during rehearsal; investigate concurrent activity')


if __name__ == '__main__':
    if len(sys.argv) == 5 and sys.argv[1] == '--verification-link':
        verification_link(sys.argv[2], sys.argv[3], sys.argv[4])
    elif len(sys.argv) == 1:
        for language in ('en', 'zh'):
            main(language)
    elif sys.argv[1:] in (['--language=en'], ['--language=zh']):
        main(sys.argv[1].split('=')[1])
    elif sys.argv[1:] == ['--scenario=buyback']:
        main('en', 'buyback')
    elif sys.argv[1:] == ['--scenario=buyback-return']:
        main('en', 'buyback-return')
    else:
        raise RuntimeError('Unknown account rehearsal arguments')
