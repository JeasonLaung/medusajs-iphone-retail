"""Run pinned offline Go relationship audit; report unresolved rows without repairing."""
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
    environment = os.environ.copy()
    environment['ROWIMPORT_RELATION_BUNDLE'] = str(bundle)
    result = subprocess.run(['go', 'test', '-count=1', '-v', '-run', '^TestPinnedRelationshipsOffline$', './internal/rowimport'],
                            cwd=ROOT / 'retail/backend', env=environment, capture_output=True, text=True, timeout=120)
    markers = [line.split('RELATION_AUDIT ', 1)[1] for line in result.stdout.splitlines() if 'RELATION_AUDIT ' in line]
    if result.returncode or len(markers) != 1:
        raise RuntimeError('Offline relationship audit incomplete; no source values printed')
    evidence = json.loads(markers[0])
    files = [ROOT / 'retail/backend/internal/rowimport' / name for name in ('field_plans.json', 'relationships.go', 'relationships_snapshot_test.go')]
    files.append(Path(__file__))
    report = {'format_version': 1, 'scope': 'Pinned v1 scalar identity/reference audit only; not v2 acceptance, complete data import or domain validation',
              'bundle_sha256': manifest['bundle_sha256'], 'evidence': evidence,
              'implementation_sha256': {p.name: hashlib.sha256(p.read_bytes().replace(b'\r\n', b'\n')).hexdigest() for p in files},
              'go_audit_completed': True, 'source_database_connection': False, 'source_values_in_report': False}
    (ROOT / 'docs/migration/relationship-snapshot-report.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    r = evidence['relations']
    print(json.dumps({'rows': r['rows'], 'rules': r['rules'], 'physical_foreign_keys': r['physical_foreign_keys'],
                      'nonnull_references': r['nonnull_references'], 'resolved': r['resolved_references'],
                      'missing': sum(r['missing'].values()), 'physical_missing': sum(r['physical_missing'].values()),
                      'relations_accepted': evidence['relations_accepted']}))


if __name__ == '__main__':
    main()
