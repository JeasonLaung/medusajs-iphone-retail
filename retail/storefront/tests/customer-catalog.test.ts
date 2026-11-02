import test from 'node:test'
import assert from 'node:assert/strict'
import {createServer} from 'node:http'
import {once} from 'node:events'
import {customerCatalogFetcher} from '../src/lib/customer-catalog.ts'
import {customerBridge} from '../src/lib/customer-bridge.ts'

const token='a'.repeat(64),shop='http://127.0.0.1:9300'
test('catalog identity is request-local, destination-bound and fails visibly on storage errors',async()=>{
 const calls:{path:string;cookie:string;method:string;body:string}[]=[],server=createServer(async(req,res)=>{
  let body='';for await(const part of req)body+=part
  assert.equal(req.headers.host,'127.0.0.1:9300')
  assert.equal(req.headers.authorization,undefined)
  assert.equal(req.headers['x-forwarded-for'],undefined)
  assert.match(String(req.headers['x-retail-bridge-proof']),/^[0-9a-f]{64}$/)
  calls.push({path:req.url!,cookie:String(req.headers.cookie),method:req.method!,body})
  res.writeHead(req.url?.includes('unavailable')?503:req.url?.includes('expired')?401:200,{'Content-Type':'application/json','Cache-Control':'private, no-store'})
  res.end(JSON.stringify({products:[]}))
 })
 server.listen(0,'127.0.0.1');await once(server,'listening')
 try{
  const gin='http://127.0.0.1:'+ (server.address() as {port:number}).port,config={ginOrigin:gin,storefrontOrigin:shop,bridgeKey:'b'.repeat(64),clientAddress:'127.0.0.1'}
  let publicCalls=0
  const publicFetch:typeof fetch=async(_url,init)=>{publicCalls++;assert.equal(new Headers(init?.headers).get('cookie'),null);return Response.json({anonymous:true})}
  const request=new Request(shop+'/gb-en/store',{headers:{Cookie:`retail_customer_session=${token}; retail_admin_session=private; medusa_token=private`,Authorization:'private','X-Forwarded-For':'private'}})
  const member=customerCatalogFetcher(request,config,publicFetch)
  assert.equal((await member(gin+'/api/store/products?region_id=eu')).status,200)
  assert.deepEqual(calls[0],{path:'/api/store/customer/products?region_id=eu',cookie:`retail_customer_session=${token}`,method:'GET',body:''})
  const search='{"region_id":"eu","queries":[]}'
  assert.equal((await member(gin+'/api/store/search',{method:'POST',body:search})).status,200)
  assert.equal(calls[1].body,search);assert.equal(calls[1].method,'POST')
  assert.equal((await member(gin+'/api/store/products?handle=unavailable')).status,503);assert.equal(publicCalls,0)
  assert.equal((await member(gin+'/api/store/products?handle=expired')).status,200);assert.equal(publicCalls,1)
  await member(gin+'/api/store/regions');await member('https://foreign.invalid/api/store/products');assert.equal(publicCalls,3);assert.equal(calls.length,4)
  const anonymous=customerCatalogFetcher(new Request(shop),config,publicFetch)
  await anonymous(gin+'/api/store/products');assert.equal(publicCalls,4);assert.equal(calls.length,4)
 }finally{server.closeAllConnections();await new Promise<void>(r=>server.close(()=>r()))}
})

test('private catalog rejects identity filters and ambiguous product queries before transport',async()=>{
 const config={ginOrigin:'http://127.0.0.1:9191',storefrontOrigin:shop};let calls=0
 for(const suffix of ['?customer_id=foreign','?customer_groups=vip','?region_id=a&region_id=b','?limit=2&unrecognized=1']){
  const response=await customerBridge(new Request(shop+'/api/store/customer/products'+suffix),config,async()=>{calls++;return Response.json({})})
  assert.equal(response.status,404)
 }
 assert.equal(calls,0)
})
