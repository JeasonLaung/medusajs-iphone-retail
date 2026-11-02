"""Verify archived, actually executed cart-transfer evidence without credentials."""
import hashlib
import json
from pathlib import Path
import re
import tarfile

ROOT = Path(__file__).resolve().parents[2]
BASE = ROOT / 'logs/gin-migration'
OWNER = '1a34a50dc73c'
OWNED = '''retail/backend/migrations/0095_customer_cart_transfer.sql
retail/backend/internal/cart/customer_transfer.go
retail/backend/internal/cart/promotion_context.go
retail/backend/internal/cart/checkout_completion.go
retail/backend/internal/customer/cart_transfer.go
retail/backend/internal/customer/cart_transfer_http.go
retail/backend/internal/customer/http.go
retail/backend/cmd/api/main.go
retail/backend/internal/identity/customer_cart_transfer_test.go
retail/backend/internal/identity/cart_initial_payment_test.go
retail/backend/internal/identity/customer_http_test.go
retail/backend/internal/identity/cart_runtime_test.go
retail/backend/internal/identity/checkout_browser_test.go
retail/backend/internal/identity/checkout_accounts_browser_test.go
retail/storefront/src/lib/customer-cart-transfer.ts
retail/storefront/src/lib/cart-commerce.ts
retail/storefront/src/lib/checkout-payment.ts
retail/storefront/src/lib/customer-client.ts
retail/storefront/src/lib/customer-bridge.ts
retail/storefront/src/components/account/context.tsx
retail/storefront/tests/customer-cart-transfer.test.ts
retail/storefront/tests/customer-bridge.test.ts
retail/storefront/tests/checkout.live.mjs
retail/storefront/tests/checkout-accounts.joint.mjs'''.splitlines()


def sha(p):
    return hashlib.sha256(p.read_bytes()).hexdigest()


