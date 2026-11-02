"""Windows native account subflow against a disposable copy of the preview DB.

Requires the running dedicated MySQL container, native Redis, Go-built preview
API, frontend dependencies, Node >=22 and Python cryptography. No mail is sent.
--scenario=buyback and --scenario=buyback-return independently rehearse customer
applications through Admin closing with explicit prices/devices in the copy.
--scenario=buyback-intake / buyback-intake-return replace the seeded device/source with actual Admin
source creation, photo upload and recycled device intake before inspection.
--scenario=repair or --scenario=exchange continues a copied paid order through
explicit fixture account ownership, real fulfillment and repair/replacement; never claims ownership
of an actual guest order in the source preview.
--scenario=refund creates its own real sandbox payment; pending is reported as
unfinished, never posted. --resume-refund=OWNER restores an ignored checkpoint
and reuses the same paid order and durable refund instruction.
--scenario=warehouse uses the copied paid order and explicit fixture ownership
to verify a new scoped employee's allocation/release/shipment and customer view.
--warehouse-from-checkpoint=OWNER instead continues an actual paid customer's
original order and ownership, with payment creation disabled. The independent
--verify-paid-warehouse=OWNER checks retained before/after SQL without replaying
checkout, payment or warehouse commands.
--scenario=mail uses actual Admin mailbox commands and production dispatchers
in separate test processes, replacing only the external Resend transport.
--scenario=payment-context continues one real sandbox checkout through two
language saves, lost success responses, refresh recovery and paid completion.
--scenario=payment-context-changed additionally reprices only the copy, retires
the prior checkout, and completes a fresh sandbox attempt at the new amount.
--resume-changed-context=OWNER reads an already completed owned checkpoint;
it never creates another remote payment.
--scenario=payment-context-restore injects a final SQL save failure only in the
copy after retiring a payment, then restores original settings and a replacement
sandbox checkout through the actual UI, including lost restore replies.
Verification links are read from authenticated encrypted intents in the owned
copy (or the captured dispatch body for mail), never returned by an application
endpoint or marked verified with SQL.
"""
import json
import hashlib
from decimal import Decimal
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


def payment_context_facts(database, cart_id, order_id):
    facts = {
        'context_receipts': int(sql(database, "SELECT COUNT(*) FROM cart_context_operations WHERE cart_id='" + cart_id + "'")),
        'order_ownership': int(sql(database, "SELECT COUNT(*) FROM customer_order_ownership WHERE order_id='" + order_id + "'")),
        'completion_receipt': int(sql(database, "SELECT COUNT(*) FROM cart_completion_receipts WHERE cart_id='" + cart_id + "'")),
        'single_checkout': int(sql(database, "SELECT COUNT(*) FROM cart_checkout_attempts WHERE cart_id='" + cart_id + "'")),
        'single_create': int(sql(database, "SELECT COUNT(*) FROM payment_effect_journal WHERE scope_id='" + cart_id + "' AND effect_kind='create_session' AND state='succeeded'")),
        'no_delete': int(sql(database, "SELECT COUNT(*) FROM payment_effect_journal WHERE scope_id='" + cart_id + "' AND effect_kind='delete_session'")),
        'continuous_completion_binding': int(sql(database, "SELECT COUNT(*) FROM cart_checkout_attempts a JOIN cart_completion_receipts r ON r.cart_id=a.cart_id AND r.operation_id=a.operation_id WHERE a.cart_id='" + cart_id + "' AND a.effective_cart_version=a.cart_version+2 AND r.input_version=a.input_version AND r.completed_version=a.effective_cart_version+1")),
    }
    if facts != {'context_receipts': 2, 'order_ownership': 1, 'completion_receipt': 1, 'single_checkout': 1, 'single_create': 1, 'no_delete': 0, 'continuous_completion_binding': 1}:
        raise RuntimeError('Payment context terminal facts differ: ' + str(facts))
    return facts


def changed_payment_context_facts(database, cart_id, order_id):
    facts = {
        'context_receipts': int(sql(database, "SELECT COUNT(*) FROM cart_context_operations WHERE cart_id='" + cart_id + "'")),
        'checkout_attempts': int(sql(database, "SELECT COUNT(*) FROM cart_checkout_attempts WHERE cart_id='" + cart_id + "'")),
        'create_effects': int(sql(database, "SELECT COUNT(*) FROM payment_effect_journal WHERE scope_id='" + cart_id + "' AND effect_kind='create_session' AND state='succeeded'")),
        'delete_effects': int(sql(database, "SELECT COUNT(*) FROM payment_effect_journal WHERE scope_id='" + cart_id + "' AND effect_kind='delete_session' AND state='succeeded'")),
        'new_amount_completion': int(sql(database, "SELECT COUNT(*) FROM cart_completion_receipts r JOIN cart_checkout_attempts a ON a.cart_id=r.cart_id AND a.operation_id=r.operation_id WHERE r.cart_id='" + cart_id + "' AND a.settlement_amount='790'")),
        'order_ownership': int(sql(database, "SELECT COUNT(*) FROM customer_order_ownership WHERE order_id='" + order_id + "'")),
    }
    if facts != {'context_receipts': 3, 'checkout_attempts': 2, 'create_effects': 2, 'delete_effects': 1, 'new_amount_completion': 1, 'order_ownership': 1}:
        raise RuntimeError('Changed payment context facts differ: ' + str(facts))
    return facts


def change_context_price(database):
    # sql() only accepts disposable owned database names. This artificial price
    # is never installed in the preview source or exposed as a business command.
    if sql(database, "SELECT COUNT(*) FROM pricing_prices WHERE id='price_01M593ZTMR4ZGW8KZJEVN74TBC' AND amount=779 AND deleted_at IS NULL") != '1':
        raise RuntimeError('Expected original isolated SKU price')
    sql(database, "UPDATE pricing_prices SET amount=780,raw_amount='{\"value\":\"780\",\"precision\":20}' WHERE id='price_01M593ZTMR4ZGW8KZJEVN74TBC'")


def context_restore_fault(database, cart_id, action):
    if not re.fullmatch(r'cart_[0-9a-f-]{36}', cart_id):
        raise RuntimeError('Owned context cart required')
    # First validate the disposable DB guard, even for trigger removal.
    if sql(database, "SELECT COUNT(*) FROM cart_ownership WHERE cart_id='" + cart_id + "'") != '1':
        raise RuntimeError('Owned context cart missing')
    if action == 'prepare':
        change_context_price(database)
        sql(database, "CREATE TRIGGER isolated_context_save_fault BEFORE INSERT ON cart_context_operations FOR EACH ROW SET NEW.result_version=IF(NEW.cart_id='" + cart_id + "',NULL,NEW.result_version)")
    elif action == 'check':
        row = sql(database, "SELECT version,payment_refresh_operation_id IS NOT NULL,checkout_attempt_id IS NOT NULL,(SELECT COUNT(*) FROM cart_context_operations WHERE cart_id='" + cart_id + "'),(SELECT COUNT(*) FROM payment_effect_journal WHERE scope_id='" + cart_id + "' AND effect_kind='delete_session' AND state='succeeded') FROM cart_ownership WHERE cart_id='" + cart_id + "'")
        if row.split('\t') != ['7', '1', '1', '2', '1']:
            raise RuntimeError('Expected frozen original context after durable delete')
    elif action == 'clear':
        sql(database, 'DROP TRIGGER IF EXISTS isolated_context_save_fault')
        sql(database, "UPDATE pricing_prices SET amount=779,raw_amount='{\"value\":\"779\",\"precision\":20}' WHERE id='price_01M593ZTMR4ZGW8KZJEVN74TBC'")
    else:
        raise RuntimeError('Unknown context fault action')


