import assert from 'node:assert/strict'
import {createServer} from 'node:http'
import {createHmac,createHash} from 'node:crypto'
import {spawn} from 'node:child_process'
import {once} from 'node:events'
import {setTimeout as delay} from 'node:timers/promises'

const key='71'.repeat(32),host='127.0.0.1',port=19370,shop=`http://${host}:${port}`
let calls=0,app
const upstream=createServer(async(req,res)=>{
 if(req.url==='/api/store/regions'){res.writeHead(503,{'Content-Type':'application/json'});res.end('{}');return}
 try{
  calls++
  let body='';for await(const chunk of req)body+=chunk
  assert.equal(req.headers.host,`${host}:${port}`)
  assert.equal(req.headers['x-forwarded-for'],undefined)
  assert.equal(req.headers.authorization,undefined)
  assert.ok(['127.0.0.1','::ffff:127.0.0.1'].includes(req.headers['x-retail-client-ip']))
  const fields=['retail-customer-bridge-v1',req.headers['x-retail-bridge-time'],req.headers['x-retail-client-ip'],req.method,req.url,req.headers.host,req.headers.origin??'',req.headers.cookie??'',req.headers['x-csrf-token']??'',createHash('sha256').update(body).digest('hex')]
  assert.equal(req.headers['x-retail-bridge-proof'],createHmac('sha256',Buffer.from(key,'hex')).update(fields.join('\n')).digest('hex'))
  assert.equal(req.headers.cookie,`retail_customer_session=${'a'.repeat(64)}`)
  res.writeHead(200,{'Content-Type':'application/json','Cache-Control':'private, no-store'});res.end('{"customer":{"id":"cust_fixture"}}')
 }catch{res.writeHead(500);res.end('Fixture assertion failed')}
})
try{
 const probe=createServer();probe.listen(port,host);await once(probe,'listening');await new Promise(resolve=>probe.close(resolve))
 upstream.listen(0,host);await once(upstream,'listening');const address=upstream.address();assert.ok(address&&typeof address!=='string')
 app=spawn(process.execPath,['scripts/start.mjs'],{cwd:process.cwd(),env:{...process.env,HOST:host,PORT:String(port),GIN_API_ORIGIN:`http://${host}:${address.port}`,STOREFRONT_ORIGIN:shop,CUSTOMER_BRIDGE_KEY:key},stdio:'ignore'})
 let ready=false
 for(let n=0;n<100;n++){
  if(app.exitCode!==null)throw new Error('Owned Astro runtime exited')
  try{const r=await fetch(shop+'/health');if([200,503].includes(r.status)){ready=true;break}}catch{}
  await delay(100)
 }
 assert.ok(ready)
 const headers={Origin:shop,Cookie:`retail_customer_session=${'a'.repeat(64)}; retail_admin_session=PRIVATE`,'X-Retail-Client-IP':'evil','X-Retail-Bridge-Proof':'evil',Authorization:'PRIVATE'}
 const result=await fetch(shop+'/api/store/customer/me',{headers})
 assert.equal(result.status,200);assert.deepEqual(await result.json(),{customer:{id:'cust_fixture'}})
 assert.equal(result.headers.get('cache-control'),'private, no-store')
 assert.equal((await fetch(shop+'/api/store/customer/me',{headers:{...headers,'X-Forwarded-For':'192.0.2.100','X-Forwarded-Host':`127.0.0.1:${port}`}})).status,403)
 assert.equal((await fetch(shop+'/api/store/customer/me',{headers:{...headers,Origin:'https://evil.example'}})).status,403)
 assert.equal((await fetch(shop+'/api/store/customer/me?token=PRIVATE',{headers})).status,404)
 assert.equal(calls,1)
 console.log('ASTRO_CUSTOMER_BRIDGE_RUNTIME '+JSON.stringify({built_formal_route:true,real_native_transport:true,hmac_verified:true,adapter_client_ip:true,forwarded_ip_rejected:true,foreign_origin_rejected:true,query_rejected:true,private_credentials_filtered:true,gin_mysql_integrated:false,customer_pages_integrated:false}))
}finally{
 if(app&&app.exitCode===null){app.kill();await once(app,'exit')}
 await new Promise(resolve=>upstream.close(resolve))
}