def main():
    snap = BASE / ('native-sql-local-' + OWNER)
    log = snap / 'logs/gin-migration'
    manifest = json.loads((snap / 'manifest.json').read_text())
    result = json.loads((log / 'joint-result.json').read_text())
    cleanup = json.loads((log / 'cleanup.json').read_text())
    archived = json.loads((snap / 'source-archive.json').read_text())
    if not all(cleanup.get(k) is True for k in ['owned_processes_terminal', 'mysql_terminal', 'private_data_removed', 'private_socket_removed', 'shared_services_unchanged']):
        raise RuntimeError('Owned fixture cleanup incomplete')
    if cleanup.get('redis_keys_deleted') != 74:
        raise RuntimeError('Unexpected owned Redis cleanup')
    if sha(snap / 'frozen-source.tar.gz') != archived['archive_sha256']:
        raise RuntimeError('Frozen archive changed')
    expected = dict(manifest['source_sha256'])
    expected.update(manifest['adapted_test_sha256'])
    expected.update(manifest['adapted_browser_sha256'])
    with tarfile.open(snap / 'frozen-source.tar.gz') as archive:
        for entry in archived['files']:
            actual = hashlib.sha256(archive.extractfile(entry['path']).read()).hexdigest()
            if actual != entry['sha256'] or actual != expected.get(entry['path'], manifest['guard_sha256'] if entry['path'].endswith('/native_sql_guard_test.go') else None):
                raise RuntimeError('Archived source changed: ' + entry['path'])
    for name in OWNED:
        if sha(ROOT / name) != manifest['source_sha256'][name]:
            raise RuntimeError('Joint/worktree source drift: ' + name)
    if archived['removed_binary']['sha256'] != result['binary_sha256']:
        raise RuntimeError('Executed binary receipt mismatch')
    if result.get('migrations') != 95 or result.get('source_frozen') is not True:
        raise RuntimeError('Frozen 95-migration execution required')
    sql = (log / 'joint-sql.log').read_text()
    if not re.search(r'^--- PASS: TestOwnedCartRuntime \([\d.]+s\)$', sql, re.M) or re.search(r'^--- FAIL:|^FAIL$', sql, re.M):
        raise RuntimeError('Actual cart runtime SQL did not pass')
    if sha(log / 'joint-sql.log') != result['sql_sha256'] or sha(log / 'joint-browser.log') != result['browser_sha256']:
        raise RuntimeError('Actual execution transcript changed')
    marker = re.search(r'CUSTOMER_CART_TRANSFER_REHEARSAL (\{[^\n]+\})', sql)
    facts = json.loads(marker.group(1)) if marker else {}
    if any(facts.get(k) is not True for k in ['real_mysql_redis', 'new_accounts_only', 'default_off', 'atomic_refresh_and_receipt', 'same_customer_preserves_email', 'foreign_customer_denied', 'empty_and_unaddressed_cart', 'server_completion_owner', 'feature_toggle_recovery', 'owner_failure_atomic']) or facts.get('concurrent_replay') != 6:
        raise RuntimeError('Transfer SQL assertions incomplete')
    browser = result['browser']
    if any(browser.get(k) is not True for k in ['anonymous_cart_login_transfer', 'lost_transfer_reply_replays_original', 'actual_gin_handlers', 'real_mysql_redis', 'registration_verification_login', 'cross_account_denied', 'logout_denies_order']) or browser['unique_orders'] != 4 or browser['browser_errors'] != 0:
        raise RuntimeError('Actual four-customer browser joint incomplete')
    if result['sql'] != dict(four_new_account_cart_bindings=True, atomic_order_owners=True, manual_attachment_audits=0, source_legacy_accounts_used=False):
        raise RuntimeError('Automatic account ownership facts changed')
    node = BASE / 'customer-cart-transfer-node-final.log'
    astro = BASE / 'customer-cart-transfer-astro-final.log'
    if 'ℹ pass 200' not in node.read_text() or 'ℹ fail 0' not in node.read_text() or '0 errors\n- 0 warnings' not in astro.read_text():
        raise RuntimeError('Final frontend checks missing')
    full_path = ROOT / 'docs/migration/native-sql-95-report.json'
    full = json.loads(full_path.read_text())
    full_snap = ROOT / full['snapshot']
    full_manifest = json.loads((full_snap / 'manifest.json').read_text())
    full_result = json.loads((full_snap / 'result.json').read_text())
    if full.get('controller_exit_code') != 0 or full.get('complete_original_sql_regression') is not True or full.get('migrations') != 95 or full.get('suite_count') != 23:
        raise RuntimeError('Actual complete 95-migration SQL run required')
    suites = full_result['suites']
    if len(suites) != 23 or [s['suite'] for s in suites] != full_manifest['suites'] or suites != full['suites'] or len(set(s['suite'] for s in suites)) != 23:
        raise RuntimeError('Complete original suite list mismatch')
    for suite in suites:
        p = full_snap / (suite['suite'] + '.log')
        content = p.read_text()
        if suite['exit_code'] != 0 or sha(p) != suite['log_sha256'] or '\nPASS\n' not in content or re.search(r'^--- (FAIL|SKIP):|^FAIL$', content, re.M):
            raise RuntimeError('Full SQL suite failed/skipped/changed: ' + suite['suite'])
    if sha(full_snap / 'identity.test') != full_result['binary_sha256'] or full_result['binary_sha256'] != full['binary_sha256']:
        raise RuntimeError('Full SQL executed binary changed')
    for name, digest in full_manifest['source_sha256'].items():
        if sha(full_snap / name) != full_manifest['adapted_test_sha256'].get(name, digest):
            raise RuntimeError('Full SQL frozen source changed: ' + name)
    for name in OWNED:
        if name.startswith('retail/backend/') and full_manifest['source_sha256'].get(name) != manifest['source_sha256'][name]:
            raise RuntimeError('Full SQL and actual browser joint source differ: ' + name)
    for filename, keys in [('cleanup.json',['owned_databases_absent','redis_prefix_cleaned','shared_server_not_restarted']), ('mysql-cleanup.json',['server_terminal','data_removed','socket_removed','main_mysql_unchanged'])]:
        actual = json.loads((full_snap / filename).read_text())
        if any(actual.get(k) is not True for k in keys):
            raise RuntimeError('Full SQL fixture cleanup incomplete')
    report = dict(format_version=1, title='匿名购物车登录转移与自动订单归属', owner=OWNER,
                  migration_count=95, main_database_migrations=86, main_feature_enabled=False,
                  actual_four_customer_joint_passed=True, full_23_sql_suites_passed=True,
                  full_sql_evidence=dict(owner=full['owner'], report_sha256=sha(full_path), suite_count=23,
                                         source_files=len(full_manifest['source_sha256']), binary_sha256=full['binary_sha256'],
                                         cleanup=full['cleanup'], mysql_cleanup=full['mysql_cleanup']),
                  frontend_tests_passed=200, astro_errors=0, astro_warnings=0, astro_hints=7,
                  source_frozen=True, production_source_changed_for_environment=False,
                  source_sha256={name:sha(ROOT/name) for name in OWNED},
                  runner_sha256=manifest['runner_sha256'], controller_sha256=manifest['controller_sha256'],
                  frozen_source_archive_sha256=archived['archive_sha256'], frozen_source_files=len(archived['files']),
                  compiled_binary_sha256=result['binary_sha256'], sql=facts, browser=browser,
                  order_ownership=result['sql'], cleanup=cleanup,
                  transcript_sha256={str(p.relative_to(ROOT)):sha(p) for p in list(log.glob('*')) if p.is_file()},
                  final_frontend_logs_sha256={str(p.relative_to(ROOT)):sha(p) for p in [node,astro]},
                  source_review='Source login transfers only current possessed cart. Same customer preserves edited checkout email; transfer refresh does not force tax calculation before address.',
                  limitations=['Default off in migration and storefront environment; no activation or upgrade of main 86-migration database.',
                               'Transfer while payment/checkout is attached or frozen remains denied; provider session invalidation and cross-device cart merge are incomplete.',
                               'No legacy identity/email ownership inheritance, real provider payment, or real mail delivery.',
                               'Actual SQL uses dedicated MySQL native foreign keys; main MySQL configuration was not changed.',
                               'C02/C06 and C01-C11 remain incomplete; shared worktree changes remain uncommitted pending coordinated commit.'],
                  goal_complete=False)
    out = ROOT / 'docs/migration/customer-cart-transfer-report.json'
    out.write_text(json.dumps(report, indent=2, ensure_ascii=False)+'\n')
    print(json.dumps(dict(joint='passed', frontend_tests=200, full_23_sql=True, source_files=len(OWNED))))


if __name__ == '__main__':
    main()