def restored_payment_context_facts(database, cart_id, order_id):
    facts = {
        'context_receipts': int(sql(database, "SELECT COUNT(*) FROM cart_context_operations WHERE cart_id='" + cart_id + "'")),
        'completed_compensation': int(sql(database, "SELECT COUNT(*) FROM cart_payment_refresh_compensations WHERE cart_id='" + cart_id + "' AND state='completed' AND result_version=8")),
        'checkout_attempts': int(sql(database, "SELECT COUNT(*) FROM cart_checkout_attempts WHERE cart_id='" + cart_id + "'")),
        'create_effects': int(sql(database, "SELECT COUNT(*) FROM payment_effect_journal WHERE scope_id='" + cart_id + "' AND effect_kind='create_session' AND state='succeeded'")),
        'delete_effects': int(sql(database, "SELECT COUNT(*) FROM payment_effect_journal WHERE scope_id='" + cart_id + "' AND effect_kind='delete_session' AND state='succeeded'")),
        'replacement_completion': int(sql(database, "SELECT COUNT(*) FROM cart_completion_receipts r JOIN cart_checkout_attempts a ON a.cart_id=r.cart_id AND a.operation_id=r.operation_id WHERE r.cart_id='" + cart_id + "' AND a.input_version=7 AND a.cart_version=8 AND a.settlement_amount='789' AND r.completed_version=9")),
        'order_ownership': int(sql(database, "SELECT COUNT(*) FROM customer_order_ownership WHERE order_id='" + order_id + "'")),
    }
    if facts != {'context_receipts': 2, 'completed_compensation': 1, 'checkout_attempts': 2, 'create_effects': 2, 'delete_effects': 1, 'replacement_completion': 1, 'order_ownership': 1}:
        raise RuntimeError('Restored payment context facts differ: ' + str(facts))
    return facts


def observe_context_payments(database, cart_id, expected_new='790.00'):
    key = config().get('MOLLIE_API_KEY', '')
    if not key.startswith('test_'):
        raise RuntimeError('Context observation requires sandbox configuration')
    rows = sql(database, "SELECT remote_id FROM mollie_payment_references WHERE scope_id='" + cart_id + "' ORDER BY created_at,remote_id").splitlines()
    if len(rows) != 2 or any(not re.fullmatch(r'tr_[A-Za-z0-9_]{1,120}', row) for row in rows):
        raise RuntimeError('Expected two retained sandbox payment references')
    observations = []
    for remote in rows:
        request = urllib.request.Request('https://api.mollie.com/v2/payments/' + remote, headers={'Authorization': 'Bearer ' + key})
        with urllib.request.urlopen(request, timeout=20) as response:
            body = json.load(response)
        if body.get('mode') != 'test' or body.get('metadata', {}).get('cart_id') != cart_id:
            raise RuntimeError('Sandbox payment scope mismatch')
        observations.append({'status': body.get('status'), 'amount': body.get('amount'), 'is_cancelable': body.get('isCancelable')})
    old, new = observations
    if old['amount'] != {'value': '789.00', 'currency': 'EUR'} or new['amount'] != {'value': expected_new, 'currency': 'EUR'} or new['status'] != 'paid':
        raise RuntimeError('Changed sandbox amount/status mismatch')
    if old['status'] not in ('canceled', 'expired', 'failed') and not (old['status'] == 'open' and old['is_cancelable'] is False):
        raise RuntimeError('Retired checkout requires reconciliation')
    return observations


def verify_payment_context_checkpoint(owner):
    config()  # Enforce the same development/loopback source guard.
    if not re.fullmatch(r'[0-9a-f]{12}', owner):
        raise RuntimeError('Owned context checkpoint required')
    logs = ROOT / 'logs/retail-native' / ('account-' + owner)
    report = json.loads((logs / 'report.json').read_text())
    browser = json.loads((logs / 'browser-result.json').read_text())
    if report.get('owner') != owner or not browser.get('passed') or browser.get('scenario') not in ('payment-context', 'payment-context-changed', 'payment-context-restore'):
        raise RuntimeError('Successful owned context browser checkpoint required')
    cart_id, order_id = browser['cart_id'], browser['order_id']
    if not re.fullmatch(r'cart_[0-9a-f-]{36}', cart_id) or not re.fullmatch(r'order_[0-9a-f-]{36}', order_id):
        raise RuntimeError('Invalid checkpoint identity')
    database = 'retail_account_' + secrets.token_hex(6)
    snapshot = '/tmp/' + database + '.sql'
    created = False
    try:
        docker('cp', str(logs / 'payment-context-final.sql'), CONTAINER + ':' + snapshot)
        docker('exec', CONTAINER, 'mysql', '--defaults-extra-file=/tmp/local-root.cnf', '-e', 'CREATE DATABASE ' + database)
        created = True
        docker('exec', CONTAINER, 'sh', '-c', 'mysql --defaults-extra-file=/tmp/local-root.cnf ' + database + ' < ' + snapshot)
        if browser['scenario'] == 'payment-context-restore':
            facts = restored_payment_context_facts(database, cart_id, order_id)
            report['provider_observations'] = observe_context_payments(database, cart_id, '789.00')
        elif browser['scenario'] == 'payment-context-changed':
            facts = changed_payment_context_facts(database, cart_id, order_id)
            report['provider_observations'] = observe_context_payments(database, cart_id)
        else:
            facts = payment_context_facts(database, cart_id, order_id)
        report['sql_payment_context_facts'] = facts
        report['checkpoint_sql_verified'] = True
        report['passed'] = True
        (logs / 'report.json').write_text(json.dumps(report, indent=2), encoding='utf-8')
        print('Original payment checkpoint verified; no provider mutations')
    finally:
        if created:
            docker('exec', CONTAINER, 'mysql', '--defaults-extra-file=/tmp/local-root.cnf', '-e', 'DROP DATABASE ' + database)
        docker('exec', CONTAINER, 'rm', '-f', snapshot)


def warehouse_payment_fingerprint(database):
    immutable = ['payments', 'payment_captures', 'order_transactions', 'cart_completion_receipts',
                 'customer_order_ownership', 'cart_checkout_attempts', 'payment_effect_journal']
    digest = hashlib.sha256(docker('exec', CONTAINER, 'mysqldump', '--defaults-extra-file=/tmp/local-root.cnf',
        '--set-gtid-purged=OFF', '--no-create-info', '--skip-comments', '--skip-dump-date', '--skip-triggers', database, *immutable))
    # Fulfillment legitimately changes these quantities and timestamps. Protect
    # every other order/item column, including prices, versions and identities.
    for table, excluded in [('orders', {'updated_at'}), ('order_items', {
            'updated_at', 'fulfilled_quantity', 'raw_fulfilled_quantity', 'shipped_quantity', 'raw_shipped_quantity'})]:
        columns = sql(database, "SELECT COLUMN_NAME FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='" + table + "' ORDER BY ORDINAL_POSITION").splitlines()
        if not columns or any(not re.fullmatch(r'[a-z0-9_]+', c) for c in columns):
            raise RuntimeError('Unexpected order projection schema')
        projection = ','.join("IFNULL(HEX(`" + c + "`),'NULL')" for c in columns if c not in excluded)
        digest.update(sql(database, 'SELECT ' + projection + ' FROM `' + table + '` ORDER BY id').encode())
    return digest.hexdigest()


