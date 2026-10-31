"""Derive refund SQL evidence from the completed owned full-suite transcript."""
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
    if set(passed) != set(SUITES) or len(passed) != len(SUITES) or re.search(r'^--- FAIL:|^FAIL$', text, re.M):
        raise RuntimeError('All 23 owned SQL suites must pass before reporting refunds')
    markers = [json.loads(line.split('NATIVE_REFUND_COMMAND_REHEARSAL ',1)[1]) for line in text.splitlines() if 'NATIVE_REFUND_COMMAND_REHEARSAL ' in line]
    if len(markers) != 1 or not markers[0].get('revoked_first_dispatch_denied') or not markers[0].get('pending_review_close_denied'):
        raise RuntimeError('Current native refund evidence missing')
    report_path = ROOT/'docs/migration/admin-auth-report.json'
    existing = json.loads(report_path.read_text())
    if report_path.stat().st_mtime_ns < log.stat().st_mtime_ns:
        raise RuntimeError('Full runner cleanup/report must finish after the transcript')
    if existing.get('cleanup') != 'both owned containers and temporary runner removed':
        raise RuntimeError('Owned cleanup receipt missing')
    files = ['retail/backend/internal/refunds/commands.go','retail/backend/internal/refunds/execution.go','retail/backend/internal/refunds/commands_test.go','retail/backend/internal/identity/order_refund_commands_test.go','retail/backend/internal/identity/cart_completion_test.go','retail/backend/internal/customer/after_sales_review.go','retail/backend/internal/authorization/model.go','retail/scripts/report-native-refunds.py','retail/admin/src/i18n.ts']
    files += [p.relative_to(ROOT).as_posix() for p in sorted((ROOT/'retail/backend/migrations').glob('*.sql'))]
    report = {'format_version':1,'scope':'Native refund command and ledger over an actual completed checkout; return/device and channel observations are artificial fixtures','sql_suites_passed':passed,'migration_count':83,'native_refund_commands':markers[0], 'http_accepted':False,'background_worker_accepted':False,'browser_return_flow_accepted':False,'real_provider_refund':False,'cleanup':existing['cleanup'],'transcript_sha256':hashlib.sha256(log.read_bytes()).hexdigest(),'implementation_sha256_lf':{name:hashlib.sha256((ROOT/name).read_bytes().replace(b'\r\n',b'\n')).hexdigest() for name in files}}
    (ROOT/'docs/migration/native-refund-command-report.json').write_text(json.dumps(report,indent=2)+'\n')
    print(json.dumps({'native_refund_command_report':'written','sql_suites':len(passed),'migrations':83}))

if __name__ == '__main__':
    main()
