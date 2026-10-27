import assert from 'node:assert/strict'
import {spawn} from 'node:child_process'
import {once} from 'node:events'
import {createServer} from 'node:net'

// The Python owner supplies its disposable Gin listener, never an application
// backend, Medusa address or deployment server.
const target=new URL(process.env.STOREFRONT_TEST_GIN??'')
assert.equal(target.protocol,'http:');assert.equal(target.hostname,'127.0.0.1');assert.ok(target.port)
assert.equal(target.pathname,'/');assert.equal(target.username,'');assert.equal(target.password,'')
const regionReply=await fetch(new URL('/api/store/regions',target))
assert.equal(regionReply.status,200)
const publicRegions=await regionReply.json()
assert.equal(publicRegions.count,1);assert.equal(publicRegions.regions[0].countries.length,2)
async function check(origin,expected){
 const probe=createServer();probe.listen(0,'127.0.0.1');await once(probe,'listening')
 const port=probe.address().port;await new Promise((resolve,reject)=>probe.close(error=>error?reject(error):resolve()))
 const env={...process.env,HOST:'127.0.0.1',PORT:String(port),ASTRO_TELEMETRY_DISABLED:'1'}
 delete env.GIN_API_ORIGIN;if(origin)env.GIN_API_ORIGIN=origin
 const child=spawn(process.execPath,['dist/server/entry.mjs'],{env,stdio:['ignore','pipe','pipe'],windowsHide:true})
 let output='';child.stdout.on('data',data=>{output=(output+data).slice(-4000)});child.stderr.on('data',data=>{output=(output+data).slice(-4000)})
 let startedError;child.on('error',error=>{startedError=error})
 try{
  let reply
  for(let i=0;i<80;i++){
   if(startedError||child.exitCode!==null)throw new Error('Owned Astro process failed to start')
   try{reply=await fetch(`http://127.0.0.1:${port}/health`,{signal:AbortSignal.timeout(7000),headers:{Cookie:'retail_admin_session=untrusted; retail_customer_session=untrusted'}});break}catch{}
   await new Promise(resolve=>setTimeout(resolve,100))
  }
  assert.ok(reply,'Owned Astro listener not ready');assert.equal(reply.status,expected)
  assert.deepEqual(await reply.json(),{status:expected===200?'ok':'unavailable',frontend:'astro'})
  assert.equal(reply.headers.get('cache-control'),'no-store');assert.equal(reply.headers.get('set-cookie'),null)
  // Business pages are added only after their actual API and UI are ported.
  assert.equal((await fetch(`http://127.0.0.1:${port}/dk-en/store`)).status,404)
  assert.equal(child.exitCode,null)
 }finally{
  if(child.exitCode===null){const ended=once(child,'exit');child.kill();await ended}
 }
}
await check(target.origin,200)
await check(undefined,503)
console.log('ASTRO_RUNTIME_REHEARSAL '+JSON.stringify({production_node_adapter:true,real_gin_regions:true,missing_configuration_503:true,response_no_store:true,no_session_cookie:true,unported_business_page_404:true,owned_processes_stopped:true}))
