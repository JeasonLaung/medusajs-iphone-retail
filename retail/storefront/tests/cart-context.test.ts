import test from 'node:test'
import assert from 'node:assert/strict'
import {CartClient,CartError} from '../src/lib/cart-client.ts'
import {cartBridge} from '../src/lib/cart-bridge.ts'

const operationID='f8230181-a32c-4127-838b-92b7c033ef01',csrfToken='a'.repeat(64)
const input={operationID,version:'9007199254740993123',regionID:'region_us',locale:'zh-CN'}
const response=(value:unknown)=>new Response(JSON.stringify(value),{status:200,headers:{'Content-Type':'application/json','Cache-Control':'private, no-store'}})
test('context transport projects directory intent and exact versions, and preserves uncertain retry',async()=>{
 const calls:{body:string;operation:string|null}[]=[]
 const client=new CartClient({readScopeCSRF:()=>undefined,fetcher:async(path,init)=>{
  assert.equal(path,'/api/store/carts/cart_new/context')
  const headers=new Headers(init!.headers)
  calls.push({body:String(init!.body),operation:headers.get('Idempotency-Key')})
  assert.equal(headers.get('X-CSRF-Token'),csrfToken)
  assert.equal(init!.credentials,'same-origin');assert.equal(init!.redirect,'error')
  if(calls.length===1)throw new Error('network')
  return response({cart_id:'cart_new',version:'9007199254740993124',replayed:true})
 }})
 const supplied={...input,currency_code:'eur',unit_price:'0',shipping_address_id:'old'}
 await assert.rejects(client.saveContext('cart_new',supplied,{csrfToken}),e=>e instanceof CartError&&e.uncertain)
 assert.deepEqual(await client.saveContext('cart_new',supplied,{csrfToken}),{cart_id:'cart_new',version:'9007199254740993124',replayed:true})
 assert.deepEqual(calls[0],calls[1]);assert.equal(calls[0].operation,operationID)
 assert.deepEqual(JSON.parse(calls[0].body),{version:input.version,region_id:input.regionID,locale:input.locale})
})
test('context malformed acknowledgement remains uncertain; invalid input never sends',async()=>{
 for(const bad of [{regionID:'bad/region'},{locale:'en\n'},{version:'01'},{locale:''}]){
  const client=new CartClient({readScopeCSRF:()=>undefined,fetcher:async()=>{assert.fail('invalid request sent')}})
  await assert.rejects(client.saveContext('cart_new',{...input,...bad},{csrfToken}),e=>e instanceof CartError&&e.kind==='input'&&!e.uncertain)
 }
 for(const ack of [{cart_id:'foreign',version:'9007199254740993124',replayed:false},{cart_id:'cart_new',version:'9007199254740993123',replayed:true},{cart_id:'cart_new',version:'9007199254740993124',replayed:false,secret:'private'}]){
  const client=new CartClient({readScopeCSRF:()=>undefined,fetcher:async()=>response(ack)})
  await assert.rejects(client.saveContext('cart_new',input,{csrfToken}),e=>e instanceof CartError&&e.kind==='protocol'&&e.uncertain)
 }
})
test('private bridge permits context POST only with the original storefront Origin',async()=>{
 const origin='http://127.0.0.1:9300',config={ginOrigin:'http://127.0.0.1:9100',storefrontOrigin:origin}
 let calls=0
 const fetcher:typeof fetch=async(url,init)=>{
  calls++;assert.equal(String(url),'http://127.0.0.1:9100/api/store/carts/cart_new/context')
  const headers=new Headers(init!.headers)
  assert.equal(headers.get('Origin'),origin);assert.equal(headers.get('X-CSRF-Token'),csrfToken)
  assert.equal(headers.get('Cookie'),`retail_cart_possession=${csrfToken}`)
  return response({cart_id:'cart_new',version:'2',replayed:false})
 }
 const request=(method:string,requestOrigin:string)=>new Request(origin+'/api/store/carts/cart_new/context',{method,headers:{Origin:requestOrigin,'Content-Type':'application/json','X-CSRF-Token':csrfToken,'Idempotency-Key':operationID,Cookie:`retail_cart_possession=${csrfToken}; admin_session=do-not-forward`},...(method==='POST'?{body:'{}'}:{})})
 assert.equal((await cartBridge(request('POST',origin),config,fetcher)).status,200)
 assert.equal((await cartBridge(request('POST','https://foreign.example'),config,fetcher)).status,403)
 assert.equal((await cartBridge(request('GET',origin),config,fetcher)).status,404)
 assert.equal(calls,1)
})
