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


def source_check(constraint, fields):
    """Compile pinned CHECK forms into data, never pass SQL into the runtime."""
    definition = constraint['definition']
    enum = re.fullmatch(r"CHECK \(([a-z_]+) = ANY \(ARRAY\[(.*)\]\)\)", definition)
    if enum:
        column, text = enum.groups()
        values = re.findall(r"'([a-z_]+)'::text", text)
        assert values and ', '.join("'" + value + "'::text" for value in values) == text, 'Unreviewed enum CHECK'
        assert fields[column]['transform'] in {'enum_text', 'utf8_text'}, 'CHECK field type mismatch'
        if fields[column]['transform'] == 'enum_text':
            assert fields[column]['allowed'] == values, 'CHECK and enum mapping disagree'
        result = {'kind': 'enum', 'columns': [column], 'allowed': values}
    else:
        forms = {
            'CHECK (id)': ('true', ['id']),
            'CHECK (last_id >= 0)': ('nonnegative_integer', ['last_id']),
            'CHECK (unit_price >= 0::numeric)': ('nonnegative_decimal', ['unit_price']),
            'CHECK (amount >= 0::numeric)': ('nonnegative_decimal', ['amount']),
            'CHECK (parent_id IS NULL OR province_code IS NOT NULL)': ('null_or_not_null', ['parent_id', 'province_code']),
            'CHECK (parent_id IS NULL OR provider_id IS NULL)': ('null_or_null', ['parent_id', 'provider_id']),
        }
        assert definition in forms, 'Unreviewed source CHECK'
        kind, columns = forms[definition]
        expected = {'true': {'strict_boolean'}, 'nonnegative_integer': {'checked_int64', 'checked_int32'},
                    'nonnegative_decimal': {'money_major_decimal'},
                    'null_or_null': {'identity_string', 'code_string', 'historical_subject_ref'},
                    'null_or_not_null': {'identity_string', 'code_string', 'historical_subject_ref', 'utf8_text'}}
        assert all(column in fields and fields[column]['transform'] in expected[kind] for column in columns), 'CHECK field type mismatch'
        result = {'kind': kind, 'columns': columns, 'allowed': []}
    return {'source_constraint': constraint['constraint_name'], **result}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    inputs = {}
    plans = []
    manifest_path = DOC / 'data-source-manifest.json'
    manifest_sha = hashlib.sha256(manifest_path.read_bytes().replace(b'\r\n', b'\n')).hexdigest()
    constraints_path = DOC / 'source-schema-constraints.json'
    constraints_sha = hashlib.sha256(constraints_path.read_bytes().replace(b'\r\n', b'\n')).hexdigest()
    constraints = json.loads(constraints_path.read_text(encoding='utf-8'))['constraints']
    inputs[constraints_path.name] = constraints_sha
    for domain in DOMAINS:
        path = DOC / (domain + '-field-mapping.json')
        inputs[path.name] = hashlib.sha256(path.read_bytes().replace(b'\r\n', b'\n')).hexdigest()
        mapping = json.loads(path.read_text(encoding='utf-8'))
        assert mapping['source_manifest_sha256'] == manifest_sha, 'Source column manifest drift'
        assert mapping['source_constraints_sha256'] == constraints_sha, 'Source constraints drift'
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
            by_name = {f['source']: f for f in fields}
            pairs = []
            for f in fields:
                if not f['source'].startswith('raw_'):
                    continue
                scalar = f['source'][4:]
                assert f['transform'] == 'exact_json_text' and scalar in by_name
                assert by_name[scalar]['transform'] in {'money_major_decimal', 'quantity_decimal', 'rate_decimal', 'budget_decimal', 'promotion_value_decimal'}
                pairs.append({'raw': f['source'], 'scalar': scalar})
            checks = [source_check(c, by_name) for c in constraints if c['source_table'] == table['source_table'] and c['constraint_type'] == 'c']
            plans.append({'source': table['source_table'], 'target': table['target_table'], 'fields': fields, 'raw_pairs': pairs, 'checks': checks})
    assert len(plans) == 159 and sum(len(p['fields']) for p in plans) == 1691
    assert len({p['source'] for p in plans}) == len(plans)
    assert sum(len(p['raw_pairs']) for p in plans) == 59, 'Raw number pair coverage drift'
    assert sum(len(p['checks']) for p in plans) == 47, 'Source CHECK coverage drift'
    content = json.dumps({'version': 1, 'source_columns_sha256': manifest_sha, 'input_sha256': inputs, 'tables': plans}, ensure_ascii=False, indent=2) + '\n'
    if args.check:
        assert OUT.read_text(encoding='utf-8').replace('\r\n', '\n') == content, 'Import field program drift'
    else:
        OUT.write_text(content, encoding='utf-8')
    print('159 table programs / 1691 fields; no business row read or SQL executed')


if __name__ == '__main__':
    main()
