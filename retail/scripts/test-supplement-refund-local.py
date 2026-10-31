"""Owned native SQL/Redis joint over the committed source plus explicit test edits.

Root setup/cleanup is delegated only for the generated, bounded 0600 SQL files.
The optional hidden root prompt is used only by the audited SQL test process.
No production data, real network provider or mail is accepted.
"""
import argparse
import hashlib
import getpass
import sys
import io
import json
import os
from pathlib import Path
import re
import secrets
import shutil
import signal
import socket
import subprocess
import tarfile
import time
import urllib.request

ROOT = Path(__file__).resolve().parents[2]
BASE = ROOT/'logs/gin-migration'
STATE = BASE/'supplement-refund-local-state.json'
OWNED = ['retail/backend/internal/payments/mollie_create.go',
 'retail/backend/internal/payments/mollie_trust_test.go',
 'retail/backend/internal/checkoutconfig/checkout.go',
 'retail/backend/internal/checkoutconfig/ca_test.go',
 'retail/backend/.env.example',
 'retail/backend/internal/refunds/worker.go',
 'retail/backend/internal/identity/order_payment_refund_test.go'] + [
 'retail/backend/internal/identity/'+n for n in [
 'checkout_browser_test.go','checkout_fulfillment_browser_test.go',
 'checkout_payment_worker_test.go','checkout_refund_worker_test.go',
 'checkout_exchange_browser_test.go','checkout_supplement_refund_test.go']
] + ['retail/admin/tests/checkout-refund.joint.mjs',
 'retail/admin/tests/checkout-fulfillment.joint.mjs',
 'retail/storefront/tests/checkout.live.mjs',
 'retail/storefront/tests/checkout-supplement-refund.joint.mjs',
 'retail/scripts/test-supplement-refund-local.py']
MYSQL='/opt/homebrew/bin/mysql'
GO='/opt/homebrew/opt/go/bin/go'
NODE='/opt/homebrew/bin/node'

def sha(p):
 return hashlib.sha256(p.read_bytes().replace(b'\r\n',b'\n')).hexdigest()

def private(p,d):
 p.write_text(json.dumps(d,indent=2)+'\n');p.chmod(0o600)

def config():
 c=json.loads(STATE.read_text());o=c['owner']
 if not re.fullmatch('[a-f0-9]{12}',o) or c['database']!='eurorefurb_supp_refund_'+o or c['user']!='retail_srf_'+o or c['redis_prefix']!='eurorefurb:s'+o+'-' or c['snapshot']!=str(BASE/('supplement-refund-local-'+o)):
  raise RuntimeError('Owned resource identity mismatch')
 return c

def replace(p,a,b):
 s=p.read_text()
 if s.count(a)!=1:raise RuntimeError('Source anchor changed: '+p.name)
 p.write_text(s.replace(a,b))

