import test from 'node:test'
import assert from 'node:assert/strict'
import {CartClient,CartError} from '../src/lib/cart-client.ts'
import {cartBridge} from '../src/lib/cart-bridge.ts'
import {confirmationFixture} from './order-confirmation.fixture.ts'
const reply=(value:unknown,status=200)=>Response.json(value,{status,headers:{'Cache-Control':'private, no-store'}})

test('private confirmation binds saved order, addresses, line identities and exact cash',async()=>{
 const api=new CartClient({readScopeCSRF:()=>undefined,fetcher:async(path,init)=>{assert.equal(path,'/api/store/orders/order_created/confirmation');assert.equal(init?.method,'GET');assert.equal(init?.credentials,'same-origin');return reply(confirmationFixture)}})
 assert.deepEqual(await api.confirmation('order_created'),confirmationFixture)
 for(const status of [401,404])assert.equal(await new CartClient({readScopeCSRF:()=>undefined,fetcher:async()=>reply({},status)}).confirmation('order_created'),null)
 await assert.rejects(new CartClient({readScopeCSRF:()=>undefined,fetcher:async()=>reply({},503)}).confirmation('order_created'),e=>e instanceof CartError&&e.kind==='unavailable')
})

test('confirmation rejects mixed orders, duplicate lines, private fields and malformed cash',async()=>{
 for(const alter of [(v:any)=>{v.id='foreign'},(v:any)=>{v.amounts.order_id='foreign'},(v:any)=>{v.items[0].id='other'},(v:any)=>{v.items.push(v.items[0])},(v:any)=>{v.payment.currency_code='usd'},(v:any)=>{v.payment.amount=158.75},(v:any)=>{v.shipping_address.metadata={private:true}},(v:any)=>{v.metadata={private:true}},(v:any)=>{v.amounts.lines[0].unit_price='NaN'},(v:any)=>{v.created_at='invalid'}]){
  const value=structuredClone(confirmationFixture);alter(value)
  await assert.rejects(new CartClient({readScopeCSRF:()=>undefined,fetcher:async()=>reply(value)}).confirmation('order_created'))
 }
})

test('private confirmation bridge stays fixed and excludes non-cart credentials',async()=>{
 const config={ginOrigin:'http://127.0.0.1:9100',storefrontOrigin:'http://127.0.0.1:9300'},path='/api/store/orders/order_created/confirmation',token='a'.repeat(64)
 const response=await cartBridge(new Request(config.storefrontOrigin+path,{headers:{Cookie:'retail_cart_possession='+token+'; customer=secret; admin=secret'}}),config,async(url,init)=>{assert.equal(String(url),config.ginOrigin+path);assert.equal(new Headers(init?.headers).get('Cookie'),'retail_cart_possession='+token);return reply(confirmationFixture)})
 assert.equal(response.status,200);assert.equal(response.headers.get('Referrer-Policy'),'no-referrer')
 assert.equal((await cartBridge(new Request(config.storefrontOrigin+path+'?email=saved@example.invalid'),config,async()=>{throw Error('must not forward')})).status,404)
})

import {orderUnitAmount,orderHasDiscount} from '../src/lib/order-display.ts'
test('source unit display divides tax-inclusive discounted line totals exactly',()=>{
 assert.equal(orderUnitAmount('150','1','eur'),'150.00')
 assert.equal(orderUnitAmount('10','3','eur'),'3.33')
 assert.equal(orderUnitAmount('1.005','1','eur'),'1.01')
 assert.equal(orderUnitAmount('100000000000000000001.005','1','eur'),'100000000000000000001.01')
 assert.equal(orderHasDiscount('148.4375','150'),true)
 assert.equal(orderHasDiscount('150','150.0'),false)
 assert.throws(()=>orderUnitAmount('1','0','eur'))
})
