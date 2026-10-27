"""Generate the two-table strict import plan from reviewed field conversions."""
import argparse
import json
from pathlib import Path

ROOT=Path(__file__).resolve().parents[2]


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check',action='store_true')
    args=parser.parse_args()
    fields=json.loads((ROOT/'retail/backend/internal/rowimport/field_plans.json').read_text(encoding='utf-8'))
    plan={'version':1,'tables':[]}
    for t in fields['tables']:
        if t['source'] not in ['inventory_level','reservation_item']:
            continue
        columns=[{'source':c['source'],'target':c['target'],'decimal':c['target_type'].startswith('DECIMAL(')} for c in t['fields']]
        plan['tables'].append({'source':t['source'],'target':t['target'],'primary':[next(c for c in columns if c['source']==key) for key in t['primary_key']],'columns':columns,'registry':False})
    if len(plan['tables'])!=2:
        raise ValueError('Inventory import scope differs')
    value=json.dumps(plan,indent=2)+'\n'
    path=ROOT/'retail/backend/internal/inventoryimport/plans.json'
    if args.check:
        if path.read_text(encoding='utf-8').replace('\r\n','\n')!=value:
            raise ValueError('Inventory import plan drift')
    else:
        path.write_text(value,encoding='utf-8')
    print(json.dumps({'inventory_tables':2,'columns':sum(len(t['columns']) for t in plan['tables'])}))


if __name__=='__main__':
    main()
