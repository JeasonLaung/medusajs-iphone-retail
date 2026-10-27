"""Pinned PayPal JSON baseline through Go complete-row sealing, no provider/SQL."""
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
    if subprocess.run(['git', 'check-ignore', '--quiet', str(snapshot)], cwd=ROOT, capture_output=True).returncode:
        raise RuntimeError('Private snapshot must be Git-ignored')
    if hashlib.sha256(snapshot.read_bytes()).hexdigest() != PINNED_SHA:
        raise RuntimeError('Pinned snapshot differs')
    environment = os.environ.copy()
    environment['ROWIMPORT_PAYPAL_CONFIG_V2'] = str(snapshot)
    result = subprocess.run(['go', 'test', '-count=1', '-v', '-run', '^TestPinnedPayPalConfigV2Offline$', './internal/rowimport'],
                            cwd=ROOT / 'retail/backend', env=environment, capture_output=True, text=True, timeout=120)
    markers = [line.split('PAYPAL_CONFIG_V2_AUDIT ', 1)[1] for line in result.stdout.splitlines() if 'PAYPAL_CONFIG_V2_AUDIT ' in line]
    if result.returncode or len(markers) != 1:
        raise RuntimeError('PayPal configuration conversion failed; private output suppressed')
    files = ['retail/backend/internal/rowimport/config_secret.go', 'retail/backend/internal/rowimport/paypal_config_json.go',
             'retail/backend/internal/rowimport/paypal_credentials.go',
             'retail/backend/internal/rowimport/paypal_config_json_test.go', 'retail/scripts/audit-paypal-config.py']
    paypal = 'medusa-store/apps/backend/node_modules/@easypayment/medusa-payment-paypal/'
    files += [paypal + name for name in ['src/modules/paypal/service.ts', 'src/modules/paypal/models/paypal_connection.ts',
                                        'src/modules/paypal/models/paypal_settings.ts', 'src/api/admin/paypal/settings/route.ts',
                                        '.medusa/server/src/modules/paypal/service.js']]
    counts = json.loads(markers[0])
    report = {'format_version': 1, 'scope': 'Actual two PayPal rows with empty credentials metadata and reviewed onboarding/API settings only; encrypted actual merchant credentials absent',
              'snapshot_sha256': PINNED_SHA, 'counts': counts, 'source_database_connection': False, 'provider_called': False,
              'new_keys_or_envelopes_persisted': False, 'private_values_in_report': False,
              'nonempty_nested_credentials_supported': True, 'actual_nonempty_nested_credentials_in_snapshot': False,
              'implementation_sha256': {f: hashlib.sha256((ROOT / f).read_bytes().replace(b'\r\n', b'\n')).hexdigest() for f in files}}
    (ROOT / 'docs/migration/paypal-config-v2-report.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps(counts))


if __name__ == '__main__':
    main()
