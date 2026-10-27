"""Build review-only relational DDL from all six explicit field designs.

No source rows, environment, database or filesystem mutation beyond artifacts.
Untranslated identities/indexes are explicit holds, never silently dropped.
"""
import argparse
from collections import defaultdict
import hashlib
import json
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / 'docs/migration'
DOMAINS = ('catalog', 'inventory-device', 'commerce', 'checkout-rules', 'operations', 'history-integration')


def digest(path):
    return hashlib.sha256(path.read_bytes().replace(b'\r\n', b'\n')).hexdigest()


def ident(name):
    if not re.fullmatch(r'[a-zA-Z_][a-zA-Z_0-9]*', name):
        raise ValueError('Unreviewed SQL identifier')
    return '`' + name + '`'


def stable_name(prefix, table, source):
    return prefix + '_' + hashlib.sha256((table + ':' + source).encode()).hexdigest()[:24]


def sql_type(design):
    if design in ('LONGTEXT', 'LONGTEXT utf8mb4_0900_bin + CHECK JSON_VALID', 'LONGTEXT utf8mb4_0900_bin encrypted envelope'):
        return 'LONGTEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin'
    if design == 'LONGBLOB (versioned authenticated envelope)':
        return 'LONGBLOB'
    if design in ('INT', 'BIGINT', 'DOUBLE', 'BOOLEAN', 'DATETIME(6)', 'DECIMAL(38,18)'):
        return design
    match = re.fullmatch(r'(VARCHAR\((?:15|16|32|191|512)\)) utf8mb4_0900_bin(?: \+ CHECK)?', design)
    if match:
        return match[1] + ' CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin'
    raise ValueError('Unreviewed target type: ' + design)


def names(text):
    result = [part.strip().strip('"') for part in text.split(',')]
    for name in result:
        ident(name)
    return result


def index_width(design):
    match = re.match(r'VARCHAR\((\d+)\)', design)
    if match:
        return int(match[1]) * 4
    return {'INT': 4, 'BIGINT': 8, 'BOOLEAN': 1, 'DATETIME(6)': 8,
            'DOUBLE': 8, 'DECIMAL(38,18)': 18}.get(design)


def predicate(text, fields):
    """Only reviewed conjunctions of NULL/boolean tests; no SQL passthrough."""
    if not text:
        return None
    pieces = re.split(r'\s+AND\s+', text)
    output = []
    for piece in pieces:
        piece = piece.strip().strip('()').strip()
        match = re.fullmatch(r'(\w+) (IS (?:NOT )?NULL|= (?:true|false))', piece)
        if not match or match[1] not in fields:
            raise ValueError('Unreviewed index predicate')
        column, operation = match.groups()
        if operation.startswith('= '):
            assert fields[column]['target_type_design'] == 'BOOLEAN'
            operation = '= ' + ('1' if operation == '= true' else '0')
        output.append(ident(fields[column]['target_column']) + ' ' + operation)
    return ' AND '.join(output)


def index_sql(index, fields, target):
    match = re.fullmatch(r'CREATE (UNIQUE )?INDEX (?:"[^"]+"|\w+) ON public\.(?:"\w+"|\w+) USING btree \(([^()]+)\)(?: WHERE (.*))?', index['definition'])
    if not match:
        return [], 'hold', 'Expression/non-btree index requires explicit conversion'
    unique, raw_keys, where = match.groups()
    try:
        keys = names(raw_keys)
        assert all(key in fields for key in keys)
        condition = predicate(where, fields)
    except (ValueError, AssertionError):
        return [], 'hold', 'Unreviewed column/expression/predicate; no automatic equivalent claimed'
    widths = [index_width(fields[key]['target_type_design']) for key in keys]
    if any(width is None for width in widths):
        return [], 'hold', 'Full text/blob identity or search index requires explicit complete-value design; no prefix substitute'
    if sum(widths) + (1 if unique and condition else 0) > 3072:
        return [], 'hold', 'Complete key exceeds 3072 byte bound; transactional identity registry/full collision verification required'
    sql = []
    columns = [ident(fields[key]['target_column']) for key in keys]
    if unique and condition:
        flag = stable_name('active_key', target, index['index_name'])
        sql.append('ALTER TABLE ' + ident(target) + ' ADD COLUMN ' + ident(flag) + ' TINYINT GENERATED ALWAYS AS (IF(' + condition + ',1,NULL)) STORED;')
        columns.append(ident(flag))
    sql.append('CREATE ' + ('UNIQUE ' if unique else '') + 'INDEX ' + ident(stable_name('idx_source', target, index['index_name'])) + ' ON ' + ident(target) + ' (' + ','.join(columns) + ');')
    return sql, 'emitted', ('Full-value unique with nullable generated participation flag; source NULL semantics retained' if unique and condition
                            else 'Full-value unique' if unique else 'Nonunique predicate broadened to full index; same query results, performance/selectivity differs' if condition else 'Full nonunique index')


