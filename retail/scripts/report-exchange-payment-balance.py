"""Report the SQL prerequisite without claiming an online supplemental payment."""
import hashlib
import json
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[2]
SUITES = ['PromotionUsage','PromotionSchema','GeoSchema','CartPriceRefresh','CartTaxRefresh','TaxImport','TaxSchema','AdminAuthentication','CatalogSchemaTransition','ProductEdits','ProductPrices','ProductInventory','InventoryStockEdits','ReservationTransactions','OrderSchema','OrderImport','CustomerHistory','OrderRead','PaymentSchema','PaymentImport','FulfillmentHistory','CartSchema','CartRuntime']


def main():
    log = ROOT / 'logs/gin-migration/warehouse-auth-sql.log'
    transcript = log.read_text()
    passed = re.findall(r'^--- PASS: TestOwned(\w+) \([\d.]+s\)$', transcript, re.M)
    if sorted(passed) != sorted(SUITES) or re.search(r'^--- FAIL:|^FAIL$', transcript, re.M):
        raise RuntimeError('All 23 SQL suites must pass')
    cleanup_report = ROOT / 'docs/migration/admin-auth-report.json'
    cleanup = json.loads(cleanup_report.read_text())
    if cleanup_report.stat().st_mtime_ns < log.stat().st_mtime_ns or cleanup.get('cleanup') != 'both owned containers and temporary runner removed':
        raise RuntimeError('Full runner cleanup required')
    def marker(name):
        rows = [json.loads(line.split(name + ' ', 1)[1]) for line in transcript.splitlines() if name + ' ' in line]
        if len(rows) != 1:
            raise RuntimeError('Unique SQL evidence required: ' + name)
        return rows[0]
    positive = marker('NATIVE_EXCHANGE_PAYMENT_BALANCE_REHEARSAL')
    current = marker('CURRENT_ORDER_BALANCE_REHEARSAL')
    if positive.get('due') != '6.00' or positive.get('current_total') != '164.75' or positive.get('invalid_cases') != 24 or not positive.get('fixture_rolled_back') or positive.get('provider_dispatch') is not False:
        raise RuntimeError('Current positive balance evidence required')
    if not all(current.get(k) is True for k in ['repeatable_read_old_snapshot_proven','separate_connection_committed_ledger','current_locked_ledger_read','current_locked_line_amount_read','artificial_ledger_only']):
        raise RuntimeError('Current snapshot regression evidence required')
    # Derive evidence only from the same source snapshot as the completed run.
    manifest = cleanup.get('implementation_sha256', {})
    migrations = {p.relative_to(ROOT).as_posix() for p in (ROOT/'retail/backend/migrations').glob('*.sql')}
    tested_migrations = {p for p in manifest if p.startswith('retail/backend/migrations/') and p.endswith('.sql')}
    if not manifest or migrations != tested_migrations or len(migrations) != 87:
        raise RuntimeError('Completed SQL source snapshot/migration set differs; use an isolated validation checkout')
    if any(not (ROOT/p).is_file() or hashlib.sha256((ROOT/p).read_bytes().replace(b'\r\n', b'\n')).hexdigest() != digest for p, digest in manifest.items()):
        raise RuntimeError('Source changed after SQL validation; do not regenerate evidence from untested files')
    files = ['retail/backend/internal/orders/current_balance.go','retail/backend/internal/orders/supplemental_payment.go','retail/backend/internal/orders/amounts.go','retail/backend/internal/orders/amount_json.go','retail/backend/internal/orders/schema.go','retail/backend/internal/orders/schema_plan.json','retail/backend/internal/orders/device_exchange.go','retail/backend/internal/orders/exchange_payment.go','retail/backend/internal/orders/version.go','retail/backend/internal/identity/exchange_payment_balance_test.go','retail/backend/internal/identity/order_refund_commands_test.go','retail/backend/internal/identity/cart_completion_test.go','retail/scripts/test-admin-auth.py','retail/scripts/report-exchange-payment-balance.py','retail/backend/internal/payments/mollie_create.go']
    files += [p.relative_to(ROOT).as_posix() for p in (ROOT/'retail/backend/internal/totals').glob('*.go')]
    files += [p.relative_to(ROOT).as_posix() for p in (ROOT/'retail/backend/migrations').glob('*.sql')]
    report = {
        'format_version': 1,
        'scope': 'SQL-only internal supplemental balance validation and current-read regression; this report does not accept customer sessions, command lifecycle, dispatch, cash receipts, API or browser flows',
        'positive_exchange_balance': positive, 'current_read': current,
        'sql_suites_passed': passed, 'migration_count': len(list((ROOT/'retail/backend/migrations').glob('*.sql'))),
        'cleanup': cleanup['cleanup'],
        'online_supplemental_payment_accepted': False, 'real_provider_called': False,
        'transcript_sha256': hashlib.sha256(log.read_bytes()).hexdigest(),
        'full_sql_report_sha256': hashlib.sha256(cleanup_report.read_bytes()).hexdigest(),
        'implementation_sha256_lf': {name: hashlib.sha256((ROOT/name).read_bytes().replace(b'\r\n',b'\n')).hexdigest() for name in sorted(set(files))},
    }
    (ROOT/'docs/migration/exchange-payment-balance-report.json').write_text(json.dumps(report, indent=2)+'\n')
    print(json.dumps({'sql_suites': len(passed), 'supplemental_due': positive['due'], 'online_payment_accepted': False}))


if __name__ == '__main__':
    main()
