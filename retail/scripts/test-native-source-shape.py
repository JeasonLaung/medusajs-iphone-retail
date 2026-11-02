"""Execute schema-guard regressions with artificial metadata in local PostgreSQL.

No tables or databases are created. Each case is a read-only transaction whose
information_schema input is replaced with a typed synthetic recordset.
Authentication uses ordinary libpq environment/PGPASSFILE, never CLI passwords.
"""
import argparse
import copy
import importlib.util
import json
from pathlib import Path
import subprocess

spec = importlib.util.spec_from_file_location('native_snapshot_export', Path(__file__).with_name('export-cell-snapshot.py'))
exporter = importlib.util.module_from_spec(spec)
spec.loader.exec_module(exporter)


def check(command):
    shape = exporter.shape
    original = copy.deepcopy(shape.columns()['product_option'])
    compact = copy.deepcopy(original)
    for i, column in enumerate(compact, 1):
        column['ordinal_position'] = i
    cases = [('original_dropped_slots', original, True), ('restored_compact_slots', compact, True)]
    for name in ['reordered', 'type', 'nullable', 'varchar_length', 'numeric_precision', 'datetime_precision', 'added', 'removed']:
        data = copy.deepcopy(compact)
        if name == 'reordered':
            data[0]['ordinal_position'], data[1]['ordinal_position'] = data[1]['ordinal_position'], data[0]['ordinal_position']
        elif name == 'type':
            data[0]['udt_name'] = 'unreviewed_type'
        elif name == 'nullable':
            data[0]['is_nullable'] = 'YES' if data[0]['is_nullable'] == 'NO' else 'NO'
        elif name == 'varchar_length':
            data[0]['character_maximum_length'] = 254
        elif name == 'numeric_precision':
            data[0]['numeric_precision'] = 18
        elif name == 'datetime_precision':
            next(c for c in data if c['column_name'] == 'created_at')['datetime_precision'] = 3
        elif name == 'added':
            extra = copy.deepcopy(data[0]);extra.update(column_name='unexpected', ordinal_position=len(data) + 1);data.append(extra)
        elif name == 'removed':
            data.pop()
        cases.append((name, data, False))
    results = {}
    for name, data, expected in cases:
        for column in data:
            column['table_schema'] = 'public'
            column['table_name'] = 'product_option'
        encoded = shape.literal(json.dumps(data))
        types = ','.join(key + (' text' if key in ('column_name', 'data_type', 'udt_name', 'is_nullable') else ' integer') for key in shape.KEYS)
        recordset = 'jsonb_to_recordset(' + encoded + '::jsonb) AS artificial(' + types + ',table_schema text,table_name text)'
        guard = shape.guard('product_option').replace('information_schema.columns', recordset)
        result = subprocess.run(command, input=('BEGIN READ ONLY;\n' + guard + '\nROLLBACK;\n').encode(),
                                stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=15)
        if (result.returncode == 0) != expected:
            raise RuntimeError('Native schema guard regression failed: ' + name)
        results[name] = 'passed'
    return {'artificial_metadata_only': True, 'database_writes': False, 'cases': results}


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--host', required=True)
    parser.add_argument('--port', type=int, required=True)
    parser.add_argument('--user', required=True)
    parser.add_argument('--database', required=True)
    parser.add_argument('--psql', default='psql')
    args = parser.parse_args()
    try:
        print(json.dumps(check(exporter.source_command(None, args.user, args.database, args.host, args.port, args.psql))))
    except (ValueError, RuntimeError, OSError, subprocess.TimeoutExpired):
        parser.exit(1, 'Native read-only schema regression failed; no business values logged.\n')