def translate_check(definition, fields):
    enum = re.fullmatch(r"CHECK \((\w+) = ANY \(ARRAY\[(.*)\]\)\)", definition)
    if enum:
        column, values = enum.groups()
        literals = re.findall(r"'((?:[^']|'')*)'::text", values)
        if not literals or ', '.join("'" + value + "'::text" for value in literals) != values:
            raise ValueError('Unreviewed enum check expression')
        return ident(fields[column]['target_column']) + ' IN (' + ','.join("'" + value + "'" for value in literals) + ')'
    expressions = {
        'CHECK (id)': 'id = 1',
        'CHECK (last_id >= 0)': 'last_id >= 0',
        'CHECK (unit_price >= 0::numeric)': 'unit_price >= 0',
        'CHECK (amount >= 0::numeric)': 'amount >= 0',
        'CHECK (parent_id IS NULL OR province_code IS NOT NULL)': 'parent_id IS NULL OR province_code IS NOT NULL',
        'CHECK (parent_id IS NULL OR provider_id IS NULL)': 'parent_id IS NULL OR provider_id IS NULL',
    }
    if definition not in expressions:
        raise ValueError('Unreviewed check expression: ' + definition)
    expression = expressions[definition]
    for column in sorted(fields, key=len, reverse=True):
        expression = re.sub(r'\b' + re.escape(column) + r'\b', ident(fields[column]['target_column']), expression)
    return expression


