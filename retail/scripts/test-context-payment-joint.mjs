import assert from 'node:assert/strict'
import {randomBytes,createHash} from 'node:crypto'
import {cp,mkdir,readFile,writeFile,symlink,readdir,stat,statfs,realpath} from 'node:fs/promises'
import {createWriteStream} from 'node:fs'
import {spawn,spawnSync} from 'node:child_process'
import {createServer} from 'node:net'
import path from 'node:path'
import {fileURLToPath} from 'node:url'

const root=fileURLToPath(new URL('../../',import.meta.url)),owner=randomBytes(6).toString('hex')
const requestedBase=path.join(root,'logs/gin-migration/context-payment-'+owner)
await mkdir(requestedBase,{recursive:true})
const base=await realpath(requestedBase)
const label='eurorefurb.context-payment='+owner,name='eurorefurb-context-'+owner
const choices=['unchanged','changed','restore','delete-unknown','create-unknown','paid','replacement-paid','replacement-authorized','same-account','unbound-account','unbound-unchanged','unbound-precision','unbound-delete-unknown','unbound-restore','unbound-create-unknown','unbound-receipt-fault','unbound-lost-restore','continuous','currency-save','currency-restore','retired-edits','group-save','group-restore']
choices.push('retry-canceled','retry-expired','retry-failed')
const args=process.argv.slice(2),matrix=args.includes('--matrix'),requested=[]
const paypalContext=args.includes('--paypal-context'),paypalOnly=args.includes('--paypal-checkout'),cardBrowser=args.includes('--paypal-card'),walletBrowser=args.includes('--paypal-wallet')||cardBrowser
const paidRecovery=args.includes('--paid-order-recovery')
const groupCompletion=args.includes('--group-completion')||paidRecovery,sqlOnly=paypalOnly||groupCompletion&&!paidRecovery
assert(!paidRecovery||args.length===1,'--paid-order-recovery is an independent paid-order browser subflow')
assert(!groupCompletion||args.length===1,'--group-completion is an independent HTTP/SQL subflow')
assert(!paypalContext||args.length===1,'--paypal-context is an independent context browser subflow')
assert(!walletBrowser||args.length===1,'--paypal-wallet is an independent wallet browser subflow')
assert(!paypalOnly||args.length===1,'--paypal-checkout is an independent HTTP/SQL subflow')
for(let i=0;i<args.length;i++){
 if(args[i]==='--matrix')continue
 if(args[i]==='--paypal-context'||args[i]==='--paypal-checkout'||args[i]==='--paypal-wallet'||args[i]==='--paypal-card'||args[i]==='--group-completion'||args[i]==='--paid-order-recovery')continue
 assert.equal(args[i],'--scenario','Use [--scenario name]... [--matrix]')
 assert(choices.includes(args[++i]),'Unknown or missing scenario')
 requested.push(args[i])
}
const selected=requested.length?[...new Set(requested)]:choices
const caseCount=paidRecovery?8:paypalContext?28:walletBrowser?4:selected.length*(matrix?4:1)
const pause=ms=>new Promise(r=>setTimeout(r,ms)),containers=[],processes=[]
let network,failed,volume
function run(cmd,args,opts={}){
 const r=spawnSync(cmd,args,{cwd:root,encoding:'utf8',windowsHide:true,timeout:300000,maxBuffer:32*1024*1024,...opts})
 if(r.error||r.status!==0){const file=path.join(base,'failure-'+Date.now()+'.log');createWriteStream(file).end(String(r.error||'')+'\n'+r.stdout+'\n'+r.stderr);throw Error(cmd+' failed; inspect '+file)}return r.stdout.trim()
}
function launch(cmd,args,cwd,env,log){
 const stream=createWriteStream(path.join(base,log));const p=spawn(cmd,args,{cwd,env,windowsHide:true,stdio:['ignore','pipe','pipe']});p.stdout.pipe(stream);p.stderr.pipe(stream);processes.push(p);return p
}
async function ready(url,proc){for(let i=0;i<180;i++){if(proc&&proc.exitCode!==null)throw Error('Child exited; inspect logs');try{const r=await fetch(url,{signal:AbortSignal.timeout(1000)});if(r.ok)return}catch{}await pause(500)}throw Error('Readiness timed out: '+url)}
const port=await new Promise(resolve=>{const s=createServer();s.listen(0,'127.0.0.1',()=>{const p=s.address().port;s.close(()=>resolve(p))})})
const origin='http://127.0.0.1:'+port,key=randomBytes(32).toString('hex')
const env=Object.fromEntries(['PATH','Path','SystemRoot','SYSTEMROOT','TEMP','TMP','HOME','USERPROFILE','APPDATA','LOCALAPPDATA'].filter(k=>process.env[k]).map(k=>[k,process.env[k]]))
const report={owner,origin,actual_gin_mysql_redis_astro:true,real_provider_called:false}
async function inventory(dir){
 const result={}
 async function walk(folder){for(const entry of await readdir(folder,{withFileTypes:true})){
  if(['node_modules','dist','.astro','.git'].includes(entry.name)||entry.name.startsWith('.env'))continue
  const target=path.join(folder,entry.name)
  if(entry.isDirectory())await walk(target)
  else if(/\.(go|sql|json|ts|tsx|mjs|css|astro)$/.test(entry.name)||['go.mod','go.sum'].includes(entry.name))result[path.relative(dir,target).replaceAll('\\','/')]=createHash('sha256').update(await readFile(target)).digest('hex')
 }}await walk(dir);return result
}
try{
 const disk=await statfs(base);report.build_volume_free_bytes=disk.bavail*disk.bsize
 assert(report.build_volume_free_bytes>=2*1024**3,'2 GiB free required on the isolated build volume')
 console.log('Freeze source and compile owned runtime fixture.')
 const backend=path.join(base,'backend'),storefront=path.join(base,'storefront')
 await cp(path.join(root,'retail/backend'),backend,{recursive:true,filter:p=>!path.basename(p).startsWith('.env')})
 await cp(path.join(root,'retail/storefront'),storefront,{recursive:true,filter:p=>!['node_modules','dist','.astro'].includes(path.basename(p))&&!path.basename(p).startsWith('.env')})
 const backendHashes=await inventory(backend),storefrontHashes=await inventory(storefront)
 assert.deepEqual(await inventory(path.join(root,'retail/backend')),backendHashes)
 assert.deepEqual(await inventory(path.join(root,'retail/storefront')),storefrontHashes)
 const admin=path.join(base,'admin');await mkdir(path.join(admin,'tests'),{recursive:true})
 const browserSource=path.join(root,paidRecovery?'retail/admin/tests/paid-order-recovery.native.mjs':paypalContext?'retail/admin/tests/paypal-context.native.mjs':walletBrowser?(cardBrowser?'retail/admin/tests/paypal-card.native.mjs':'retail/admin/tests/paypal-wallet.native.mjs'):'retail/admin/tests/context-payment.native.mjs')
 await cp(browserSource,path.join(admin,'tests/context-payment.native.mjs'))
 const browserHash=createHash('sha256').update(await readFile(browserSource)).digest('hex')
 await symlink(path.join(root,'retail/admin/node_modules'),path.join(admin,'node_modules'),process.platform==='win32'?'junction':'dir')
 // Keep build caches owned by this frozen checkout. Linking the entire modules
 // directory would let Vite write caches into the running storefront checkout.
 let sourceModules=await realpath(path.join(root,'retail/storefront/node_modules'))
 const ownedModules=path.join(storefront,'node_modules')
 // Astro's generated asset imports require the build and dependency real paths
 // to be on one Windows volume. Evidence folders may be relocated by junction.
 if(process.platform==='win32'&&path.parse(base).root.toLowerCase()!==path.parse(sourceModules).root.toLowerCase()){
  const lock=await readFile(path.join(root,'retail/storefront/package-lock.json'))
  const digest=createHash('sha256').update(lock).digest('hex')
  const cacheRoot=path.join(path.dirname(base),'context-dependencies-'+digest.slice(0,16)),cache=path.join(cacheRoot,'node_modules'),marker=path.join(cacheRoot,'.owned-dependencies-ready')
  let ready=false;try{ready=(await readFile(marker,'utf8'))===digest}catch{}
  if(!ready){
   await cp(sourceModules,cache,{recursive:true,dereference:true,filter:p=>!['.astro','.vite','.cache','.vite-temp'].includes(path.basename(p))})
   await writeFile(marker,digest)
  }
  sourceModules=cache;report.same_volume_dependency_copy=true;report.dependency_lock_sha256=digest
 }
 await mkdir(ownedModules)
 for(const entry of await readdir(sourceModules,{withFileTypes:true})){
  if(['.astro','.vite','.cache'].includes(entry.name))continue
  const source=path.join(sourceModules,entry.name),target=path.join(ownedModules,entry.name)
  if((await stat(source)).isDirectory())await symlink(source,target,process.platform==='win32'?'junction':'dir')
  else await cp(source,target)
 }
 const binary=path.join(base,'identity-linux.test')
 const goTmp=path.join(base,'go-tmp');await mkdir(goTmp)
 run('go',['-C',backend,'test','-trimpath','-c','-o',binary,'./internal/identity'],{env:{...process.env,GOOS:'linux',GOARCH:'amd64',CGO_ENABLED:'0',GOTMPDIR:goTmp}})
 if(!sqlOnly)run(process.execPath,['node_modules/astro/bin/astro.mjs','build'],{cwd:storefront,env:{...env,ASTRO_TELEMETRY_DISABLED:'1',PUBLIC_CUSTOMER_CART_TRANSFER_ENABLED:'true'}})
 report.binary_sha256=createHash('sha256').update(await readFile(binary)).digest('hex')
 // Docker Desktop does not publish host ports on an --internal network.
 // Use an owned bridge and publish only the Gin fixture to host loopback.
 // MySQL uses sockets only; Redis binds container loopback, no provider adapter
 // has network I/O or real credentials.
 network=run('docker',['network','create','--label',label,name])
 const mysql=run('docker',['run','-d','--name',name+'-mysql','--label',label,'--network',network,'-p','127.0.0.1::9100','-e','MYSQL_ALLOW_EMPTY_PASSWORD=yes','mysql:9.6','--innodb-native-foreign-keys=ON','--skip-networking','--mysqlx=OFF','--event-scheduler=OFF','--skip-log-bin','--innodb-buffer-pool-size=128M'])
 containers.push(mysql)
 const inspect=JSON.parse(run('docker',['inspect',mysql]))[0];volume=inspect.Mounts[0].Name
 assert.match(volume,/^[a-f0-9]{64}$/)
 const api='http://127.0.0.1:'+inspect.NetworkSettings.Ports['9100/tcp'][0].HostPort
 const redis=run('docker',['run','-d','--label',label,'--network','container:'+mysql,'redis:7-alpine','redis-server','--bind','127.0.0.1','--save','','--appendonly','no'])
 containers.push(redis)
 let sqlReady=false
 for(let i=0;i<120;i++){
  const logs=run('docker',['logs',mysql])
  const r=spawnSync('docker',['exec',mysql,'mysql','-uroot','-Nse','SELECT @@innodb_native_foreign_keys,@@foreign_key_checks'],{windowsHide:true,encoding:'utf8'})
  if(logs.includes('MySQL init process done.')&&r.status===0&&r.stdout.trim()==='1\t1'){sqlReady=true;break}await pause(1000)
 }
 assert(sqlReady,'Owned MySQL native FK unavailable')
 run('docker',['cp',binary,mysql+':/tmp/identity.test']);run('docker',['exec',mysql,'chmod','700','/tmp/identity.test'])
 run('docker',['exec',mysql,'mkdir','-p','/work/retail/backend/internal/identity','/work/docs/migration'])
 run('docker',['cp',path.join(root,'docs/migration/fixtures'),mysql+':/work/docs/migration/fixtures'])
 const fixture=launch('docker',['exec','-w','/work/retail/backend/internal/identity','-e','GIN_MODE=release','-e','CONTEXT_PAYMENT_BROWSER=1',...(groupCompletion?['-e','CUSTOMER_GROUP_COMPLETION_REHEARSAL=1']:[]),...(paidRecovery?['-e','PAID_ORDER_RECOVERY_BROWSER=1']:[]),...(paypalOnly?['-e','PAYPAL_CHECKOUT_REHEARSAL=1']:[]),...(walletBrowser?['-e','PAYPAL_WALLET_BROWSER=1']:[]),...(cardBrowser?['-e','PAYPAL_CARD_BROWSER=1']:[]),...(paypalContext?['-e','PAYPAL_CONTEXT_BROWSER=1']:[]),'-e','CONTEXT_PAYMENT_CASE_COUNT='+caseCount,'-e','CONTEXT_PAYMENT_ORIGIN='+origin,'-e','CONTEXT_PAYMENT_KEY='+key,mysql,'/tmp/identity.test','-test.run','^TestOwnedContextPaymentBrowser$','-test.v','-test.timeout','12m'],root,env,'fixture.log')
 if(sqlOnly){
  assert.equal(await new Promise((resolve,reject)=>{fixture.once('exit',resolve);fixture.once('error',reject)}),0,'PayPal HTTP/SQL subflow failed; inspect fixture.log')
  const fixtureLog=await readFile(path.join(base,'fixture.log'),'utf8')
  if(groupCompletion){assert(fixtureLog.includes('CUSTOMER_GROUP_COMPLETION_SQL'));assert(fixtureLog.includes('CART_PROMOTION_COMPLETION_REHEARSAL'))}
  else{assert(fixtureLog.includes('PAYPAL_CHECKOUT_SQL:'));assert(fixtureLog.includes('PAYPAL_COMPLETION_SQL:'))}
  assert.deepEqual(await inventory(path.join(root,'retail/backend')),backendHashes)
  Object.assign(report,{passed:true,subflow:groupCompletion?'customer-group-completion':'paypal-checkout',actual_gin_mysql_redis_astro:false,actual_gin_http_mysql_redis:true,astro_executed:false,frozen_source_verified:true,source_sha256:{backend:backendHashes}})
 }else{
 // Private fixture route readiness uses an authenticated harmless facts read.
 for(let i=0;i<180;i++){if(fixture.exitCode!==null)throw Error('Fixture exited before browser; inspect fixture.log');try{const r=await fetch(api+'/__fixture/facts',{headers:{'X-Fixture-Key':key},signal:AbortSignal.timeout(1000)});if(r.status===503)break}catch{}await pause(500);if(i===179)throw Error('Fixture HTTP not ready')}
 const astro=launch(process.execPath,['scripts/start.mjs'],storefront,{...env,HOST:'127.0.0.1',PORT:String(port),GIN_API_ORIGIN:api,STOREFRONT_ORIGIN:origin,CUSTOMER_BRIDGE_KEY:key,ASTRO_TELEMETRY_DISABLED:'1'},'astro.log')
 await ready(origin+(paidRecovery?'/dk-en/payment-result':'/dk-en/about'),astro)
 console.log('Run actual browser save/restore cases against owned services.')
 const browser=launch(process.execPath,['tests/context-payment.native.mjs'],admin,{...env,...(process.env.CONTEXT_BROWSER_EXECUTABLE?{CONTEXT_BROWSER_EXECUTABLE:process.env.CONTEXT_BROWSER_EXECUTABLE}:{}),CONTEXT_ASTRO_ORIGIN:origin,CONTEXT_API_ORIGIN:api,CONTEXT_FIXTURE_KEY:key,CONTEXT_BROWSER_OUTPUT:base,CONTEXT_SCENARIOS:JSON.stringify(selected),CONTEXT_MATRIX:matrix?'1':'0'},'browser.log')
 const code=await new Promise((resolve,reject)=>{browser.once('exit',resolve);browser.once('error',reject)})
 assert.equal(code,0,'Browser failed; inspect browser.log')
 const result=JSON.parse(await readFile(path.join(base,'browser-result.json')));assert.equal(result.cases.length,caseCount);assert.deepEqual(result.errors,[])
 const ended=new Promise(resolve=>fixture.once('exit',resolve))
 assert.equal((await fetch(api+'/__fixture/finish',{method:'POST',headers:{'X-Fixture-Key':key}})).status,200)
 assert.equal(await ended,0,'SQL fixture assertions failed; inspect fixture.log')
 assert((await readFile(path.join(base,'fixture.log'),'utf8')).includes(paidRecovery?'PAID_ORDER_RECOVERY_SQL':'CONTEXT_PAYMENT_BROWSER_SQL'))
 assert.deepEqual(await inventory(backend),backendHashes);assert.deepEqual(await inventory(storefront),storefrontHashes)
 assert.deepEqual(await inventory(path.join(root,'retail/backend')),backendHashes);assert.deepEqual(await inventory(path.join(root,'retail/storefront')),storefrontHashes)
 assert.equal(createHash('sha256').update(await readFile(browserSource)).digest('hex'),browserHash)
 Object.assign(report,{passed:true,browser:result,frozen_source_verified:true,source_sha256:{backend:backendHashes,storefront:storefrontHashes,browser:browserHash}})
 }
}catch(e){failed=e;report.passed=false;console.error(e.message)}finally{
 for(const p of processes.reverse())if(p.exitCode===null){p.kill();await Promise.race([new Promise(r=>p.once('exit',r)),pause(5000)])}
 for(const id of containers.reverse()){
  const i=JSON.parse(run('docker',['inspect',id]))[0];assert.equal(i.Config.Labels['eurorefurb.context-payment'],owner);run('docker',['rm','-f','-v',id])
 }
 if(network){const i=JSON.parse(run('docker',['network','inspect',network]))[0];assert.equal(i.Labels['eurorefurb.context-payment'],owner);run('docker',['network','rm',network])}
 assert.equal(run('docker',['ps','-aq','--filter','label='+label]),'')
 if(volume)assert(!run('docker',['volume','ls','-q']).split('\n').includes(volume))
 report.owned_resources_removed=true;report.finished_utc=new Date().toISOString()
 await writeFile(path.join(base,'report.json'),JSON.stringify(report,null,2)+'\n');console.log('Report: '+base)
}
if(failed)process.exitCode=1
