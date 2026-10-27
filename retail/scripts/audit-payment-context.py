"""Offline reviewed payment context projection; no provider calls or database writes."""
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
        raise RuntimeError('Source snapshot differs from evidence')
    environment = os.environ.copy()
    environment['ROWIMPORT_PAYMENT_CONTEXT_BUNDLE'] = str(bundle)
    result = subprocess.run(['go', 'test', '-count=1', '-v', '-run', '^TestPinnedPaymentContextOffline$', './internal/rowimport'],
                            cwd=ROOT / 'retail/backend', env=environment, capture_output=True, text=True, timeout=120)
    markers = [line.split('PAYMENT_CONTEXT_AUDIT ', 1)[1] for line in result.stdout.splitlines() if 'PAYMENT_CONTEXT_AUDIT ' in line]
    if result.returncode or len(markers) != 1:
        raise RuntimeError('Payment context review incomplete; private errors suppressed')
    counts = json.loads(markers[0])
    files = ['retail/backend/internal/rowimport/payment_context.go', 'retail/backend/internal/rowimport/payment_context_envelope.go',
             'retail/backend/internal/rowimport/payment_context_snapshot_test.go', 'retail/scripts/audit-payment-context.py']
    report = {'format_version': 1, 'scope': 'Pinned payment_session.context projection and ephemeral sealing only; not other payment JSON, source replay, SQL import, provider reconciliation or new-account ownership',
              'bundle_sha256': manifest['bundle_sha256'], 'counts': counts,
              'implementation_sha256': {f: hashlib.sha256((ROOT / f).read_bytes().replace(b'\r\n', b'\n')).hexdigest() for f in files},
              'source_database_connection': False, 'provider_called': False, 'target_keys_or_envelopes_persisted': False, 'private_values_in_report': False}
    (ROOT / 'docs/migration/payment-context-review-report.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps(counts))


if __name__ == '__main__':
    main()
