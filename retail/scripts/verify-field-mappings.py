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
    for domain in ('catalog', 'inventory-device', 'commerce', 'checkout-rules'):
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
    remaining = sorted({t['source_table'] for t in manifest['tables']} - tables)
    print(json.dumps({'scope': 'Mapping evidence integrity only; not MySQL, all-field completion or business acceptance',
                      'mapped_tables': len(tables), 'mapped_columns': len(seen), 'remaining_tables': len(remaining),
                      'remaining_columns': len(columns) - len(seen), 'preflight_issues': reports}))


if __name__ == '__main__':
    main()