def prepare():
 if STATE.exists():raise RuntimeError('Inspect and clean existing owned state first')
 if shutil.disk_usage(ROOT).free<512*1024*1024:raise RuntimeError('512 MiB free required')
 BASE.mkdir(parents=True,exist_ok=True)
 sockets=[socket.socket() for _ in range(3)]
 try:
  for s in sockets:s.bind(('127.0.0.1',0))
  ports=[s.getsockname()[1] for s in sockets]
 finally:
  for s in sockets:s.close()
 o=secrets.token_hex(6)
 c=dict(owner=o,database='eurorefurb_supp_refund_'+o,user='retail_srf_'+o,
  password=secrets.token_urlsafe(30),redis_prefix='eurorefurb:s'+o+'-',
  api_port=ports[0],storefront_port=ports[1],admin_port=ports[2],fixture_key=secrets.token_hex(32),
  snapshot=str(BASE/('supplement-refund-local-'+o)),commit=subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip())
 snap=Path(c['snapshot']);snap.mkdir()
 # Exclude all unrelated uncommitted catalog/review changes and migration counts.
 archive=subprocess.check_output(['git','archive',c['commit'],'retail/backend','retail/admin','retail/storefront','docs/migration/fixtures'],cwd=ROOT)
 with tarfile.open(fileobj=io.BytesIO(archive)) as tar:tar.extractall(snap,filter='data')
 for n in OWNED:
  (snap/n).parent.mkdir(parents=True,exist_ok=True);shutil.copy2(ROOT/n,snap/n)
 for f in ['admin','storefront']:(snap/'retail'/f/'node_modules').symlink_to(ROOT/'retail'/f/'node_modules',target_is_directory=True)
 (snap/'logs/gin-migration').mkdir(parents=True)
 original={str(p.relative_to(snap)):sha(p) for p in snap.rglob('*') if p.is_file() and 'node_modules' not in p.parts}
 for p in (snap/'retail/backend').rglob('*_test.go'):
  s=p.read_text().replace('127.0.0.1:6379','127.0.0.1:16379').replace('eurorefurb:',c['redis_prefix']).replace('cart_runtime_rehearsal',c['database'])
  s=s.replace('19379',str(c['storefront_port'])).replace('19383',str(c['admin_port'])).replace('0.0.0.0:9100','127.0.0.1:'+str(c['api_port'])).replace('127.0.0.1:9100','127.0.0.1:'+str(c['api_port']))
  p.write_text(s)
 runtime=snap/'retail/backend/internal/identity/cart_runtime_test.go'
 replace(runtime,'cfg.User, cfg.Passwd, cfg.Net, cfg.Addr = "root", os.Getenv("MYSQL_PWD"), "unix", "/var/run/mysqld/mysqld.sock"','cfg.User, cfg.Passwd, cfg.Net, cfg.Addr = os.Getenv("SUPPLEMENT_TEST_USER"), os.Getenv("MYSQL_PWD"), "tcp", "127.0.0.1:3306"')
 replace(runtime,'duration = 8 * time.Minute','duration = 12 * time.Minute')
 for f,old,name in [('checkout_refund_worker_test.go','/opt/refund-worker','refund-worker'),('checkout_payment_worker_test.go','/opt/order-payment-worker','payment-worker')]:
  p=snap/'retail/backend/internal/identity'/f
  s=p.read_text().replace(old,str(snap/name)).replace('"MYSQL_DSN=root:" + os.Getenv("MYSQL_PWD")','"MYSQL_DSN=" + os.Getenv("SUPPLEMENT_TEST_USER") + ":" + os.Getenv("MYSQL_PWD")').replace('@unix(/var/run/mysqld/mysqld.sock)/','@tcp(127.0.0.1:3306)/')
  p.write_text(s)
 for p in (snap/'retail').rglob('*.mjs'):
  if 'node_modules' in p.parts:continue
  p.write_text(p.read_text().replace('19379',str(c['storefront_port'])).replace('19383',str(c['admin_port'])))
 admin=snap/'retail/admin/tests/checkout-fulfillment.joint.mjs'
 replace(admin, 'server=await createServer({root:', "server=await createServer({cacheDir:fileURLToPath(new URL('../.vite-supp-refund',import.meta.url)),root:")
 c['source_sha256_lf']=original
 c['adapted_source_sha256_lf']={n:sha(snap/n) for n in original if sha(snap/n)!=original[n]}
 if any(not (n.endswith('_test.go') or n.endswith('.mjs')) for n in c['adapted_source_sha256_lf']):raise RuntimeError('Production source adaptation forbidden')
 setup=snap/'setup.sql';setup.write_text('\n'.join(f"CREATE USER '{c['user']}'@'{h}' IDENTIFIED BY '{c['password']}';\nGRANT ALL PRIVILEGES ON `{c['database']}`.* TO '{c['user']}'@'{h}';" for h in ['127.0.0.1','localhost'])+'\n');setup.chmod(0o600)
 cleanup=snap/'cleanup.sql';cleanup.write_text(f"DROP DATABASE IF EXISTS `{c['database']}`;\n"+'\n'.join(f"DROP USER IF EXISTS '{c['user']}'@'{h}';" for h in ['127.0.0.1','localhost'])+f"\nSELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{c['database']}';\nSELECT COUNT(*) FROM mysql.user WHERE user='{c['user']}';\n");cleanup.chmod(0o600)
 private(STATE,c)
 public={k:c[k] for k in ['owner','database','user','redis_prefix','snapshot','commit','api_port','storefront_port','admin_port']}
 private(snap/'resources.json',public);print(json.dumps(public))

