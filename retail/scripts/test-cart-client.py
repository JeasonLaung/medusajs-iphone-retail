"""Astro cart transport: unit checks, build and browser against an artificial HTTP API.

No Docker, Gin, source database, application resources or existing dev service.
This does not claim full cart or transaction acceptance.
"""
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess

ROOT = Path(__file__).resolve().parents[2]
APP = ROOT / 'retail/storefront'


def main():
    if shutil.disk_usage(ROOT).free < 2 * (1 << 30):
        raise RuntimeError('At least 2 GiB free workspace disk required')
    node = Path(os.environ.get('STOREFRONT_TEST_NODE') or shutil.which('node') or '')
    version = subprocess.check_output([str(node), '--version'], text=True).strip()
    if tuple(int(x) for x in version.lstrip('v').split('.')) < (22, 23, 4):
        raise RuntimeError('Isolated compatible Node required')
    env = os.environ.copy()
    env['PATH'] = str(node.parent) + os.pathsep + env.get('PATH', '')
    env['ASTRO_TELEMETRY_DISABLED'] = '1'
    npm = shutil.which('npm.cmd' if os.name == 'nt' else 'npm', path=env['PATH'])
    checked = subprocess.run([npm, 'test'], cwd=APP, env=env, capture_output=True, timeout=60)
    observed = {key: int(value) for key, value in re.findall(r'^# (tests|pass|fail|cancelled|skipped|todo) (\d+)\s*$', checked.stdout.decode('utf-8'), re.M)}
    if checked.returncode or observed != {'tests': 43, 'pass': 43, 'fail': 0, 'cancelled': 0, 'skipped': 0, 'todo': 0}:
        raise RuntimeError('Cart transport unit checks failed')
    built = subprocess.run([npm, 'run', 'build'], cwd=APP, env=env, capture_output=True, timeout=150)
    if built.returncode:
        (ROOT/'logs/gin-migration/cart-client-build.log').write_bytes(built.stdout+built.stderr)
        raise RuntimeError('Astro cart transport type/build check failed; inspect ignored output')
    browser = subprocess.run([str(node), 'tests/cart-client.browser.mjs'], cwd=APP, env=env, capture_output=True, timeout=60)
    markers = [line.split('CART_CLIENT_BROWSER ', 1)[1] for line in browser.stdout.decode('utf-8').splitlines() if line.startswith('CART_CLIENT_BROWSER ')]
    if browser.returncode or len(markers) != 1:
        (ROOT/'logs/gin-migration/cart-client-browser.log').write_bytes(browser.stdout+browser.stderr)
        raise RuntimeError('Browser cart transport checks failed; inspect ignored output')
    files = ['retail/storefront/src/lib/cart-commerce.ts','retail/storefront/tests/cart-commerce.test.ts','retail/storefront/src/lib/cart-quantities.ts','retail/scripts/test-cart-client.py', 'retail/storefront/src/lib/cart-client.ts',
             'retail/storefront/tests/cart-client.test.ts', 'retail/storefront/tests/cart-client.browser.mjs',
             'retail/storefront/package.json', 'retail/storefront/package-lock.json',
             'retail/storefront/astro.config.mjs', 'retail/storefront/tsconfig.json']
    files += ['retail/storefront/src/components/cart/types.ts','retail/storefront/tests/cart-display.test.ts']
    report = {'format_version': 1, 'scope': 'Private browser transport against artificial HTTP API only; not Gin/MySQL/Redis or production cart acceptance',
              'node_version': version, 'unit_tests_passed': 43, 'type_check_and_build_passed': True,
              'browser': json.loads(markers[0]), 'application_resources_used': False,
              'existing_services_started_stopped_or_built': False,
              'cleanup': 'browser context, browser and owned loopback listener closed',
              'implementation_sha256': {name: hashlib.sha256((ROOT/name).read_bytes().replace(b'\r\n', b'\n')).hexdigest() for name in files}}
    (ROOT/'docs/migration/cart-client-report.json').write_text(json.dumps(report, indent=2)+'\n', encoding='utf-8')
    print(json.dumps({'cart_client': 'passed', 'unit_tests': 43, 'real_browser': True, 'fixture_api': True, 'real_gin': False}))


if __name__ == '__main__':
    main()
