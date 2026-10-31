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
    if len(markers) != 1 or not markers[0].get('revoked_first_dispatch_denied') or not markers[0].get('pending_review_close_denied') or not markers[0].get('explicit_failed_canceled_successors') or not markers[0].get('http_real_auth_csrf') or markers[0].get('worker_concurrency') != 6:
        raise RuntimeError('Current native refund evidence missing')
    # The original 0083 marker names its concurrent-active-duplicate assertion
    # "single_instruction_per_return". It never asserts one historical row after
    # adding explicit successors. Preserve the transcript hash and name the
    # derived assertion precisely in this lifecycle report.
    commands = dict(markers[0])
    commands['single_active_instruction_per_return'] = commands.pop('single_instruction_per_return')
    report_path = ROOT/'docs/migration/admin-auth-report.json'
    existing = json.loads(report_path.read_text())
    if report_path.stat().st_mtime_ns < log.stat().st_mtime_ns:
        raise RuntimeError('Full runner cleanup/report must finish after the transcript')
    if existing.get('cleanup') != 'both owned containers and temporary runner removed':
        raise RuntimeError('Owned cleanup receipt missing')
    # Derive evidence only from the same source snapshot as the completed run.
    manifest = existing.get('implementation_sha256', {})
    migrations = {p.relative_to(ROOT).as_posix() for p in (ROOT/'retail/backend/migrations').glob('*.sql')}
    tested_migrations = {p for p in manifest if p.startswith('retail/backend/migrations/') and p.endswith('.sql')}
    if not manifest or migrations != tested_migrations or len(migrations) != 87:
        raise RuntimeError('Completed SQL source snapshot/migration set differs; use an isolated validation checkout')
    if any(not (ROOT/p).is_file() or hashlib.sha256((ROOT/p).read_bytes().replace(b'\r\n', b'\n')).hexdigest() != digest for p, digest in manifest.items()):
        raise RuntimeError('Source changed after SQL validation; do not regenerate evidence from untested files')
    files = ['retail/backend/internal/orders/change_summary.go','retail/backend/internal/orders/supplemental_payment.go','retail/backend/internal/orders/schema.go','retail/backend/internal/orders/current_balance.go','retail/backend/internal/identity/exchange_refund_commands_test.go','retail/backend/internal/customernotices/events.go','retail/backend/cmd/process-order-refunds/main.go','retail/backend/cmd/api/main.go','retail/backend/internal/server/router.go','retail/backend/internal/checkoutconfig/checkout.go','retail/backend/internal/checkoutconfig/checkout_test.go','retail/backend/.env.example','retail/admin/src/api.ts','retail/backend/internal/identity/order_refund_http_test.go','retail/backend/internal/refunds/commands.go','retail/backend/internal/refunds/execution.go','retail/backend/internal/refunds/commands_test.go','retail/backend/internal/identity/order_refund_commands_test.go','retail/backend/internal/identity/cart_completion_test.go','retail/backend/internal/customer/after_sales_review.go','retail/backend/internal/authorization/model.go','retail/scripts/report-native-refunds.py','retail/admin/src/i18n.ts']
    files += [p.relative_to(ROOT).as_posix() for p in sorted((ROOT/'retail/backend/migrations').glob('*.sql'))]
    files += [p.relative_to(ROOT).as_posix() for p in sorted((ROOT/'retail/backend/internal/refunds').glob('*.go'))]
    files = sorted(set(files))
    runtime = [json.loads(line.split('CART_RUNTIME_REHEARSAL ',1)[1]) for line in text.splitlines() if 'CART_RUNTIME_REHEARSAL ' in line]
    if len(runtime)!=1 or runtime[0].get('migrations')!=87:
        raise RuntimeError('Current migration 87 evidence required')
    exchange = [json.loads(line.split('NATIVE_EXCHANGE_REFUND_CREDIT_REHEARSAL ',1)[1]) for line in text.splitlines() if 'NATIVE_EXCHANGE_REFUND_CREDIT_REHEARSAL ' in line]
    if len(exchange)!=1 or exchange[0].get('credit')!='12.00' or not exchange[0].get('stale_first_dispatch_denied'):
        raise RuntimeError('Native exchange credit evidence required')
    report = {'exchange_refund_credit':exchange[0],'format_version':1,'scope':'SQL-only native refund lifecycle, authenticated Gin HTTP and durable RunDue worker over an actual completed checkout; return/device facts and channel observations are artificial fixtures; this SQL rehearsal does not exercise browser return or a separately launched worker; shopping-refund-joint-report.json records that separate actual acceptance','sql_suites_passed':passed,'migration_count':87,'native_refund_commands':commands,'test_marker_field_renamed':{'single_instruction_per_return':'single_active_instruction_per_return'}, 'http_accepted':True,'worker_run_due_accepted':True,'worker_cli_process_accepted':False,'browser_return_flow_accepted':False,'real_provider_refund':False,'cleanup':existing['cleanup'],'transcript_sha256':hashlib.sha256(log.read_bytes()).hexdigest(),'implementation_sha256_lf':{name:hashlib.sha256((ROOT/name).read_bytes().replace(b'\r\n',b'\n')).hexdigest() for name in files}}
    (ROOT/'docs/migration/native-refund-lifecycle-report.json').write_text(json.dumps(report,indent=2)+'\n')
    print(json.dumps({'native_refund_lifecycle_report':'written','sql_suites':len(passed),'migrations':87}))

if __name__ == '__main__':
    main()
