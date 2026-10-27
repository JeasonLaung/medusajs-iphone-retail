"""Compare standalone Go totals against pinned, installed legacy arithmetic."""
import hashlib
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[2]


def main():
    checked = []
    for filename, count, sources in [('capture-totals.cjs', 18, 9), ('capture-totals-boundaries.cjs', 29, 10)]:
        result = subprocess.run(['node', str(ROOT / 'retail/scripts' / filename), '--check'],
                                cwd=ROOT, capture_output=True, timeout=60)
        if result.returncode:
            raise RuntimeError('Legacy arithmetic source or fixture verification failed')
        report = json.loads(result.stdout)
        if report['cases'] != count or report['source_hashes'] != sources or report['mode'] != 'check':
            raise RuntimeError('Arithmetic oracle coverage differs')
        checked.append(report)
    tested = subprocess.run(['go', 'test', './internal/totals', '-count=1', '-json'],
                            cwd=ROOT / 'retail/backend', capture_output=True, timeout=90)
    if tested.returncode:
        raise RuntimeError('Go arithmetic verification failed')
    events = [json.loads(line) for line in tested.stdout.splitlines()]
    passed = {e.get('Test') for e in events if e.get('Action') == 'pass' and '/' not in e.get('Test', '/')}
    if passed != {'TestLegacyParity', 'TestInvalidInputsRejectWholeResult',
                  'TestDecimalRoundingAndExactOperations', 'TestParallelCalculationAndIndependentResults'}:
        raise RuntimeError('Arithmetic tests missing')
    markers = {}
    for event in events:
        output = event.get('Output', '')
        for name in ['TOTALS_PARITY', 'TOTALS_REJECTIONS']:
            if output.startswith(name + ' '):
                if name in markers:
                    raise RuntimeError('Repeated arithmetic evidence marker')
                markers[name] = json.loads(output[len(name) + 1:])
    if set(markers) != {'TOTALS_PARITY', 'TOTALS_REJECTIONS'}:
        raise RuntimeError('Arithmetic evidence missing')
    parity, rejected = markers['TOTALS_PARITY'], markers['TOTALS_REJECTIONS']
    if parity['cases'] != 47 or parity['exact_decimal_fields'] != 1644 or rejected['invalid_cases'] != 29:
        raise RuntimeError('Arithmetic case coverage differs')
    vetted = subprocess.run(['go', 'vet', './internal/totals'], cwd=ROOT / 'retail/backend',
                            capture_output=True, timeout=90)
    if vetted.returncode:
        raise RuntimeError('Go arithmetic vet failed')
    names = ['retail/scripts/test-totals.py', 'retail/scripts/capture-totals.cjs',
             'retail/scripts/capture-totals-boundaries.cjs', 'docs/migration/fixtures/legacy-totals.json',
             'docs/migration/fixtures/legacy-totals-boundaries.json']
    names += [str(p.relative_to(ROOT)).replace('\\', '/')
              for p in (ROOT / 'retail/backend/internal/totals').glob('*.go')]
    report = {'format_version': 1,
              'scope': 'Standalone Go arithmetic parity against installed legacy utilities; artificial inputs only',
              'counts': {'original_cases': 18, 'additional_boundary_cases': 29,
                         'exact_decimal_fields': 1644, 'invalid_cases_rejected_atomically': 29,
                         'parallel_workers': 8, 'calculations_per_worker': 25},
              'division_decimal_places': 20, 'division_rounding': 'half_up_away_from_zero',
              'addition_multiplication_exact': True, 'input_unchanged': True,
              'partial_results_on_error': False, 'vet_passed': True,
              'runtime_medusa_dependency': False, 'floating_point_used': False,
              'database_or_server_used': False, 'historical_business_rows_tested': False,
              'http_or_ui_integration_accepted': False, 'checkout_accepted': False,
              'provider_settlement_rounding_accepted': False,
              'implementation_sha256': {name: hashlib.sha256((ROOT / name).read_bytes().replace(b'\r\n', b'\n')).hexdigest()
                                        for name in sorted(names)}}
    (ROOT / 'docs/migration/go-totals-report.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'parity': parity, 'rejections': rejected, 'vet': 'passed', 'runtime_medusa_dependency': False}))


if __name__ == '__main__':
    main()
