"""Derive the native supplemental creation/GET/cash SQL evidence, not UI acceptance."""
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
        raise RuntimeError('All 23 owned SQL suites required')
    cleanup_path = ROOT/'docs/migration/admin-auth-report.json'
    cleanup = json.loads(cleanup_path.read_text())
    if cleanup_path.stat().st_mtime_ns < log.stat().st_mtime_ns or cleanup.get('cleanup') != 'both owned containers and temporary runner removed':
        raise RuntimeError('Completed full SQL cleanup required')
    manifest = cleanup.get('implementation_sha256', {})
    migrations = {p.relative_to(ROOT).as_posix() for p in (ROOT/'retail/backend/migrations').glob('*.sql')}
    tested = {p for p in manifest if p.startswith('retail/backend/migrations/') and p.endswith('.sql')}
    if migrations != tested or len(migrations) != 87:
        raise RuntimeError('Completed SQL migration snapshot differs')
    if not manifest or any(not (ROOT/p).is_file() or hashlib.sha256((ROOT/p).read_bytes().replace(b'\r\n',b'\n')).hexdigest() != digest for p,digest in manifest.items()):
        raise RuntimeError('Source changed after SQL validation')
    def marker(name):
        rows = [json.loads(line.split(name+' ',1)[1]) for line in text.splitlines() if name+' ' in line]
        if len(rows) != 1:
            raise RuntimeError('Unique evidence required: '+name)
        return rows[0]
    execution = marker('NATIVE_ORDER_PAYMENT_EXECUTION_REHEARSAL')
    cash = marker('NATIVE_ORDER_PAYMENT_CONSUMPTION_REHEARSAL')
    for name in ['actual_separate_completed_checkout','committed_native_positive_exchange','one_post_per_instruction','durable_claim_before_post','live_lease_no_post_or_discovery','revoked_first_dispatch_denied','disabled_first_dispatch_denied','stale_unverified_dispatch_denied','disabled_provider_dispatch_denied','changed_amount_dispatch_denied','explicit_failed_successor','lost_response_get_only','foreign_discovery_denied','late_creation_failure_atomic','protected_creation_survives_disabled_inbox','recovery_after_account_cart_order_retirement','disabled_feature_recovery','reconstructed_service_no_network_for_saved_creation','creation_or_discovery_not_cash']:
        if execution.get(name) is not True:
            raise RuntimeError('Missing creation/recovery assertion: '+name)
    for name in ['actual_notification_get_and_encrypted_evidence','authorized_not_cash','open_does_not_regress_authorized','cross_store_denied','corrupt_envelope_denied','changed_balance_retains_paid_evidence','late_ledger_failure_atomic','last_consumption_failure_atomic','worker_run_once_accepted','idle_cross_store_worker','unique_payment_capture_transaction','collection_completed_session_captured','summary_zero_difference','local_retry_without_network','revoked_account_retired_cart_recovery','canceled_order_cash_without_reopening','late_unpaid_does_not_regress_cash','applied_releases_version_hold','original_cart_preserved','no_new_order_stock_or_device_mutation']:
        if cash.get(name) is not True:
            raise RuntimeError('Missing cash assertion: '+name)
    if execution.get('due') != '6.00' or execution.get('concurrent_prepare') != 6 or execution.get('concurrent_run') != 6 or cash.get('paid_due') != '6.00' or cash.get('exact_total_cash') != '164.75' or cash.get('concurrent_consumers') != 6:
        raise RuntimeError('Current exact cash/concurrency evidence required')
    for row,names in [(execution,['real_provider','customer_session_or_browser','cash_consumed']),(cash,['real_provider','customer_session_browser_cli','worker_cli_process_accepted','payment_notice_accepted'])]:
        if any(row.get(name) is not False for name in names):
            raise RuntimeError('Unexpected broader acceptance claim')
    sources = dict(manifest)
    name = 'retail/scripts/report-order-payment-runtime.py'
    sources[name] = hashlib.sha256((ROOT/name).read_bytes().replace(b'\r\n',b'\n')).hexdigest()
    report = {'format_version':1,'scope':'SQL-only native supplemental creation, GET-only lost-response recovery, encrypted notification consumption, unique cash and independent Worker.RunOnce over a separate actual native checkout/exchange; account ownership, device receipt and provider are artificial fixtures; no customer session, browser, separately launched CLI or real channel acceptance','migration_count':87,'execution':execution,'cash':cash,'sql_suites_passed':passed,'cleanup':cleanup['cleanup'],'worker_run_once_accepted':True,'worker_cli_process_accepted':False,'customer_session_http_browser_accepted':False,'customer_payment_notice_accepted':False,'supplemental_payment_refund_accepted':False,'real_provider_called':False,'transcript_sha256':hashlib.sha256(log.read_bytes()).hexdigest(),'full_sql_report_sha256':hashlib.sha256(cleanup_path.read_bytes()).hexdigest(),'implementation_sha256_lf':sources}
    (ROOT/'docs/migration/order-payment-runtime-report.json').write_text(json.dumps(report,indent=2)+'\n')
    print(json.dumps({'order_payment_runtime':'passed','sql_suites':len(passed),'supplemental_cash':'6.00','browser_cli_accepted':False}))


if __name__ == '__main__':
    main()
