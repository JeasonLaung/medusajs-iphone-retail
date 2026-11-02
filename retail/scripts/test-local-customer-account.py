"""Windows native account subflow against a disposable copy of the preview DB.

Requires the running dedicated MySQL container, native Redis, Go-built preview
API, frontend dependencies, Node >=22 and Python cryptography. No mail is sent.
--scenario=buyback and --scenario=buyback-return independently rehearse customer
applications through Admin closing with explicit prices/devices in the copy.
--scenario=repair or --scenario=exchange continues a copied paid order through
explicit fixture account ownership, real fulfillment and repair/replacement; never claims ownership
of an actual guest order in the source preview.
--scenario=refund creates its own real sandbox payment; pending is reported as
unfinished, never posted. --resume-refund=OWNER restores an ignored checkpoint
and reuses the same paid order and durable refund instruction.
--scenario=warehouse uses the copied paid order and explicit fixture ownership
to verify a new scoped employee's allocation/release/shipment and customer view.
--scenario=mail uses actual Admin mailbox commands and production dispatchers
in separate test processes, replacing only the external Resend transport.
Verification links are read from authenticated encrypted intents in the owned
copy (or the captured dispatch body for mail), never returned by an application
endpoint or marked verified with SQL.
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
              'buyback_prices', 'buyback_photo_bytes', 'buyback_device_links',
              'after_sales_requests', 'device_return_receipts', 'device_service_receipts')
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


def prepare_repair_owner(database, email):
    # Explicit isolated test ownership, not an application claim-by-email flow.
    if not re.fullmatch(r'local-account-[0-9a-f]{12}-1@example\.invalid', email):
        raise RuntimeError('Isolated fixture email required')
    sql(database, "INSERT INTO customer_order_ownership(order_id,customer_id,cart_id,store_id,created_at) "
        "SELECT r.order_id,c.id,r.cart_id,x.store_id,UTC_TIMESTAMP(6) FROM customer_accounts c "
        "JOIN cart_completion_receipts r ON r.order_id='order_8bffc6dc-112e-4ed5-8a32-57421bc2ae4d' "
        "JOIN cart_ownership x ON x.cart_id=r.cart_id WHERE c.email='" + email + "' AND c.verified_at IS NOT NULL")


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


def main(language, scenario='account', resume=None):
    mail = scenario == 'mail'
    buyback = scenario in ('buyback', 'buyback-return')
    exchange = scenario == 'exchange'
    refund = scenario == 'refund'
    warehouse = scenario == 'warehouse'
    repair = scenario in ('repair', 'exchange', 'refund', 'warehouse')
    admin_required = buyback or repair or mail
    values = config()
    if resume is not None:
        if not refund or not re.fullmatch(r'[0-9a-f]{12}', resume):
            raise RuntimeError('Only an owned refund checkpoint can resume')
        checkpoint = ROOT / 'logs/retail-native' / ('account-' + resume)
        checkpoint_report = json.loads((checkpoint / 'report.json').read_text())
        paid = json.loads((checkpoint / 'refund-new-paid-order.json').read_text())
        if not re.fullmatch(r'order_[0-9a-f-]{36}', paid.get('order_id', '')) or not paid.get('real_mollie_test_payment'):
            raise RuntimeError('Owned paid sandbox checkpoint required')
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
    customer_owner = checkpoint_report.get('customer_owner', resume) if resume else owner
    report['customer_owner'] = customer_owner
    try:
        # Fail before creating resources when another rehearsal owns these ports.
        for port in ((19100, 19200, 19300) if admin_required else (19100, 19300)):
            with socket.socket() as check:
                check.bind(('127.0.0.1', port))
        snapshot = '/tmp/account-' + owner + '.sql'
        if resume:
            docker('cp', str(checkpoint / 'refund-final.sql'), CONTAINER + ':' + snapshot)
            shutil.copy2(checkpoint / 'refund-new-paid-order.json', logs / 'refund-new-paid-order.json')
            report['resumed_paid_checkpoint'] = resume
        else:
            docker('exec', CONTAINER, 'mysqldump', '--defaults-extra-file=/tmp/local-root.cnf',
                   '--single-transaction', '--set-gtid-purged=OFF', '--skip-triggers',
                   '--result-file=' + snapshot, 'eurorefurb_local_trial')
        docker('cp', CONTAINER + ':' + snapshot, str(logs / 'snapshot.sql'))
        docker('exec', CONTAINER, 'mysql', '--defaults-extra-file=/tmp/local-root.cnf', '-e',
               'CREATE DATABASE ' + database + "; GRANT ALL ON " + database + ".* TO 'retail_local'@'%';")
        created = True
        docker('exec', CONTAINER, 'sh', '-c',
               'mysql --defaults-extra-file=/tmp/local-root.cnf ' + database + ' < ' + snapshot)
        if resume:
            email = sql(database, "SELECT a.email FROM customer_accounts a JOIN customer_order_ownership o ON o.customer_id=a.id WHERE o.order_id='" + paid['order_id'] + "' AND a.verified_at IS NOT NULL")
            match = re.fullmatch(r'local-account-([0-9a-f]{12})-1@example.invalid', email)
            if not match:
                raise RuntimeError('Owned verified checkpoint customer required')
            customer_owner = match.group(1)
            report['customer_owner'] = customer_owner
            facts = sql(database, "SELECT (SELECT COUNT(*) FROM customer_order_ownership WHERE order_id='" + paid['order_id'] + "'),(SELECT COUNT(*) FROM order_refund_operations),(SELECT COUNT(*) FROM device_assignments)").split('\t')
            if facts not in (['1', '0', '0'], ['1', '1', '1']):
                raise RuntimeError('Unsupported owned refund checkpoint')
            existing_refund = facts[1] == '1'
            if existing_refund:
                row = sql(database, "SELECT request_id,operation_id FROM order_refund_operations WHERE order_id='" + paid['order_id'] + "'").split('\t')
                if len(row) != 2 or not re.fullmatch(r'asr_[0-9a-f-]{36}', row[0]) or not re.fullmatch(r'[0-9a-f-]{36}', row[1]):
                    raise RuntimeError('Owned refund receipt identity mismatch')
                (logs / 'refund-existing.json').write_text(json.dumps({'request_id': row[0], 'operation_id': row[1]}))
        if repair and not resume:
            sql(database, "UPDATE customer_after_sales_runtime SET enabled=TRUE WHERE id=1; "
                "UPDATE after_sales_review_runtime SET enabled=TRUE WHERE id=1; "
                "UPDATE device_return_runtime SET enabled=TRUE WHERE id=1; "
                "UPDATE device_service_runtime SET enabled=TRUE WHERE id=1; "
                "INSERT INTO device_sources(id,name,type,location_id,created_at,updated_at) VALUES"
                "('isolated-repair-source','Isolated repair fixture','self','sloc_01M4FBP3NDPZHJKVV3VY682QTN',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)); "
                "INSERT INTO devices(id,serial_number,imei,variant_id,source_id,location_id,cost_cents,currency_code,bin,notes,state,category,grade,battery_health,photos,created_at,updated_at) VALUES"
                "('isolated-repair-device','ISOLATED-REPAIR-001','490154203237518','variant_01M51RN7H7HPV9RH69T32QMD2Q','isolated-repair-source','sloc_01M4FBP3NDPZHJKVV3VY682QTN',0,'eur','','','available','sale','S',90,'[]',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)); "
                "INSERT INTO device_variant_tracking(id,variant_id,inventory_item_id,created_at,updated_at) "
                "SELECT 'isolated-repair-tracking','variant_01M51RN7H7HPV9RH69T32QMD2Q','iitem_01M51RN7HTPYQ7YFPXABBF8VJ8',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6) "
                "WHERE NOT EXISTS(SELECT 1 FROM device_variant_tracking WHERE variant_id='variant_01M51RN7H7HPV9RH69T32QMD2Q' AND deleted_at IS NULL);")
            if refund:
                sql(database, "UPDATE order_refund_runtime SET enabled=TRUE WHERE id=1 AND version=4")
            if warehouse:
                sql(database, "UPDATE warehouse_user_runtime SET enabled=TRUE WHERE id=1 AND version=1; "
                    "UPDATE warehouse_scope_runtime SET enabled=TRUE WHERE id=1 AND version=1")
            if exchange or warehouse:
                sql(database, "INSERT INTO devices(id,serial_number,imei,variant_id,source_id,location_id,cost_cents,currency_code,bin,notes,state,category,grade,battery_health,photos,created_at,updated_at) "
                    "VALUES('isolated-exchange-device','CHECKOUT-DEVICE-5','490154203237526','variant_01M51RN7H7HPV9RH69T32QMD2Q','isolated-repair-source','sloc_01M4FBP3NDPZHJKVV3VY682QTN',0,'eur','','','available','sale','S',90,'[]',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
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
        # Real sandbox refunds must use a freshly paid order in this copy,
        # never the remote payment belonging to the source preview order.
        env['ORDER_REFUNDS_ENABLED'] = 'true' if refund else 'false'
        if mail:
            env['NOTIFICATION_MAIL_ENABLED'] = 'true'
            # Fresh mailbox encryption belongs only to this disposable database.
            # Imported historical secrets are neither needed nor decrypted.
            env['MERCHANT_KEY_ACTIVE'] = 'isolated-mail'
            env['MERCHANT_KEYRING_JSON'] = json.dumps({'isolated-mail': secrets.token_hex(32)})
            fixture = logs / 'mail-subflow.test.exe'
            subprocess.run(['go', 'test', '-c', '-o', str(fixture), './internal/identity'],
                           cwd=ROOT / 'retail/backend', check=True, capture_output=True, timeout=120)
        if refund:
            if not values.get('MOLLIE_API_KEY', '').startswith('test_'):
                raise RuntimeError('Refund rehearsal requires a Mollie test key')
            env['CHECKOUT_PAYMENT_ENABLED'] = 'true'
            worker = logs / 'refund-worker.exe'
            subprocess.run(['go', 'build', '-o', str(worker), './cmd/process-order-refunds'],
                           cwd=ROOT / 'retail/backend', check=True, capture_output=True, timeout=120)
        services = [
            ('api', [str(ROOT / 'logs/retail-native/retail-api.exe')], ROOT / 'retail/backend'),
            ('storefront', [str(node), 'node_modules/astro/bin/astro.mjs', 'dev', '--host', '127.0.0.1',
                            '--port', '19300'], storefront)]
        if admin_required:
            env['ADMIN_TEST_API_TARGET'] = 'http://127.0.0.1:19100'
            services.append(('admin', [str(node), 'node_modules/vite/bin/vite.js', '--host', '127.0.0.1',
                                      '--port', '19200', '--strictPort'], ROOT / 'retail/admin'))
        if refund:
            services.append(('refund-worker', [str(worker)], ROOT / 'retail/backend'))
        for label, command, cwd in services:
            stream = open(logs / (label + '.log'), 'wb')
            streams.append(stream)
            processes.append(subprocess.Popen(command, cwd=cwd, env=env, stdout=stream, stderr=stream,
                                               creationflags=subprocess.CREATE_NO_WINDOW))
        urls = ['http://127.0.0.1:19100/health', 'http://127.0.0.1:19300/gb-en/account']
        if admin_required:
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
        if mail:
            browser_env.update(WINDOWS_MAIL_SUBFLOW='1', WINDOWS_MAIL_FIXTURE=str(fixture))
        if resume:
            browser_env.update(ACCOUNT_TEST_CUSTOMER_OWNER=customer_owner, ACCOUNT_TEST_RESUME_PAID='true',
                               ACCOUNT_TEST_REFUND_EXISTING='true' if existing_refund else 'false')
        script = 'tests/local-notification-mail.live.mjs' if mail else 'tests/local-repair.live.mjs' if repair else 'tests/local-buyback.live.mjs' if buyback else 'tests/local-customer-account.live.mjs'
        result = subprocess.run([str(node), script],
                                cwd=ROOT / 'retail/storefront', env=browser_env,
                                stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=420 if refund else 240)
        (logs / 'browser.log').write_bytes(result.stdout)
        if result.returncode:
            raise RuntimeError('Account browser check failed; inspect ignored browser.log')
        report.update(json.loads((logs / 'browser-result.json').read_text()))
        report['verified_accounts'] = int(sql(database, "SELECT COUNT(*) FROM customer_accounts "
             "WHERE email LIKE 'local-account-" + customer_owner + "-%@example.invalid' AND verified_at IS NOT NULL"))
        if report['verified_accounts'] != 1:
            raise RuntimeError('Expected exactly one newly verified fixture account')
        if mail:
            facts = {
                'verification_sent': int(sql(database, "SELECT COUNT(*) FROM customer_verification_mail WHERE status='sent'")),
                'order_sent': int(sql(database, "SELECT COUNT(*) FROM order_notification_outbox WHERE event='order.placed' AND status='sent'")),
                'order_retry_audit': int(sql(database, "SELECT COUNT(*) FROM audit_events WHERE action='order_mail.retry'")),
                'approved_mailbox': int(sql(database, "SELECT COUNT(*) FROM notification_mail_accounts")),
                'mailbox_receipts': int(sql(database, "SELECT COUNT(*) FROM notification_mail_commands")),
            }
            if facts != {'verification_sent': 1, 'order_sent': 1, 'order_retry_audit': 1, 'approved_mailbox': 1, 'mailbox_receipts': 2}:
                raise RuntimeError('Mail terminal facts differ: ' + str(facts))
            report['sql_mail_facts'] = facts
        if warehouse:
            order = report['order_id']
            if not re.fullmatch(r'order_[0-9a-f-]{36}', order):
                raise RuntimeError('Unexpected fixture order')
            checks = {
                'warehouse_user': "SELECT COUNT(*) FROM admin_users WHERE email='local-warehouse-" + owner + "@example.invalid'",
                'canceled_parcel': "SELECT COUNT(*) FROM order_fulfillments o JOIN fulfillments f ON f.id=o.fulfillment_id WHERE o.order_id='" + order + "' AND f.canceled_at IS NOT NULL",
                'shipped_parcel': "SELECT COUNT(*) FROM order_fulfillments o JOIN fulfillments f ON f.id=o.fulfillment_id WHERE o.order_id='" + order + "' AND f.canceled_at IS NULL AND f.shipped_at IS NOT NULL",
                'shipped_device': "SELECT COUNT(*) FROM device_assignments a JOIN devices d ON d.id=a.device_id WHERE a.order_id='" + order + "' AND a.status='shipped' AND d.id='isolated-repair-device' AND d.state='shipped'",
                'unused_replacement': "SELECT COUNT(*) FROM devices WHERE id='isolated-exchange-device' AND state='available'",
                'stock_once': "SELECT COUNT(*) FROM inventory_levels WHERE inventory_item_id='iitem_01M51RN7HTPYQ7YFPXABBF8VJ8' AND location_id='sloc_01M4FBP3NDPZHJKVV3VY682QTN' AND stocked_quantity=49 AND reserved_quantity=0 AND deleted_at IS NULL",
            }
            facts = {name: int(sql(database, query)) for name, query in checks.items()}
            if any(value != 1 for value in facts.values()):
                raise RuntimeError('Warehouse terminal facts differ: ' + str(facts))
            report['sql_warehouse_facts'] = facts
        if repair and not warehouse:
            request = report['request_id']
            if not re.fullmatch(r'[A-Za-z0-9_-]{1,191}', request):
                raise RuntimeError('Unexpected owned repair ID')
            status = sql(database, "SELECT status FROM after_sales_requests WHERE id='" + request + "'")
            counts = [int(sql(database, "SELECT COUNT(*) FROM " + table + " WHERE request_id='" + request + "'"))
                      for table in ('device_return_receipts', 'device_service_receipts')]
            expected_counts = [1, 1] if refund else [1, 2]
            if status != ('processing' if refund and not report.get('refund_completed') else 'completed') or counts != expected_counts:
                raise RuntimeError('Repair terminal status or receipt counts differ')
            report['sql_status'] = status
            report['sql_device_return_receipts'] = counts[0]
            report['sql_device_service_receipts'] = counts[1]
            report['sql_device_state'] = sql(database, "SELECT state FROM devices WHERE id='isolated-repair-device'")
            if report['sql_device_state'] != ('available' if exchange or refund else 'shipped'):
                raise RuntimeError('Repaired device was not shipped')
            if refund:
                order = report['order_id']
                if not re.fullmatch(r'order_[0-9a-f-]{36}', order) or order == 'order_8bffc6dc-112e-4ed5-8a32-57421bc2ae4d':
                    raise RuntimeError('Refund must belong to a distinct new sandbox order')
                checks = {
                    'refund_instruction': "SELECT COUNT(*) FROM order_refund_operations WHERE order_id='" + order + "' AND state='refunded' AND applied=TRUE AND amount='789.00' AND remote_refund_id IS NOT NULL",
                    'refund_ledger': "SELECT COUNT(*) FROM payment_refunds r JOIN order_refund_operations o ON r.id=CONCAT('ref_',o.operation_id) WHERE o.order_id='" + order + "' AND r.amount=789",
                    'negative_transaction': "SELECT COUNT(*) FROM order_transactions WHERE order_id='" + order + "' AND reference='refund' AND amount=-789",
                    'refunded_summary': "SELECT COUNT(*) FROM order_summaries s JOIN orders o ON o.id=s.order_id AND o.version=s.version WHERE s.order_id='" + order + "' AND JSON_UNQUOTE(JSON_EXTRACT(s.totals,'$.refunded_total'))='789'",
                    'read_refund_notice': "SELECT COUNT(*) FROM customer_account_notices WHERE resource_id='" + order + "' AND kind='refund.created' AND read_at IS NOT NULL",
                    'automatic_ownership': "SELECT COUNT(*) FROM customer_order_ownership WHERE order_id='" + order + "' AND store_id='" + values['STOREFRONT_STORE_ID'] + "'",
                    'source_order_untouched': "SELECT COUNT(*) FROM order_refund_operations WHERE order_id='order_8bffc6dc-112e-4ed5-8a32-57421bc2ae4d'",
                }
                if not report.get('refund_completed'):
                    checks['refund_instruction'] = "SELECT COUNT(*) FROM order_refund_operations WHERE order_id='" + order + "' AND state IN ('pending','queued','processing','uncertain') AND applied=FALSE AND amount='789.00' AND dispatched_at IS NOT NULL AND remote_refund_id IS NOT NULL"
                    checks.pop('refunded_summary')
                    checks.pop('read_refund_notice')
                facts = {name: int(sql(database, query)) for name, query in checks.items()}
                zero = {'source_order_untouched'}
                if not report.get('refund_completed'):
                    zero.update(('refund_ledger', 'negative_transaction'))
                if any(value != (0 if name in zero else 1) for name, value in facts.items()):
                    raise RuntimeError('Sandbox refund ledger differs: ' + str(facts))
                report['sql_refund_facts'] = facts
            if exchange:
                report['sql_replacement_state'] = sql(database, "SELECT state FROM devices WHERE id='isolated-exchange-device'")
                if report['sql_replacement_state'] != 'shipped':
                    raise RuntimeError('Replacement was not shipped')
                order = report['order_id']
                if not re.fullmatch(r'order_[0-9a-f-]{36}', order):
                    raise RuntimeError('Unexpected copied order ID')
                checks = {
                    'exchange': "SELECT COUNT(*) FROM order_exchanges WHERE order_id='" + order + "' AND deleted_at IS NULL",
                    'received_return': "SELECT COUNT(*) FROM returns WHERE order_id='" + order + "' AND status='received' AND deleted_at IS NULL",
                    'replacement_link': "SELECT COUNT(*) FROM device_assignments a JOIN device_assignments old ON old.id=a.replacement_of AND old.order_id=a.order_id WHERE a.order_id='" + order + "' AND a.device_id='isolated-exchange-device' AND a.status='shipped' AND old.device_id='isolated-repair-device' AND old.status='returned' AND a.item_id<>old.item_id",
                    'shipped_parcels': "SELECT COUNT(*) FROM order_fulfillments l JOIN fulfillments f ON f.id=l.fulfillment_id WHERE l.order_id='" + order + "' AND f.shipped_at IS NOT NULL AND f.canceled_at IS NULL",
                    'settled_summary': "SELECT COUNT(*) FROM order_summaries s JOIN orders o ON o.id=s.order_id AND o.version=s.version WHERE s.order_id='" + order + "' AND JSON_UNQUOTE(JSON_EXTRACT(s.totals,'$.pending_difference'))='0' AND JSON_UNQUOTE(JSON_EXTRACT(s.totals,'$.paid_total'))='789' AND JSON_UNQUOTE(JSON_EXTRACT(s.totals,'$.refunded_total'))='0'",
                }
                facts = {name: int(sql(database, query)) for name, query in checks.items()}
                if facts != {'exchange': 1, 'received_return': 1, 'replacement_link': 1, 'shipped_parcels': 2, 'settled_summary': 1}:
                    raise RuntimeError('Native exchange ledger or balance differs: ' + str(facts))
                report['sql_exchange_facts'] = facts
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
        if refund and created:
            try:
                # Keep an ignored durable copy even on browser failure: a real
                # test refund may already exist or still need reconciliation.
                (logs / 'refund-final.sql').write_bytes(docker('exec', CONTAINER, 'mysqldump',
                    '--defaults-extra-file=/tmp/local-root.cnf', '--single-transaction',
                    '--set-gtid-purged=OFF', '--no-tablespaces', database))
                report['refund_durable_snapshot_retained'] = True
            except Exception:
                cleanup_errors.append('refund evidence snapshot')
        try:
            report['redis_keys_removed'] = redis_cleanup(prefix)
        except Exception:
            cleanup_errors.append('Redis cleanup')
        try:
            if created and (not refund or report.get('refund_durable_snapshot_retained')):
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
    elif len(sys.argv) == 4 and sys.argv[1] == '--prepare-repair-owner':
        prepare_repair_owner(sys.argv[2], sys.argv[3])
    elif len(sys.argv) == 1:
        for language in ('en', 'zh'):
            main(language)
    elif sys.argv[1:] in (['--language=en'], ['--language=zh']):
        main(sys.argv[1].split('=')[1])
    elif sys.argv[1:] == ['--scenario=buyback']:
        main('en', 'buyback')
    elif sys.argv[1:] == ['--scenario=buyback-return']:
        main('en', 'buyback-return')
    elif sys.argv[1:] == ['--scenario=repair']:
        main('en', 'repair')
    elif sys.argv[1:] == ['--scenario=exchange']:
        main('en', 'exchange')
    elif sys.argv[1:] == ['--scenario=refund']:
        main('en', 'refund')
    elif sys.argv[1:] == ['--scenario=warehouse']:
        main('en', 'warehouse')
    elif sys.argv[1:] == ['--scenario=mail']:
        main('en', 'mail')
    elif len(sys.argv) == 2 and sys.argv[1].startswith('--resume-refund='):
        main('en', 'refund', sys.argv[1].split('=', 1)[1])
    else:
        raise RuntimeError('Unknown account rehearsal arguments')
