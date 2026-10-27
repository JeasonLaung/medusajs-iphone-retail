"""Generate the strict source-to-target field program; never read business rows."""
import argparse
import hashlib
import json
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[2]
DOC = ROOT / 'docs/migration'
OUT = ROOT / 'retail/backend/internal/rowimport/field_plans.json'
DOMAINS = ('catalog', 'inventory-device', 'commerce', 'checkout-rules', 'operations', 'history-integration')
TRANSFORMS = set('utc_datetime6 identity_string utf8_text exact_json_text strict_boolean checked_int32 enum_text money_major_decimal historical_subject_ref code_string quantity_decimal protected_exact_json protected_string binary_text finite_float32_to_float64 device_identity_text legacy_float_rate_decimal budget_decimal checked_int64 rate_decimal promotion_value_decimal text_array_json minor_unit_integer'.split())


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    inputs = {}
    plans = []
    for domain in DOMAINS:
        path = DOC / (domain + '-field-mapping.json')
        inputs[path.name] = hashlib.sha256(path.read_bytes().replace(b'\r\n', b'\n')).hexdigest()
        mapping = json.loads(path.read_text(encoding='utf-8'))
        for table in mapping['tables']:
            fields = []
            for field in mapping['fields']:
                if field['source_table'] != table['source_table']:
                    continue
                assert field['target_table'] == table['target_table']
                assert field['transform'] in TRANSFORMS, 'Unreviewed transform'
                size = re.match(r'VARCHAR\((\d+)\)', field['target_type_design'])
                enum = []
                if field['transform'] == 'enum_text':
                    match = re.match(r'Allowed: ([a-z_ ]+);', field['assertions'])
                    assert match, 'Enum field lacks explicit allowed values'
                    enum = match[1].split()
                fields.append({'source': field['source_column'], 'target': field['target_column'],
                               'transform': field['transform'], 'nullable': field['nullable'],
                               'target_type': field['target_type_design'], 'max_chars': int(size[1]) if size else 0,
                               'allowed': enum})
            assert len(fields) == table['source_columns']
            assert len({f['target'] for f in fields}) == len(fields)
            plans.append({'source': table['source_table'], 'target': table['target_table'], 'fields': fields})
    assert len(plans) == 159 and sum(len(p['fields']) for p in plans) == 1691
    assert len({p['source'] for p in plans}) == len(plans)
    content = json.dumps({'version': 1, 'input_sha256': inputs, 'tables': plans}, ensure_ascii=False, indent=2) + '\n'
    if args.check:
        assert OUT.read_text(encoding='utf-8').replace('\r\n', '\n') == content, 'Import field program drift'
    else:
        OUT.write_text(content, encoding='utf-8')
    print('159 table programs / 1691 fields; no business row read or SQL executed')


if __name__ == '__main__':
    main()
