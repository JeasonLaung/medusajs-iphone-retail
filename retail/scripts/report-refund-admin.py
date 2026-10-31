#!/usr/bin/env python3
"""Record the final isolated Admin refund/after-sales browser run without claiming a live business journey."""
import hashlib
import json
import pathlib
import re

ROOT = pathlib.Path(__file__).resolve().parents[2]
LOG = pathlib.Path('/tmp/refund-admin-ui.log')
text = LOG.read_text()
assert re.search(r'\b15 passed \(', text), 'Final 15-test browser run must pass'
assert not re.search(r'\b(?:failed|interrupted|did not run)\b', text), 'Incomplete browser run'
tests = re.findall(r'✓\s+\d+\s+tests/auth\.spec\.ts:\d+:\d+\s+› (.+?) \([\d.]+s\)', text)
assert len(tests) == 15
for lang in ['zh', 'en']:
    for width in [390, 1440]:
        assert any(t.endswith(f'{lang} {width}') for t in tests)
sources = [
    'retail/admin/src/OrderRefund.tsx', 'retail/admin/src/refundApi.ts',
    'retail/admin/src/OrderAfterSales.tsx', 'retail/admin/src/api.ts',
    'retail/admin/src/Permissions.tsx', 'retail/admin/src/App.tsx',
    'retail/admin/tests/auth.spec.ts', 'retail/admin/playwright.config.ts',
    'retail/scripts/report-refund-admin.py',
]
assert all((ROOT / f).stat().st_mtime < LOG.stat().st_mtime for f in sources if not f.endswith('report-refund-admin.py')), 'Sources changed after browser run'
report = {
    'format_version': 1,
    'scope': 'React Admin return and exchange credit refund drawer with mocked business API responses; no actual shopper return or real provider refund',
    'browser': 'local Google Chrome via Playwright; isolated Vite port 19200',
    'tests_passed': tests,
    'passed': 15,
    'languages': ['zh', 'en'], 'widths': [390, 1440],
    'immutable_prepare_and_cancel_retry': True,
    'same_order_drawer_close_reopen_retains_pending_command': True,
    'explicit_predecessor_new_attempt': True,
    'disabled_malformed_revoked_writes_rejected': True,
    'existing_review_receipt_service_replacement_regression': True,
    'real_business_api': False, 'continuous_new_customer_refund': False,
    'real_provider_refund': False,
    'transcript_sha256': hashlib.sha256(LOG.read_bytes()).hexdigest(),
    'implementation_sha256_lf': {f: hashlib.sha256((ROOT / f).read_bytes().replace(b'\r\n', b'\n')).hexdigest() for f in sources},
    'screenshots_sha256': {f'refund-{kind}-{lang}-{width}.png': hashlib.sha256((ROOT / 'logs/gin-migration/admin-ui' / f'refund-{kind}-{lang}-{width}.png').read_bytes()).hexdigest() for kind in ['return','exchange'] for lang in ['zh','en'] for width in [390,1440]},
}
out = ROOT / 'docs/migration/refund-admin-ui-report.json'
out.write_text(json.dumps(report, ensure_ascii=False, indent=2)+'\n')
print('Refund Admin: 15 browser tests; mocked business APIs; SQL/CLI customer acceptance is recorded separately')
