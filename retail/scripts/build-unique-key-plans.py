"""Pin source long-identity scopes and typed target-row adapters; no row access."""
import argparse
import hashlib
import json
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[2]
DOC = ROOT / 'docs/migration'
OUTPUT = ROOT / 'retail/backend/internal/uniquekey/source_plans.json'


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    paths = sorted(DOC.glob('*-field-mapping.json'))
    fields = {}
    for path in paths:
        for field in json.loads(path.read_text(encoding='utf-8'))['fields']:
            key = field['source_table'], field['source_column']
            assert key not in fields
            fields[key] = field
    review_path = DOC / 'mysql-relational-review.json'
    paths.append(review_path)
    review = json.loads(review_path.read_text(encoding='utf-8'))
    plans = []
    for index in review['index_decisions']:
        if index['status'] != 'hold' or not index['definition'].startswith('CREATE UNIQUE'):
            continue
        match = re.fullmatch(r'CREATE UNIQUE INDEX (?:"\w+"|\w+) ON public\.\w+ USING btree \((\w+(?:, \w+)*)\)(?: WHERE (.*))?', index['definition'])
        assert match, 'Unreviewed long-key index grammar'
        columns, where = match.groups()
        members = []
        protected = False
        for name in ['id', *columns.split(', ')]:
            field = fields[index['source_table'], name]
            design = field['target_type_design']
            kind = 'bool' if design == 'BOOLEAN' else 'text'
            protected |= design.startswith('LONGBLOB') or 'encrypted' in design
            assert design == 'BOOLEAN' or design.startswith(('LONGTEXT', 'VARCHAR', 'LONGBLOB'))
            members.append({'column': field['target_column'], 'kind': kind, 'nullable': field['nullable']})
        conditions = []
        if where is not None:
            allowed = {
                '(deleted_at IS NULL)': [('deleted_at', 'is_null')],
                '((deleted_at IS NULL) AND (is_exclusive = false))': [('deleted_at', 'is_null'), ('is_exclusive', 'is_false')],
                '((province_code IS NULL) AND (deleted_at IS NULL))': [('province_code', 'is_null'), ('deleted_at', 'is_null')],
            }
            assert where in allowed, 'Unreviewed long-key predicate'
            for name, operation in allowed[where]:
                field = fields[index['source_table'], name]
                conditions.append({'column': field['target_column'], 'operation': operation,
                                   'kind': 'time' if field['target_type_design'] == 'DATETIME(6)' else 'bool' if field['target_type_design'] == 'BOOLEAN' else 'text',
                                   'nullable': field['nullable']})
        scope = 'source_uk_' + hashlib.sha256((index['source_table'] + ':' + index['index_name']).encode()).hexdigest()[:24]
        plans.append({'source_table': index['source_table'], 'source_index': index['index_name'],
                      'source_definition': index['definition'], 'target_table': fields[index['source_table'], 'id']['target_table'],
                      'scope': scope, 'owner': members[0], 'keys': members[1:], 'conditions': conditions,
                      'protected': protected, 'status': 'requires-secret-registry-and-domain-writer-integration' if protected else 'adapter-not-connected-to-domain-writers'})
    assert len(plans) == 28 and sum(plan['protected'] for plan in plans) == 1
    assert len({plan['scope'] for plan in plans}) == len(plans)
    report = {'version': 1, 'source_hashes': {path.name: hashlib.sha256(path.read_bytes().replace(b'\r\n', b'\n')).hexdigest() for path in paths}, 'plans': plans}
    content = json.dumps(report, ensure_ascii=False, indent=2) + '\n'
    if args.check:
        assert OUTPUT.read_text(encoding='utf-8').replace('\r\n', '\n') == content, 'Registry adapter plans drift'
    else:
        OUTPUT.write_text(content, encoding='utf-8')
    print('28 source plans; 27 ordinary adapters; 1 authenticated protected adapter; domain writers pending; no business rows read')


if __name__ == '__main__':
    main()
