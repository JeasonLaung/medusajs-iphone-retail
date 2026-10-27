"""Classify cart/payment history and validate rule graphs; aggregates only, no external calls."""
import argparse
import hashlib
import json
import subprocess
from collections import Counter, defaultdict
from decimal import Decimal
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--bundle', required=True)
    args = parser.parse_args()
    target = Path(args.bundle).resolve()
    if subprocess.run(['git', 'check-ignore', '--quiet', str(target)], cwd=ROOT, capture_output=True).returncode:
        raise ValueError('Snapshot must remain Git-ignored')
    raw = target.read_bytes()
    source = json.loads((ROOT / 'docs/migration/data-source-manifest.json').read_text(encoding='utf-8-sig'))
    if hashlib.sha256(raw).hexdigest() != source['bundle_sha256']:
        raise ValueError('Snapshot differs from source evidence')
    tables = json.loads(raw, parse_float=Decimal)['tables']
    active = {name: [row for row in rows if row.get('deleted_at') is None] for name, rows in tables.items()}
    indexes = {name: {r['id']: r for r in rows if 'id' in r} for name, rows in tables.items()}
    classes, checks, issues, observations = Counter(), Counter(), Counter(), Counter()
    linked_collections, linked_orders = defaultdict(list), defaultdict(list)
    sessions = defaultdict(list)
    for link in active['cart_payment_collection']:
        linked_collections[link['cart_id']].append(link['payment_collection_id'])
    for link in active['order_cart']:
        linked_orders[link['cart_id']].append(link['order_id'])
    for session in active['payment_session']:
        sessions[session['payment_collection_id']].append(session)
    for cart in tables['cart']:
        checks['cart_classification'] += 1
        keys = linked_collections[cart['id']]
        states = {s['status'] for key in keys for s in sessions[key]}
        if cart.get('deleted_at'):
            label = 'soft-deleted-history'
        elif cart.get('completed_at'):
            label = 'completed-history'
        elif states & {'captured', 'authorized'}:
            label = 'incomplete-with-captured-or-authorized-session-needs-reconciliation'
        elif states & {'pending', 'pending_authorization', 'requires_more'}:
            label = 'incomplete-with-processable-session-needs-provider-reconciliation'
        elif keys:
            label = 'incomplete-with-payment-history'
        else:
            label = 'incomplete-without-linked-payment-collection'
        classes[label] += 1
        if len(linked_orders[cart['id']]) > 1:
            issues['cart:multiple_active_order_links'] += 1
        if cart.get('completed_at') and not cart.get('deleted_at') and len(linked_orders[cart['id']]) != 1:
            issues['cart:completed_without_unique_order_link'] += 1
    for link in active['order_cart']:
        cart = indexes['cart'].get(link['cart_id'])
        if cart and not cart.get('completed_at'):
            observations['order_cart:linked_cart_not_completed'] += 1
    for region in active['tax_region']:
        checks['tax_region_parent_rules'] += 1
        if region.get('parent_id'):
            parent = indexes['tax_region'].get(region['parent_id'])
            if not parent or parent['country_code'] != region['country_code'] or parent.get('parent_id'):
                issues['tax_region:invalid_country_parent'] += 1
            if not region.get('province_code') or region.get('provider_id') is not None:
                issues['tax_region:invalid_province_provider_or_code'] += 1
    for rule in active['shipping_option_rule']:
        checks['shipping_rule_value_shapes'] += 1
        value = rule['value']
        if rule['operator'] in {'in', 'nin'}:
            if not isinstance(value, list):
                issues['shipping_option_rule:membership_requires_array'] += 1
        elif isinstance(value, (list, dict)) or value is None:
            issues['shipping_option_rule:comparison_requires_scalar'] += 1
    for budget in active['promotion_campaign_budget']:
        checks['promotion_budget_semantics'] += 1
        if budget['type'] == 'spend_by_attribute':
            issues['promotion_budget:runtime_handling_unverified_spend_by_attribute'] += 1
        if budget['type'] in {'use_by_attribute', 'spend_by_attribute'} and not budget.get('attribute'):
            issues['promotion_budget:attribute_missing'] += 1
        if budget['type'] in {'spend', 'spend_by_attribute'} and not budget.get('currency_code'):
            issues['promotion_budget:spend_currency_missing'] += 1
    report = {'format_version': 1, 'bundle_sha256': source['bundle_sha256'],
              'scope': 'Fixed snapshot cart classification and rule graph checks only; local session status is not provider settlement evidence; no source writes or session restoration',
              'checks': dict(sorted(checks.items())), 'cart_classes': dict(sorted(classes.items())),
              'blocking_findings': dict(sorted(issues.items())), 'observations': dict(sorted(observations.items())),
              'active_promotion_rows': len(active['promotion']), 'active_tax_rate_rows': len(active['tax_rate']),
              'disposition': 'Keep completed/incomplete payment facts and historical references; do not restore old cookies/identity or replay provider effects. Isolate processable/confirmed attempts for explicit provider reconciliation before cutover.'}
    output = ROOT / 'docs/migration/checkout-rules-semantic-report.json'
    output.write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'checks': sum(checks.values()), 'cart_classes': report['cart_classes'],
                      'blocking_findings': report['blocking_findings'], 'observations': report['observations'],
                      'active_promotions': report['active_promotion_rows'], 'active_tax_rates': report['active_tax_rate_rows']}))
    if issues:
        raise SystemExit(1)


if __name__ == '__main__':
    main()
