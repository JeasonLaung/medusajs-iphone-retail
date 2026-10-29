"""Validate the new Astro app against an already-owned disposable Gin listener.

Called only by test-admin-live.py while it owns Gin/MySQL/Redis. It never starts,
stops or builds the existing Next storefront and accepts no source/production DSN.
"""
import hashlib
import json
import os
import re
from pathlib import Path
import shutil
import subprocess
from urllib.parse import urlsplit

ROOT=Path(__file__).resolve().parents[2]
APP=ROOT/'retail/storefront'

def main():
    target=urlsplit(os.environ.get('STOREFRONT_TEST_GIN',''))
    if target.scheme!='http' or target.hostname!='127.0.0.1' or not target.port or target.path not in ('','/') or target.query or target.fragment or target.username or target.password:
        raise RuntimeError('Owned loopback Gin listener required')
    node=Path(os.environ.get('STOREFRONT_TEST_NODE') or shutil.which('node') or '').resolve()
    version=subprocess.run([str(node),'--version'],capture_output=True,check=True).stdout.decode().strip()
    numbers=tuple(int(x) for x in version.lstrip('v').split('.'))
    if numbers<(22,23,4):
        raise RuntimeError('Astro requires Node >=22.23.4; set STOREFRONT_TEST_NODE to its isolated executable')
    env=os.environ.copy()
    env['PATH']=str(node.parent)+os.pathsep+env.get('PATH','')
    env['ASTRO_TELEMETRY_DISABLED']='1'
    npm=str(node.parent/('npm.cmd' if os.name=='nt' else 'npm'))
    if not Path(npm).is_file():npm=shutil.which('npm.cmd' if os.name=='nt' else 'npm',path=env['PATH'])
    subprocess.run([str(node),str(ROOT/'retail/scripts/generate-storefront-styles.mjs'),'--check'],cwd=ROOT,env=env,capture_output=True,timeout=60,check=True)
    subprocess.run([str(node),str(ROOT/'retail/scripts/capture-store-locales.cjs'),'--check'],cwd=ROOT,env=env,capture_output=True,timeout=30,check=True)
    for command in [[npm,'run','build'],[npm,'test']]:
        checked=subprocess.run(command,cwd=APP,env=env,capture_output=True,timeout=120)
        if checked.returncode:
            (ROOT/'logs/gin-migration/astro-check-result.log').write_bytes(checked.stdout+checked.stderr)
            raise RuntimeError('Astro build or unit checks failed; inspect ignored output')
    tap=checked.stdout.decode('utf-8')
    observed={key:int(value) for key,value in re.findall(r'^# (tests|pass|fail|cancelled|skipped|todo) (\d+)\s*$',tap,re.M)}
    if observed!={'tests':67,'pass':67,'fail':0,'cancelled':0,'skipped':0,'todo':0}:
        raise RuntimeError('Astro unit coverage differs; no partial success report written')
    tested=subprocess.run([str(node),'tests/runtime.integration.mjs'],cwd=APP,env=env,capture_output=True,timeout=90)
    markers=[line.split('ASTRO_RUNTIME_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if line.startswith('ASTRO_RUNTIME_REHEARSAL ')]
    if tested.returncode or len(markers)!=1:
        (ROOT/'logs/gin-migration/astro-runtime-result.log').write_bytes(tested.stdout+tested.stderr)
        raise RuntimeError('Astro owned runtime check failed; inspect ignored output')
    pages=subprocess.run([str(node),'tests/product-page.browser.mjs'],cwd=APP,env=env,capture_output=True,timeout=120)
    page_markers=[line.split('ASTRO_FORMAL_PRODUCT_PAGE ',1)[1] for line in pages.stdout.decode('utf-8').splitlines() if line.startswith('ASTRO_FORMAL_PRODUCT_PAGE ')]
    if pages.returncode or len(page_markers)!=1:
        (ROOT/'logs/gin-migration/astro-formal-pages-result.log').write_bytes(pages.stdout+pages.stderr)
        raise RuntimeError('Astro formal page browser check failed; inspect ignored output')
    browser=subprocess.run([str(node),'tests/selection.browser.mjs'],cwd=APP,env=env,capture_output=True,timeout=90)
    browser_markers=[line.split('ASTRO_SELECTION_BROWSER ',1)[1] for line in browser.stdout.decode('utf-8').splitlines() if line.startswith('ASTRO_SELECTION_BROWSER ')]
    if browser.returncode or len(browser_markers)!=1:
        (ROOT/'logs/gin-migration/astro-selection-browser-result.log').write_bytes(browser.stdout+browser.stderr)
        raise RuntimeError('Astro isolated selection browser check failed; inspect ignored output')
    presentation=subprocess.run([str(node),'tests/presentation.browser.mjs'],cwd=APP,env=env,capture_output=True,timeout=120)
    presentation_markers=[line.split('ASTRO_PRESENTATION_BROWSER ',1)[1] for line in presentation.stdout.decode('utf-8').splitlines() if line.startswith('ASTRO_PRESENTATION_BROWSER ')]
    if presentation.returncode or len(presentation_markers)!=1:
        (ROOT/'logs/gin-migration/astro-presentation-browser-result.log').write_bytes(presentation.stdout+presentation.stderr)
        raise RuntimeError('Astro original presentation browser comparison failed; inspect ignored output')
    actions=subprocess.run([str(node),'tests/actions.browser.mjs'],cwd=APP,env=env,capture_output=True,timeout=240)
    actions_markers=[line.split('ASTRO_ACTIONS_BROWSER ',1)[1] for line in actions.stdout.decode('utf-8').splitlines() if line.startswith('ASTRO_ACTIONS_BROWSER ')]
    if actions.returncode or len(actions_markers)!=1:
        (ROOT/'logs/gin-migration/astro-actions-browser-result.log').write_bytes(actions.stdout+actions.stderr)
        raise RuntimeError('Astro product actions browser comparison failed; inspect ignored output')
    cart=subprocess.run([str(node),'tests/cart.browser.mjs'],cwd=APP,env=env,capture_output=True,timeout=420)
    cart_markers=[line.split('ASTRO_CART_BROWSER ',1)[1] for line in cart.stdout.decode('utf-8').splitlines() if line.startswith('ASTRO_CART_BROWSER ')]
    if cart.returncode or len(cart_markers)!=1:
        (ROOT/'logs/gin-migration/astro-cart-browser-result.log').write_bytes(cart.stdout+cart.stderr)
        raise RuntimeError('Astro original cart browser comparison failed; inspect ignored output')
    footer=subprocess.run([str(node),'tests/footer.browser.mjs'],cwd=APP,env=env,capture_output=True,timeout=180)
    footer_markers=[line.split('ASTRO_FOOTER_BROWSER ',1)[1] for line in footer.stdout.decode('utf-8').splitlines() if line.startswith('ASTRO_FOOTER_BROWSER ')]
    if footer.returncode or len(footer_markers)!=1:
        (ROOT/'logs/gin-migration/astro-footer-browser-result.log').write_bytes(footer.stdout+footer.stderr)
        raise RuntimeError('Astro original footer browser comparison failed; inspect ignored output')
    header=subprocess.run([str(node),'tests/header.browser.mjs'],cwd=APP,env=env,capture_output=True,timeout=180)
    header_markers=[line.split('ASTRO_HEADER_BROWSER ',1)[1] for line in header.stdout.decode('utf-8').splitlines() if line.startswith('ASTRO_HEADER_BROWSER ')]
    if header.returncode or len(header_markers)!=1:
        (ROOT/'logs/gin-migration/astro-header-browser-result.log').write_bytes(header.stdout+header.stderr)
        raise RuntimeError('Astro original header browser comparison failed; inspect ignored output')
    audit=subprocess.run([npm,'audit','--json'],cwd=APP,env=env,capture_output=True,timeout=60)
    try:
        counts=json.loads(audit.stdout)['metadata']['vulnerabilities']
    except (ValueError,KeyError,TypeError):
        (ROOT/'logs/gin-migration/astro-audit-result.log').write_bytes(audit.stdout+audit.stderr)
        raise RuntimeError('Dependency audit returned no complete report; no success recorded') from None
    if audit.returncode or not isinstance(counts,dict) or any(type(counts.get(key)) is not int or counts[key]!=0 for key in ('info','low','moderate','high','critical','total')):
        raise RuntimeError('New Astro dependencies have unresolved or incomplete audit evidence')
    files=['retail/scripts/test-storefront-runtime.py']+[str(p.relative_to(ROOT)).replace('\\','/') for p in APP.rglob('*') if p.is_file() and not any(part in ('node_modules','dist','.astro') for part in p.relative_to(APP).parts)]
    files+=['retail/admin/package-lock.json','medusa-store/apps/storefront/src/lib/util/product-variants.ts',
            'medusa-store/apps/storefront/src/app/[countryCode]/(main)/products/[handle]/page.tsx',
            'medusa-store/apps/storefront/src/modules/products/components/product-actions/index.tsx',
            'medusa-store/apps/storefront/src/modules/products/components/image-gallery/index.tsx',
            'medusa-store/apps/storefront/src/modules/products/components/product-detail/index.tsx',
            'medusa-store/apps/storefront/src/modules/products/components/product-detail/styles.module.css',
            'medusa-store/apps/storefront/src/modules/products/i18n.ts',
            'medusa-store/pnpm-lock.yaml']
    files += ['retail/scripts/capture-store-locales.cjs','docs/migration/fixtures/legacy-store-locales.json','retail/backend/internal/catalog/default_locales.json']
    files += ['retail/scripts/generate-storefront-styles.mjs','docs/migration/storefront-styles.json']
    files += ['medusa-store/apps/storefront/'+name for name in ['tailwind.config.js','src/styles/globals.css','src/modules/layout/i18n.ts','src/modules/layout/templates/footer/index.tsx','src/modules/layout/templates/footer/styles.module.css','src/modules/common/components/brand-logo/index.tsx','src/modules/common/components/ui/index.tsx','src/lib/util/store-series-url.ts']]
    source_root=ROOT/'medusa-store/apps/storefront/src'
    for directory in ['modules/products/components/grade-guide','modules/products/components/sticky-purchase-bar',
                      'modules/common/components/selection-row','modules/common/components/grade-badge',
                      'modules/common/components/storefront-theme']:
        files += [str(p.relative_to(ROOT)).replace('\\','/') for p in (source_root/directory).rglob('*') if p.is_file()]
    files += ['medusa-store/apps/storefront/src/'+name for name in [
        'lib/hooks/use-in-view.tsx','lib/util/iphone-thumbnails.json','lib/util/get-product-price.ts',
        'lib/util/get-apple-launch-price.ts','lib/util/get-percentage-diff.ts','lib/util/money.ts',
        'lib/util/isEmpty.ts','lib/util/cart-inventory.ts','modules/products/components/color-swatches/index.tsx']]
    files += [str(p.relative_to(ROOT)).replace('\\','/') for p in (source_root/'modules/cart').rglob('*') if p.is_file()]
    files += ['medusa-store/apps/storefront/src/modules/common/components/page-entry-scroll/index.tsx']
    files += ['medusa-store/apps/storefront/src/modules/layout/components/'+folder+'/'+name for folder in ['store-header','region-language'] for name in ['index.tsx','styles.module.css']]
    files += ['medusa-store/apps/storefront/node_modules/@medusajs/icons/dist/esm/'+name+'.js' for name in ['chevron-down','x-mark']]
    public_assets={}
    for path in (APP/'public').rglob('*'):
        if not path.is_file():continue
        relative=path.relative_to(APP/'public')
        source=ROOT/'medusa-store/apps/storefront/public'/relative
        if not source.is_file() or path.read_bytes()!=source.read_bytes():
            raise RuntimeError('Migrated public asset differs from original')
        public_assets[str(relative).replace('\\','/')]=hashlib.sha256(path.read_bytes()).hexdigest()
    report={'format_version':1,'scope':'Formal product/store-search/about/help/journal SSR routes against owned Gin; synthetic public page browser checks and original component comparisons; checkout and full storefront not accepted','node_version':version,'unit_tests_passed':observed['pass'],'type_check_passed':True,'production_build_passed':True,'runtime':json.loads(markers[0]),'formal_pages_browser':json.loads(page_markers[0]),'selection_browser':json.loads(browser_markers[0]),'audit_vulnerabilities':counts,'real_application_resources_used':False,'full_storefront_accepted':False,'implementation_sha256':{name:hashlib.sha256((ROOT/name).read_bytes().replace(b'\r\n',b'\n')).hexdigest() for name in files}}
    report['presentation_browser']=json.loads(presentation_markers[0])
    report['actions_browser']=json.loads(actions_markers[0])
    report['header_browser']=json.loads(header_markers[0])
    report['footer_browser']=json.loads(footer_markers[0])
    report['original_styles_provenance_verified']=True
    report['cart_browser']=json.loads(cart_markers[0])
    report['public_assets_equal_to_original_sha256']=public_assets
    (ROOT/'docs/migration/astro-runtime-report.json').write_text(json.dumps(report,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({'astro_runtime':'passed','unit_tests':observed['pass'],'audit_vulnerabilities':0}))

if __name__=='__main__':main()