def build():
    hashes = {}
    def read(name):
        path = OUT / name
        hashes[name] = digest(path)
        return json.loads(path.read_text(encoding='utf-8-sig'))
    manifest = read('data-source-manifest.json')
    schema = read('source-schema-constraints.json')
    disposition = read('nonimport-field-disposition.json')
    mappings = [read(domain + '-field-mapping.json') for domain in DOMAINS]
    fields, tables = defaultdict(dict), {}
    for mapping in mappings:
        assert mapping['source_manifest_sha256'] == hashes['data-source-manifest.json']
        assert mapping['source_constraints_sha256'] == hashes['source-schema-constraints.json']
        for table in mapping['tables']:
            assert table['source_table'] not in tables
            tables[table['source_table']] = table['target_table']
        for field in mapping['fields']:
            assert field['source_column'] not in fields[field['source_table']]
            fields[field['source_table']][field['source_column']] = field
    assert len(tables) == 159 and sum(len(group) for group in fields.values()) == 1691
    assert len(set(tables.values())) == len(tables), 'Multiple source tables share target; explicit merge required'
    excluded = {table['source_table'] for table in disposition['tables']}
    assert set(tables).isdisjoint(excluded) and len(excluded) == 34
    assert set(tables) | excluded == {table['source_table'] for table in manifest['tables']}
    for table in manifest['tables']:
        if table['source_table'] in tables:
            assert set(fields[table['source_table']]) == {column['column_name'] for column in table['columns']}
    constraints = defaultdict(list)
    for item in schema['constraints']:
        constraints[item['source_table']].append(item)
    statements, deferred_fks, decisions = [], [], []
    for source in sorted(tables):
        target, members = tables[source], fields[source]
        parts = []
        for field in members.values():
            column = ident(field['target_column'])
            parts.append(column + ' ' + sql_type(field['target_type_design']) + (' NULL' if field['nullable'] else ' NOT NULL'))
            if '+ CHECK JSON_VALID' in field['target_type_design']:
                parts.append('CONSTRAINT ' + ident(stable_name('ck_json', target, field['target_column'])) + ' CHECK (' + column + ' IS NULL OR JSON_VALID(' + column + '))')
            if field['target_type_design'] == 'BOOLEAN':
                parts.append('CONSTRAINT ' + ident(stable_name('ck_bool', target, field['target_column'])) + ' CHECK (' + column + ' IN (0,1))')
        primary = [c for c in constraints[source] if c['constraint_type'] == 'p']
        assert len(primary) == 1
        for constraint in constraints[source]:
            kind, definition = constraint['constraint_type'], constraint['definition']
            decision = {'source_table': source, 'target_table': target, 'source_constraint': constraint['constraint_name'], 'kind': kind, 'source_definition': definition}
            if kind == 'p':
                match = re.fullmatch(r'PRIMARY KEY \((.*)\)', definition)
                assert match
                keys = names(match[1])
                assert all(not members[key]['nullable'] for key in keys), 'Nullable primary key mismatch'
                parts.append('PRIMARY KEY (' + ','.join(ident(members[key]['target_column']) for key in keys) + ')')
                decision['status'] = 'emitted'
            elif kind == 'c':
                expression = translate_check(definition, members)
                parts.append('CONSTRAINT ' + ident(stable_name('ck_source', target, constraint['constraint_name'])) + ' CHECK (' + expression + ')')
                decision['status'] = 'emitted'
            elif kind == 'f':
                match = re.fullmatch(r'FOREIGN KEY \(([^)]+)\) REFERENCES ("?\w+"?)\(([^)]+)\)(.*)', definition)
                assert match, 'Unreviewed foreign key'
                child_keys, parent, parent_keys, actions = match.groups()
                parent = parent.strip('"')
                assert re.fullmatch(r'(?: ON (?:UPDATE|DELETE) (?:CASCADE|SET NULL|RESTRICT|NO ACTION))*', actions), 'Unreviewed foreign key actions'
                child_keys, parent_keys = names(child_keys), names(parent_keys)
                checked_columns = set()
                for check in constraints[source]:
                    if check['constraint_type'] == 'c':
                        checked_columns |= {column for column in members if re.search(r'\b' + re.escape(column) + r'\b', check['definition'])}
                if checked_columns.intersection(child_keys) and ('CASCADE' in actions or 'SET NULL' in actions):
                    decision.update(status='hold', reason='MySQL CHECK column cannot also have cascading/SET NULL FK action (3823); preserve CHECK and require explicit parent mutation transaction/constraint redesign')
                elif parent not in tables:
                    assert parent in excluded
                    decision.update(status='hold', reason='Parent table explicitly excluded/rebuilt; historical/new-identity reference requires explicit target design')
                elif any(sql_type(members[c]['target_type_design']) != sql_type(fields[parent][p]['target_type_design']) for c, p in zip(child_keys, parent_keys)):
                    decision.update(status='hold', reason='Mapped FK types differ; cannot alter/truncate silently')
                else:
                    assert len(child_keys) == len(parent_keys)
                    if 'ON DELETE SET NULL' in actions:
                        assert all(members[c]['nullable'] for c in child_keys)
                    deferred_fks.append('ALTER TABLE ' + ident(target) + ' ADD CONSTRAINT ' + ident(stable_name('fk_source', target, constraint['constraint_name'])) + ' FOREIGN KEY (' + ','.join(ident(members[c]['target_column']) for c in child_keys) + ') REFERENCES ' + ident(tables[parent]) + ' (' + ','.join(ident(fields[parent][p]['target_column']) for p in parent_keys) + ')' + actions + ';')
                    decision['status'] = 'emitted-after-all-tables'
            elif kind == 'u':
                decision.update(status='hold', reason='Full-value unique identity/index conversion and protected-history policy must be designed explicitly')
            else:
                raise ValueError('Unknown constraint kind')
            decisions.append(decision)
        statements.append('CREATE TABLE ' + ident(target) + ' (\n  ' + ',\n  '.join(parts) + '\n) ENGINE=InnoDB ROW_FORMAT=DYNAMIC DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin;')
    index_decisions, index_statements = [], []
    for index in schema['indexes']:
        if index['source_table'] in excluded:
            status, reason = 'excluded-source-table', 'Legacy table is not imported; see nonimport field disposition'
        elif any(c['constraint_name'] == index['index_name'] and c['constraint_type'] == 'p' for c in constraints[index['source_table']]):
            status, reason = 'covered-primary-key', 'Mapped source primary key emitted'
        else:
            emitted, status, reason = index_sql(index, fields[index['source_table']], tables[index['source_table']])
            index_statements.extend(emitted)
        index_decisions.append({**index, 'status': status, 'reason': reason})
    by_index = {(entry['source_table'], entry['index_name']): entry for entry in index_decisions}
    for decision in decisions:
        if decision['kind'] == 'u' and decision['status'] == 'hold':
            index = by_index[(decision['source_table'], decision['source_constraint'])]
            if index['status'] == 'emitted':
                decision.update(status='emitted-via-unique-index', reason='Exact full-value unique index emitted')
    for constraint in schema['constraints']:
        if constraint['source_table'] in excluded:
            decisions.append({**constraint, 'status': 'excluded-source-table', 'reason': 'Legacy table not imported; no automatic new auth/grant restoration'})
    assert len(decisions) == len(schema['constraints'])
    sql = ('-- REVIEW ONLY: not a production migration. Unique indexes/defaults/identity holds remain.\n'
           '-- No IF NOT EXISTS: a pre-existing table must not hide incompatible schema.\n'
           '-- All imported fields are explicit; importer must supply required values; no source nextval/trigger/default runs.\n'
           "SET SESSION sql_mode='STRICT_ALL_TABLES,NO_ZERO_DATE,NO_ZERO_IN_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION';\n"
           "SET SESSION time_zone='+00:00';\n\n" + '\n\n'.join(statements + index_statements + deferred_fks) + '\n')
    report = {'format_version': 1, 'status': 'review-ddl-with-explicit-holds-not-production-migration',
              'tables': len(tables), 'source_columns': sum(len(group) for group in fields.values()),
              'source_sha256': hashes, 'ddl_sha256': hashlib.sha256(sql.encode()).hexdigest(),
              'constraint_decisions': decisions, 'index_decisions': index_decisions,
              'counts': {'source_constraints': len(decisions), 'emitted_constraints': sum(d['status'].startswith('emitted') for d in decisions),
                         'held_constraints': sum(d['status'] == 'hold' for d in decisions), 'source_indexes': len(index_decisions),
                         'emitted_indexes': sum(d['status'] == 'emitted' for d in index_decisions),
                         'generated_index_columns': sum(' ADD COLUMN ' in statement for statement in index_statements),
                         'held_indexes': sum(d['status'] == 'hold' for d in index_decisions)},
              'remaining': ['Held long-text/expression indexes and long identities', 'Command defaults, new auth/permissions, outbox and operation schemas',
                            'Complete source constraints equivalence, orphan reconciliation and full import', 'Migration executor/checksums/locking/failure recovery']}
    return sql, report


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    sql, report = build()
    outputs = {'mysql-relational-review.sql': sql, 'mysql-relational-review.json': json.dumps(report, ensure_ascii=False, indent=2) + '\n'}
    for name, content in outputs.items():
        path = OUT / name
        if args.check:
            assert path.read_text(encoding='utf-8').replace('\r\n', '\n') == content, 'Review DDL drift: ' + name
        else:
            path.write_text(content, encoding='utf-8')
    print(json.dumps({'tables': report['tables'], 'columns': report['source_columns'], **report['counts'], 'status': report['status']}))


if __name__ == '__main__':
    main()
