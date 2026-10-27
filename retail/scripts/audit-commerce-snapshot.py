"""Audit transaction relationships and capture/refund sums; emit metadata/counts only."""
import argparse
import hashlib
import json
import subprocess
from collections import Counter, defaultdict
from decimal import Decimal, localcontext
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def audit(tables, mapping):
    by_id = {name: {r['id']: r for r in rows if 'id' in r} for name, rows in tables.items()}
    active = lambda table: [r for r in tables[table] if r.get('deleted_at') is None]
    source_by_target = {t['target_table']: t['source_table'] for t in mapping['tables']}
    for related in ('catalog', 'inventory-device'):
        other = json.loads((ROOT / 'docs/migration' / f'{related}-field-mapping.json').read_text(encoding='utf-8-sig'))
        source_by_target.update({t['target_table']: t['source_table'] for t in other['tables']})
    source_by_target.update({'historical_customer_subjects': 'customer', 'carts': 'cart', 'fulfillment_shipping_options': 'shipping_option'})
    references = []
    findings, observations, checks = Counter(), Counter(), Counter()
    affected = defaultdict(set)
    for field in mapping['fields']:
        if not field['reference']:
            continue
        source_table, source_column = field['source_table'], field['source_column']
        target_table, target_column = field['reference'].split(' ')[0].split('.')
        referenced = source_by_target[target_table]
        keys = {r[target_column] for r in tables[referenced] if r.get(target_column) is not None}
        missing = [r for r in tables[source_table] if r.get(source_column) is not None and r[source_column] not in keys]
        if missing:
            references.append({'source_table': source_table, 'source_column': source_column,
                               'referenced_table': referenced, 'unresolved_values': len(missing),
                               'soft_deleted_rows': sum(r.get('deleted_at') is not None for r in missing),
                               'not_soft_deleted_rows': sum(r.get('deleted_at') is None for r in missing)})
            for index, row in enumerate(tables[source_table]):
                if row.get(source_column) is not None and row[source_column] not in keys:
                    affected[source_table].add(index)
    money = lambda value: Decimal(str(value)) if value is not None else Decimal(0)
    captured, refunded = defaultdict(Decimal), defaultdict(Decimal)
    for table, target in [('capture', captured), ('refund', refunded)]:
        for row in active(table):
            checks[f'{table}_amounts'] += 1
            amount = money(row['amount'])
            if amount < 0:
                findings[f'{table}:negative_amount'] += 1
            target[row['payment_id']] += amount
    collection_captured, collection_refunded = defaultdict(Decimal), defaultdict(Decimal)
    for row in active('payment'):
        key = row['id']
        checks['payment_capture_refund_balances'] += 1
        if captured[key] > money(row['amount']):
            findings['payment:captured_exceeds_amount'] += 1
        if refunded[key] > captured[key]:
            findings['payment:refunded_exceeds_captured'] += 1
        collection = by_id['payment_collection'].get(row['payment_collection_id'])
        session = by_id['payment_session'].get(row['payment_session_id'])
        if collection and collection['currency_code'] != row['currency_code']:
            findings['payment:collection_currency_mismatch'] += 1
        if session and (session['payment_collection_id'] != row['payment_collection_id'] or session['currency_code'] != row['currency_code']):
            findings['payment:session_context_mismatch'] += 1
        collection_captured[row['payment_collection_id']] += captured[key]
        collection_refunded[row['payment_collection_id']] += refunded[key]
    for row in active('payment_collection'):
        checks['payment_collection_captured_refunded_aggregates'] += 1
        if row.get('captured_amount') is not None and money(row['captured_amount']) != collection_captured[row['id']]:
            findings['payment_collection:captured_aggregate_mismatch'] += 1
        if row.get('refunded_amount') is not None and money(row['refunded_amount']) != collection_refunded[row['id']]:
            findings['payment_collection:refunded_aggregate_mismatch'] += 1
    for row in active('order'):
        checks['order_current_version_membership'] += 1
        items = [r for r in active('order_item') if r['order_id'] == row['id'] and r['version'] == row['version']]
        summary = [r for r in active('order_summary') if r['order_id'] == row['id'] and r['version'] == row['version']]
        if not items:
            observations['order:current_version_without_items'] += 1
        if len(summary) != 1:
            findings['order:current_version_summary_not_unique'] += 1
    seen = set()
    for row in active('order_item'):
        checks['order_item_version_keys'] += 1
        key = (row['order_id'], row['version'], row['item_id'])
        if key in seen:
            findings['order_item:duplicate_order_version_item'] += 1
        seen.add(key)
    return {'checks': dict(sorted(checks.items())), 'unresolved_references': references,
            'distinct_rows_with_unresolved_references': {k: len(v) for k, v in sorted(affected.items())},
            'financial_or_version_findings': dict(sorted(findings.items())), 'observations': dict(sorted(observations.items()))}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--bundle', required=True)
    args = parser.parse_args()
    bundle_path = Path(args.bundle).resolve()
    if subprocess.run(['git', 'check-ignore', '--quiet', str(bundle_path)], cwd=ROOT, capture_output=True).returncode:
        raise ValueError('Source snapshot must be Git-ignored')
    raw = bundle_path.read_bytes()
    manifest = json.loads((ROOT / 'docs/migration/data-source-manifest.json').read_text(encoding='utf-8-sig'))
    if hashlib.sha256(raw).hexdigest() != manifest['bundle_sha256']:
        raise ValueError('Snapshot differs from recorded source evidence')
    mapping_path = ROOT / 'docs/migration/commerce-field-mapping.json'
    mapping = json.loads(mapping_path.read_text(encoding='utf-8-sig'))
    tables = json.loads(raw, parse_float=Decimal)['tables']
    with localcontext() as context:
        context.prec = 100
        result = audit(tables, mapping)
    report = {'format_version': 1, 'bundle_sha256': manifest['bundle_sha256'],
              'mapping_sha256': hashlib.sha256(mapping_path.read_bytes().replace(b'\r\n', b'\n')).hexdigest(),
              'scope': 'Fixed snapshot transaction-reference and active capture/refund/version audit only; no provider call, no repair, no MySQL or full historical totals acceptance',
              **result, 'source_repair_performed': False,
              'disposition': 'Preserve all original rows/references in restricted migration history; unresolved/financially inconsistent aggregates remain read-only and require reconciliation; never invent transactions or drop rows to satisfy foreign keys'}
    target = ROOT / 'docs/migration/commerce-semantic-report.json'
    target.write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'checks': sum(result['checks'].values()),
                      'unresolved_reference_values': sum(r['unresolved_values'] for r in result['unresolved_references']),
                      'distinct_affected_rows': sum(result['distinct_rows_with_unresolved_references'].values()),
                      'financial_or_version_findings': result['financial_or_version_findings'], 'observations': result['observations']}))
    if result['unresolved_references'] or result['financial_or_version_findings']:
        raise SystemExit(1)


if __name__ == '__main__':
    main()
