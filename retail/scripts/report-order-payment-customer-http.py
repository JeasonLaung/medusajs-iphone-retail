"""Verify supplemental customer HTTP evidence from the complete frozen SQL run."""
import hashlib
import json
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[2]
SUITES = ['PromotionUsage','PromotionSchema','GeoSchema','CartPriceRefresh','CartTaxRefresh','TaxImport','TaxSchema','AdminAuthentication','CatalogSchemaTransition','ProductEdits','ProductPrices','ProductInventory','InventoryStockEdits','ReservationTransactions','OrderSchema','OrderImport','CustomerHistory','OrderRead','PaymentSchema','PaymentImport','FulfillmentHistory','CartSchema','CartRuntime']


def main():
    transcript = ROOT/'logs/gin-migration/warehouse-auth-sql.log'
    report_path = ROOT/'docs/migration/admin-auth-report.json'
    text = transcript.read_text()
    passed = re.findall(r'^--- PASS: TestOwned(\w+) \([\d.]+s\)$', text, re.M)
    if sorted(passed) != sorted(SUITES) or re.search(r'^--- FAIL:|^FAIL$', text, re.M):
        raise RuntimeError('Complete 23-suite SQL run required')
    full = json.loads(report_path.read_text())
    if report_path.stat().st_mtime_ns < transcript.stat().st_mtime_ns or full.get('cleanup') != 'both owned containers and temporary runner removed':
        raise RuntimeError('Completed owned resource cleanup required')
    manifest = dict(full.get('implementation_sha256', {}))
    migrations = {p.relative_to(ROOT).as_posix() for p in (ROOT/'retail/backend/migrations').glob('*.sql')}
    tested = {p for p in manifest if p.startswith('retail/backend/migrations/') and p.endswith('.sql')}
    if migrations != tested or len(migrations) != 87:
        raise RuntimeError('Frozen migration snapshot changed')
    required_sources = ['retail/backend/internal/customer/order_payments.go','retail/backend/internal/customer/order_payments_http.go','retail/backend/internal/customer/http.go','retail/backend/internal/customer/accounts.go','retail/backend/internal/customer/addresses.go','retail/backend/internal/orderpayments/cancellation.go','retail/backend/internal/identity/order_payment_customer_http_test.go','retail/backend/internal/identity/order_payment_execution_test.go','retail/backend/cmd/api/main.go']
    if any(name not in manifest for name in required_sources) or any(not (ROOT/name).is_file() or hashlib.sha256((ROOT/name).read_bytes().replace(b'\r\n',b'\n')).hexdigest() != digest for name,digest in manifest.items()):
        raise RuntimeError('Source missing from tested manifest or changed after validation')
    rows = [json.loads(line.split('ORDER_PAYMENT_CUSTOMER_HTTP_REHEARSAL ',1)[1]) for line in text.splitlines() if 'ORDER_PAYMENT_CUSTOMER_HTTP_REHEARSAL ' in line]
    if len(rows) != 1:
        raise RuntimeError('Unique customer HTTP evidence required')
    evidence = rows[0]
    for field in ['real_redis_customer_capability','authenticated_transaction','revocation_while_waiting_account_lock_denied','csrf_origin','realm_and_owner_denied','strict_wire','private_receipt','revoked_disabled_stale_unverified_denied','late_cancel_rollback','feature_off_read_cancel']:
        if evidence.get(field) is not True:
            raise RuntimeError('Missing assertion: '+field)
    if evidence.get('concurrent_prepare') != 6 or evidence.get('provider_calls') != 0 or any(evidence.get(field) is not False for field in ['registration_browser','payment_notice']):
        raise RuntimeError('Unexpected scope/concurrency claim')
    folder = ROOT/'logs/gin-migration/order-payment-customer-http-validation-87'
    frontend = json.loads((folder/'frontend-source.json').read_text())
    node = (folder/'bridge.log').read_text()
    astro = (folder/'astro.log').read_text()
    if not re.search(r'(?:ℹ |# )tests 12\b', node) or not re.search(r'(?:ℹ |# )pass 12\b', node) or not re.search(r'(?:ℹ |# )fail 0\b', node) or '- 0 errors' not in astro or '- 0 warnings' not in astro:
        raise RuntimeError('Passed bridge/proof tests and Astro check required')
    for field,path in [('node_transcript_sha256',folder/'bridge.log'),('astro_transcript_sha256',folder/'astro.log')]:
        if hashlib.sha256(path.read_bytes()).hexdigest() != frontend.get(field):
            raise RuntimeError('Frontend transcript changed')
    frontend_manifest = frontend.get('implementation_sha256_lf', {})
    if len(frontend_manifest) != 9 or any(not (ROOT/name).is_file() or hashlib.sha256((ROOT/name).read_bytes().replace(b'\r\n',b'\n')).hexdigest() != digest for name,digest in frontend_manifest.items()):
        raise RuntimeError('Frontend source changed after validation')
    manifest.update(frontend_manifest)
    name = 'retail/scripts/report-order-payment-customer-http.py'
    manifest[name] = hashlib.sha256((ROOT/name).read_bytes().replace(b'\r\n',b'\n')).hexdigest()
    report = {'format_version':1,'scope':'Private Gin prepare/read/cancel over real Redis customer sessions and dedicated SQL ownership/account fixtures. Actual customer login derives the session version from the fixture password, enabled/verified status and auth version; Astro bridge forwards only these fixed paths/current customer capability and refuses incorrect success status; no registration browser, redirect UI, independent payment CLI, notice, supplemental refund or real provider acceptance. HTTP stages only; recovery and cash remain worker responsibilities.','migration_count':87,'customer_http':evidence,'frontend_bridge_tests':12,'astro_check':{'files':396,'errors':0,'warnings':0,'hints':7},'frontend_evidence':frontend,'sql_suites_passed':passed,'cleanup':full['cleanup'],'customer_session_http_accepted':True,'customer_payment_browser_accepted':False,'worker_cli_process_accepted':False,'customer_payment_notice_accepted':False,'supplemental_payment_refund_accepted':False,'real_provider_called':False,'transcript_sha256':hashlib.sha256(transcript.read_bytes()).hexdigest(),'full_sql_report_sha256':hashlib.sha256(report_path.read_bytes()).hexdigest(),'implementation_sha256_lf':manifest}
    (ROOT/'docs/migration/order-payment-customer-http-report.json').write_text(json.dumps(report,indent=2)+'\n')
    print(json.dumps({'order_payment_customer_http':'passed','sql_suites':len(passed),'browser_cli_accepted':False}))


if __name__ == '__main__':
    main()
