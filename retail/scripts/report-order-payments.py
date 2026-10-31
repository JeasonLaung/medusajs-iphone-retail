"""Derive disabled, offline order-payment command evidence from full owned SQL."""
import hashlib
import json
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[2]
SUITES = ['PromotionUsage','PromotionSchema','GeoSchema','CartPriceRefresh','CartTaxRefresh','TaxImport','TaxSchema','AdminAuthentication','CatalogSchemaTransition','ProductEdits','ProductPrices','ProductInventory','InventoryStockEdits','ReservationTransactions','OrderSchema','OrderImport','CustomerHistory','OrderRead','PaymentSchema','PaymentImport','FulfillmentHistory','CartSchema','CartRuntime']


def main():
    log = ROOT/'logs/gin-migration/warehouse-auth-sql.log'
    text = log.read_text()
    passed = re.findall(r'^--- PASS: TestOwned(\w+) \([\d.]+s\)$', text, re.M)
    if sorted(passed) != sorted(SUITES) or re.search(r'^--- FAIL:|^FAIL$', text, re.M):
        raise RuntimeError('All 23 owned SQL suites must pass')
    cleanup_path = ROOT/'docs/migration/admin-auth-report.json'
    cleanup = json.loads(cleanup_path.read_text())
    if cleanup_path.stat().st_mtime_ns < log.stat().st_mtime_ns or cleanup.get('cleanup') != 'both owned containers and temporary runner removed':
        raise RuntimeError('Full runner cleanup required')
    rows = [json.loads(line.split('NATIVE_ORDER_PAYMENT_COMMANDS_REHEARSAL ',1)[1]) for line in text.splitlines() if 'NATIVE_ORDER_PAYMENT_COMMANDS_REHEARSAL ' in line]
    if len(rows) != 1 or rows[0].get('migration') != 87 or rows[0].get('due') != '6.00':
        raise RuntimeError('Current supplemental command evidence required')
    commands = rows[0]
    for name in ['default_disabled','actual_completed_checkout','native_positive_exchange','protected_request_and_session','order_scope_not_cart_scope','immutable_uuid_replay','current_account_and_explicit_ownership','revoked_generation_replay_denied','one_active_instruction','payment_refund_mutual_exclusion','unapplied_refund_blocks_payment','explicit_successor_chain','local_cancel_even_disabled','late_stage_failure_atomic','late_cancel_failure_atomic','caller_rollback_preserved','claimed_effect_cancel_denied','pending_version_change_denied','disabled_pending_version_change_denied','cancel_releases_version','completed_cart_preserved','independent_retired_cart_capability','artificial_account_ownership']:
        if commands.get(name) is not True:
            raise RuntimeError('Missing supplemental command assertion: '+name)
    for name in ['real_customer_session','concurrency_tested','provider_dispatch','cash_created','browser_cli_acceptance']:
        if commands.get(name) is not False:
            raise RuntimeError('Unexpected supplemental acceptance claim: '+name)
    # Derive evidence only from the same source snapshot as the completed run.
    manifest = cleanup.get('implementation_sha256', {})
    migrations = {p.relative_to(ROOT).as_posix() for p in (ROOT/'retail/backend/migrations').glob('*.sql')}
    tested_migrations = {p for p in manifest if p.startswith('retail/backend/migrations/') and p.endswith('.sql')}
    if not manifest or migrations != tested_migrations or len(migrations) != 87:
        raise RuntimeError('Completed SQL source snapshot/migration set differs; use an isolated validation checkout')
    if any(not (ROOT/p).is_file() or hashlib.sha256((ROOT/p).read_bytes().replace(b'\r\n', b'\n')).hexdigest() != digest for p, digest in manifest.items()):
        raise RuntimeError('Source changed after SQL validation; do not regenerate evidence from untested files')
    files = ['retail/backend/internal/identity/order_payment_commands_test.go','retail/backend/internal/identity/exchange_payment_balance_test.go','retail/backend/internal/identity/order_refund_commands_test.go','retail/backend/internal/identity/cart_completion_test.go','retail/backend/internal/orderimport/import.go','retail/scripts/test-admin-auth.py','retail/scripts/report-order-payments.py']
    for folder in ['orderpayments','orders','refunds','payments','protected']:
        files += [p.relative_to(ROOT).as_posix() for p in (ROOT/'retail/backend/internal'/folder).glob('*') if p.is_file()]
    files += [p.relative_to(ROOT).as_posix() for p in (ROOT/'retail/backend/migrations').glob('*.sql')]
    report = {
        'format_version':1,
        'scope':'Offline native order-payment staging, current account/ownership checks, protected request/session, local cancellation, explicit successors and order-version/refund exclusion; account ownership is an artificial SQL fixture on a real completed checkout and rollback-only native exchange',
        'migration_count':87,'native_order_runtime_version':2,'refund_runtime_version':4,
        'commands':commands,'sql_suites_passed':passed,'cleanup':cleanup['cleanup'],
        'online_supplemental_payment_accepted':False,'customer_session_or_http_accepted':False,
        'provider_create_or_recovery_accepted':False,'paid_capture_consumption_accepted':False,
        'worker_or_cli_accepted':False,'real_provider_called':False,
        'transcript_sha256':hashlib.sha256(log.read_bytes()).hexdigest(),
        'full_sql_report_sha256':hashlib.sha256(cleanup_path.read_bytes()).hexdigest(),
        'implementation_sha256_lf':{p:hashlib.sha256((ROOT/p).read_bytes().replace(b'\r\n',b'\n')).hexdigest() for p in sorted(set(files))},
    }
    (ROOT/'docs/migration/order-payment-commands-report.json').write_text(json.dumps(report,indent=2)+'\n')
    print(json.dumps({'order_payment_commands':'passed','sql_suites':len(passed),'online_payment_accepted':False}))


if __name__ == '__main__':
    main()
