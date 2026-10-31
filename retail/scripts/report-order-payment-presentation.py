"""Bind current SQL presentation evidence and separate built-Astro fixture UI checks."""
import hashlib
import json
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[2]
SUITES = ['PromotionUsage','PromotionSchema','GeoSchema','CartPriceRefresh','CartTaxRefresh','TaxImport','TaxSchema','AdminAuthentication','CatalogSchemaTransition','ProductEdits','ProductPrices','ProductInventory','InventoryStockEdits','ReservationTransactions','OrderSchema','OrderImport','CustomerHistory','OrderRead','PaymentSchema','PaymentImport','FulfillmentHistory','CartSchema','CartRuntime']


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    transcript = ROOT/'logs/gin-migration/warehouse-auth-sql.log'
    full_path = ROOT/'docs/migration/admin-auth-report.json'
    text = transcript.read_text()
    passed = re.findall(r'^--- PASS: TestOwned(\w+) \([\d.]+s\)$', text, re.M)
    full = json.loads(full_path.read_text())
    if sorted(passed) != sorted(SUITES) or re.search(r'^--- FAIL:|^FAIL$', text, re.M) or full_path.stat().st_mtime_ns < transcript.stat().st_mtime_ns or full.get('cleanup') != 'both owned containers and temporary runner removed':
        raise RuntimeError('All 23 SQL suites and cleanup required')
    manifest = dict(full.get('implementation_sha256', {}))
    migrations = {p.relative_to(ROOT).as_posix() for p in (ROOT/'retail/backend/migrations').glob('*.sql')}
    if len(migrations) != 87 or migrations != {p for p in manifest if p.startswith('retail/backend/migrations/') and p.endswith('.sql')}:
        raise RuntimeError('Tested migration snapshot changed')
    required = ['retail/backend/internal/orderpayments/presentation.go','retail/backend/internal/orderpayments/presentation_test.go','retail/backend/internal/customer/order_payments.go','retail/backend/internal/customer/order_payments_http.go','retail/backend/internal/customer/http.go','retail/backend/internal/identity/order_payment_customer_http_test.go','retail/backend/internal/identity/order_payment_consumption_test.go']
    if any(p not in manifest for p in required):
        raise RuntimeError('Presentation sources absent from SQL manifest')
    def marker(source, name):
        rows = [json.loads(line.split(name+' ',1)[1]) for line in source.splitlines() if name+' ' in line]
        if len(rows) != 1:
            raise RuntimeError('Unique evidence required: '+name)
        return rows[0]
    http = marker(text,'ORDER_PAYMENT_CUSTOMER_HTTP_REHEARSAL')
    cash = marker(text,'ORDER_PAYMENT_PRESENTATION_REHEARSAL')
    for name in ['exact_current_consent_and_history','active_instruction_no_second_consent','protected_checkout_url','stale_version_hides_url','corrupt_creation_url_denied']:
        if http.get(name) is not True:
            raise RuntimeError('Missing HTTP presentation assertion: '+name)
    for name in ['exact_zero_due_after_cash','paid_receipt_only_after_application','retired_cart_independent','paid_checkout_url_absent','new_generation_read']:
        if cash.get(name) is not True:
            raise RuntimeError('Missing cash presentation assertion: '+name)
    folder = ROOT/'logs/gin-migration/order-payment-ui-validation-87'
    frontend = json.loads((folder/'frontend-source.json').read_text())
    frontend_manifest = frontend.get('implementation_sha256_lf',{})
    if not frontend_manifest or any(p not in frontend_manifest for p in ['retail/storefront/src/lib/order-payments.ts','retail/storefront/src/lib/order-payment-return.ts','retail/storefront/src/lib/customer-client.ts','retail/storefront/src/lib/customer-bridge.ts','retail/storefront/src/components/order/supplemental-payment.tsx','retail/storefront/src/components/account/order-details.tsx','retail/storefront/src/pages/api/mollie-return.ts','retail/storefront/tests/order-payment.browser.mjs']):
        raise RuntimeError('Frontend dependency manifest incomplete')
    for name,expected in frontend.get('transcript_sha256',{}).items():
        if digest(folder/name) != expected:
            raise RuntimeError('Frontend validation transcript changed')
    expected_logs = {'browser.log','build.log','node.log','legacy-checkout.log','legacy-orders.log'}
    if set(frontend.get('transcript_sha256',{})) != expected_logs:
        raise RuntimeError('All browser, build, node and legacy transcripts required')
    browser = marker((folder/'browser.log').read_text(),'ORDER_PAYMENT_BROWSER_REHEARSAL')
    for name in ['built_astro','signed_formal_customer_routes','exact_consent','lost_post_same_uuid','reload_recovers_existing_instruction','explicit_canceled_predecessor','no_cart_bearer','read_only_provider_return','paid_not_success_until_applied','original_and_supplemental_amounts','no_refresh_loop','no_overflow']:
        if browser.get(name) is not True:
            raise RuntimeError('Missing UI assertion: '+name)
    if browser.get('languages') != ['en','zh'] or browser.get('widths') != [390,1440] or browser.get('browser_errors') != 0 or any(browser.get(k) is not False for k in ['real_gin_mysql','independent_payment_cli','real_provider']) or cash.get('provider_called_by_presentation') is not False:
        raise RuntimeError('Unexpected end-to-end or provider acceptance claim')
    node = (folder/'node.log').read_text()
    build = (folder/'build.log').read_text()
    if not all(re.search(r'(?:ℹ |# )'+pattern+r'\b',node) for pattern in ['tests 24','pass 24','fail 0']) or '- 0 errors' not in build or '- 0 warnings' not in build or '[build] Complete!' not in build:
        raise RuntimeError('Passed final node/type/build checks required')
    legacy_checkout = marker((folder/'legacy-checkout.log').read_text(),'ASTRO_CHECKOUT_CONTACT_BROWSER')
    legacy_orders = marker((folder/'legacy-orders.log').read_text(),'CUSTOMER_ORDERS_BROWSER')
    if legacy_checkout.get('stable_return_callback') is not True or legacy_checkout.get('browser_errors') != 0 or legacy_orders.get('account_list_and_details') is not True or legacy_orders.get('browser_errors') != 0:
        raise RuntimeError('Passed legacy checkout/orders browser checks required')
    manifest.update(frontend_manifest)
    own = 'retail/scripts/report-order-payment-presentation.py'
    manifest[own] = hashlib.sha256((ROOT/own).read_bytes().replace(b'\r\n',b'\n')).hexdigest()
    if any(not (ROOT/p).is_file() or hashlib.sha256((ROOT/p).read_bytes().replace(b'\r\n',b'\n')).hexdigest() != sha for p,sha in manifest.items()):
        raise RuntimeError('Source changed after validation')
    report = {'format_version':1,'scope':'Current owned supplemental consent/history and protected checkout URL over dedicated SQL/real customer Redis HTTP; separately, a built Astro page and signed formal bridge against an explicit local API/provider fixture. Four language/viewport cases, lost-response same UUID, reload recovery, explicit cancel predecessor, customer-only read-only return and paid-after-local-application display are accepted. These are separate proofs; no actual positive-exchange customer/CLI continuous chain, payment notice, supplemental refund, real provider or mail acceptance.','migration_count':87,'sql_suites_passed':passed,'cleanup':full['cleanup'],'http_presentation':http,'cash_presentation':cash,'fixture_browser':browser,'legacy_checkout_browser':legacy_checkout,'legacy_orders_browser':legacy_orders,'node_tests_passed':24,'astro_files':int(re.search(r'Result \((\d+) files\)',build).group(1)),'frontend_evidence':frontend,'actual_positive_exchange_customer_cli_accepted':False,'payment_notice_accepted':False,'supplemental_refund_accepted':False,'real_provider_called':False,'transcript_sha256':digest(transcript),'full_sql_report_sha256':digest(full_path),'implementation_sha256_lf':manifest}
    (ROOT/'docs/migration/order-payment-presentation-report.json').write_text(json.dumps(report,indent=2)+'\n')
    print(json.dumps({'order_payment_presentation':'passed','sql_suites':23,'fixture_browser_cases':4,'actual_positive_exchange_customer_cli':False}))


if __name__ == '__main__':
    main()