def verify_paid_warehouse_checkpoint(owner):
    config()  # Enforce the existing dedicated local source guard.
    if not re.fullmatch(r'[0-9a-f]{12}', owner):
        raise RuntimeError('Owned warehouse checkpoint required')
    logs = ROOT / 'logs/retail-native' / ('account-' + owner)
    browser = json.loads((logs / 'browser-result.json').read_text())
    if not browser.get('passed') or browser.get('scenario') != 'paid-warehouse' or not browser.get('paid_checkpoint_continuation'):
        raise RuntimeError('Completed actual paid-order warehouse browser scenario required')
    order = browser['order_id']
    if not re.fullmatch(r'order_[0-9a-f-]{36}', order):
        raise RuntimeError('Owned order identity required')
    fingerprints, stocks, facts = [], [], None
    for name in ['snapshot.sql', 'payment-context-final.sql']:
        database = 'retail_account_' + secrets.token_hex(6)
        snapshot = '/tmp/warehouse-verify-' + database + '.sql'
        created = False
        try:
            docker('cp', str(logs / name), CONTAINER + ':' + snapshot)
            docker('exec', CONTAINER, 'mysql', '--defaults-extra-file=/tmp/local-root.cnf', '-e', 'CREATE DATABASE ' + database)
            created = True
            docker('exec', CONTAINER, 'sh', '-c', 'mysql --defaults-extra-file=/tmp/local-root.cnf ' + database + ' < ' + snapshot)
            fingerprints.append(warehouse_payment_fingerprint(database))
            stocks.append(sql(database, "SELECT stocked_quantity,reserved_quantity FROM inventory_levels WHERE inventory_item_id='iitem_01M51RN7HTPYQ7YFPXABBF8VJ8' AND location_id='sloc_01M4FBP3NDPZHJKVV3VY682QTN' AND deleted_at IS NULL").split('\t'))
            if name == 'payment-context-final.sql':
                facts = sql(database, "SELECT (SELECT COUNT(*) FROM order_fulfillments o JOIN fulfillments f ON f.id=o.fulfillment_id WHERE o.order_id='" + order + "' AND f.canceled_at IS NOT NULL),(SELECT COUNT(*) FROM order_fulfillments o JOIN fulfillments f ON f.id=o.fulfillment_id WHERE o.order_id='" + order + "' AND f.canceled_at IS NULL AND f.shipped_at IS NOT NULL),(SELECT COUNT(*) FROM order_items WHERE order_id='" + order + "' AND quantity=1 AND fulfilled_quantity=1 AND shipped_quantity=1),(SELECT COUNT(*) FROM device_assignments a JOIN devices d ON d.id=a.device_id WHERE a.order_id='" + order + "' AND a.status='shipped' AND d.id='isolated-repair-device' AND d.state='shipped'),(SELECT COUNT(*) FROM devices WHERE id='isolated-exchange-device' AND state='available'),(SELECT COUNT(*) FROM admin_users WHERE email='local-warehouse-" + owner + "@example.invalid')").split('\t')
        finally:
            if created:
                docker('exec', CONTAINER, 'mysql', '--defaults-extra-file=/tmp/local-root.cnf', '-e', 'DROP DATABASE ' + database)
            docker('exec', CONTAINER, 'rm', '-f', snapshot)
    if fingerprints[0] != fingerprints[1] or facts != ['1'] * 6 or any(Decimal(a) - Decimal(b) != 1 for a, b in zip(*stocks)):
        raise RuntimeError('Paid warehouse checkpoint financial, identity, shipment or stock invariant differs')
    result = {'passed': True, 'owner': owner, 'order_id': order, 'financial_ownership_and_order_identity_sha256': fingerprints[0],
              'stock_before': stocks[0], 'stock_after': stocks[1], 'sql_terminal_facts': facts,
              'replayed_business_commands': False, 'provider_calls': False, 'verification_databases_removed': True}
    (logs / 'warehouse-checkpoint-verification.json').write_text(json.dumps(result, indent=2))
    print('Paid warehouse checkpoint SQL verified: ' + owner)


def verify_supplement_checkpoint(owner):
    values = config()
    if not re.fullmatch(r'[0-9a-f]{12}', owner):
        raise RuntimeError('Owned supplement checkpoint required')
    logs = ROOT / 'logs/retail-native' / ('account-' + owner)
    run = json.loads((logs / 'report.json').read_text())
    paid = json.loads((logs / 'supplement-paid.json').read_text())
    order, operation = paid['order_id'], paid['operation_id']
    if not run.get('passed') or not paid.get('passed') or not paid.get('real_mollie_test_payment') or not re.fullmatch(r'order_[0-9a-f-]{36}', order) or not re.fullmatch(r'[0-9a-f-]{36}', operation):
        raise RuntimeError('Completed real supplement checkpoint required')
    database = 'retail_account_' + secrets.token_hex(6)
    snapshot = '/tmp/supplement-verify-' + database + '.sql'
    created = False
    try:
        docker('cp', str(logs / 'payment-context-final.sql'), CONTAINER + ':' + snapshot)
        docker('exec', CONTAINER, 'mysql', '--defaults-extra-file=/tmp/local-root.cnf', '-e',
               'CREATE DATABASE ' + database + "; GRANT ALL ON " + database + ".* TO 'retail_local'@'%';")
        created = True
        docker('exec', CONTAINER, 'sh', '-c', 'mysql --defaults-extra-file=/tmp/local-root.cnf ' + database + ' < ' + snapshot)
        checks = {
            'one_applied_instruction': "SELECT COUNT(*) FROM order_payment_operations WHERE order_id='" + order + "' AND operation_id='" + operation + "' AND state='paid' AND applied=TRUE AND amount='7.79'",
            'one_channel_create': "SELECT COUNT(*) FROM payment_effect_journal WHERE scope_id='" + order + "' AND effect_kind='create_session' AND state='succeeded'",
            'original_checkout_completion': "SELECT COUNT(*) FROM cart_completion_receipts WHERE order_id='" + order + "'",
            'two_payment_captures': "SELECT COUNT(*) FROM payment_captures c JOIN payments p ON p.id=c.payment_id JOIN order_payment_collections o ON o.payment_collection_id=p.payment_collection_id WHERE o.order_id='" + order + "'",
            'two_cash_transactions': "SELECT COUNT(*) FROM order_transactions WHERE order_id='" + order + "' AND amount>0",
            'one_received_notice': "SELECT COUNT(*) FROM customer_account_notices WHERE resource_id='" + order + "' AND kind='payment.received'",
            'original_customer_ownership': "SELECT COUNT(*) FROM customer_order_ownership WHERE order_id='" + order + "'",
            'one_completed_exchange': "SELECT COUNT(*) FROM after_sales_requests WHERE order_id='" + order + "' AND type='exchange' AND status='completed'",
            'two_shipped_parcels': "SELECT COUNT(*) FROM order_fulfillments o JOIN fulfillments f ON f.id=o.fulfillment_id WHERE o.order_id='" + order + "' AND f.shipped_at IS NOT NULL AND f.canceled_at IS NULL",
        }
        facts = {name: int(sql(database, query)) for name, query in checks.items()}
        if any(value != (2 if name.startswith('two_') else 1) for name, value in facts.items()):
            raise RuntimeError('Supplement checkpoint SQL facts differ: ' + str(facts))
        tables = ['orders', 'order_items', 'payments', 'payment_captures', 'order_transactions', 'order_payment_operations', 'payment_effect_journal', 'inventory_levels', 'device_assignments', 'customer_account_notices']
        def fingerprint():
            return hashlib.sha256(docker('exec', CONTAINER, 'mysqldump', '--defaults-extra-file=/tmp/local-root.cnf', '--set-gtid-purged=OFF', '--no-create-info', '--skip-comments', '--skip-dump-date', '--skip-triggers', database, *tables)).hexdigest()
        before = fingerprint()
        env = dict(os.environ, **values)
        env.update(MYSQL_DSN=values['MYSQL_DSN'].replace('/eurorefurb_local_trial?', '/' + database + '?'),
                   STOREFRONT_ORIGIN='http://127.0.0.1:19300', MOLLIE_BACKEND_ORIGIN='http://127.0.0.1:19100',
                   CHECKOUT_PAYMENT_ENABLED='true', MOLLIE_NOTIFICATIONS_ENABLED='true', ORDER_PAYMENTS_ENABLED='true')
        result = subprocess.run([str(logs / 'order-payment-worker.exe'), '--once'], cwd=ROOT / 'retail/backend',
                                env=env, capture_output=True, timeout=90)
        (logs / 'supplement-worker-restart.log').write_bytes(result.stdout + result.stderr)
        if result.returncode or fingerprint() != before:
            raise RuntimeError('Completed supplement worker restart changed protected state')
        verification = {'passed': True, 'owner': owner, 'order_id': order, 'sql_facts': facts,
                        'completed_worker_restart_once': True, 'protected_ten_tables_unchanged': True}
    finally:
        if created:
            docker('exec', CONTAINER, 'mysql', '--defaults-extra-file=/tmp/local-root.cnf', '-e', 'DROP DATABASE ' + database + "; REVOKE ALL ON " + database + ".* FROM 'retail_local'@'%';")
        docker('exec', CONTAINER, 'rm', '-f', snapshot)
    verification['verification_resources_removed'] = True
    (logs / 'supplement-checkpoint-verification.json').write_text(json.dumps(verification, indent=2))
    print('Real supplement checkpoint and worker restart verified: ' + owner)


