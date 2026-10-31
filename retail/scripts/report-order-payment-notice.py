"""Bind transactional payment notice, current inbox HTTP and independent UI evidence."""
import hashlib
import json
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[2]
EVIDENCE = ROOT/'logs/gin-migration/order-payment-notice-validation-88'
SUITES = ['PromotionUsage','PromotionSchema','GeoSchema','CartPriceRefresh','CartTaxRefresh','TaxImport','TaxSchema','AdminAuthentication','CatalogSchemaTransition','ProductEdits','ProductPrices','ProductInventory','InventoryStockEdits','ReservationTransactions','OrderSchema','OrderImport','CustomerHistory','OrderRead','PaymentSchema','PaymentImport','FulfillmentHistory','CartSchema','CartRuntime']


def sha(path):
    return hashlib.sha256(path.read_bytes().replace(b'\r\n',b'\n')).hexdigest()


def marker(text, name):
    rows = [json.loads(line.split(name+' ',1)[1]) for line in text.splitlines() if name+' ' in line]
    if len(rows) != 1:
        raise RuntimeError('Unique evidence required: '+name)
    return rows[0]


def main():
    log = ROOT/'logs/gin-migration/warehouse-auth-sql.log'
    text = log.read_text()
    passed = re.findall(r'^--- PASS: TestOwned(\w+) \([\d.]+s\)$',text,re.M)
    if sorted(passed) != sorted(SUITES) or re.search(r'^--- FAIL:|^FAIL$',text,re.M):
        raise RuntimeError('All 23 SQL suites required')
    full_path = ROOT/'docs/migration/admin-auth-report.json'
    full = json.loads(full_path.read_text())
    cli_path = EVIDENCE/'cli-source.json'
    cli = json.loads(cli_path.read_text())
    cli_log = EVIDENCE/'cli-sql.log'
    if not re.search(r'^--- PASS: TestOwnedCartRuntime \([\d.]+s\)$',cli_log.read_text(),re.M) or re.search(r'^--- FAIL:|^FAIL$',cli_log.read_text(),re.M):
        raise RuntimeError('Completed independent CLI SQL check required')
    migrations = {p.relative_to(ROOT).as_posix() for p in (ROOT/'retail/backend/migrations').glob('*.sql')}
    if len(migrations) != 88 or 'retail/backend/migrations/0093_order_payment_customer_notice.sql' not in migrations or any('/0088_' in p or '/0089_' in p or '/0090_' in p or '/0091_' in p or '/0092_' in p for p in migrations):
        raise RuntimeError('Committed 87 plus notice 0093 snapshot required')
    sources = {}
    for report,path,transcript in [(full,full_path,log),(cli,cli_path,cli_log)]:
        if report.get('cleanup') != 'both owned containers and temporary runner removed' or path.stat().st_mtime_ns < transcript.stat().st_mtime_ns:
            raise RuntimeError('Owned cleanup evidence required')
        manifest = report.get('implementation_sha256',{})
        tested = {p for p in manifest if p.startswith('retail/backend/migrations/') and p.endswith('.sql')}
        if tested != migrations:
            raise RuntimeError('Tested migration manifest differs')
        if not manifest or any(not (ROOT/p).is_file() or sha(ROOT/p) != digest for p,digest in manifest.items()):
            raise RuntimeError('Source changed after verification')
        for p,digest in manifest.items():
            if p in sources and sources[p] != digest:
                raise RuntimeError('SQL snapshots differ')
            sources[p] = digest
    notice = marker(text,'ORDER_PAYMENT_NOTICE_REHEARSAL')
    for key in ['cash_notice_same_transaction','late_notice_failure_rolls_back_cash','last_ack_rolls_back_notice','concurrent_consumers_one_notice','late_observation_no_duplicate','explicit_new_account_owner','disabled_unverified_unowned_no_recipient','real_login_sql_redis','current_generation_owner_inbox','private_http_projection','foreign_read_and_mark_isolation','owner_csrf_and_read_replay','revoked_session_denied']:
        if notice.get(key) is not True:
            raise RuntimeError('Notice assertion absent: '+key)
    if any(notice.get(k) is not False for k in ['registration_browser','real_provider_or_mail','customer_cli_joint']):
        raise RuntimeError('Unexpected joint/channel claim')
    cash = marker(text,'NATIVE_ORDER_PAYMENT_CONSUMPTION_REHEARSAL')
    if cash.get('payment_notice_accepted') is not True or cash.get('exact_total_cash') != '164.75' or cash.get('concurrent_consumers') != 6:
        raise RuntimeError('Current unique cash and notice proof required')
    cli_evidence = marker(cli_log.read_text(),'ORDER_PAYMENT_CLI_REHEARSAL')
    if cli_evidence != cli.get('order_payment_cli') or cli_evidence.get('distinct_processes') != 4 or any(cli_evidence.get(k) is not True for k in ['production_binary','single_post','last_ack_rolls_back_cash','new_process_local_reapply_without_network','disabled_recipient_no_notice']):
        raise RuntimeError('Disabled recipient independent CLI proof required')
    if not re.fullmatch('[0-9a-f]{64}',cli.get('order_payment_cli_binary_sha256','')):
        raise RuntimeError('Actual compiled CLI digest required')
    frontend = json.loads((EVIDENCE/'frontend-source.json').read_text())
    for p,digest in frontend['implementation_sha256_lf'].items():
        if any(part in ('.astro','node_modules','dist') for part in Path(p).parts):
            raise RuntimeError('Generated frontend files are not source evidence')
        if sha(ROOT/p) != digest:
            raise RuntimeError('Frontend changed after verification')
        sources[p] = digest
    for name,digest in frontend['transcript_sha256'].items():
        if sha(EVIDENCE/name) != digest:
            raise RuntimeError('Frontend transcript changed')
    node = (EVIDENCE/'node.log').read_text()
    if not re.search(r'(?:#|ℹ) pass 27\b',node) or not re.search(r'(?:#|ℹ) fail 0\b',node):
        raise RuntimeError('All 27 Node tests required')
    astro = (EVIDENCE/'astro.log').read_text()
    if not re.search(r'0 errors?\b',astro) or not re.search(r'0 warnings?\b',astro):
        raise RuntimeError('Astro diagnostics required')
    browser = marker((EVIDENCE/'browser.log').read_text(),'CUSTOMER_NOTICES_BROWSER')
    if any(browser.get(k) is not True for k in ['synthetic_api','original_dialog_structure','unread_badge','pagination_20','single_and_all_read','detail_navigation','supplemental_payment_notice','error_retains_data','retry','chinese_english','mobile_390']) or browser.get('browser_errors') != 0:
        raise RuntimeError('Original notice dialog browser assertions required')
    required = ['retail/backend/internal/customernotices/events.go','retail/backend/internal/orderpayments/consumption.go','retail/backend/internal/identity/order_payment_notice_test.go','retail/backend/internal/identity/order_payment_consumption_test.go','retail/backend/internal/identity/order_payment_cli_test.go','retail/storefront/src/lib/customer-notices.ts','retail/storefront/src/components/notifications/i18n.ts','retail/storefront/src/components/notifications/notification-bell.tsx','retail/storefront/tests/customer-notices.browser.mjs']
    if any(p not in sources for p in required):
        raise RuntimeError('Notice dependencies missing')
    own = 'retail/scripts/report-order-payment-notice.py'
    sources[own] = sha(ROOT/own)
    report = {'format_version':1,'scope':'Transactional supplemental payment.received inbox event keyed by unique local payment; actual notification GET/encryption/cash with late notice and ACK rollback, six concurrent consumers and current-generation real SQL/Redis customer Login/private HTTP inbox. Four production CLI processes separately verify disabled-recipient recovery without a notice. Existing notification dialog browser uses an explicit synthetic API; en/zh labels, unread, pagination, read/retry/navigation and mobile layout pass. These are separate proofs, not the four-new-customer positive-exchange browser/CLI continuous chain.','migration_count':88,'notice_migration':'0093_order_payment_customer_notice.sql','parallel_catalog_migrations_included':False,'notice':notice,'cash':cash,'disabled_recipient_cli':cli_evidence,'production_cli_binary_sha256':cli['order_payment_cli_binary_sha256'],'fixture_browser':browser,'node_tests_passed':27,'sql_suites_passed':passed,'cleanup':full['cleanup'],'actual_positive_exchange_customer_cli_joint_accepted':False,'supplemental_refund_accepted':False,'real_provider_or_mail_called':False,'transcript_sha256':{'full_sql':sha(log),'cli_sql':sha(cli_log),**frontend['transcript_sha256']},'input_report_sha256':{'full_sql':sha(full_path),'cli':sha(cli_path)},'implementation_sha256_lf':sources}
    (ROOT/'docs/migration/order-payment-notice-report.json').write_text(json.dumps(report,indent=2)+'\n')
    print(json.dumps({'order_payment_notice':'passed','sql_suites':len(passed),'migrations':len(migrations),'source_fingerprints':len(sources),'customer_browser_cli_joint':False}))


if __name__ == '__main__':
    main()
