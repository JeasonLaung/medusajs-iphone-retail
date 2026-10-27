"""Derive fixed catalog importer SQL layout from reviewed field and key plans."""
import argparse
import hashlib
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    paths = ['docs/migration/catalog-field-mapping.json', 'retail/backend/internal/rowimport/field_plans.json',
             'retail/backend/internal/uniquekey/source_plans.json']
    mapping, fields, keys = [json.loads((ROOT / p).read_text(encoding='utf-8')) for p in paths]
    all_fields = {t['source']: t for t in fields['tables']}
    plans = []
    for table in mapping['tables']:
        plan = all_fields[table['source_table']]
        assert plan['target'] == table['target_table']
        assert plan['primary_key']
        assert all(not f['transform'].startswith('protected_') for f in plan['fields'])
        fields_by_name = {f['source']: f for f in plan['fields']}
        primary = []
        for name in plan['primary_key']:
            field = fields_by_name[name]
            assert not field['nullable'] and field['transform'] in ('identity_string', 'code_string')
            primary.append({'source': name, 'target': field['target']})
        registry = [k for k in keys['plans'] if k['target_table'] == plan['target']]
        assert all(not k['protected'] for k in registry)
        plans.append({'source': plan['source'], 'target': plan['target'], 'primary': primary,
                      'columns': [{'source': f['source'], 'target': f['target'], 'decimal': f['target_type'].startswith('DECIMAL(38,18)')}
                                  for f in plan['fields']], 'registry': bool(registry)})
    assert len(plans) == 36
    report = {'version': 1, 'input_sha256': {p: hashlib.sha256((ROOT / p).read_bytes().replace(b'\r\n', b'\n')).hexdigest() for p in paths}, 'tables': plans}
    output = ROOT / 'retail/backend/internal/catalogimport/plans.json'
    expected = json.dumps(report, ensure_ascii=False, indent=2) + '\n'
    if args.check:
        assert output.read_text(encoding='utf-8') == expected
    else:
        output.parent.mkdir(exist_ok=True)
        output.write_text(expected, encoding='utf-8')
    print(json.dumps({'tables': len(plans), 'registry_tables': sum(p['registry'] for p in plans), 'check': args.check}))


if __name__ == '__main__':
    main()
