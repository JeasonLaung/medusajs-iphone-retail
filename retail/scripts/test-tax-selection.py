"""Verify independent Go tax selection against installed original pure methods.

No application startup, database, account, external provider or Docker service.
"""
import hashlib
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[2]


def run(args, cwd=ROOT):
    result = subprocess.run(args, cwd=cwd, capture_output=True, timeout=120)
    if result.returncode:
        raise RuntimeError(result.stdout.decode('utf-8', errors='replace') + result.stderr.decode('utf-8', errors='replace'))
    return result.stdout.decode('utf-8')


def main():
    original = json.loads(run(['node', 'retail/scripts/capture-tax-selection.cjs', '--check']).strip())
    if original != {'tax_selection_cases': 24, 'system_provider_cases': 6, 'source_files': 2, 'external_resources_used': False}:
        raise RuntimeError('Original tax coverage changed')
    tested = run(['go', 'test', '-count=1', '-json', './internal/tax'], ROOT / 'retail/backend')
    events = [json.loads(line) for line in tested.splitlines() if line.strip()]
    passed = [row['Test'] for row in events if row['Action'] == 'pass' and 'Test' in row]
    failed = [row for row in events if row['Action'] in ('fail', 'skip')]
    if failed or len([name for name in passed if '/' not in name]) != 3 or len([name for name in passed if '/' in name]) != 30:
        raise RuntimeError('Go tax evidence incomplete')
    run(['go', 'vet', './internal/tax'], ROOT / 'retail/backend')
    run(['go', 'build', './internal/tax'], ROOT / 'retail/backend')
    files = [p.relative_to(ROOT).as_posix() for p in (ROOT / 'retail/backend/internal/tax').glob('*.go')]
    files += ['retail/scripts/capture-tax-selection.cjs', 'retail/scripts/test-tax-selection.py', 'docs/migration/fixtures/legacy-tax-selection.json']
    files += [p.relative_to(ROOT).as_posix() for p in (ROOT / 'retail/backend/internal/totals').glob('*.go')]
    report = {
        'format_version': 1,
        'scope': 'Pure Go candidate filtering/selection and built-in system provider projection against actual original methods; not SQL or checkout acceptance',
        'original': original,
        'go_top_level_tests': 3,
        'go_oracle_subtests': 30,
        'go_vet_and_build': True,
        'concurrent_workers': 32,
        'iterations_per_worker': 20,
        'input_unchanged_and_output_unaliased': True,
        'invalid_duplicate_oversize_rejected': True,
        'db_or_external_provider_used': False,
        'cart_transaction_integrated': False,
        'tax_domain_accepted': False,
        'implementation_sha256': {name: hashlib.sha256((ROOT / name).read_bytes().replace(b'\r\n', b'\n')).hexdigest() for name in sorted(files)},
    }
    (ROOT / 'docs/migration/tax-selection-report.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({key: value for key, value in report.items() if key != 'implementation_sha256'}))


if __name__ == '__main__':
    main()
