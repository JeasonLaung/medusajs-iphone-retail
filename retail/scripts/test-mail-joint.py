"""Real mail configuration and customer verification over owned Gin/MySQL/Redis.

No external mail: only the worker's HTTP transport is replaced, in its test
process. The order fixture verifies durable admin recovery, not paid checkout.
"""
import json
import os
from pathlib import Path
import queue
import re
import secrets
import shutil
import subprocess
import threading
import time
import urllib.request
import uuid

ROOT = Path(__file__).resolve().parents[2]
LABEL = 'eurorefurb.mail-browser-rehearsal'


def docker(*args, timeout=60):
    try:
        return subprocess.run(['docker', *args], capture_output=True, timeout=timeout)
    except subprocess.TimeoutExpired:
        raise RuntimeError('Owned mail Docker operation timed out; arguments suppressed') from None


def main():
    if shutil.disk_usage(ROOT).free < 2 * (1 << 30):
        raise RuntimeError('At least 2 GiB free workspace disk required')
    for image in ['mysql:8.4', 'redis:7-alpine']:
        if docker('image', 'inspect', image).returncode:
            raise RuntimeError('Existing fixture images required')
    backend = Path(os.environ.get('MAIL_JOINT_BACKEND', str(ROOT/'retail/backend'))).resolve()
    owner, password = uuid.uuid4().hex, secrets.token_urlsafe(32)
    binaries = [ROOT/'logs/gin-migration'/('mail-'+owner+suffix) for suffix in ['-api', '-prep.test']]
    for binary in binaries:
        if binary.exists() or subprocess.run(['git', 'check-ignore', '--quiet', str(binary)], cwd=ROOT, capture_output=True).returncode:
            raise RuntimeError('New ignored output required')
    containers, network, browser = [], None, None
    log = ROOT/'logs/gin-migration/mail-joint-browser.log'
    try:
        env = dict(os.environ, GOOS='linux', GOARCH='amd64', CGO_ENABLED='0')
        for binary, command in zip(binaries, [['go','build','-o',str(binaries[0]),'./cmd/api'], ['go','test','-c','-o',str(binaries[1]),'./internal/identity']]):
            if subprocess.run(command, cwd=backend, env=env, capture_output=True, timeout=120).returncode:
                raise RuntimeError('Mail runner compilation failed')
        made = docker('network','create','--label',LABEL+'='+owner,'eurorefurb-mail-'+owner[:12])
        if made.returncode: raise RuntimeError('Owned mail network failed')
        network = made.stdout.decode().strip()
        made = docker('run','-d','--name','eurorefurb-mail-mysql-'+owner[:12],'--label',LABEL+'='+owner,'--network',network,
                      '--cpus','1','--memory','768m','--memory-swap','768m','--pids-limit','192','--tmpfs','/var/lib/mysql:rw,size=512m',
                      '--tmpfs','/tmp:rw,size=32m,mode=1777','-p','127.0.0.1::9100','-e','MYSQL_ROOT_PASSWORD='+password,
                      '-e','MYSQL_DATABASE=browser_identity','mysql:8.4','--skip-log-bin','--innodb-buffer-pool-size=64M',
                      '--performance-schema=OFF','--mysqlx=OFF','--max-connections=16')
        if made.returncode: raise RuntimeError('Owned mail SQL failed')
        mysql_id = made.stdout.decode().strip(); containers.append(mysql_id)
        made = docker('run','-d','--name','eurorefurb-mail-redis-'+owner[:12],'--label',LABEL+'='+owner,'--network','container:'+mysql_id,
                      '--cpus','1','--memory','128m','--memory-swap','128m','--pids-limit','64','--tmpfs','/data:rw,size=16m',
                      'redis:7-alpine','redis-server','--bind','127.0.0.1','--save','','--appendonly','no','--maxmemory','16mb','--maxmemory-policy','noeviction')
        if made.returncode: raise RuntimeError('Owned mail Redis failed')
        containers.append(made.stdout.decode().strip())
        for _ in range(100):
            if not docker('exec','-e','MYSQL_PWD='+password,mysql_id,'mysql','-uroot','--protocol=socket','-e','SELECT 1',timeout=10).returncode: break
            time.sleep(.5)
        else: raise RuntimeError('Owned mail SQL not ready')
        for binary, target in zip(binaries, ['/opt/api','/opt/prep.test']):
            with binary.open('rb') as source:
                copied = subprocess.run(['docker','exec','-i',mysql_id,'/bin/sh','-c','umask 077; cat > '+target],stdin=source,capture_output=True,timeout=60)
            if copied.returncode or docker('exec',mysql_id,'chmod','700',target).returncode: raise RuntimeError('Owned binary transfer failed')
        for fixture in ['TestPrepareOwnedBrowserAPI','TestPrepareReviewedCatalogFixture']:
            tested = docker('exec','-e','MYSQL_PWD='+password,'-e','BROWSER_IDENTITY_FIXTURE=1',mysql_id,'/opt/prep.test','-test.v','-test.run','^'+fixture+'$','-test.timeout','40s')
            (ROOT/'logs/gin-migration/mail-joint-preparation.log').write_bytes(tested.stdout+tested.stderr)
            if tested.returncode: raise RuntimeError('Owned mail preparation failed; inspect ignored log')
        app_env = {'STOREFRONT_STORE_ID':'fixture-store','GIN_MODE':'release','APP_BIND':'0.0.0.0','APP_PORT':'9100','APP_ENV':'development',
                   'MYSQL_DSN':'root:'+password+'@unix(/var/run/mysqld/mysqld.sock)/browser_identity?parseTime=true&loc=UTC',
                   'SESSION_REDIS_URL':'redis://127.0.0.1:6379/0','SESSION_REDIS_PREFIX':'eurorefurb:mail_joint:v1:',
                   'ADMIN_ORIGIN':'http://127.0.0.1:19384','STOREFRONT_ORIGIN':'http://127.0.0.1:19383','SESSION_SECURE':'false',
                   'CUSTOMER_AUTH_ENABLED':'true','CUSTOMER_BRIDGE_KEY':secrets.token_hex(32),'CUSTOMER_KEY_ACTIVE':'fixture',
                   'CUSTOMER_KEYRING_JSON':json.dumps({'fixture':secrets.token_hex(32)}),'NOTIFICATION_MAIL_ENABLED':'true',
                   'MERCHANT_KEY_ACTIVE':'fixture','MERCHANT_KEYRING_JSON':json.dumps({'fixture':secrets.token_hex(32)})}
        args = ['exec','-d','-w','/opt']
        for key,value in app_env.items(): args.extend(['-e',key+'='+value])
        if docker(*args,mysql_id,'/opt/api').returncode: raise RuntimeError('Owned mail API failed')
        match = re.fullmatch(r'127\.0\.0\.1:(\d+)\s*',docker('port',mysql_id,'9100/tcp').stdout.decode())
        if not match: raise RuntimeError('Loopback-only API port required')
        target = 'http://127.0.0.1:'+match[1]
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        for _ in range(40):
            try:
                with opener.open(target+'/health',timeout=2) as response:
                    if response.status == 200: break
            except OSError: pass
            time.sleep(.25)
        else: raise RuntimeError('Owned mail API not ready')
        def worker(phase):
            args = ['exec','-e','MYSQL_PWD='+password,'-e','BROWSER_IDENTITY_FIXTURE=1','-e','MAIL_JOINT_PHASE='+phase]
            for key in ['MERCHANT_KEY_ACTIVE','MERCHANT_KEYRING_JSON','CUSTOMER_KEY_ACTIVE','CUSTOMER_KEYRING_JSON']:
                args.extend(['-e',key+'='+app_env[key]])
            result = docker(*args,mysql_id,'/opt/prep.test','-test.v','-test.run','^TestOwnedMailBrowserWorker$','-test.timeout','40s')
            (ROOT/'logs/gin-migration'/('mail-joint-'+phase+'.log')).write_bytes(result.stdout+result.stderr)
            if result.returncode or b'OWNED_MAIL_JOINT_PHASE_PASSED' not in result.stdout: raise RuntimeError('Owned mail worker '+phase+' failed; inspect ignored log')
        browser_env = dict(os.environ, ADMIN_TEST_API_TARGET=target, CUSTOMER_BRIDGE_KEY=app_env['CUSTOMER_BRIDGE_KEY'])
        node = browser_env.get('ADMIN_TEST_NODE') or shutil.which('node')
        with log.open('w',encoding='utf-8') as output:
            browser = subprocess.Popen([node,'tests/mail.joint.mjs'],cwd=ROOT/'retail/storefront',env=browser_env,stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=output,text=True,encoding='utf-8')
            lines = queue.Queue()
            def consume():
                for line in browser.stdout:
                    output.write(line); output.flush(); lines.put(line)
                lines.put(None)
            reader = threading.Thread(target=consume,daemon=True); reader.start()
            deadline, report, dispatched = time.monotonic()+300, None, False
            while time.monotonic() < deadline:
                try: line = lines.get(timeout=1)
                except queue.Empty: continue
                if line is None: break
                if line.strip() == 'MAIL_WORKER_REQUIRED':
                    if dispatched: raise RuntimeError('Repeated owned mail dispatch marker')
                    dispatched=True; worker('verification')
                    link = docker('exec',mysql_id,'cat','/tmp/joint-mail-link.json')
                    if link.returncode: raise RuntimeError('Private fixture link unavailable')
                    browser.stdin.write(json.dumps(json.loads(link.stdout))+'\n'); browser.stdin.flush()
                if line.startswith('MAIL_BROWSER_JOINT '): report=json.loads(line.split(' ',1)[1])
            try: code=browser.wait(timeout=10)
            except subprocess.TimeoutExpired: raise RuntimeError('Owned mail browser exceeded deadline') from None
            reader.join(timeout=2)
        if code or not report: raise RuntimeError('Owned mail browser failed; inspect ignored log')
        worker('recovered')
        report.update(durable_sql_verified=True,provider_transport_intercepted=True)
        (ROOT/'docs/migration/mail-joint-report.json').write_text(json.dumps(report,indent=2)+'\n',encoding='utf-8')
        print(json.dumps({'mail_browser_joint':True,'durable_mysql_verified':True,'real_external_send':False}))
    finally:
        if browser is not None and browser.poll() is None:
            browser.terminate()
            try: browser.wait(timeout=10)
            except subprocess.TimeoutExpired: browser.kill(); browser.wait(timeout=10)
        for container in reversed(containers):
            if docker('inspect',container,'--format','{{index .Config.Labels "'+LABEL+'"}}').stdout.decode().strip()!=owner:
                raise RuntimeError('Owned mail cleanup refused: label mismatch')
            if docker('rm','-f',container).returncode: raise RuntimeError('Owned mail cleanup failed')
        if network and docker('network','rm',network).returncode: raise RuntimeError('Owned mail network cleanup failed')
        for binary in binaries: binary.unlink(missing_ok=True)


if __name__ == '__main__': main()
