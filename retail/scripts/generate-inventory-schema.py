"""Generate versioned inventory read schema from the pinned reviewed DDL."""
import argparse
import hashlib
import importlib.util
import json
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[2]


def build():
    script = Path(__file__)
    spec = importlib.util.spec_from_file_location('inventory_review_builder', script.with_name('build-mysql-ddl.py'))
    builder = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(builder)
    ddl, review = builder.build()
    if hashlib.sha256((ROOT / 'docs/migration/mysql-relational-review.sql').read_bytes().replace(b'\r\n', b'\n')).hexdigest() != review['ddl_sha256']:
        raise ValueError('Reviewed inventory schema drift')
    fields = json.loads((ROOT / 'retail/backend/internal/rowimport/field_plans.json').read_text(encoding='utf-8'))
    source = {t['source']: t for t in fields['tables']}
    selected = [source['inventory_level'], source['reservation_item']]
    targets = {t['target'] for t in selected}
    parents = set(json.loads((ROOT / 'retail/backend/internal/catalog/schema_plan.json').read_text(encoding='utf-8'))['tables'])
    statements = []
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
            raise ValueError('Unexpected generated inventory SQL')
        if match[1] in targets:
            ref = re.search(r'REFERENCES `([a-z_]+)`', text)
            if ref and ref[1] not in targets | parents:
                raise ValueError('Inventory physical dependency missing')
            statements.append(text + ';')
    model = '\n\n'.join(statements) + '\n'
    sha = hashlib.sha256(model.encode()).hexdigest()
    sql = '-- Inventory quantities and reservation evidence; runtime writes remain disabled.\n' + model + '''
CREATE TABLE inventory_runtime_schema (
 id TINYINT PRIMARY KEY CHECK (id=1),
 schema_version INT NOT NULL CHECK (schema_version=1),
 model_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 reads_enabled BOOLEAN NOT NULL CHECK (reads_enabled IN (0,1)),
 writes_enabled BOOLEAN NOT NULL CHECK (writes_enabled=FALSE)
) ENGINE=InnoDB;
INSERT INTO inventory_runtime_schema VALUES (1,1,''' + "'" + sha + "'" + ''',TRUE,FALSE);
'''
    report = {'format_version': 1, 'schema_version': 1, 'model_sha256': sha,
              'migration': '0012_inventory_relational.sql', 'tables': sorted(targets),
              'source_columns': sum(len(t['fields']) for t in selected),
              'reads_enabled': True, 'writes_enabled': False, 'full_business_accepted': False,
              'unresolved_semantic_dependencies': ['reservation_item.line_item_id: historical soft-deleted orphan evidence and order domain resolution remain required',
                                                   'reservation_item.location_id: semantic location relation is checked by reads; reviewed physical DDL has no FK'],
              'migration_sha256_lf': hashlib.sha256(sql.encode()).hexdigest()}
    return sql, json.dumps(report, indent=2) + '\n'


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    sql, report = build()
    for name, value in {'retail/backend/migrations/0012_inventory_relational.sql': sql,
                        'retail/backend/internal/inventory/schema_plan.json': report}.items():
        path = ROOT / name
        if args.check:
            if path.read_text(encoding='utf-8').replace('\r\n', '\n') != value:
                raise ValueError('Inventory schema artifact drift')
        else:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(value, encoding='utf-8')
    print(json.dumps({'inventory_tables': 2, 'writes_enabled': False}))


if __name__ == '__main__':
    main()