def verify_completed_refund_checkpoint(owner):
    values = config()
    if not re.fullmatch(r'[0-9a-f]{12}', owner) or not values.get('MOLLIE_API_KEY', '').startswith('test_'):
        raise RuntimeError('Owned local test refund checkpoint required')
    logs = ROOT / 'logs/retail-native' / ('account-' + owner)
    run = json.loads((logs / 'report.json').read_text())
    order = run.get('order_id', '')
    if not run.get('passed') or not run.get('refund_completed') or not run.get('real_mollie_sandbox_refund') or not re.fullmatch(r'order_[0-9a-f-]{36}', order):
        raise RuntimeError('Completed real sandbox refund evidence required')
    worker = logs / 'current-refund-worker.exe'
    subprocess.run(['go', 'build', '-o', str(worker), './cmd/process-order-refunds'],
                   cwd=ROOT / 'retail/backend', check=True, capture_output=True, timeout=120)
    database = 'retail_account_' + secrets.token_hex(6)
    snapshot = '/tmp/refund-verify-' + database + '.sql'
    created = False
    try:
        docker('cp', str(logs / 'refund-final.sql'), CONTAINER + ':' + snapshot)
        docker('exec', CONTAINER, 'mysql', '--defaults-extra-file=/tmp/local-root.cnf', '-e',
               'CREATE DATABASE ' + database + "; GRANT ALL ON " + database + ".* TO 'retail_local'@'%';")
        created = True
        docker('exec', CONTAINER, 'sh', '-c', 'mysql --defaults-extra-file=/tmp/local-root.cnf ' + database + ' < ' + snapshot)
        facts = sql(database, "SELECT (SELECT COUNT(*) FROM order_refund_operations WHERE order_id='" + order + "' AND state='refunded' AND applied=TRUE AND amount='789.00'),(SELECT COUNT(*) FROM order_transactions WHERE order_id='" + order + "' AND amount=-789 AND reference='refund'),(SELECT COUNT(*) FROM customer_account_notices WHERE resource_id='" + order + "' AND kind='refund.created'),(SELECT COUNT(*) FROM after_sales_requests WHERE order_id='" + order + "' AND status='completed' AND type='return'),(SELECT COUNT(*) FROM customer_order_ownership WHERE order_id='" + order + "')").split('\t')
        if facts != ['1'] * 5:
            raise RuntimeError('Completed refund checkpoint facts differ')
        tables = ['orders', 'order_items', 'payments', 'payment_captures', 'payment_refunds', 'order_transactions', 'order_refund_operations', 'inventory_levels', 'device_assignments', 'customer_account_notices']
        def fingerprint():
            return hashlib.sha256(docker('exec', CONTAINER, 'mysqldump', '--defaults-extra-file=/tmp/local-root.cnf', '--set-gtid-purged=OFF', '--no-create-info', '--skip-comments', '--skip-dump-date', '--skip-triggers', database, *tables)).hexdigest()
        before = fingerprint()
        env = dict(os.environ, **values)
        env.update(MYSQL_DSN=values['MYSQL_DSN'].replace('/eurorefurb_local_trial?', '/' + database + '?'),
                   STOREFRONT_ORIGIN='http://127.0.0.1:19300', MOLLIE_BACKEND_ORIGIN='http://127.0.0.1:19100',
                   CHECKOUT_PAYMENT_ENABLED='true', ORDER_REFUNDS_ENABLED='true')
        result = subprocess.run([str(worker), '--once'], cwd=ROOT / 'retail/backend', env=env,
                                capture_output=True, timeout=90)
        (logs / 'completed-refund-worker-restart.log').write_bytes(result.stdout + result.stderr)
        if result.returncode or fingerprint() != before:
            raise RuntimeError('Completed refund worker restart changed protected facts')
        verification = {'passed': True, 'owner': owner, 'order_id': order,
                        'worker_sha256': hashlib.sha256(worker.read_bytes()).hexdigest(),
                        'five_unique_terminal_facts': facts, 'current_worker_restart_once': True,
                        'protected_ten_tables_unchanged': True, 'business_commands_replayed': False}
    finally:
        if created:
            docker('exec', CONTAINER, 'mysql', '--defaults-extra-file=/tmp/local-root.cnf', '-e', 'DROP DATABASE ' + database + "; REVOKE ALL ON " + database + ".* FROM 'retail_local'@'%';")
        docker('exec', CONTAINER, 'rm', '-f', snapshot)
    verification['verification_resources_removed'] = True
    (logs / 'completed-refund-worker-verification.json').write_text(json.dumps(verification, indent=2))
    print('Completed real refund and current worker restart verified: ' + owner)


def verify_intake_delivery_checkpoint(owner):
    values = config()
    if not re.fullmatch(r'[0-9a-f]{12}', owner):
        raise RuntimeError('Owned intake checkpoint required')
    logs = ROOT / 'logs/retail-native' / ('account-' + owner)
    run = json.loads((logs / 'report.json').read_text())
    location = run.get('intake_location_id', '')
    if not run.get('passed') or run.get('scenario') != 'intake-warehouse' or not re.fullmatch(r'sloc_[0-9a-f-]{36}', location):
        raise RuntimeError('Successful real intake warehouse checkpoint required')
    database = 'retail_account_' + secrets.token_hex(6)
    snapshot = '/tmp/intake-verify-' + database + '.sql'
    created = False
    try:
        docker('cp', str(logs / 'payment-context-final.sql'), CONTAINER + ':' + snapshot)
        docker('exec', CONTAINER, 'mysql', '--defaults-extra-file=/tmp/local-root.cnf', '-e', 'CREATE DATABASE ' + database + "; GRANT ALL ON " + database + ".* TO 'retail_local'@'%';")
        created = True
        docker('exec', CONTAINER, 'sh', '-c', 'mysql --defaults-extra-file=/tmp/local-root.cnf ' + database + ' < ' + snapshot)
        before = warehouse_payment_fingerprint(database)
        env = dict(os.environ, **values)
        env.update(MYSQL_DSN=values['MYSQL_DSN'].replace('/eurorefurb_local_trial?', '/' + database + '?'), WINDOWS_DELIVERY_RECOVERY='1', WINDOWS_DELIVERY_LOCATION=location)
        result = subprocess.run(['go', 'test', './internal/warehouse', '-run', '^TestOwnedWindowsDeliveryRecovery$', '-count=1', '-v'], cwd=ROOT / 'retail/backend', env=env, capture_output=True, timeout=90)
        (logs / 'delivery-recovery.log').write_bytes(result.stdout + result.stderr)
        if result.returncode or warehouse_payment_fingerprint(database) != before:
            raise RuntimeError('Delivery recovery failed or payment facts changed')
        report = {'passed': True, 'owner': owner, 'current_backend_service_test': True, 'concurrent_calls': 6, 'fresh_receipt': 1, 'replayed_receipts': 5, 'stale_and_changed_commands_rejected': True, 'warehouse_principal_denied': True, 'late_failure_rollback': True, 'soft_deleted_link_restored_once': True, 'protected_payment_ownership_unchanged': True, 'checkout_or_warehouse_flow_replayed': False}
    finally:
        if created:
            docker('exec', CONTAINER, 'mysql', '--defaults-extra-file=/tmp/local-root.cnf', '-e', 'DROP DATABASE ' + database + "; REVOKE ALL ON " + database + ".* FROM 'retail_local'@'%';")
        docker('exec', CONTAINER, 'rm', '-f', snapshot)
    report['resources_removed'] = True
    (logs / 'delivery-recovery-report.json').write_text(json.dumps(report, indent=2), encoding='utf-8')
    print('Intake delivery checkpoint verified: ' + owner)


