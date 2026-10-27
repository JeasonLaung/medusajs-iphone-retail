"""Run Go scalar CHECK audit against the pinned, ignored snapshot; metadata only."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[2]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--bundle', required=True)
    args = parser.parse_args()
    bundle = Path(args.bundle).resolve()
    if subprocess.run(['git', 'check-ignore', '--quiet', str(bundle)], cwd=ROOT, capture_output=True).returncode:
        raise RuntimeError('Snapshot must be Git-ignored')
    manifest = json.loads((ROOT / 'docs/migration/data-source-manifest.json').read_text(encoding='utf-8'))
    if hashlib.sha256(bundle.read_bytes()).hexdigest() != manifest['bundle_sha256']:
        raise RuntimeError('Snapshot differs from pinned evidence')
    plan_path = ROOT / 'retail/backend/internal/rowimport/field_plans.json'
    environment = os.environ.copy()
    environment['ROWIMPORT_SOURCE_CHECK_BUNDLE'] = str(bundle)
    result = subprocess.run(['go', 'test', '-count=1', '-v', '-run', '^TestPinnedSourceChecksOffline$', './internal/rowimport'],
                            cwd=ROOT / 'retail/backend', env=environment, capture_output=True, text=True, timeout=120)
    # Never echo test/compiler stderr or business payloads. The fixed marker
    # contains counts and reviewed constraint names only, even on audit failure.
    evidence = [line.split('SOURCE_CHECK_AUDIT ', 1)[1] for line in result.stdout.splitlines() if 'SOURCE_CHECK_AUDIT ' in line]
    if len(evidence) != 1:
        raise RuntimeError('Offline Go audit did not produce complete metadata')
    counts = json.loads(evidence[0])
    report = {'format_version': 1, 'scope': 'Pinned v1 scalar source CHECK audit only; not v2 acceptance, MySQL import, or business reconciliation',
              'bundle_sha256': manifest['bundle_sha256'],
              'plan_sha256': hashlib.sha256(plan_path.read_bytes().replace(b'\r\n', b'\n')).hexdigest(),
              'implementation_sha256': {path.name: hashlib.sha256(path.read_bytes().replace(b'\r\n', b'\n')).hexdigest() for path in (
                  ROOT / 'retail/backend/internal/rowimport/source_check.go',
                  ROOT / 'retail/backend/internal/rowimport/source_check_snapshot_test.go', Path(__file__))},
              'counts': counts, 'go_test_passed': result.returncode == 0,
              'source_database_connection': False, 'source_business_values_in_report': False}
    (ROOT / 'docs/migration/source-check-snapshot-report.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps(counts))
    if result.returncode:
        raise RuntimeError('Offline source CHECK audit failed; see aggregate report')


if __name__ == '__main__':
    main()
