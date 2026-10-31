"""Separate real production CLI process evidence from browser/provider acceptance."""
import hashlib
import json
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[2]
EVIDENCE = ROOT/'logs/gin-migration/order-payment-cli-validation-87'
SUITES = ['PromotionUsage','PromotionSchema','GeoSchema','CartPriceRefresh','CartTaxRefresh','TaxImport','TaxSchema','AdminAuthentication','CatalogSchemaTransition','ProductEdits','ProductPrices','ProductInventory','InventoryStockEdits','ReservationTransactions','OrderSchema','OrderImport','CustomerHistory','OrderRead','PaymentSchema','PaymentImport','FulfillmentHistory','CartSchema','CartRuntime']


def sha(path):
    return hashlib.sha256(path.read_bytes().replace(b'\r\n', b'\n')).hexdigest()


def main():
    cli_path = EVIDENCE/'cli-source.json'
    cli = json.loads(cli_path.read_text())
    full_path = ROOT/'docs/migration/admin-auth-report.json'
    full = json.loads(full_path.read_text())
    full_log = ROOT/'logs/gin-migration/warehouse-auth-sql.log'
    cli_log = EVIDENCE/'cli-sql.log'
    text, cli_text = full_log.read_text(), cli_log.read_text()
    passed = re.findall(r'^--- PASS: TestOwned(\w+) \([\d.]+s\)$', text, re.M)
    if sorted(passed) != sorted(SUITES) or re.search(r'^--- FAIL:|^FAIL$',text,re.M):
        raise RuntimeError('Complete 23 SQL suites required')
    if not re.search(r'^--- PASS: TestOwnedCartRuntime \([\d.]+s\)$',cli_text,re.M) or re.search(r'^--- FAIL:|^FAIL$',cli_text,re.M):
        raise RuntimeError('Actual CLI cart SQL verification required')
    for report, path, log in [(cli,cli_path,cli_log),(full,full_path,full_log)]:
        if report.get('cleanup') != 'both owned containers and temporary runner removed' or path.stat().st_mtime_ns < log.stat().st_mtime_ns:
            raise RuntimeError('Completed owned cleanup required')
        manifest = report.get('implementation_sha256',{})
        if not manifest or any(not (ROOT/p).is_file() or sha(ROOT/p) != digest for p,digest in manifest.items()):
            raise RuntimeError('Source changed after verification')
        migrations = {p.relative_to(ROOT).as_posix() for p in (ROOT/'retail/backend/migrations').glob('*.sql')}
        tested = {p for p in manifest if p.startswith('retail/backend/migrations/') and p.endswith('.sql')}
        if migrations != tested or len(migrations) != 87:
            raise RuntimeError('Explicit tested migration snapshot differs')
    rows = [json.loads(line.split('ORDER_PAYMENT_CLI_REHEARSAL ',1)[1]) for line in cli_text.splitlines() if 'ORDER_PAYMENT_CLI_REHEARSAL ' in line]
    if len(rows) != 1 or rows[0] != cli.get('order_payment_cli'):
        raise RuntimeError('One matching production CLI marker required')
    evidence = rows[0]
    for key in ['production_binary','owned_tls_proxy','durable_claim_before_post','single_post','lost_response_process_exit','new_process_get_discovery','protected_creation_and_paid_observation','last_ack_rolls_back_cash','new_process_local_reapply_without_network','revoked_account_retired_cart_disabled_feature_recovery','canceled_order_not_reopened','unique_payment_capture_transaction_ack','idle_process']:
        if evidence.get(key) is not True:
            raise RuntimeError('Missing CLI assertion: '+key)
    if evidence.get('distinct_processes') != 4 or evidence.get('cash_total') != '164.75' or evidence.get('pending_difference') != '0' or any(evidence.get(k) is not False for k in ['real_provider','registration_browser','payment_notice']):
        raise RuntimeError('CLI amount/process/scope differs')
    binary = cli.get('order_payment_cli_binary_sha256','')
    if not re.fullmatch('[0-9a-f]{64}',binary):
        raise RuntimeError('Compiled production binary digest required')
    sources = dict(full['implementation_sha256'])
    for path,digest in cli['implementation_sha256'].items():
        if path in sources and sources[path] != digest:
            raise RuntimeError('CLI and full SQL sources differ')
        sources[path] = digest
    required = ['retail/backend/cmd/process-order-payments/main.go','retail/backend/internal/orderpayments/worker.go','retail/backend/internal/orderpayments/execution.go','retail/backend/internal/orderpayments/consumption.go','retail/backend/internal/identity/order_payment_cli_test.go','retail/backend/internal/identity/order_payment_execution_test.go','retail/scripts/test-admin-auth.py']
    if any(path not in sources for path in required):
        raise RuntimeError('Production CLI dependencies absent')
    own = 'retail/scripts/report-order-payment-cli.py'
    sources[own] = sha(ROOT/own)
    report = {'format_version':1,'scope':'Four independently launched production process-order-payments --once processes over a dedicated no-external-network MySQL/Redis native checkout/exchange fixture and owned api.mollie.com TLS CONNECT proxy. Lost POST response requires GET-only discovery in a new process; final notification ACK fault rolls back cash and a third process locally reapplies paid evidence after account/cart retirement and feature disablement. Fourth process is idle. Account ownership, catalog, receipt and provider remain artificial; no actual four-customer positive-exchange browser/CLI joint acceptance, payment notice, supplemental refund, real merchant or mail acceptance.','migration_count':87,'cli':evidence,'production_binary_sha256':binary,'sql_suites_passed':passed,'cleanup':full['cleanup'],'actual_positive_exchange_customer_cli_joint_accepted':False,'customer_payment_notice_accepted':False,'supplemental_payment_refund_accepted':False,'real_provider_called':False,'transcript_sha256':{'cli':sha(cli_log),'full_sql':sha(full_log)},'input_report_sha256':{'cli':sha(cli_path),'full_sql':sha(full_path)},'implementation_sha256_lf':sources}
    (ROOT/'docs/migration/order-payment-cli-report.json').write_text(json.dumps(report,indent=2)+'\n')
    print(json.dumps({'order_payment_cli':'passed','independent_processes':4,'sql_suites':len(passed),'sources':len(sources),'customer_browser_joint_accepted':False}))


if __name__ == '__main__':
    main()
