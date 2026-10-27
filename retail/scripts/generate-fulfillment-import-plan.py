"""Generate fulfillment evidence import plans in physical dependency order."""
import argparse
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    fields = json.loads((ROOT / 'retail/backend/internal/rowimport/field_plans.json').read_text(encoding='utf-8'))
    schema = json.loads((ROOT / 'retail/backend/internal/fulfillment/schema_plan.json').read_text(encoding='utf-8'))
    all_tables = {t['source']: t for t in fields['tables']}
    selected = set(schema['source_tables'])
    deferred = set()
    pending = set(selected)
    ordered = []
    while pending:
        ready = sorted(name for name in pending if not any(
            r.get('physical_constraint') and r['parent_table'] in pending and (name, r['column']) not in deferred
            for r in all_tables[name]['references']))
        if not ready:
            raise ValueError('Unreviewed physical fulfillment dependency cycle')
        ordered.extend(ready)
        pending.difference_update(ready)
    plan = {'version': 1, 'model_sha256': schema['model_sha256'], 'tables': []}
    for name in ordered:
        t = all_tables[name]
        if any(f['transform'].startswith('protected_') for f in t['fields']):
            raise ValueError('Protected field requires dedicated history policy')
        columns = [{'source': f['source'], 'target': f['target'], 'decimal': f['target_type'].startswith('DECIMAL('),
                    'deferred': (name, f['source']) in deferred} for f in t['fields']]
        refs = []
        for r in t['references']:
            parent = all_tables[r['parent_table']]
            refs.append({'column': r['column'], 'parent': parent['target'],
                         'parent_column': next(f['target'] for f in parent['fields'] if f['source'] == r['parent_column'])})
        plan['tables'].append({'source': name, 'target': t['target'], 'columns': columns,
                               'primary': [next(c for c in columns if c['source'] == key) for key in t['primary_key']],
                               'references': refs})
    if len(plan['tables']) != 7 or sum(len(t['columns']) for t in plan['tables']) != 72:
        raise ValueError('Fulfillment import scope differs')
    registry = json.loads((ROOT / 'retail/backend/internal/uniquekey/source_plans.json').read_text(encoding='utf-8'))
    if any(p['source_table'] in selected for p in registry['plans']):
        raise ValueError('Fulfillment imports require a new full-value registry policy')
    value = json.dumps(plan, indent=2) + '\n'
    path = ROOT / 'retail/backend/internal/fulfillmentimport/plans.json'
    if args.check:
        if path.read_text(encoding='utf-8').replace('\r\n', '\n') != value:
            raise ValueError('Fulfillment import plan drift')
    else:
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(value, encoding='utf-8')
    print(json.dumps({'fulfillment_import_tables': 7, 'source_columns': 72, 'operations_enabled': False}))


if __name__ == '__main__':
    main()
