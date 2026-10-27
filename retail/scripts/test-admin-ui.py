"""Build and browser-check Ant login/account UI using artificial HTTP fixtures."""
import hashlib
import json
import os
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[2]
APP = ROOT / 'retail/admin'


def main():
    npm = 'npm.cmd' if os.name == 'nt' else 'npm'
    built = subprocess.run([npm, 'run', 'build'], cwd=APP, capture_output=True, timeout=120)
    if built.returncode:
        raise RuntimeError('Admin type check or production build failed')
    # Optional executable override is explicit. Otherwise Playwright uses its
    # installed pinned browser; this script never downloads one automatically.
    tested = subprocess.run([npm, 'exec', 'playwright', 'test', '--', '--reporter=json'],
                            cwd=APP, capture_output=True, timeout=180)
    if tested.returncode:
        (ROOT / 'logs/gin-migration/admin-ui-result.json').write_bytes(tested.stdout)
        raise RuntimeError('Artificial admin browser checks failed; inspect ignored test output')
    result = json.loads(tested.stdout)
    if result['stats']['expected'] != 10 or result['stats']['unexpected'] or result['stats']['skipped'] or result['stats']['flaky']:
        raise RuntimeError('Browser test coverage differs')
    files = ['retail/scripts/test-admin-ui.py'] + [str(p.relative_to(ROOT)).replace('\\', '/')
             for p in APP.rglob('*') if p.is_file() and not any(part in ('node_modules', 'dist') for part in p.relative_to(APP).parts)]
    screenshots = sorted((ROOT / 'logs/gin-migration/admin-ui').glob('*.png'))
    if len(screenshots) != 9:
        raise RuntimeError('Expected three widths and nine language/state screenshots')
    report = {'format_version': 1, 'scope': 'Ant login/account and product list/detail UI with artificial HTTP responses only; not real browser-to-Gin/MySQL/Redis acceptance',
              'counts': {'browser_tests_passed': 10, 'widths': [390, 768, 1440], 'screenshots': 9},
              'languages': ['zh-CN', 'en'], 'browser_executable_override_used': bool(os.environ.get('ADMIN_TEST_BROWSER')),
              'type_check_passed': True, 'production_build_passed': True,
              'initial_bundle_size_warning': b'500 kB' in built.stderr or b'500 kB' in built.stdout,
              'application_backend_used': False, 'real_accounts_used': False, 'full_admin_accepted': False,
              'capabilities_in_browser_storage': False,
              'screenshot_sha256': {p.name: hashlib.sha256(p.read_bytes()).hexdigest() for p in screenshots},
              'implementation_sha256': {name: hashlib.sha256((ROOT / name).read_bytes().replace(b'\r\n', b'\n')).hexdigest() for name in sorted(files)}}
    (ROOT / 'docs/migration/admin-ui-report.json').write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'tests': 10, 'screenshots': 9, 'type_check': 'passed', 'build': 'passed', 'real_backend_used': False}))


if __name__ == '__main__':
    main()
