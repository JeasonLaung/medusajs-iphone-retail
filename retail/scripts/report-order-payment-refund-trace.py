"""Bind supplemental refund SQL provenance and existing shopping-chain regression."""
import hashlib
import json
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[2]
SUITES = ['PromotionUsage','PromotionSchema','GeoSchema','CartPriceRefresh','CartTaxRefresh','TaxImport','TaxSchema','AdminAuthentication','CatalogSchemaTransition','ProductEdits','ProductPrices','ProductInventory','InventoryStockEdits','ReservationTransactions','OrderSchema','OrderImport','CustomerHistory','OrderRead','PaymentSchema','PaymentImport','FulfillmentHistory','CartSchema','CartRuntime']


def sha(p):
    return hashlib.sha256(p.read_bytes().replace(b'\r\n',b'\n')).hexdigest()


def main():
    full_path=ROOT/'docs/migration/admin-auth-report.json'
    full=json.loads(full_path.read_text())
    log=ROOT/'logs/gin-migration/warehouse-auth-sql.log'
    text=log.read_text()
    passed=re.findall(r'^--- PASS: TestOwned(\w+) \([\d.]+s\)$',text,re.M)
    if sorted(passed)!=sorted(SUITES) or re.search(r'^--- FAIL:|^FAIL$',text,re.M):
        raise RuntimeError('Completed full 23 SQL suites required')
    if full.get('cleanup')!='both owned containers and temporary runner removed' or full_path.stat().st_mtime_ns<log.stat().st_mtime_ns:
        raise RuntimeError('Completed SQL cleanup required')
    rows=[json.loads(line.split('SUPPLEMENTAL_REFUND_TRACE_REHEARSAL ',1)[1]) for line in text.splitlines() if 'SUPPLEMENTAL_REFUND_TRACE_REHEARSAL ' in line]
    if len(rows)!=1:
        raise RuntimeError('Unique supplemental refund evidence required')
    evidence=rows[0]
    required=['actual_native_checkout_exchange_supplement_cash','return_facts_artificial','protected_creation_and_paid_payload_required','corrupt_and_cross_session_denied','first_dispatch_rechecks_evidence','cross_store_denied','lost_post_get_only','late_notice_fault_rolls_back_cash','saved_refund_local_reapply','unique_refund_negative_transaction_notice','original_capture_not_refunded','retired_cart_revoked_customer_independent']
    if any(evidence.get(k) is not True for k in required) or evidence.get('capture_limit')!='6.00' or evidence.get('concurrent_same_uuid')!=6 or evidence.get('refunded_total')!='6' or evidence.get('pending_difference_without_return_amount_change')!='6':
        raise RuntimeError('Supplemental refund assertions missing')
    if evidence.get('customer_browser_cli_joint') is not False or evidence.get('real_provider') is not False:
        raise RuntimeError('Unexpected customer/channel claim')
    joint_path=ROOT/'docs/migration/shopping-exchange-payment-joint-report.json'
    joint=json.loads(joint_path.read_text())
    if joint.get('cleanup_complete') is not True or joint.get('source_frozen_before_build_and_verified_after') is not True or joint.get('browser',{}).get('browser_errors')!=0 or joint.get('supplemental_payment',{}).get('single_post_each') is not True:
        raise RuntimeError('Actual original positive shopping-chain regression required')
    sources={}
    for manifest in [full.get('implementation_sha256',{}),joint.get('source_sha256_lf',{})]:
        if not manifest:
            raise RuntimeError('Source manifest required')
        for p,digest in manifest.items():
            if sha(ROOT/p)!=digest or p in sources and sources[p]!=digest:
                raise RuntimeError('Source changed after verification: '+p)
            sources[p]=digest
    migrations={p.relative_to(ROOT).as_posix() for p in (ROOT/'retail/backend/migrations').glob('*.sql')}
    if len(migrations)!=88 or migrations!={p for p in sources if p.startswith('retail/backend/migrations/') and p.endswith('.sql')}:
        raise RuntimeError('Isolated committed 88 migration snapshot required')
    for p in ['retail/backend/internal/refunds/commands.go','retail/backend/internal/refunds/supplemental.go','retail/backend/internal/identity/order_payment_refund_test.go']:
        if p not in full['implementation_sha256']:
            raise RuntimeError('New refund source absent from full SQL proof')
    script='retail/scripts/report-order-payment-refund-trace.py';sources[script]=sha(ROOT/script)
    report={'format_version':1,'scope':'SQL-only supplemental-cash refund provenance over actual native checkout/exchange/cash; returned device/request are artificial fixtures. Separate four-new-customer positive payment/shipment regression does not claim customer supplemental refund joint acceptance.','migration_count':88,'sql_suites_passed':passed,'sql_refund':evidence,'cleanup':full['cleanup'],'original_positive_customer_cli_chain_regression':True,'supplemental_payment_refund_sql_accepted':True,'supplemental_customer_refund_browser_cli_joint':False,'real_provider_or_email':False,'return_amount_change_applied':False,'source_sha256_lf':sources,'transcript_sha256_lf':{str(p.relative_to(ROOT)):sha(p) for p in [log,full_path,joint_path,ROOT/'logs/gin-migration/shopping-exchange-payment-joint-sql.log',ROOT/'logs/gin-migration/shopping-exchange-payment-joint-browser.log']}}
    (ROOT/'docs/migration/order-payment-refund-trace-report.json').write_text(json.dumps(report,indent=2)+'\n')
    print(json.dumps({'supplemental_refund_trace':'passed','source_hashes':len(sources),'customer_refund_joint':False}))


if __name__=='__main__':
    main()
