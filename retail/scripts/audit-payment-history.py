"""Verify the actual read-only v2 stream and complete historical payment rows."""
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
    parser.add_argument('--snapshot', required=True)
    args = parser.parse_args()
    snapshot = Path(args.snapshot).resolve()
    if subprocess.run(['git', 'check-ignore', '--quiet', str(snapshot)], cwd=ROOT, capture_output=True).returncode:
        raise RuntimeError('Private v2 stream must be Git-ignored')
    if hashlib.sha256(snapshot.read_bytes()).hexdigest() != PINNED_SHA:
        raise RuntimeError('Pinned v2 snapshot differs')
    environment = os.environ.copy()
    environment['ROWIMPORT_PAYMENT_HISTORY_V2'] = str(snapshot)
    result = subprocess.run(['go', 'test', '-count=1', '-v', '-run', '^TestPinnedPaymentHistoryV2Offline$', './internal/rowimport'],
                            cwd=ROOT / 'retail/backend', env=environment, capture_output=True, text=True, timeout=120)
    markers = [line.split('PAYMENT_HISTORY_V2_AUDIT ', 1)[1] for line in result.stdout.splitlines() if 'PAYMENT_HISTORY_V2_AUDIT ' in line]
    if result.returncode or len(markers) != 1:
        raise RuntimeError('V2 payment history audit failed; private errors suppressed')
    evidence = json.loads(markers[0])
    files = ['retail/backend/internal/rowimport/history_payment.go', 'retail/backend/internal/rowimport/history_payment_test.go',
             'retail/backend/internal/rowimport/history_payment_snapshot_test.go', 'retail/backend/internal/rowimport/convert.go',
             'retail/scripts/audit-payment-history.py', 'retail/scripts/export-cell-snapshot.py', 'retail/scripts/source-shape-sql.py',
             'retail/scripts/cell-record-sql.py', 'retail/backend/internal/rowimport/field_plans.json']
    report = {'format_version': 1, 'snapshot_format_version': 2, 'snapshot_sha256': PINNED_SHA,
              'source_export': {'read_only': True, 'isolation': 'repeatable read', 'column_shape_guard_tables': 159,
                                'constraint_and_index_drift_guard': False, 'original_v1_snapshot_overwritten': False},
              'scope': 'Complete source stream framing/hash and relationships; all 42 payment_session/payment rows converted in memory, no SQL import or runtime worker',
              **evidence,
              'implementation_sha256': {f: hashlib.sha256((ROOT / f).read_bytes().replace(b'\r\n', b'\n')).hexdigest() for f in files},
              'provider_called': False, 'target_rows_written': False, 'target_keys_or_envelopes_persisted': False,
              'private_values_in_report': False, 'relational_acceptance': False}
    (ROOT / 'docs/migration/payment-history-v2-report.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps(evidence['counts']))


if __name__ == '__main__':
    main()
