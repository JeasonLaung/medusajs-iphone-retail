"""Pinned account-holder baseline conversion, no SQL/provider/ownership effects."""
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
    snapshot = parser.parse_args().snapshot.resolve()
    if not snapshot.is_file() or subprocess.run(['git', 'check-ignore', '--quiet', str(snapshot)], cwd=ROOT, capture_output=True).returncode:
        raise RuntimeError('Explicit ignored v2 snapshot required')
    if hashlib.sha256(snapshot.read_bytes()).hexdigest() != PINNED_SHA:
        raise RuntimeError('Pinned account-holder source differs')
    environment = os.environ.copy()
    environment['ROWIMPORT_ACCOUNT_HOLDER_SNAPSHOT'] = str(snapshot)
    result = subprocess.run(['go', 'test', '-count=1', '-v', '-run', '^TestPinnedAccountHolderHistoryOffline$', './internal/rowimport'],
                            cwd=ROOT / 'retail/backend', env=environment, capture_output=True, text=True, timeout=120)
    markers = [line.split('ACCOUNT_HOLDER_AUDIT ', 1)[1] for line in result.stdout.splitlines() if 'ACCOUNT_HOLDER_AUDIT ' in line]
    if result.returncode or len(markers) != 1:
        raise RuntimeError('Account-holder conversion failed; private output suppressed')
    counts = json.loads(markers[0])
    files = ['retail/scripts/audit-account-holder.py'] + [
        'retail/backend/internal/rowimport/' + name for name in
        ('history_account_holder.go', 'history_account_holder_test.go', 'convert.go', 'field_plans.json', 'record.go', 'stream.go')]
    files += ['retail/backend/internal/protected/envelope.go']
    providers = list((ROOT / 'medusa-store/node_modules/.pnpm').glob('@medusajs+payment@2.21.2_*/node_modules/@medusajs/payment'))
    if len(providers) != 1:
        raise RuntimeError('Pinned installed source payment module ambiguous')
    for name in ['dist/models/account-holder.js', 'dist/providers/system.js', 'dist/services/payment-module.js']:
        files.append(str((providers[0] / name).relative_to(ROOT)).replace('\\', '/'))
    report = {
        'format_version': 1, 'scope': 'One actual system-provider account-holder baseline through full v2 validation and ephemeral Go conversion only; no SQL, ownership or full import acceptance',
        'snapshot_sha256': PINNED_SHA, 'counts': counts,
        'source_database_connection': False, 'target_database_connection': False, 'provider_called': False,
        'new_keys_or_envelopes_persisted': False, 'private_values_in_report': False,
        'legacy_customer_payment_links_restored': False, 'historical_external_reference_is_authorization': False,
        'sql_operation_holds_persisted': False,
        'implementation_sha256': {file: hashlib.sha256((ROOT / file).read_bytes().replace(b'\r\n', b'\n')).hexdigest() for file in files},
    }
    (ROOT / 'docs/migration/account-holder-conversion-report.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps(counts))


if __name__ == '__main__':
    main()
