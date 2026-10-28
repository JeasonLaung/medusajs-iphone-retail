"""Generate the formal tax evidence schema; no source data or database access."""
import argparse
import hashlib
import importlib.util
import json
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[2]
SOURCES = {'tax_provider','tax_region','tax_rate','tax_rate_rule'}


def build():
    spec = importlib.util.spec_from_file_location('tax_review_builder', Path(__file__).with_name('build-mysql-ddl.py'))
    builder = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(builder)
    ddl, review = builder.build()
    checked = (ROOT / 'docs/migration/mysql-relational-review.sql').read_bytes().replace(b'\r\n', b'\n')
    if hashlib.sha256(checked).hexdigest() != review['ddl_sha256']:
        raise ValueError('Reviewed tax schema drift')
    tables = {t['source']: t for t in json.loads((ROOT / 'retail/backend/internal/rowimport/field_plans.json').read_text(encoding='utf-8'))['tables']}
    selected = [tables[name] for name in sorted(SOURCES)]
    targets = {t['target'] for t in selected}
    columns = sum(len(t['fields']) for t in selected)
    if len(targets) != 4 or columns != 36:
        raise ValueError('Tax evidence scope differs')
    parents = set()
    for name in ['catalog', 'inventory', 'orders', 'payments']:
        parents.update(json.loads((ROOT / f'retail/backend/internal/{name}/schema_plan.json').read_text(encoding='utf-8'))['tables'])
    if targets & parents:
        raise ValueError('Tax schema overlaps an existing domain')
    statements, created = [], set()
    for text in re.sub(r'^--[^\n]*\n', '', ddl, flags=re.MULTILINE).split(';'):
        text = re.sub(r'^\s*(?:--[^\n]*\n\s*)*', '', text).strip()
        if not text:
            continue
        if text.startswith('SET SESSION '):
            statements.append(text + ';')
            continue
        match = re.match(r'(?:CREATE TABLE|ALTER TABLE) `([a-z_]+)`', text)
        if not match:
            match = re.match(r'CREATE (?:UNIQUE )?INDEX `[a-z_0-9]+` ON `([a-z_]+)`', text)
        if not match:
            raise ValueError('Unexpected reviewed tax statement')
        if match[1] in targets:
            for parent in re.findall(r'REFERENCES `([a-z_]+)`', text):
                if parent not in targets | parents:
                    raise ValueError('Missing physical tax dependency')
            if text.startswith('CREATE TABLE '):
                created.add(match[1])
            statements.append(text + ';')
    if created != targets:
        raise ValueError('Incomplete tax table creation')
    # PostgreSQL enum types have no pg_constraint row. The reviewed DDL
    # supplies VARCHAR storage but its source-check pass cannot enforce them.
    # Apply the explicit allowlists from the already reviewed field plans.
    enums = []
    for table in selected:
        for field in table['fields']:
            if field['allowed']:
                allowed = field['allowed']
                if any(not re.fullmatch(r'[a-z_]+', value) for value in allowed):
                    raise ValueError('Unsupported tax enum literal')
                column = field['target']
                name = 'ck_tax_enum_' + hashlib.sha256((table['target'] + '.' + column).encode()).hexdigest()[:24]
                values = ','.join("'" + value + "'" for value in allowed)
                statements.append('ALTER TABLE `' + table['target'] + '` ADD CONSTRAINT `' + name + '` CHECK (`' + column + '` IN (' + values + '));')
                enums.append({'table': table['target'], 'column': column, 'allowed': allowed})
    model = '\n\n'.join(statements) + '\n'
    model_sha = hashlib.sha256(model.encode()).hexdigest()
    absent = ','.join("'" + name + "'" for name in sorted(targets | {'tax_runtime_schema'}))
    guard = '''-- Preserve all pre-existing tables: no drop, rename or overwrite.
CREATE TABLE tax_schema_transition_guard (
 id TINYINT PRIMARY KEY CHECK (id=1),
 preconditions_met BOOLEAN NOT NULL CHECK (preconditions_met=TRUE)
) ENGINE=InnoDB;
INSERT INTO tax_schema_transition_guard(id,preconditions_met)
SELECT 1,NOT EXISTS(SELECT 1 FROM information_schema.tables
 WHERE table_schema=DATABASE() AND table_name IN (''' + absent + '''));

'''
    marker = '''
-- Schema readiness is not identity ownership, import acceptance or checkout.
-- Tax configuration readiness does not authorize cart writes or provider execution.
CREATE TABLE tax_runtime_schema (
 id TINYINT PRIMARY KEY CHECK (id=1),
 schema_version INT NOT NULL CHECK (schema_version=1),
 model_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 reads_enabled BOOLEAN NOT NULL CHECK (reads_enabled IN (0,1)),
 writes_enabled BOOLEAN NOT NULL CHECK (writes_enabled=FALSE)
) ENGINE=InnoDB;
INSERT INTO tax_runtime_schema VALUES (1,1,''' + "'" + model_sha + "'" + ''',TRUE,FALSE);
'''
    sql = guard + model + marker
    unresolved = []
    for table in selected:
        for ref in table['references']:
            if tables[ref['parent_table']]['target'] not in targets | parents:
                unresolved.append({'source': table['source'], 'column': ref['column'], 'parent': ref['parent_table'],
                                   'policy': 'preserve-reference-no-fabrication-no-ownership-grant'})
    holds = [d for d in review['constraint_decisions'] + review['index_decisions']
             if d['source_table'] in SOURCES and d['status'] == 'hold']
    report = {'format_version': 1, 'schema_version': 1, 'model_sha256': model_sha,
              'migration': '0028_tax_relational.sql', 'tables': sorted(targets),
              'source_tables': sorted(SOURCES), 'source_columns': columns,
              'reads_enabled': True, 'writes_enabled': False,
              'full_business_accepted': False, 'historical_accounts_restored': False,
              'explicit_enum_constraints': enums,
              'unresolved_semantic_dependencies': unresolved, 'unresolved_source_indexes': holds,
              'migration_sha256_lf': hashlib.sha256(sql.encode()).hexdigest()}
    return sql, json.dumps(report, ensure_ascii=False, indent=2) + '\n'


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    sql, report = build()
    for name, value in {'retail/backend/migrations/0028_tax_relational.sql': sql,
                        'retail/backend/internal/tax/schema_plan.json': report}.items():
        path = ROOT / name
        if args.check:
            if path.read_text(encoding='utf-8').replace('\r\n', '\n') != value:
                raise ValueError('Tax schema artifact drift')
        else:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(value, encoding='utf-8')
    print(json.dumps({'tax_tables': 4, 'source_columns': 36, 'writes_enabled': False}))


if __name__ == '__main__':
    main()