def run(root_fixture=False):
 c=config();snap=Path(c['snapshot']);env=os.environ.copy()
 env.update(PATH='/opt/homebrew/opt/go/bin:/opt/homebrew/bin:'+env.get('PATH',''),MYSQL_PWD=c['password'],SUPPLEMENT_TEST_USER=c['user'],IDENTITY_REHEARSAL='1',CHECKOUT_BROWSER_FIXTURE='1',CHECKOUT_FIXTURE_KEY=c['fixture_key'],CUSTOMER_ORDERS_TEST_GIN='http://127.0.0.1:'+str(c['api_port']),ADMIN_TEST_BROWSER='/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',ASTRO_TELEMETRY_DISABLED='1',CHECKOUT_REFUND_WORKER_BINARY=str(snap/'refund-worker'),CHECKOUT_PAYMENT_WORKER_BINARY=str(snap/'payment-worker'))
 for k in ['PRODUCT_BROWSE','FRESH_CART','ACCOUNTS','FULFILLMENT','EXCHANGE','EXCHANGE_PAYMENT','SUPPLEMENT_REFUND']:env['CHECKOUT_'+k]='1'
 for k in ['REFUND','EXCHANGE_REFUND','REPAIR','REVIEWS']:env['CHECKOUT_'+k]='0'
 for p in [c['api_port'],c['admin_port'],c['storefront_port']]:
  if p in [8000,9001,9100,9200,9300,19379,19383]:raise RuntimeError('Reserved port')
  with socket.socket() as s:s.bind(('127.0.0.1',p))
 def mysql(sql):
  return subprocess.run([MYSQL,'--host=127.0.0.1','--user='+c['user'],'--batch','--skip-column-names','--execute',sql],env=env,capture_output=True,text=True)
 exists=mysql("SELECT schema_name FROM information_schema.schemata WHERE schema_name='"+c['database']+"'")
 if exists.returncode or exists.stdout.strip():raise RuntimeError('User setup missing or owned database exists; no test fact reuse')
 expected={n:c['adapted_source_sha256_lf'].get(n,d) for n,d in c['source_sha256_lf'].items()}
 def frozen():
  for n,d in expected.items():
   if sha(snap/n)!=d:raise RuntimeError('Frozen source changed: '+n)
 frozen();processes=[];log=snap/'logs/gin-migration'
 def stop(p):
  if p.poll() is None:
   os.killpg(p.pid,signal.SIGTERM)
   try:p.wait(timeout=5)
   except subprocess.TimeoutExpired:os.killpg(p.pid,signal.SIGKILL);p.wait(timeout=5)
 try:
  for target,out in [('process-order-refunds','refund-worker'),('process-order-payments','payment-worker')]:
   with (log/(out+'-build.log')).open('w') as f:
    subprocess.run([GO,'build','-o',str(snap/out),'./cmd/'+target],cwd=snap/'retail/backend',env=env,stdout=f,stderr=subprocess.STDOUT,timeout=120,check=True)
  with (log/'astro-build.log').open('w') as f:subprocess.run([NODE,'node_modules/astro/bin/astro.mjs','build'],cwd=snap/'retail/storefront',env=env,stdout=f,stderr=subprocess.STDOUT,timeout=150,check=True)
  backend_env=env.copy()
  if root_fixture:
   if not sys.stdin.isatty():raise RuntimeError('Root fixture requires hidden PTY prompt')
   backend_env['MYSQL_PWD']=getpass.getpass('Owned fixture root password (hidden): ')
   backend_env['SUPPLEMENT_TEST_USER']='root'
  with (log/'joint-sql.log').open('w') as f:
   backend=subprocess.Popen([GO,'test','./internal/identity','-run','^TestOwnedCartRuntime$','-count=1','-timeout','15m','-v'],cwd=snap/'retail/backend',env=backend_env,stdout=f,stderr=subprocess.STDOUT,start_new_session=True);processes.append(backend)
   backend_env.pop('MYSQL_PWD',None)
   op=urllib.request.build_opener(urllib.request.ProxyHandler({}));ready=False
   for _ in range(600):
    if backend.poll() is not None:raise RuntimeError('SQL fixture stopped before readiness')
    try:
     req=urllib.request.Request(env['CUSTOMER_ORDERS_TEST_GIN']+'/__fixture/ready',headers={'X-Fixture-Key':c['fixture_key']})
     with op.open(req,timeout=1) as r:ready=r.status==200
     if ready:break
    except OSError:pass
    time.sleep(.2)
   if not ready:raise RuntimeError('SQL fixture readiness deadline')
   with (log/'joint-browser.log').open('w') as output:
    browser=subprocess.Popen([NODE,'tests/checkout.live.mjs'],cwd=snap/'retail/storefront',env=env,stdout=output,stderr=subprocess.STDOUT,start_new_session=True);processes.append(browser)
    if browser.wait(timeout=660):raise RuntimeError('Browser joint failed')
   if backend.wait(timeout=90):raise RuntimeError('SQL postconditions failed')
  frozen()
  text=(log/'joint-sql.log').read_text();browser_text=(log/'joint-browser.log').read_text()
  markers=[json.loads(x.split('CHECKOUT_SUPPLEMENT_REFUND_JOINT_SQL ',1)[1]) for x in text.splitlines() if 'CHECKOUT_SUPPLEMENT_REFUND_JOINT_SQL ' in x]
  browsers=[json.loads(x.split('CHECKOUT_JOINT_BROWSER ',1)[1]) for x in browser_text.splitlines() if x.startswith('CHECKOUT_JOINT_BROWSER ')]
  if len(markers)!=1 or len(browsers)!=1 or '\nPASS\n' not in text or not browsers[0].get('same_new_order_supplemental_cash_return_refund_completion_denied_by_source_rule') or browsers[0].get('browser_errors')!=0:raise RuntimeError('Joint evidence incomplete')
  private(snap/'joint-result.json',dict(sql=markers[0],browser=browsers[0],source_frozen_before_build_and_verified_after=True,compiled_cli_sha256={n:sha(snap/n) for n in ['payment-worker','refund-worker']}))
  print(json.dumps({'supplement_refund_joint':'passed','root_cleanup_required':True}))
 finally:
  for p in reversed(processes):stop(p)
  redis='/opt/homebrew/bin/redis-cli';found=subprocess.run([redis,'-p','16379','--scan','--pattern',c['redis_prefix']+'*'],capture_output=True,text=True,check=True)
  keys=found.stdout.splitlines()
  if any(not k.startswith(c['redis_prefix']) for k in keys):raise RuntimeError('Unowned Redis key')
  for i in range(0,len(keys),100):subprocess.run([redis,'-p','16379','DEL',*keys[i:i+100]],capture_output=True,check=True)
  dropped=mysql('DROP DATABASE IF EXISTS `'+c['database']+'`; SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name=\''+c['database']+'\'')
  if dropped.returncode or dropped.stdout.strip()!='0':raise RuntimeError('Owned database cleanup failed')
  private(snap/'process-cleanup.json',dict(owned_processes_terminal=all(p.poll() is not None for p in processes),owned_database_absent=True,owned_redis_keys_deleted=len(keys),root_user_cleanup_pending=True))

if __name__=='__main__':
 parser=argparse.ArgumentParser();parser.add_argument('mode',choices=['prepare','run']);parser.add_argument('--root-fixture',action='store_true');args=parser.parse_args()
 prepare() if args.mode=='prepare' else run(args.root_fixture)
