"""Generate the formal order evidence schema; no source data or database access."""
import argparse
import hashlib
import importlib.util
import json
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[2]
SOURCES = {
    'customer', 'order', 'order_address', 'order_cart', 'order_item',
    'order_line_item', 'order_line_item_adjustment', 'order_line_item_tax_line',
    'order_shipping', 'order_shipping_method', 'order_shipping_method_adjustment',
    'order_shipping_method_tax_line', 'order_summary', 'order_transaction',
    'order_credit_line', 'order_payment_collection', 'order_fulfillment',
    'order_promotion', 'order_change', 'order_change_action', 'order_claim',
    'order_claim_item', 'order_claim_item_image', 'order_exchange',
    'order_exchange_item', 'return', 'return_item', 'return_reason', 'return_fulfillment',
}


def build():
    spec = importlib.util.spec_from_file_location('order_review_builder', Path(__file__).with_name('build-mysql-ddl.py'))
    builder = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(builder)
    ddl, review = builder.build()
    checked = (ROOT / 'docs/migration/mysql-relational-review.sql').read_bytes().replace(b'\r\n', b'\n')
    if hashlib.sha256(checked).hexdigest() != review['ddl_sha256']:
        raise ValueError('Reviewed order schema drift')
    tables = {t['source']: t for t in json.loads((ROOT / 'retail/backend/internal/rowimport/field_plans.json').read_text(encoding='utf-8'))['tables']}
    selected = [tables[name] for name in sorted(SOURCES)]
    targets = {t['target'] for t in selected}
    columns = sum(len(t['fields']) for t in selected)
    if len(targets) != 29 or columns != 390:
        raise ValueError('Order evidence scope differs')
    parents = set()
    for name in ['catalog', 'inventory']:
        parents.update(json.loads((ROOT / f'retail/backend/internal/{name}/schema_plan.json').read_text(encoding='utf-8'))['tables'])
    if targets & parents:
        raise ValueError('Order schema overlaps an existing domain')
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
            raise ValueError('Unexpected reviewed order statement')
        if match[1] in targets:
            for parent in re.findall(r'REFERENCES `([a-z_]+)`', text):
                if parent not in targets | parents:
                    raise ValueError('Missing physical order dependency')
            if text.startswith('CREATE TABLE '):
                created.add(match[1])
            statements.append(text + ';')
    if created != targets:
        raise ValueError('Incomplete order table creation')
    # PostgreSQL enum types have no pg_constraint row. The reviewed DDL
    # supplies VARCHAR storage but its source-check pass cannot enforce them.
    # Apply the explicit allowlists from the already reviewed field plans.
    enums = []
    for table in selected:
        for field in table['fields']:
            if field['allowed']:
                allowed = field['allowed']
                if any(not re.fullmatch(r'[a-z_]+', value) for value in allowed):
                    raise ValueError('Unsupported order enum literal')
                column = field['target']
                name = 'ck_order_enum_' + hashlib.sha256((table['target'] + '.' + column).encode()).hexdigest()[:24]
                values = ','.join("'" + value + "'" for value in allowed)
                statements.append('ALTER TABLE `' + table['target'] + '` ADD CONSTRAINT `' + name + '` CHECK (`' + column + '` IN (' + values + '));')
                enums.append({'table': table['target'], 'column': column, 'allowed': allowed})
    model = '\n\n'.join(statements) + '\n'
    model_sha = hashlib.sha256(model.encode()).hexdigest()
    absent = ','.join("'" + name + "'" for name in sorted(targets | {'order_runtime_schema'}))
    guard = '''-- Preserve all pre-existing tables: no drop, rename or overwrite.
CREATE TABLE order_schema_transition_guard (
 id TINYINT PRIMARY KEY CHECK (id=1),
 preconditions_met BOOLEAN NOT NULL CHECK (preconditions_met=TRUE)
) ENGINE=InnoDB;
INSERT INTO order_schema_transition_guard(id,preconditions_met)
SELECT 1,NOT EXISTS(SELECT 1 FROM information_schema.tables
 WHERE table_schema=DATABASE() AND table_name IN (''' + absent + '''));

'''
    marker = '''
-- Schema readiness is not identity ownership, import acceptance or checkout.
-- Historical subjects have no login credentials or new-account grants.
CREATE TABLE order_runtime_schema (
 id TINYINT PRIMARY KEY CHECK (id=1),
 schema_version INT NOT NULL CHECK (schema_version=1),
 model_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 reads_enabled BOOLEAN NOT NULL CHECK (reads_enabled IN (0,1)),
 writes_enabled BOOLEAN NOT NULL CHECK (writes_enabled=FALSE)
) ENGINE=InnoDB;
INSERT INTO order_runtime_schema VALUES (1,1,''' + "'" + model_sha + "'" + ''',TRUE,FALSE);
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
              'migration': '0016_order_relational.sql', 'tables': sorted(targets),
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
    for name, value in {'retail/backend/migrations/0016_order_relational.sql': sql,
                        'retail/backend/internal/orders/schema_plan.json': report}.items():
        path = ROOT / name
        if args.check:
            if path.read_text(encoding='utf-8').replace('\r\n', '\n') != value:
                raise ValueError('Order schema artifact drift')
        else:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(value, encoding='utf-8')
    print(json.dumps({'order_tables': 29, 'source_columns': 390, 'writes_enabled': False}))


if __name__ == '__main__':
    main()
