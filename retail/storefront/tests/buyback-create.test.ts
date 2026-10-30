import test from 'node:test'
import assert from 'node:assert/strict'
import {customerClient} from '../src/lib/customer-client.ts'
import {customerBridge} from '../src/lib/customer-bridge.ts'
import {customerProof} from '../src/lib/customer-proof.ts'
import type {BuybackDraft} from '../src/lib/buyback-client.ts'
const csrf='b'.repeat(64),id='bbr_00000000-0000-4000-8000-000000000001',origin='http://127.0.0.1:9300'
const input:BuybackDraft={submission_key:'00000000-0000-4000-8000-000000000002',contact_name:'Customer',phone:'+45123456',return_address:{name:'Customer',address_1:'Street 1',address_2:'',city:'City',postal_code:'1000',country_code:'dk'},reference_acknowledged:true,privacy_acknowledged:true,items:[{model:'iphone-13',capacity:'128GB',assessment:{battery:'90',checks:[true,true,true,true,true],faults:['none'],glass:['none'],screen:'none',frame:'none',back:'none'}}]}
test('buyback client requires created status and keeps original submission identity after an uncertain response',async()=>{
 const oldDocument=Object.getOwnPropertyDescriptor(globalThis,'document'),oldLocation=Object.getOwnPropertyDescriptor(globalThis,'location')
 Object.defineProperty(globalThis,'document',{value:{cookie:`retail_customer_csrf=${csrf}`},configurable:true})
 Object.defineProperty(globalThis,'location',{value:{protocol:'http:'},configurable:true})
 const calls:unknown[]=[]
 try{
  let count=0
  const api=customerClient(async(url,init)=>{
   assert.equal(String(url),'/api/store/customer/buyback/requests');assert.equal(init?.method,'POST')
   assert.equal(init?.credentials,'same-origin');assert.equal(init?.redirect,'error');assert.equal(init?.cache,'no-store')
   assert.equal(new Headers(init?.headers).get('X-CSRF-Token'),csrf)
   calls.push(JSON.parse(String(init?.body)))
   if(count++===0)throw Error('response lost')
   return Response.json({request:{id,email:'customer@example.invalid'}},{status:201})
  })
  assert.deepEqual(await api.createBuyback(input),{data:null,error:'BUYBACK_REQUEST_FAILED'})
  assert.deepEqual(await api.createBuyback(input),{data:{request:{id}},error:null})
  assert.deepEqual(calls,[input,input])
  for(const status of [200,202,401,403,409,503]){
   assert.deepEqual(await customerClient(async()=>Response.json({request:{id}},{status})).createBuyback(input),{data:null,error:status===401?'BUYBACK_LOGIN_REQUIRED':'BUYBACK_REQUEST_FAILED'})
  }
  for(const body of [{},{request:{id:'foreign'}},{request:{id},private:'unexpected'}])assert.equal((await customerClient(async()=>Response.json(body,{status:201})).createBuyback(input)).error,'BUYBACK_REQUEST_FAILED')
 }finally{
  if(oldDocument)Object.defineProperty(globalThis,'document',oldDocument);else Reflect.deleteProperty(globalThis,'document')
  if(oldLocation)Object.defineProperty(globalThis,'location',oldLocation);else Reflect.deleteProperty(globalThis,'location')
 }
})
test('buyback bridge preserves signed raw bytes and permits only exact POST creation without cookie changes',async()=>{
 const config={ginOrigin:'http://127.0.0.1:9191',storefrontOrigin:origin,bridgeKey:'a'.repeat(64),clientAddress:'127.0.0.1'},path='/api/store/customer/buyback/requests',raw='{"phone":"first","phone":"second"}'
 const request=(suffix='',method='POST')=>new Request(origin+path+suffix,{method,headers:{Origin:origin,'Content-Type':'application/json','X-CSRF-Token':csrf,Cookie:`retail_customer_session=${csrf}; medusa_token=PRIVATE`},...(method==='POST'?{body:raw}:{})})
 let calls=0
 const transport:typeof fetch=async(url,init)=>{
  calls++;assert.equal(String(url),config.ginOrigin+path);assert.equal(new TextDecoder().decode(init?.body as Uint8Array),raw)
  assert.equal(new Headers(init?.headers).get('Cookie'),`retail_customer_session=${csrf}`)
  assert.equal(new Headers(init?.headers).get('X-CSRF-Token'),csrf)
  const headers=new Headers(init?.headers),time=Number(headers.get('X-Retail-Bridge-Time'))*1000
  const expected=customerProof(config.bridgeKey,config.clientAddress,'POST',path,headers,new TextEncoder().encode(raw),time)
  assert.equal(headers.get('X-Retail-Bridge-Proof'),expected.get('X-Retail-Bridge-Proof'))
  return Response.json({request:{id}},{status:201,headers:{'Cache-Control':'private, no-store'}})
 }
 const result=await customerBridge(request(),config,transport)
 assert.equal(result.status,201);assert.deepEqual(await result.json(),{request:{id}})
 for(const [suffix,method] of [['?unexpected=1','POST'],['','GET'],['/extra','POST']])assert.equal((await customerBridge(request(suffix,method),config,transport)).status,404)
 assert.equal(calls,1)
 for(const status of [200,202])assert.equal((await customerBridge(request(),config,async()=>Response.json({request:{id}},{status,headers:{'Cache-Control':'private, no-store'}}))).status,503)
 assert.equal((await customerBridge(request(),config,async()=>Response.json({request:{id}},{status:201,headers:{'Cache-Control':'private, no-store','Set-Cookie':`retail_customer_session=${csrf}; Path=/; HttpOnly; SameSite=Lax`}}))).status,503)
})
