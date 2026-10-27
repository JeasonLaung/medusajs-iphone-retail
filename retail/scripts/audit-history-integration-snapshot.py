"""Read-only number-clock and pending-runtime diagnostics; no sensitive values emitted."""
import argparse
import hashlib
import json
import re
import subprocess
from collections import Counter
from datetime import datetime, timezone, timedelta
from decimal import Decimal
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def audit(tables):
    checks, issues, holds = Counter(), Counter(), Counter()
    for table, field in [('order_number_clock', 'last_tick'), ('sku_number_counter', 'last_id')]:
        rows = tables[table]
        checks['singleton_counter'] += 1
        if len(rows) != 1 or rows[0]['id'] is not True or type(rows[0][field]) is not int or not 0 <= rows[0][field] < 2**63:
            issues[table + ':invalid_singleton'] += 1
    if not issues:
        counter = tables['sku_number_counter'][0]['last_id']
        for variant in tables['product_variant']:
            sku = variant.get('sku') or ''
            if re.fullmatch(r'ERP[0-9]{5,18}', sku):
                checks['historical_erp_counter_floor'] += 1
                if max(int(sku[3:]) - 10000, 0) > counter:
                    issues['sku_number_counter:below_historical_allocated_sku'] += 1
        clock = tables['order_number_clock'][0]['last_tick']
        epoch = datetime(1970, 1, 1, tzinfo=timezone.utc)
        shanghai = timezone(timedelta(hours=8))
        for order in tables['order']:
            number = order.get('custom_display_id') or ''
            if not re.fullmatch(r'\d{19}', number):
                holds['order_number:legacy_format_requires_preservation'] += 1
                continue
            checks['historical_order_clock_floor'] += 1
            try:
                instant = datetime.strptime(number[:14], '%Y%m%d%H%M%S').replace(tzinfo=shanghai).astimezone(timezone.utc)
                delta = instant - epoch
                tick = (delta.days * 86400 + delta.seconds) * 100000 + int(number[14:])
                if tick > clock:
                    issues['order_number_clock:below_historical_number'] += 1
            except ValueError:
                issues['order_number:invalid_calendar'] += 1
    for row in tables['workflow_execution']:
        holds['workflow_execution:manual_side_effect_reconciliation'] += 1
    for row in tables['script_migrations']:
        checks['script_completion'] += 1
        if row.get('finished_at') is None:
            holds['script_migrations:unfinished_review'] += 1
    for row in tables['paypal_webhook_event']:
        checks['paypal_webhook_attempt_count'] += 1
        if type(row['attempt_count']) is not int or row['attempt_count'] < 0:
            issues['paypal_webhook_event:invalid_attempt_count'] += 1
        if row['status'] not in {'processed', 'ignored'}:
            holds['paypal_webhook_event:reconcile_before_retry'] += 1
    for row in tables['notification']:
        holds['notification:historical_no_replay'] += 1
    for row in tables['paypal_connection']:
        holds['paypal_connection:credential_review_no_refresh'] += 1
    return dict(sorted(checks.items())), dict(sorted(issues.items())), dict(sorted(holds.items()))


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--bundle', required=True)
    args = parser.parse_args()
    path = Path(args.bundle).resolve()
    if subprocess.run(['git', 'check-ignore', '--quiet', str(path)], cwd=ROOT, capture_output=True).returncode:
        raise ValueError('Snapshot must remain Git-ignored')
    raw = path.read_bytes()
    manifest = json.loads((ROOT / 'docs/migration/data-source-manifest.json').read_text(encoding='utf-8-sig'))
    if hashlib.sha256(raw).hexdigest() != manifest['bundle_sha256']:
        raise ValueError('Source snapshot changed')
    checks, issues, holds = audit(json.loads(raw, parse_float=Decimal)['tables'])
    report = {'format_version': 1, 'bundle_sha256': manifest['bundle_sha256'],
              'scope': 'Historical numbering floors and pending-work observations only; not concurrent allocation, decryption, identity exclusion or provider reconciliation acceptance',
              'checks': checks, 'blocking_findings': issues, 'migration_holds': holds}
    (ROOT / 'docs/migration/history-integration-semantic-report.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'checks': sum(checks.values()), 'blocking_findings': issues, 'migration_holds': holds}))
    if issues:
        raise SystemExit(1)
