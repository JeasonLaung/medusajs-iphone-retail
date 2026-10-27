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
    for command in [[npm,'run','build'],[npm,'test']]:
        checked=subprocess.run(command,cwd=APP,env=env,capture_output=True,timeout=120)
        if checked.returncode:
            (ROOT/'logs/gin-migration/astro-check-result.log').write_bytes(checked.stdout+checked.stderr)
            raise RuntimeError('Astro build or unit checks failed; inspect ignored output')
    tap=checked.stdout.decode('utf-8')
    observed={key:int(value) for key,value in re.findall(r'^# (tests|pass|fail|cancelled|skipped|todo) (\d+)\s*$',tap,re.M)}
    if observed!={'tests':16,'pass':16,'fail':0,'cancelled':0,'skipped':0,'todo':0}:
        raise RuntimeError('Astro unit coverage differs; no partial success report written')
    tested=subprocess.run([str(node),'tests/runtime.integration.mjs'],cwd=APP,env=env,capture_output=True,timeout=90)
    markers=[line.split('ASTRO_RUNTIME_REHEARSAL ',1)[1] for line in tested.stdout.decode().splitlines() if line.startswith('ASTRO_RUNTIME_REHEARSAL ')]
    if tested.returncode or len(markers)!=1:
        (ROOT/'logs/gin-migration/astro-runtime-result.log').write_bytes(tested.stdout+tested.stderr)
        raise RuntimeError('Astro owned runtime check failed; inspect ignored output')
    browser=subprocess.run([str(node),'tests/selection.browser.mjs'],cwd=APP,env=env,capture_output=True,timeout=90)
    browser_markers=[line.split('ASTRO_SELECTION_BROWSER ',1)[1] for line in browser.stdout.decode('utf-8').splitlines() if line.startswith('ASTRO_SELECTION_BROWSER ')]
    if browser.returncode or len(browser_markers)!=1:
        (ROOT/'logs/gin-migration/astro-selection-browser-result.log').write_bytes(browser.stdout+browser.stderr)
        raise RuntimeError('Astro isolated selection browser check failed; inspect ignored output')
    audit=subprocess.run([npm,'audit','--json'],cwd=APP,env=env,capture_output=True,timeout=60)
    counts=json.loads(audit.stdout)['metadata']['vulnerabilities']
    if counts.get('total')!=0:raise RuntimeError('New Astro dependencies have unresolved audit advisories')
    files=['retail/scripts/test-storefront-runtime.py']+[str(p.relative_to(ROOT)).replace('\\','/') for p in APP.rglob('*') if p.is_file() and not any(part in ('node_modules','dist','.astro') for part in p.relative_to(APP).parts)]
    files+=['retail/admin/package-lock.json','medusa-store/apps/storefront/src/lib/util/product-variants.ts',
            'medusa-store/apps/storefront/src/app/[countryCode]/(main)/products/[handle]/page.tsx',
            'medusa-store/apps/storefront/src/modules/products/components/product-actions/index.tsx',
            'medusa-store/apps/storefront/src/modules/products/components/image-gallery/index.tsx']
    report={'format_version':1,'scope':'Astro Node SSR regions foundation, source-level product adapter to owned Gin, and selection browser check in a separate artificial test root; no business page acceptance','node_version':version,'unit_tests_passed':observed['pass'],'type_check_passed':True,'production_build_passed':True,'runtime':json.loads(markers[0]),'selection_browser':json.loads(browser_markers[0]),'audit_vulnerabilities':counts,'real_application_resources_used':False,'full_storefront_accepted':False,'implementation_sha256':{name:hashlib.sha256((ROOT/name).read_bytes().replace(b'\r\n',b'\n')).hexdigest() for name in files}}
    (ROOT/'docs/migration/astro-runtime-report.json').write_text(json.dumps(report,indent=2)+'\n',encoding='utf-8')
    print(json.dumps({'astro_runtime':'passed','unit_tests':observed['pass'],'audit_vulnerabilities':0}))

if __name__=='__main__':main()
