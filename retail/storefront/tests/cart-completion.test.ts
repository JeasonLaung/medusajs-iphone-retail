import test from 'node:test'
import assert from 'node:assert/strict'
import {CartClient,CartError} from '../src/lib/cart-client.ts'
import {cartBridge} from '../src/lib/cart-bridge.ts'
const csrf='a'.repeat(64)
const reply=(value:unknown,status=200)=>Response.json(value,{status,headers:{'Cache-Control':'private, no-store'}})
const completed={cart_id:'cart',order_id:'order_created',version:'9007199254740993123'}

test('completion uses empty private intention and accepts only the bound receipt',async()=>{
 const client=new CartClient({readScopeCSRF:()=>undefined,fetcher:async(path,init)=>{
  assert.equal(path,'/api/store/carts/cart/complete');assert.equal(init?.body,'{}');assert.equal(init?.method,'POST');assert.equal(init?.credentials,'same-origin');assert.equal(init?.cache,'no-store');assert.equal(init?.redirect,'error')
  const h=new Headers(init?.headers);assert.equal(h.get('X-CSRF-Token'),csrf);assert.equal(h.get('Idempotency-Key'),null)
  return reply(completed)
 }})
 assert.deepEqual(await client.complete('cart',{csrfToken:csrf}),completed)
 for(const body of [{...completed,cart_id:'other'},{...completed,order_id:''},{...completed,version:6},{...completed,version:'18446744073709551616'},{...completed,payment_id:'private'},{...completed,order_url:'https://foreign.example'}]){
  await assert.rejects(new CartClient({readScopeCSRF:()=>undefined,fetcher:async()=>reply(body)}).complete('cart',{csrfToken:csrf}),e=>e instanceof CartError&&e.kind==='protocol'&&e.uncertain)
 }
})

test('lost completion responses remain uncertain; dependency failures never mean unpaid',async()=>{
 for(const fetcher of [async()=>{throw Error('offline')},async()=>reply({},503)])await assert.rejects(new CartClient({readScopeCSRF:()=>undefined,fetcher}).complete('cart',{csrfToken:csrf}),e=>e instanceof CartError&&e.uncertain)
 await assert.rejects(new CartClient({readScopeCSRF:()=>undefined,fetcher:async()=>reply({},409)}).complete('cart',{csrfToken:csrf}),e=>e instanceof CartError&&e.kind==='conflict'&&!e.uncertain)
 const fail=async()=>{throw Error('invalid input must not dispatch')}
 await assert.rejects(new CartClient({readScopeCSRF:()=>undefined,fetcher:fail}).complete('../private',{csrfToken:csrf}),e=>e instanceof CartError&&e.kind==='input')
})

test('completion bridge forwards only cart credentials to its fixed upstream path',async()=>{
 const config={ginOrigin:'http://127.0.0.1:9100',storefrontOrigin:'http://127.0.0.1:9300'}
 const path=config.storefrontOrigin+'/api/store/carts/cart/complete'
 const request=()=>new Request(path,{method:'POST',headers:{Origin:config.storefrontOrigin,'Content-Type':'application/json','X-CSRF-Token':csrf,Cookie:'retail_cart_possession='+csrf+'; admin=secret'},body:'{}'})
 const response=await cartBridge(request(),config,async(url,init)=>{assert.equal(String(url),config.ginOrigin+'/api/store/carts/cart/complete');const h=new Headers(init?.headers);assert.equal(h.get('Cookie'),'retail_cart_possession='+csrf);assert.equal(h.get('X-CSRF-Token'),csrf);assert.equal(h.get('Authorization'),null);assert.equal(init?.body?.toString(),'123,125');return reply(completed)})
 assert.equal(response.status,200);assert.equal(response.headers.get('Cache-Control'),'private, no-store');assert.deepEqual(await response.json(),completed)
 for(const req of [new Request(path+'?paid=true',{method:'POST'}),new Request(path,{method:'GET'})])assert.equal((await cartBridge(req,config,async()=>{throw Error('must not forward')})).status,404)
 assert.equal((await cartBridge(new Request(path,{method:'POST',headers:{Origin:'https://foreign.example'}}),config,async()=>{throw Error('must not forward')})).status,403)
})
