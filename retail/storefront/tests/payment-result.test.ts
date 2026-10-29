import test from 'node:test'
import assert from 'node:assert/strict'
import {CartClient,CartError} from '../src/lib/cart-client.ts'
import {cartBridge} from '../src/lib/cart-bridge.ts'
const csrf='a'.repeat(64)
const reply=(body:unknown,status=200)=>Response.json(body,{status,headers:{'Cache-Control':'private, no-store'}})
test('payment status sends empty intention and accepts exact bound server facts only',async()=>{
 for(const state of ['pending','authorized','paid_pending_order','failed','requires_review']){
  const client=new CartClient({readScopeCSRF:()=>undefined,fetcher:async(path,init)=>{
   assert.equal(path,'/api/store/carts/cart/payment-status');assert.equal(init?.method,'POST');assert.equal(init?.body,'{}');assert.equal(init?.cache,'no-store');assert.equal(init?.credentials,'same-origin');assert.equal(new Headers(init?.headers).get('X-CSRF-Token'),csrf)
   return reply({cart_id:'cart',version:'9007199254740993123',state})
  }})
  assert.equal((await client.paymentStatus('cart',{csrfToken:csrf})).state,state)
 }
 for(const body of [{cart_id:'foreign',version:'2',state:'pending'},{cart_id:'cart',version:'2',state:'confirmed'},{cart_id:'cart',version:'2',state:'paid_pending_order',payment_id:'tr_private'},{cart_id:'cart',version:2,state:'pending'}])await assert.rejects(new CartClient({readScopeCSRF:()=>undefined,fetcher:async()=>reply(body)}).paymentStatus('cart',{csrfToken:csrf}))
 await assert.rejects(new CartClient({readScopeCSRF:()=>undefined,fetcher:async()=>reply({},503)}).paymentStatus('cart',{csrfToken:csrf}),e=>e instanceof CartError&&e.uncertain)
})
test('payment-status bridge stays fixed, private and CSRF-bound',async()=>{
 const config={ginOrigin:'http://127.0.0.1:9100',storefrontOrigin:'http://127.0.0.1:9300'}
 const req=new Request(config.storefrontOrigin+'/api/store/carts/cart/payment-status',{method:'POST',headers:{Origin:config.storefrontOrigin,'Content-Type':'application/json','X-CSRF-Token':csrf,Cookie:'retail_cart_possession='+csrf+'; admin=secret'},body:'{}'})
 const response=await cartBridge(req,config,async(path,init)=>{assert.equal(new URL(String(path)).pathname,'/api/store/carts/cart/payment-status');assert.equal(new Headers(init?.headers).get('Cookie'),'retail_cart_possession='+csrf);return reply({cart_id:'cart',version:'2',state:'pending'})});assert.equal(response.status,200)
 assert.equal((await cartBridge(new Request(config.storefrontOrigin+'/api/store/carts/cart/payment-status?paid=true',{method:'POST'}),config,async()=>{throw Error('must not dispatch')})).status,404)
})
