"""Generate the explicit catalog/parent schema migration; no DB or .env access."""
import argparse
import hashlib
import importlib.util
import json
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[2]
SCRIPT = Path(__file__)
DRAFT = ['catalog_products', 'catalog_variants', 'catalog_prices', 'catalog_categories',
         'catalog_category_products', 'catalog_collections', 'commerce_regions']


def digest(path):
    return hashlib.sha256(path.read_bytes().replace(b'\r\n', b'\n')).hexdigest()


def build():
    spec = importlib.util.spec_from_file_location('review_builder', SCRIPT.with_name('build-mysql-ddl.py'))
    builder = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(builder)
    ddl, review = builder.build()
    if digest(ROOT / 'docs/migration/mysql-relational-review.sql') != review['ddl_sha256']:
        raise ValueError('Review schema drift')
    fields = json.loads((ROOT / 'retail/backend/internal/rowimport/field_plans.json').read_text(encoding='utf-8'))
    program = json.loads((ROOT / 'retail/backend/internal/catalogimport/plans.json').read_text(encoding='utf-8'))
    all_tables = {table['source']: table for table in fields['tables']}
    scope = {table['source'] for table in program['tables']}
    while True:
        previous = scope.copy()
        for table in previous:
            scope.update(ref['parent_table'] for ref in all_tables[table]['references'])
        if scope == previous:
            break
    targets = {all_tables[name]['target'] for name in scope}
    if len(scope) != 45 or sum(len(all_tables[name]['fields']) for name in scope) != 374:
        raise ValueError('Reviewed catalog closure differs')
    selected = []
    # These are generated SQL statements, not arbitrary user SQL: the builder
    # has a restricted literal grammar with no embedded semicolons or routines.
    for statement in re.sub(r'^--[^\n]*\n', '', ddl, flags=re.MULTILINE).split(';'):
        statement = re.sub(r'^\s*(?:--[^\n]*\n\s*)*', '', statement).strip()
        if not statement:
            continue
        if statement.startswith('SET SESSION '):
            selected.append(statement + ';')
            continue
        match = re.match(r'(?:CREATE TABLE|ALTER TABLE) `([a-z_]+)`', statement)
        if not match:
            match = re.match(r'CREATE (?:UNIQUE )?INDEX `[a-z_0-9]+` ON `([a-z_]+)`', statement)
        if not match:
            raise ValueError('Unexpected generated SQL statement')
        if match[1] in targets:
            parent = re.search(r'REFERENCES `([a-z_]+)`', statement)
            if parent and parent[1] not in targets:
                raise ValueError('Unresolved schema dependency')
            selected.append(statement + ';')
    model = '\n\n'.join(selected) + '\n'
    model_sha = hashlib.sha256(model.encode()).hexdigest()
    backups = ['legacy_0001_' + name for name in DRAFT]
    absent = sorted((targets - set(DRAFT)) | set(backups))
    absent_sql = ','.join("'" + name + "'" for name in absent)
    checks = ['NOT EXISTS(SELECT 1 FROM `' + name + '` LIMIT 1)' for name in DRAFT]
    checks.append('NOT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name IN (' + absent_sql + '))')
    guard = '''-- Explicit empty-draft transition. Existing rows are never discarded or hidden.
-- A failed precondition stops before RENAME; the runner requires reconciliation.
CREATE TABLE catalog_schema_transition_guard (
  id TINYINT PRIMARY KEY CHECK (id=1),
  preconditions_met BOOLEAN NOT NULL CHECK (preconditions_met=TRUE)
) ENGINE=InnoDB;
INSERT INTO catalog_schema_transition_guard(id,preconditions_met)
SELECT 1,(''' + '\n AND '.join(checks) + ''');
RENAME TABLE ''' + ',\n '.join('`' + old + '` TO `' + new + '`' for old, new in zip(DRAFT, backups)) + ';\n\n'
    marker = '''
-- Read schema only. Commands must implement their own permissions, unique-key
-- registry, concurrency and audit before any later write capability is enabled.
CREATE TABLE catalog_runtime_schema (
  id TINYINT PRIMARY KEY CHECK (id=1),
  schema_version INT NOT NULL CHECK (schema_version=1),
  model_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  reads_enabled BOOLEAN NOT NULL CHECK (reads_enabled IN (0,1)),
  writes_enabled BOOLEAN NOT NULL CHECK (writes_enabled=FALSE)
) ENGINE=InnoDB;
INSERT INTO catalog_runtime_schema VALUES (1,1,''' + "'" + model_sha + "'" + ''',TRUE,FALSE);
'''
    sql = guard + model + marker
    holds = [d for d in review['constraint_decisions'] + review['index_decisions']
             if d['source_table'] in scope and d['status'] == 'hold']
    report = {'format_version': 1, 'schema_version': 1, 'model_sha256': model_sha,
              'migration': '0010_catalog_relational.sql', 'tables': sorted(targets), 'source_columns': 374,
              'empty_draft_tables_retained': dict(zip(DRAFT, backups)), 'reads_enabled': True,
              'writes_enabled': False, 'full_business_accepted': False, 'unresolved_source_indexes': holds,
              'input_sha256': {name: digest(ROOT / name) for name in [
                  'retail/scripts/generate-catalog-schema.py', 'retail/scripts/build-mysql-ddl.py',
                  'retail/backend/internal/catalogimport/plans.json', 'retail/backend/internal/rowimport/field_plans.json',
                  'docs/migration/mysql-relational-review.sql']},
              'migration_sha256_lf': hashlib.sha256(sql.encode()).hexdigest()}
    return sql, report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    sql, report = build()
    outputs = {'retail/backend/migrations/0010_catalog_relational.sql': sql,
               'retail/backend/internal/catalog/schema_plan.json': json.dumps(report, ensure_ascii=False, indent=2) + '\n'}
    for name, content in outputs.items():
        path = ROOT / name
        if args.check:
            if path.read_text(encoding='utf-8').replace('\r\n', '\n') != content:
                raise ValueError('Catalog schema artifact drift: ' + name)
        else:
            path.write_text(content, encoding='utf-8')
    print(json.dumps({'tables': 45, 'source_columns': 374, 'held_indexes': len(report['unresolved_source_indexes']), 'writes_enabled': False}))


if __name__ == '__main__':
    main()
