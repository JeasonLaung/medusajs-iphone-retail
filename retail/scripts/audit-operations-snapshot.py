"""Check custom business graph/queue boundaries in an ignored snapshot; aggregate output only."""
import argparse
import hashlib
import json
import subprocess
from collections import Counter
from decimal import Decimal
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def audit(tables):
    checks, issues, holds = Counter(), Counter(), Counter()
    requests = {r['id']: r for r in tables['buyback_request']}
    item_ids = {}
    for request in requests.values():
        checks['buyback_item_shapes'] += 1
        items = request['items']
        if not isinstance(items, list) or not items:
            issues['buyback_request:invalid_items'] += 1
            continue
        ids = [item.get('id') for item in items if isinstance(item, dict)]
        if len(ids) != len(items) or any(not isinstance(i, str) or not i for i in ids) or len(set(ids)) != len(ids):
            issues['buyback_request:invalid_or_duplicate_item_id'] += 1
            continue
        item_ids[request['id']] = set(ids)
        if request['status'] in {'paid', 'returned', 'canceled'} and not request.get('closed_at'):
            issues['buyback_request:terminal_without_closed_at'] += 1
    for table in ('buyback_photo', 'buyback_device_link'):
        for row in tables[table]:
            checks['buyback_item_membership'] += 1
            if row['request_id'] not in requests or row['item_id'] not in item_ids.get(row['request_id'], set()):
                issues[table + ':item_not_in_request'] += 1
            if table == 'buyback_photo':
                if row['review_status'] != 'expired' and not row.get('storage_key'):
                    issues['buyback_photo:unexpired_without_storage_key'] += 1
                if row.get('storage_key'):
                    holds['private_photo_files_require_manifest_and_checksum'] += 1
    for contact in tables['mail_contact']:
        checks['mail_consent'] += 1
        if contact['subscribed'] and (contact['suppressed'] or not contact.get('consent_source')):
            issues['mail_contact:subscription_conflicts_with_suppression_or_consent'] += 1
    for table in ('mail_message', 'mail_delivery'):
        for row in tables[table]:
            checks['mail_attempt_boundary'] += 1
            if row['attempts'] < 0:
                issues[table + ':negative_attempts'] += 1
            if table == 'mail_message' and row.get('created_by') == 'registration-verification':
                holds['legacy_registration_mail_never_replay'] += 1
            if row['status'] in {'queued', 'sending'}:
                holds[table + ':pending_work_reconciliation_hold'] += 1
    for campaign in tables['mail_campaign']:
        if campaign['status'] in {'queued', 'sending', 'paused'}:
            holds['mail_campaign:reconciliation_hold'] += 1
    for source in tables['seo_source']:
        checks['seo_source_offset'] += 1
        offset = Decimal(str(source['read_offset']))
        if not offset.is_finite() or offset < 0 or offset != offset.to_integral_value():
            issues['seo_source:invalid_byte_offset'] += 1
        if source['enabled']:
            holds['seo_source:host_path_and_fingerprint_review'] += 1
    for monitor in tables['seo_monitor']:
        if monitor['enabled']:
            holds['seo_monitor:external_calls_disabled_on_import'] += 1
    for run in tables['seo_run']:
        if run['status'] == 'running':
            holds['seo_run:interrupted_not_completed_or_replayed'] += 1
    for review in tables['product_review']:
        checks['product_review_rating'] += 1
        if isinstance(review['rating'], bool) or not isinstance(review['rating'], int) or not 1 <= review['rating'] <= 5:
            issues['product_review:rating_out_of_range'] += 1
        if review['verified_purchase'] and not review.get('order_id'):
            issues['product_review:verified_without_order_reference'] += 1
    return dict(sorted(checks.items())), dict(sorted(issues.items())), dict(sorted(holds.items()))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--bundle', required=True)
    args = parser.parse_args()
    path = Path(args.bundle).resolve()
    if subprocess.run(['git', 'check-ignore', '--quiet', str(path)], cwd=ROOT, capture_output=True).returncode:
        raise ValueError('Snapshot must remain Git-ignored')
    raw = path.read_bytes()
    source = json.loads((ROOT / 'docs/migration/data-source-manifest.json').read_text(encoding='utf-8-sig'))
    if hashlib.sha256(raw).hexdigest() != source['bundle_sha256']:
        raise ValueError('Snapshot differs from source evidence')
    checks, issues, holds = audit(json.loads(raw, parse_float=Decimal)['tables'])
    report = {'format_version': 1, 'bundle_sha256': source['bundle_sha256'],
              'scope': 'Custom business graph and migration hold counts only; not key decryption, file integrity, provider evidence, historical email body token removal or runtime acceptance',
              'checks': checks, 'blocking_findings': issues, 'migration_holds': holds,
              'disposition': 'Preserve business and consent history. Hold imported jobs separately without rewriting source statuses; old identity mail never replays. No file reads, external requests, emails or source writes.'}
    (ROOT / 'docs/migration/operations-semantic-report.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'checks': sum(checks.values()), 'blocking_findings': issues, 'migration_holds': holds}))
    if issues:
        raise SystemExit(1)


if __name__ == '__main__':
    main()
