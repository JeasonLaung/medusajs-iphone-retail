"""Offline known registration-mail redaction audit, never delivery or database writes."""
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
        raise RuntimeError('Unexpected source snapshot')
    environment = os.environ.copy()
    environment['ROWIMPORT_MAIL_HISTORY_BUNDLE'] = str(bundle)
    result = subprocess.run(['go', 'test', '-count=1', '-v', '-run', '^TestPinnedHistoricalMailOffline$', './internal/rowimport'],
                            cwd=ROOT / 'retail/backend', env=environment, capture_output=True, text=True, timeout=120)
    markers = [line.split('MAIL_HISTORY_AUDIT ', 1)[1] for line in result.stdout.splitlines() if 'MAIL_HISTORY_AUDIT ' in line]
    if result.returncode or len(markers) != 1:
        raise RuntimeError('Historical mail audit incomplete; private errors suppressed')
    counts = json.loads(markers[0])
    files = ['retail/backend/internal/rowimport/history_mail.go', 'retail/backend/internal/rowimport/history_mail_snapshot_test.go',
             'retail/backend/internal/rowimport/convert.go', 'medusa-store/apps/backend/src/lib/registration-email.ts',
             'medusa-store/apps/backend/src/workflows/registration-email.ts', 'retail/scripts/audit-history-mail.py']
    report = {'format_version': 1, 'scope': 'Pinned known registration mail removal and mandatory effect disposition only; not general secret scanning, SQL import or worker suppression enforcement',
              'bundle_sha256': manifest['bundle_sha256'], 'counts': counts,
              'implementation_sha256': {f: hashlib.sha256((ROOT / f).read_bytes().replace(b'\r\n', b'\n')).hexdigest() for f in files},
              'source_database_connection': False, 'mail_delivery_attempted': False, 'redacted_rows_persisted': False, 'private_values_in_report': False}
    (ROOT / 'docs/migration/history-mail-redaction-report.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps(counts))


if __name__ == '__main__':
    main()
