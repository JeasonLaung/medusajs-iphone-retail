import test from 'node:test'
import assert from 'node:assert/strict'
import {customerBridge} from '../src/lib/customer-bridge.ts'
import {customerProof} from '../src/lib/customer-proof.ts'
const origin='http://127.0.0.1:9300',root='/api/store/customer/buyback/requests/bbr_fixture',token='b'.repeat(64)
const config={ginOrigin:'http://127.0.0.1:9191',storefrontOrigin:origin,bridgeKey:'a'.repeat(64),clientAddress:'127.0.0.1'}
const request=(body:string,path='')=>new Request(origin+root+path,{method:'POST',headers:{Origin:origin,'Content-Type':'application/json','X-CSRF-Token':token,Cookie:`retail_customer_session=${token}; medusa_token=PRIVATE`},body})
const reply=(body:unknown,status=200)=>Response.json(body,{status,headers:{'Cache-Control':'private, no-store'}})
test('buyback command bridge preserves original bytes and signs fixed session/CSRF-bound command path',async()=>{
 const raw='{"action":"cancel","action":"decline_offer"}'
 const result=await customerBridge(request(raw),config,async(url,init)=>{
  assert.equal(String(url),config.ginOrigin+root);assert.equal(init?.method,'POST')
  assert.equal(new TextDecoder().decode(init?.body as Uint8Array),raw)
  const headers=new Headers(init?.headers)
  assert.equal(headers.get('Cookie'),`retail_customer_session=${token}`);assert.equal(headers.get('X-CSRF-Token'),token)
  const proof=customerProof(config.bridgeKey,config.clientAddress,'POST',root,headers,new TextEncoder().encode(raw),Number(headers.get('X-Retail-Bridge-Time'))*1000)
  assert.equal(headers.get('X-Retail-Bridge-Proof'),proof.get('X-Retail-Bridge-Proof'))
  return reply({request:{id:'bbr_fixture',status:'canceled'}})
 })
 assert.equal(result.status,200);assert.equal(result.headers.get('Cache-Control'),'private, no-store')
 for(const status of [201,202])assert.equal((await customerBridge(request('{"action":"cancel"}'),config,async()=>reply({},status))).status,503)
 let calls=0
 const transport:typeof fetch=async()=>{calls++;return reply({})}
 for(const path of ['?','?offset=0','?customer_id=foreign','/command'])assert.equal((await customerBridge(request('{}',path),config,transport)).status,404)
 assert.equal((await customerBridge(request('x'.repeat(16385)),config,transport)).status,413);assert.equal(calls,0)
})
test('buyback command errors expose only original localized business code and matching status',async()=>{
 for(const [status,code] of [[404,'BUYBACK_NOT_FOUND'],[409,'BUYBACK_INVALID_STATE'],[409,'BUYBACK_IDENTITY_REQUIRED'],[409,'BUYBACK_PHOTOS_REQUIRED'],[409,'BUYBACK_OFFER_CHANGED']]){
  const result=await customerBridge(request('{"action":"cancel"}'),config,async()=>reply({error:{code,message:'PRIVATE_DATABASE'}},status as number))
  assert.deepEqual(await result.json(),{error:{code,message:'Buyback request unavailable'}})
 }
 for(const [status,code] of [[400,'BUYBACK_OFFER_CHANGED'],[409,'PRIVATE'],[401,'BUYBACK_INVALID_STATE']]){
  const result=await customerBridge(request('{}'),config,async()=>reply({error:{code,message:'PRIVATE_DATABASE'}},status as number))
  assert.equal(result.status,status);assert.equal((await result.json()).error.code,'customer.request_unavailable')
 }
})
