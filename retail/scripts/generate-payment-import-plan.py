"""Generate all payment history import plans; protected policies stay explicit."""
import argparse
import json
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2]

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check',action='store_true')
    args=parser.parse_args()
    schema=json.loads((ROOT/'retail/backend/internal/payments/schema_plan.json').read_text(encoding='utf-8'))
    fields=json.loads((ROOT/'retail/backend/internal/rowimport/field_plans.json').read_text(encoding='utf-8'))
    all_tables={t['source']:t for t in fields['tables']}
    selected=set(schema['source_tables'])
    pending=set(selected)
    ordered=[]
    while pending:
        ready=sorted(name for name in pending if not any(r.get('physical_constraint') and r['parent_table'] in pending for r in all_tables[name]['references']))
        if not ready: raise ValueError('Unreviewed physical payment cycle')
        ordered.extend(ready)
        pending.difference_update(ready)
    result={'version':1,'model_sha256':schema['model_sha256'],'tables':[]}
    registry=json.loads((ROOT/'retail/backend/internal/uniquekey/source_plans.json').read_text(encoding='utf-8'))
    for name in ordered:
        t=all_tables[name]
        mode='holder_history' if name=='account_holder' else 'payment_history' if name in ('payment','payment_session') else 'ordinary'
        if mode=='ordinary' and (any(f['transform'].startswith('protected_') for f in t['fields']) or any(p['source_table']==name for p in registry['plans'])):
            raise ValueError('Ordinary payment table needs a new protected/unique policy')
        columns=[{'source':f['source'],'target':f['target'],'decimal':f['target_type'].startswith('DECIMAL(')} for f in t['fields']]
        refs=[]
        for r in t['references']:
            parent=all_tables[r['parent_table']]
            refs.append({'column':r['column'],'parent':parent['target'],'parent_column':next(f['target'] for f in parent['fields'] if f['source']==r['parent_column'])})
        result['tables'].append({'source':name,'target':t['target'],'mode':mode,'columns':columns,'primary':[next(c for c in columns if c['source']==key) for key in t['primary_key']],'references':refs})
    if len(result['tables'])!=8 or sum(len(t['columns']) for t in result['tables'])!=83:
        raise ValueError('Payment import scope differs')
    value=json.dumps(result,indent=2)+'\n'
    path=ROOT/'retail/backend/internal/paymentimport/plans.json'
    if args.check:
        if path.read_text(encoding='utf-8').replace('\r\n','\n')!=value:raise ValueError('Payment import plan drift')
    else:
        path.parent.mkdir(parents=True,exist_ok=True)
        path.write_text(value,encoding='utf-8')
    print(json.dumps({'payment_import_tables':8,'source_columns':83,'protected_policy_tables':3,'provider_called':False}))
if __name__=='__main__':main()
