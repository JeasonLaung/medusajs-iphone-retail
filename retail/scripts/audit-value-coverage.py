"""Inspect all mapped v2 values without SQL, secret decryption or source edits."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[2]
PINNED_SHA = 'b58a26812eeea8e75c6bd3e4ec4635b6260d8e704bc9a63fd095f0e7d9f13985'


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--snapshot', type=Path, required=True)
    args = parser.parse_args()
    snapshot = args.snapshot.resolve()
    if not snapshot.is_file() or subprocess.run(['git', 'check-ignore', '--quiet', str(snapshot)], cwd=ROOT, capture_output=True).returncode:
        raise RuntimeError('Explicit Git-ignored snapshot required')
    if hashlib.sha256(snapshot.read_bytes()).hexdigest() != PINNED_SHA:
        raise RuntimeError('Snapshot differs from fixed v2 evidence')
    environment = os.environ.copy()
    environment['ROWIMPORT_VALUE_SNAPSHOT'] = str(snapshot)
    result = subprocess.run(['go', 'test', '-count=1', '-v', '-run', '^TestPinnedV2ValueCoverage$', './internal/rowimport'],
                            cwd=ROOT / 'retail/backend', env=environment, capture_output=True, text=True, timeout=120)
    markers = [line.split('VALUE_COVERAGE_AUDIT ', 1)[1] for line in result.stdout.splitlines() if 'VALUE_COVERAGE_AUDIT ' in line]
    if result.returncode or len(markers) != 1:
        raise RuntimeError('Complete value audit failed; private output suppressed')
    evidence = json.loads(markers[0])
    tables = evidence['tables']
    counts = {
        'source_tables': len(tables), 'populated_tables': sum(t['rows'] > 0 for t in tables.values()),
        'empty_tables': sum(t['rows'] == 0 for t in tables.values()),
        'source_rows': sum(t['rows'] for t in tables.values()),
        'field_evaluations': sum(t['fields'] for t in tables.values()),
        'sql_null_fields': sum(t['sql_nulls'] for t in tables.values()),
        'ordinary_convertible_rows': sum(t['ordinary_convertible_rows'] for t in tables.values()),
        'protected_rows': sum(t['protected_rows'] for t in tables.values()),
        'history_policy_rows': sum(t['history_policy_rows'] for t in tables.values()),
        'invalid_rows': sum(t['invalid_rows'] for t in tables.values()),
        'invalid_fields': sum(sum(t['invalid_columns'].values()) for t in tables.values()),
        'number_pair_failure_rows': sum(t['number_pair_failures'] for t in tables.values()),
        'source_check_failure_rows': sum(t['source_check_failures'] for t in tables.values()),
    }
    if counts['source_tables'] != 159 or counts['source_rows'] != 27140:
        raise RuntimeError('Value audit coverage differs')
    files = ['retail/scripts/audit-value-coverage.py'] + [
        'retail/backend/internal/rowimport/' + name for name in
        ('inspect.go', 'inspect_test.go', 'value_snapshot_test.go', 'convert.go', 'stream.go', 'record.go',
         'field_plans.json', 'source_check.go', 'number_pair.go')]
    # Include every strict scalar implementation without hashing private inputs.
    files += [str(p.relative_to(ROOT)).replace('\\', '/') for p in (ROOT / 'retail/backend/internal/importvalue').glob('*.go') if not p.name.endswith('_test.go')]
    report = {
        'format_version': 1, 'scope': 'Complete pinned v2 scalar/JSON shape and raw-pair/CHECK inspection only; no domain, protected-policy, SQL or full import acceptance',
        'snapshot_sha256': PINNED_SHA, 'counts': counts, 'tables': tables,
        'full_stream_verified': True, 'source_database_connection': False, 'target_database_connection': False,
        'protected_values_decrypted': False, 'private_values_in_report': False,
        'complete_import_accepted': False, 'protected_and_history_counts_overlap': True,
        'empty_tables_are_real_sample_coverage': False,
        'implementation_sha256': {file: hashlib.sha256((ROOT / file).read_bytes().replace(b'\r\n', b'\n')).hexdigest() for file in files},
    }
    (ROOT / 'docs/migration/value-coverage-report.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps(counts))
    print(json.dumps({'tables_with_invalid_values': {name: {'columns': t['invalid_columns'], 'number_pair_failures': t['number_pair_failures'], 'source_check_failures': t['source_check_failures']}
                                                   for name, t in tables.items() if t['invalid_rows']}}))


if __name__ == '__main__':
    main()
