"""Verify cumulative mapping coverage and report hashes; never treat known issues as acceptance."""
import hashlib
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / 'docs/migration'


def digest(path):
    return hashlib.sha256(path.read_bytes().replace(b'\r\n', b'\n')).hexdigest()


def main():
    manifest = json.loads((OUT / 'data-source-manifest.json').read_text(encoding='utf-8-sig'))
    columns = {(t['source_table'], c['column_name']): c for t in manifest['tables'] for c in t['columns']}
    seen, tables = set(), set()
    reports = {}
    for domain in ('catalog', 'inventory-device', 'commerce', 'checkout-rules', 'operations', 'history-integration'):
        path = OUT / f'{domain}-field-mapping.json'
        mapping = json.loads(path.read_text(encoding='utf-8-sig'))
        assert mapping['source_manifest_sha256'] == digest(OUT / 'data-source-manifest.json'), 'Stale source schema hash'
        assert mapping['source_constraints_sha256'] == digest(OUT / 'source-schema-constraints.json'), 'Stale constraints hash'
        current_tables = {t['source_table'] for t in mapping['tables']}
        assert not tables & current_tables, 'Source table mapped in multiple domains'
        tables |= current_tables
        for field in mapping['fields']:
            key = (field['source_table'], field['source_column'])
            assert key in columns and key not in seen, 'Unknown or duplicate mapped field'
            assert columns[key]['udt_name'] == field['source_type'], 'Source field type changed'
            assert (columns[key]['is_nullable'] == 'YES') == field['nullable'], 'Source nullability changed'
            seen.add(key)
        assert {key for key in columns if key[0] in current_tables} <= seen, 'Mapped table has missing fields'
        report = json.loads((OUT / f'{domain}-preflight-report.json').read_text(encoding='utf-8-sig'))
        assert report['bundle_sha256'] == manifest['bundle_sha256'], 'Preflight uses different snapshot'
        assert report['mapping_sha256'] == digest(path), 'Preflight uses stale mapping'
        reports[domain] = {'failed_values': sum(report['failures'].values()),
                           'unresolved_history': sum(report.get('historical_unresolved_references', {}).values())}
    mapped_table_count, mapped_column_count = len(tables), len(seen)
    disposition = json.loads((OUT / 'nonimport-field-disposition.json').read_text(encoding='utf-8-sig'))
    assert disposition['source_manifest_sha256'] == digest(OUT / 'data-source-manifest.json'), 'Stale nonimport source hash'
    assert disposition['source_constraints_sha256'] == digest(OUT / 'source-schema-constraints.json'), 'Stale nonimport constraint hash'
    excluded_tables = {t['source_table'] for t in disposition['tables']}
    assert not tables & excluded_tables, 'Table both mapped and excluded'
    for field in disposition['fields']:
        key = (field['source_table'], field['source_column'])
        assert key in columns and key not in seen, 'Unknown or duplicate nonimport field'
        assert field['source_type'] == columns[key]['udt_name'], 'Nonimport type changed'
        assert field['nullable'] == (columns[key]['is_nullable'] == 'YES'), 'Nonimport nullability changed'
        assert field['reason'] and field['target_table'] is None and field['target_column'] is None, 'Nonimport field lacks explicit reason or has target'
        seen.add(key)
    tables |= excluded_tables
    assert {key for key in columns if key[0] in excluded_tables} <= seen, 'Nonimport table missing field decisions'
    remaining = sorted({t['source_table'] for t in manifest['tables']} - tables)
    print(json.dumps({'scope': 'Mapping evidence integrity only; not MySQL, all-field completion or business acceptance',
                      'mapped_tables': mapped_table_count, 'mapped_columns': mapped_column_count,
                      'nonimport_tables': len(excluded_tables), 'nonimport_fields': len(disposition['fields']),
                      'remaining_tables': len(remaining),
                      'remaining_columns': len(columns) - len(seen), 'preflight_issues': reports}))


if __name__ == '__main__':
    main()