def main(language, scenario='account', resume=None):
    intake_warehouse = scenario == 'intake-warehouse'
    device_entry = scenario == 'device-entry'
    product_entry = scenario in ('product-entry', 'device-entry')
    mail = scenario == 'mail'
    reviews = scenario == 'reviews'
    paid_warehouse = scenario in ('paid-warehouse', 'intake-warehouse')
    supplement = scenario == 'supplement'
    payment_context = scenario in ('payment-context', 'payment-context-changed', 'payment-context-restore', 'reviews', 'paid-warehouse', 'intake-warehouse', 'supplement')
    buyback_intake = scenario in ('buyback-intake', 'buyback-intake-return')
    buyback_return = scenario in ('buyback-return', 'buyback-intake-return')
    buyback = scenario == 'buyback' or buyback_intake or buyback_return
    exchange = scenario in ('exchange', 'supplement')
    refund = scenario == 'refund'
    warehouse = scenario in ('warehouse', 'paid-warehouse', 'intake-warehouse')
    repair = scenario in ('repair', 'exchange', 'refund', 'warehouse', 'paid-warehouse', 'intake-warehouse', 'supplement')
    admin_required = buyback or repair or mail or reviews or product_entry
    values = config()
    api_binary = Path(os.environ.get('ACCOUNT_TEST_API_BINARY') or ROOT / 'logs/retail-native/retail-api.exe').resolve()
    if not api_binary.is_relative_to((ROOT / 'logs/retail-native').resolve()) or api_binary.suffix != '.exe' or not api_binary.is_file():
        raise RuntimeError('Isolated API executable must be an existing owned native log artifact')
    if resume is not None:
        if not (refund or payment_context) or not re.fullmatch(r'[0-9a-f]{12}', resume):
            raise RuntimeError('Only an owned payment checkpoint can resume')
        checkpoint = ROOT / 'logs/retail-native' / ('account-' + resume)
        checkpoint_report = json.loads((checkpoint / 'report.json').read_text())
        if (paid_warehouse or supplement) and (not checkpoint_report.get('passed') or not checkpoint_report.get('payment_durable_snapshot_retained')):
            raise RuntimeError('Fulfillment continuation requires a verified durable paid checkpoint')
        if refund:
            paid = json.loads((checkpoint / 'refund-new-paid-order.json').read_text())
            if not re.fullmatch(r'order_[0-9a-f-]{36}', paid.get('order_id', '')) or not paid.get('real_mollie_test_payment'):
                raise RuntimeError('Owned paid sandbox checkpoint required')
        else:
            state = json.loads((checkpoint / 'refund-payment-localstorage.json').read_text())
            intent = json.loads(state['retail:checkout-payment-intent:v1'])
            if not re.fullmatch(r'cart_[0-9a-f-]{36}', intent.get('cartID', '')):
                raise RuntimeError('Owned checkout intent required')
            paid = {'cart_id': intent['cartID']}
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
    def entry_source_fingerprint():
        return hashlib.sha256(docker('exec', CONTAINER, 'mysqldump', '--defaults-extra-file=/tmp/local-root.cnf', '--set-gtid-purged=OFF', '--no-create-info', '--skip-comments', '--skip-dump-date', '--skip-triggers', 'eurorefurb_local_trial', 'catalog_products', 'catalog_variants', 'pricing_prices', 'inventory_levels', 'catalog_product_edits', 'catalog_variant_creations', 'catalog_price_edits', 'inventory_level_creations', 'inventory_locations', 'device_sources', 'devices', 'device_variant_tracking', 'device_photos', 'device_tracking_changes', 'device_source_changes', 'device_intake_changes')).hexdigest()
    entry_source_before = entry_source_fingerprint() if product_entry or buyback_intake else None
    report = {'owner': owner, 'database': database, 'real_email_delivery': False,
              'business_api_mocking': False, 'main_database_modified': False,
              'api_sha256': hashlib.sha256(api_binary.read_bytes()).hexdigest()}
    customer_owner = checkpoint_report.get('customer_owner', resume) if resume else owner
    report['customer_owner'] = customer_owner
    try:
        # Fail before creating resources when another rehearsal owns these ports.
        for port in ((19100, 19200, 19300) if admin_required else (19100, 19300)):
            with socket.socket() as check:
                check.bind(('127.0.0.1', port))
        snapshot = '/tmp/account-' + owner + '.sql'
        if resume:
            docker('cp', str(checkpoint / ('refund-final.sql' if refund else 'payment-context-final.sql')), CONTAINER + ':' + snapshot)
            if refund:
                shutil.copy2(checkpoint / 'refund-new-paid-order.json', logs / 'refund-new-paid-order.json')
            else:
                shutil.copy2(checkpoint / 'refund-payment-localstorage.json', logs / 'refund-payment-localstorage.json')
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
            if payment_context:
                paid['order_id'] = sql(database, "SELECT order_id FROM cart_completion_receipts WHERE cart_id='" + paid['cart_id'] + "'")
                if not re.fullmatch(r'order_[0-9a-f-]{36}', paid['order_id']):
                    raise RuntimeError('Context checkpoint must already be completed; no new payment is created')
                (logs / 'context-resume.json').write_text(json.dumps(paid))
            email = sql(database, "SELECT a.email FROM customer_accounts a JOIN customer_order_ownership o ON o.customer_id=a.id WHERE o.order_id='" + paid['order_id'] + "' AND a.verified_at IS NOT NULL")
            match = re.fullmatch(r'local-account-([0-9a-f]{12})-1@example.invalid', email)
            if not match:
                raise RuntimeError('Owned verified checkpoint customer required')
            customer_owner = match.group(1)
            report['customer_owner'] = customer_owner
            facts = sql(database, "SELECT (SELECT COUNT(*) FROM customer_order_ownership WHERE order_id='" + paid['order_id'] + "'),(SELECT COUNT(*) FROM order_refund_operations),(SELECT COUNT(*) FROM device_assignments)").split('\t')
            if facts not in (['1', '0', '0'], ['1', '1', '1']) or payment_context and facts != ['1', '0', '0']:
                raise RuntimeError('Unsupported owned refund checkpoint')
            existing_refund = facts[1] == '1'
            if existing_refund:
                row = sql(database, "SELECT request_id,operation_id FROM order_refund_operations WHERE order_id='" + paid['order_id'] + "'").split('\t')
                if len(row) != 2 or not re.fullmatch(r'asr_[0-9a-f-]{36}', row[0]) or not re.fullmatch(r'[0-9a-f-]{36}', row[1]):
                    raise RuntimeError('Owned refund receipt identity mismatch')
                (logs / 'refund-existing.json').write_text(json.dumps({'request_id': row[0], 'operation_id': row[1]}))
        if repair and not intake_warehouse and (not resume or paid_warehouse or supplement):
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
                "FROM catalog_products WHERE handle='swappie-iphone-13' AND deleted_at IS NULL; ")
            if not buyback_intake:
                sql(database, "INSERT INTO device_sources(id,name,type,location_id,created_at,updated_at) VALUES"
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
                   CHECKOUT_CONTEXT_PAYMENT_ENABLED='false',
                   CUSTOMER_AUTH_ENABLED='true', CUSTOMER_VERIFICATION_MAIL_ENABLED='false',
                   NOTIFICATION_MAIL_ENABLED='false', ORDER_MAIL_ENABLED='false',
                   ORDER_PAYMENTS_ENABLED='false', MOLLIE_NOTIFICATIONS_ENABLED='false',
                   GIN_API_ORIGIN='http://127.0.0.1:19100')
        # Real sandbox refunds must use a freshly paid order in this copy,
        # never the remote payment belonging to the source preview order.
        env['ORDER_REFUNDS_ENABLED'] = 'true' if refund else 'false'
        if intake_warehouse:
            sql(database, 'UPDATE warehouse_management_runtime SET enabled=TRUE WHERE id=1 AND version=1; UPDATE warehouse_user_runtime SET enabled=TRUE WHERE id=1 AND version=1; UPDATE warehouse_scope_runtime SET enabled=TRUE WHERE id=1 AND version=1; UPDATE inventory_command_capabilities SET enabled=TRUE WHERE command=\'inventory.create_level\' AND version=1')
        if device_entry or buyback_intake:
            sql(database, 'UPDATE warehouse_management_runtime SET enabled=TRUE WHERE id=1 AND version=1')
        if product_entry:
            sql(database, "UPDATE catalog_command_capabilities SET enabled=TRUE WHERE command='product.create' AND version=1; UPDATE inventory_command_capabilities SET enabled=TRUE WHERE command='inventory.create_level' AND version=1; UPDATE catalog_variant_create_runtime SET enabled=TRUE WHERE id=1 AND version=1; UPDATE catalog_price_edit_runtime SET enabled=TRUE WHERE id=1 AND version=1")
        if mail:
            env['NOTIFICATION_MAIL_ENABLED'] = 'true'
            # Fresh mailbox encryption belongs only to this disposable database.
            # Imported historical secrets are neither needed nor decrypted.
            env['MERCHANT_KEY_ACTIVE'] = 'isolated-mail'
            env['MERCHANT_KEYRING_JSON'] = json.dumps({'isolated-mail': secrets.token_hex(32)})
            fixture = logs / 'mail-subflow.test.exe'
            subprocess.run(['go', 'test', '-c', '-o', str(fixture), './internal/identity'],
                           cwd=ROOT / 'retail/backend', check=True, capture_output=True, timeout=120)
        if refund or payment_context:
            if not values.get('MOLLIE_API_KEY', '').startswith('test_'):
                raise RuntimeError('Payment rehearsal requires a Mollie test key')
            env['CHECKOUT_PAYMENT_ENABLED'] = 'true'
        if payment_context:
            env['CHECKOUT_CONTEXT_PAYMENT_ENABLED'] = 'true'
            if sql(database, 'SELECT version FROM cart_payment_refresh_runtime WHERE id=1') != '1':
                raise RuntimeError('Payment context runtime version mismatch')
            sql(database, 'UPDATE cart_payment_refresh_runtime SET enabled=TRUE WHERE id=1 AND version=1')
        if reviews:
            if sql(database, 'SELECT version FROM product_review_runtime WHERE id=1') != '1':
                raise RuntimeError('Product review runtime version mismatch')
            sql(database, 'UPDATE product_review_runtime SET reads_enabled=TRUE,writes_enabled=TRUE WHERE id=1 AND version=1')
            protected_tables = ['orders', 'order_items', 'payments', 'payment_captures', 'order_transactions', 'inventory_levels', 'inventory_reservations', 'device_assignments', 'pricing_prices']
            def protected_fingerprint():
                return hashlib.sha256(docker('exec', CONTAINER, 'mysqldump', '--defaults-extra-file=/tmp/local-root.cnf', '--set-gtid-purged=OFF', '--no-create-info', '--skip-comments', '--skip-dump-date', '--skip-triggers', database, *protected_tables)).hexdigest()
            review_before = protected_fingerprint()
            env['CHECKOUT_PAYMENT_ENABLED'] = 'false'
            env['CHECKOUT_CONTEXT_PAYMENT_ENABLED'] = 'false'
        if paid_warehouse:
            env['CHECKOUT_PAYMENT_ENABLED'] = 'false'
            env['CHECKOUT_CONTEXT_PAYMENT_ENABLED'] = 'false'
            stock_before = sql(database, "SELECT stocked_quantity,reserved_quantity FROM inventory_levels WHERE inventory_item_id='iitem_01M51RN7HTPYQ7YFPXABBF8VJ8' AND location_id='sloc_01M4FBP3NDPZHJKVV3VY682QTN' AND deleted_at IS NULL").split('\t')
            def payment_fingerprint():
                return warehouse_payment_fingerprint(database)
            payment_before = payment_fingerprint()
        if refund:
            worker = logs / 'refund-worker.exe'
            subprocess.run(['go', 'build', '-o', str(worker), './cmd/process-order-refunds'],
                           cwd=ROOT / 'retail/backend', check=True, capture_output=True, timeout=120)
        if supplement:
            env.update(CHECKOUT_PAYMENT_ENABLED='true', MOLLIE_NOTIFICATIONS_ENABLED='true', ORDER_PAYMENTS_ENABLED='true',
                       CHECKOUT_CONTEXT_PAYMENT_ENABLED='false', CHECKOUT_EXCHANGE_PAYMENT='1')
            sql(database, 'UPDATE order_payment_runtime SET enabled=TRUE WHERE id=1 AND version=1; UPDATE mollie_notification_runtime SET enabled=TRUE WHERE id=1 AND version=1')
            worker = logs / 'order-payment-worker.exe'
            subprocess.run(['go', 'build', '-o', str(worker), './cmd/process-order-payments'],
                           cwd=ROOT / 'retail/backend', check=True, capture_output=True, timeout=120)
        services = [
            ('api', [str(api_binary)], ROOT / 'retail/backend'),
            ('storefront', [str(node), 'node_modules/astro/bin/astro.mjs', 'dev', '--host', '127.0.0.1',
                            '--port', '19300'], storefront)]
        if admin_required:
            env['ADMIN_TEST_API_TARGET'] = 'http://127.0.0.1:19100'
            services.append(('admin', [str(node), 'node_modules/vite/bin/vite.js', '--host', '127.0.0.1',
                                      '--port', '19200', '--strictPort'], ROOT / 'retail/admin'))
        if refund:
            services.append(('refund-worker', [str(worker)], ROOT / 'retail/backend'))
        if supplement:
            services.append(('order-payment-worker', [str(worker)], ROOT / 'retail/backend'))
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
        if resume and refund:
            browser_env.update(ACCOUNT_TEST_CUSTOMER_OWNER=customer_owner, ACCOUNT_TEST_RESUME_PAID='true',
                               ACCOUNT_TEST_REFUND_EXISTING='true' if existing_refund else 'false')
        if paid_warehouse or supplement:
            browser_env.update(ACCOUNT_TEST_CUSTOMER_OWNER=customer_owner, ACCOUNT_TEST_RESUME_PAID='true')
            (logs / 'refund-new-paid-order.json').write_text(json.dumps(paid))
        script = 'tests/local-payment-context.live.mjs' if payment_context else 'tests/local-notification-mail.live.mjs' if mail else 'tests/local-repair.live.mjs' if repair else 'tests/local-buyback.live.mjs' if buyback else 'tests/local-customer-account.live.mjs'
        if resume and payment_context:
            browser_env['ACCOUNT_TEST_CUSTOMER_OWNER'] = customer_owner
            script = 'tests/local-payment-context-resume.live.mjs'
        if product_entry:
            script = 'tests/local-product-entry.live.mjs'
        if reviews:
            script = 'tests/local-product-reviews.live.mjs'
        if paid_warehouse or supplement:
            script = 'tests/local-repair.live.mjs'
        result = subprocess.run([str(node), script],
                                cwd=ROOT / 'retail/storefront', env=browser_env,
                                stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=420 if refund else 240)
        (logs / 'browser.log').write_bytes(result.stdout)
        if result.returncode:
            raise RuntimeError('Account browser check failed; inspect ignored browser.log')
        report.update(json.loads((logs / 'browser-result.json').read_text()))
        report['verified_accounts'] = int(sql(database, "SELECT COUNT(*) FROM customer_accounts "
             "WHERE email LIKE 'local-account-" + customer_owner + "-%@example.invalid' AND verified_at IS NOT NULL"))
        if product_entry:
            product = report['product_id']
            if not re.fullmatch(r'prod_[0-9a-f-]{36}', product):
                raise RuntimeError('Owned created product identity invalid')
            facts = sql(database, "SELECT (SELECT COUNT(*) FROM catalog_product_edits WHERE product_id='" + product + "'),(SELECT COUNT(*) FROM catalog_variant_creations WHERE product_id='" + product + "'),(SELECT COUNT(*) FROM catalog_price_edits WHERE product_id='" + product + "'),(SELECT COUNT(*) FROM inventory_level_creations WHERE product_id='" + product + "')").split('\t')
            if facts != ['1', '1', '1', '1']:
                raise RuntimeError('Expected unique product, SKU, price and stock receipts')
            report['sql_entry_receipts'] = dict(zip(['product', 'variant', 'price', 'stock'], map(int, facts)))
        if device_entry:
            variant = report['variant_id']
            if not re.fullmatch(r'variant_[0-9a-f-]{36}', variant):
                raise RuntimeError('Owned intake variant identity invalid')
            facts = sql(database, "SELECT (SELECT COUNT(*) FROM device_tracking_changes WHERE variant_id='" + variant + "'),(SELECT COUNT(*) FROM device_intake_changes WHERE variant_id='" + variant + "'),(SELECT COUNT(*) FROM devices WHERE variant_id='" + variant + "' AND category='sale' AND state='available'),(SELECT COUNT(*) FROM devices WHERE variant_id='" + variant + "' AND category='returned' AND state='inspection'),(SELECT COUNT(*) FROM device_photos)").split('\t')
            if facts != ['1', '1', '1', '1', '1']:
                raise RuntimeError('Device intake tracking/batch/states/photo facts differ')
            report['sql_device_facts'] = dict(zip(['tracking_receipt', 'intake_receipt', 'available_sale', 'inspection_returned', 'photos'], map(int, facts)))
        if report['verified_accounts'] != (0 if product_entry else 1):
            raise RuntimeError('Expected exactly one newly verified fixture account')
        if payment_context and not (reviews or paid_warehouse or supplement):
            cart_id, order_id = report['cart_id'], report['order_id']
            if not re.fullmatch(r'cart_[0-9a-f-]{36}', cart_id) or not re.fullmatch(r'order_[0-9a-f-]{36}', order_id):
                raise RuntimeError('Owned payment context result identity invalid')
            if scenario == 'payment-context-changed':
                facts = changed_payment_context_facts(database, cart_id, order_id)
            elif scenario == 'payment-context-restore':
                facts = restored_payment_context_facts(database, cart_id, order_id)
                report['provider_observations'] = observe_context_payments(database, cart_id, '789.00')
            else:
                facts = payment_context_facts(database, cart_id, order_id)
            report['sql_payment_context_facts'] = facts
        if reviews:
            facts = sql(database, "SELECT (SELECT COUNT(*) FROM product_reviews WHERE account_id IS NOT NULL AND verified_purchase=TRUE AND status='approved'),(SELECT COUNT(*) FROM product_review_commands WHERE command='submit'),(SELECT COUNT(*) FROM product_review_commands WHERE command='moderate')").split('\t')
            if facts != ['1', '1', '4'] or protected_fingerprint() != review_before:
                raise RuntimeError('Review terminal facts or protected business fingerprint differ')
            report['sql_review_facts'] = {'approved_verified_review': 1, 'submission_receipt': 1, 'moderation_receipts': 4, 'protected_business_unchanged': True}
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
            expected_stock = str(Decimal(stock_before[0]) - 1) if paid_warehouse else '49'
            expected_reserved = str(Decimal(stock_before[1]) - 1) if paid_warehouse else '0'
            checks = {
                'warehouse_user': "SELECT COUNT(*) FROM admin_users WHERE email='local-warehouse-" + owner + "@example.invalid'",
                'canceled_parcel': "SELECT COUNT(*) FROM order_fulfillments o JOIN fulfillments f ON f.id=o.fulfillment_id WHERE o.order_id='" + order + "' AND f.canceled_at IS NOT NULL",
                'shipped_parcel': "SELECT COUNT(*) FROM order_fulfillments o JOIN fulfillments f ON f.id=o.fulfillment_id WHERE o.order_id='" + order + "' AND f.canceled_at IS NULL AND f.shipped_at IS NOT NULL",
                'shipped_device': "SELECT COUNT(*) FROM device_assignments a JOIN devices d ON d.id=a.device_id WHERE a.order_id='" + order + "' AND a.status='shipped' AND d.id='isolated-repair-device' AND d.state='shipped'",
                'unused_replacement': "SELECT COUNT(*) FROM devices WHERE id='isolated-exchange-device' AND state='available'",
                'stock_once': "SELECT COUNT(*) FROM inventory_levels WHERE inventory_item_id='iitem_01M51RN7HTPYQ7YFPXABBF8VJ8' AND location_id='sloc_01M4FBP3NDPZHJKVV3VY682QTN' AND stocked_quantity=" + expected_stock + " AND reserved_quantity=" + expected_reserved + " AND deleted_at IS NULL",
            }
            if intake_warehouse:
                device, spare, location = report['intake_device_id'], report['intake_spare_id'], report['intake_location_id']
                if not all(re.fullmatch(pattern, value) for pattern, value in [(r'dev_[0-9a-f-]{36}', device), (r'dev_[0-9a-f-]{36}', spare), (r'sloc_[0-9a-f-]{36}', location)]):
                    raise RuntimeError('Owned intake result identity invalid')
                checks['shipped_device'] = checks['shipped_device'].replace('isolated-repair-device', device)
                checks['unused_replacement'] = checks['unused_replacement'].replace('isolated-exchange-device', spare)
                checks['stock_once'] = checks['stock_once'].replace('stocked_quantity=' + expected_stock, 'stocked_quantity=' + stock_before[0])
                checks['intake_stock'] = "SELECT COUNT(*) FROM inventory_levels WHERE inventory_item_id='iitem_01M51RN7HTPYQ7YFPXABBF8VJ8' AND location_id='" + location + "' AND stocked_quantity=1 AND reserved_quantity=0 AND deleted_at IS NULL"
                checks['intake_batch'] = "SELECT COUNT(*) FROM device_intake_changes WHERE variant_id='variant_01M51RN7H7HPV9RH69T32QMD2Q'"
                checks['intake_photo'] = "SELECT COUNT(*) FROM device_photos"
                checks['return_inspection'] = "SELECT COUNT(*) FROM devices WHERE location_id='" + location + "' AND category='returned' AND state='inspection'"
            facts = {name: int(sql(database, query)) for name, query in checks.items()}
            if any(value != 1 for value in facts.values()):
                raise RuntimeError('Warehouse terminal facts differ: ' + str(facts))
            report['sql_warehouse_facts'] = facts
            if paid_warehouse:
                if payment_fingerprint() != payment_before:
                    raise RuntimeError('Paid order, ownership or payment facts changed during fulfillment')
                report['paid_checkpoint_financial_and_ownership_unchanged'] = True
                report['stock_before'] = stock_before
                report['stock_after'] = [stock_before[0] if intake_warehouse else expected_stock, expected_reserved]
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
                if supplement:
                    checks['settled_summary'] = checks['settled_summary'].replace("='789'", "='796.79'")
                    checks['applied_supplement'] = "SELECT COUNT(*) FROM order_payment_operations WHERE order_id='" + order + "' AND state='paid' AND applied=TRUE AND amount=7.79"
                facts = {name: int(sql(database, query)) for name, query in checks.items()}
                if facts != dict({'exchange': 1, 'received_return': 1, 'replacement_link': 1, 'shipped_parcels': 2, 'settled_summary': 1}, **({'applied_supplement': 1} if supplement else {})):
                    raise RuntimeError('Native exchange ledger or balance differs: ' + str(facts))
                report['sql_exchange_facts'] = facts
        if buyback:
            request = report['request_id']
            if not re.fullmatch(r'bbr_[0-9a-f-]{36}', request):
                raise RuntimeError('Unexpected owned request ID')
            rows = sql(database, "SELECT status,CAST(final_amount AS CHAR),payment_reference,return_tracking,closed_at IS NOT NULL "
                       "FROM buyback_requests WHERE id='" + request + "'").split('\t')
            expected = 'returned' if buyback_return else 'paid'
            if rows[0] != expected or rows[-1] != '1' or float(rows[1]) != 270.01:
                raise RuntimeError('Buyback SQL terminal state differs')
            if rows[2:4] != (['', 'RETURN-JOINT'] if buyback_return else ['TRANSFER-JOINT', '']):
                raise RuntimeError('Buyback payment reference or return tracking differs')
            report['sql_payment_reference'] = rows[2]
            report['sql_return_tracking'] = rows[3]
            report['sql_terminal_status'] = rows[0]
            report['sql_final_amount'] = rows[1]
            report['sql_closed'] = True
            report['sql_private_photos'] = int(sql(database, 'SELECT COUNT(*) FROM buyback_photo_bytes'))
            report['sql_device_links'] = int(sql(database, "SELECT COUNT(*) FROM buyback_device_links WHERE request_id='" + request + "'"))
            if report['sql_private_photos'] != 3 or report['sql_device_links'] != 1:
                raise RuntimeError('Buyback photo/device facts differ')
            if buyback_intake:
                device, source, location = (report[key] for key in ('intake_device_id', 'intake_source_id', 'intake_location_id'))
                if not all(re.fullmatch(prefix + r'_[0-9a-f-]{36}', value) for prefix, value in (('dev', device), ('dsrc', source), ('sloc', location))):
                    raise RuntimeError('Unexpected actual intake identities')
                facts = sql(database, "SELECT "
                    "(SELECT COUNT(*) FROM devices WHERE id='" + device + "' AND source_id='" + source + "' AND location_id='" + location + "' AND serial_number='SN-JOINT' AND imei='000000000000001' AND category='recycled' AND state='inspection' AND JSON_LENGTH(photos)=1),"
                    "(SELECT COUNT(*) FROM buyback_device_links WHERE request_id='" + request + "' AND device_id='" + device + "' AND deleted_at IS NULL),"
                    "(SELECT COUNT(*) FROM device_intake_changes),"
                    "(SELECT COUNT(*) FROM device_source_changes),"
                    "(SELECT COUNT(*) FROM device_photos),"
                    "(SELECT COUNT(*) FROM devices WHERE id='isolated-buyback-device')").split('\t')
                if list(map(int, facts)) != [1, 1, 1, 1, 1, 0]:
                    raise RuntimeError('Actual buyback intake facts differ: ' + str(facts))
                report['sql_actual_intake_facts'] = dict(zip(['inspection_device', 'linked_actual_device', 'intake_receipt', 'source_receipt', 'device_photo', 'seeded_device'], map(int, facts)))
    except BaseException:
        report['passed'] = False
        raise
    finally:
        for process in reversed(processes):
            if process.poll() is None:
                subprocess.run(['taskkill', '/PID', str(process.pid), '/T', '/F'],
                               capture_output=True, timeout=15)
                process.wait(timeout=10)
        for stream in streams:
            stream.close()
        cleanup_errors = []
        if (refund or payment_context) and created:
            try:
                # Keep an ignored durable copy even on browser failure: a real
                # test payment/refund may already exist and need reconciliation.
                (logs / ('refund-final.sql' if refund else 'payment-context-final.sql')).write_bytes(docker('exec', CONTAINER, 'mysqldump',
                    '--defaults-extra-file=/tmp/local-root.cnf', '--single-transaction',
                    '--set-gtid-purged=OFF', '--no-tablespaces', database))
                report['payment_durable_snapshot_retained'] = True
                if refund:
                    report['refund_durable_snapshot_retained'] = True
            except Exception:
                cleanup_errors.append('payment evidence snapshot')
        try:
            report['redis_keys_removed'] = redis_cleanup(prefix)
        except Exception:
            cleanup_errors.append('Redis cleanup')
        try:
            if created and (not (refund or payment_context) or report.get('payment_durable_snapshot_retained')):
                docker('exec', CONTAINER, 'mysql', '--defaults-extra-file=/tmp/local-root.cnf', '-e',
                       'DROP DATABASE ' + database + "; REVOKE ALL ON " + database + ".* FROM 'retail_local'@'%';")
            docker('exec', CONTAINER, 'rm', '-f', '/tmp/account-' + owner + '.sql')
        except Exception:
            cleanup_errors.append('database cleanup')
        if product_entry or buyback_intake:
            report['source_product_entry_unchanged'] = entry_source_fingerprint() == entry_source_before
            if not report['source_product_entry_unchanged']:
                report['passed'] = False
                cleanup_errors.append('source catalog changed during rehearsal')
        report['source_account_counts_before'] = source_counts
        report['source_account_counts_after'] = source_account_counts()
        report['resources_removed'] = not cleanup_errors
        report['cleanup_errors'] = cleanup_errors
        if cleanup_errors or report['source_account_counts_after'] != source_counts:
            report['passed'] = False
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
    elif sys.argv[1:] == ['--scenario=buyback-intake']:
        main('en', 'buyback-intake')
    elif sys.argv[1:] == ['--scenario=buyback-intake-return']:
        main('en', 'buyback-intake-return')
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
    elif len(sys.argv) == 2 and sys.argv[1].startswith('--verify-intake-delivery='):
        verify_intake_delivery_checkpoint(sys.argv[1].split('=', 1)[1])
    elif len(sys.argv) == 2 and sys.argv[1].startswith('--intake-warehouse-from-checkpoint='):
        main('en', 'intake-warehouse', sys.argv[1].split('=', 1)[1])
    elif len(sys.argv) == 2 and sys.argv[1].startswith('--warehouse-from-checkpoint='):
        main('en', 'paid-warehouse', sys.argv[1].split('=', 1)[1])
    elif len(sys.argv) == 2 and sys.argv[1].startswith('--supplement-from-checkpoint='):
        main('en', 'supplement', sys.argv[1].split('=', 1)[1])
    elif len(sys.argv) == 2 and sys.argv[1].startswith('--verify-supplement='):
        verify_supplement_checkpoint(sys.argv[1].split('=', 1)[1])
    elif len(sys.argv) == 2 and sys.argv[1].startswith('--verify-completed-refund='):
        verify_completed_refund_checkpoint(sys.argv[1].split('=', 1)[1])
    elif len(sys.argv) == 3 and sys.argv[1] == '--supplement-tax':
        database = sys.argv[2]
        if not re.fullmatch(r'retail_account_[0-9a-f]{12}', database):
            raise RuntimeError('Owned supplement database required')
        config()
        sql(database, "INSERT INTO tax_rates(id,rate,code,name,is_default,is_combinable,tax_region_id,created_at,updated_at) VALUES('isolated-supplement-tax',1,'isolated','Isolated exchange tax',TRUE,FALSE,'txreg_01M4FBP3N26K57MNJK7SJC376H',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))")
    elif len(sys.argv) == 2 and sys.argv[1].startswith('--verify-paid-warehouse='):
        verify_paid_warehouse_checkpoint(sys.argv[1].split('=', 1)[1])
    elif sys.argv[1:] == ['--scenario=device-entry']:
        main('en', 'device-entry')
    elif sys.argv[1:] == ['--scenario=product-entry']:
        main('en', 'product-entry')
    elif sys.argv[1:] == ['--scenario=mail']:
        main('en', 'mail')
    elif sys.argv[1:] == ['--scenario=payment-context']:
        main('en', 'payment-context')
    elif sys.argv[1:] == ['--scenario=payment-context-changed']:
        main('en', 'payment-context-changed')
    elif sys.argv[1:] == ['--scenario=payment-context-restore']:
        main('en', 'payment-context-restore')
    elif len(sys.argv) == 5 and sys.argv[1] == '--context-restore-fault':
        context_restore_fault(sys.argv[2], sys.argv[3], sys.argv[4])
    elif len(sys.argv) == 3 and sys.argv[1] == '--change-context-price':
        change_context_price(sys.argv[2])
    elif len(sys.argv) == 2 and sys.argv[1].startswith('--verify-payment-context='):
        verify_payment_context_checkpoint(sys.argv[1].split('=', 1)[1])
    elif len(sys.argv) == 2 and sys.argv[1].startswith('--resume-changed-context='):
        main('en', 'payment-context-changed', sys.argv[1].split('=', 1)[1])
    elif len(sys.argv) == 2 and sys.argv[1].startswith('--reviews-from-checkpoint='):
        main('en', 'reviews', sys.argv[1].split('=', 1)[1])
    elif len(sys.argv) == 2 and sys.argv[1].startswith('--resume-refund='):
        main('en', 'refund', sys.argv[1].split('=', 1)[1])
    else:
        raise RuntimeError('Unknown account rehearsal arguments')
