import test from 'node:test'
import assert from 'node:assert/strict'
import {customerBridge} from '../src/lib/customer-bridge.ts'
import {customerClient} from '../src/lib/customer-client.ts'
import {orderListProjection,sumOrderQuantities} from '../src/lib/customer-orders.ts'
import {confirmationFixture} from './order-confirmation.fixture.ts'
const shop='http://127.0.0.1:9300',root='/api/store/customer',token='a'.repeat(64),csrf='b'.repeat(64),config={ginOrigin:'http://127.0.0.1:9100',storefrontOrigin:shop}
const order={id:'order_created',display_id:'1001',custom_display_id:'2026103000000000001',status:'pending',created_at:'2026-10-30T00:00:00Z',currency_code:'eur',total:'158.75',original_total:'158.75',items:[{id:'order_line',title:'Phone',thumbnail:null,quantity:'1',sku:'SKU-128'}]}
const listing=()=>({orders:[structuredClone(order)],count:1,limit:20,offset:0})
const response=(v:unknown)=>Response.json(v,{headers:{'Cache-Control':'private, no-store'}})
test('order bridge forwards the independent cart cookie only for attach; canonical list query and detail stay private',async()=>{
 for(const [method,path] of [['POST','/orders/order_created/attach'],['GET','/orders?offset=20'],['GET','/orders/order_created']]){
  const r=await customerBridge(new Request(shop+root+path,{method,headers:{Origin:shop,'Content-Type':'application/json','X-CSRF-Token':csrf,Cookie:`retail_customer_session=${token}; retail_customer_csrf=${csrf}; retail_cart_possession=${token}; retail_admin_session=PRIVATE`},body:method==='POST'?'{}':undefined}),{...config,bridgeKey:'71'.repeat(32),clientAddress:'127.0.0.1'},async(url,init)=>{
   assert.equal(String(url),config.ginOrigin+root+path);const h=new Headers(init?.headers);assert.ok(h.get('X-Retail-Bridge-Proof'));assert.equal(h.get('Cookie'),`retail_customer_session=${token}; retail_customer_csrf=${csrf}`+(method==='POST'?`; retail_cart_possession=${token}`:''));return response({order_id:order.id})
  });assert.equal(r.status,200)
 }
 for(const path of ['/orders?email=secret','/orders?offset=01','/orders?offset=100001','/orders?offset=0&offset=0','/orders?offset=0;bad','/orders?','/orders/order_created?email=x'])assert.equal((await customerBridge(new Request(shop+root+path),config,async()=>{throw Error('must not call')})).status,404)
 const req=new Request(shop+root+'/orders/order_created/attach',{method:'POST',headers:{Origin:shop,'Content-Type':'application/json',Cookie:`retail_cart_possession=${token}; retail_cart_possession=${token}`},body:'{}'})
 assert.equal((await customerBridge(req,config,async()=>{throw Error('must not call')})).status,401)
})
test('order projection preserves exact totals, SKU and quantities and rejects private fields and mixed rows',()=>{
 assert.deepEqual(orderListProjection(listing(),0),listing())
 for(const alter of [(v:any)=>v.orders.push(v.orders[0]),(v:any)=>v.orders[0].items.push(v.orders[0].items[0]),(v:any)=>v.orders[0].total=158.75,(v:any)=>v.orders[0].email='private',(v:any)=>v.offset=20,(v:any)=>v.orders[0].items[0].quantity='-1',(v:any)=>v.orders[0].created_at='bad']){const v=listing();alter(v);assert.throws(()=>orderListProjection(v,0))}
 assert.equal(sumOrderQuantities(['100000000000000000001.000000000000000001','2.999999999999999999']),'100000000000000000004')
})
test('customer client reads privately and attaches only the order ID with customer CSRF and an empty body',async()=>{
 Object.defineProperty(globalThis,'document',{value:{cookie:'retail_customer_csrf='+csrf},configurable:true});Object.defineProperty(globalThis,'location',{value:{protocol:'http:'},configurable:true})
 const calls:string[]=[]
 const api=customerClient(async(url,init)=>{calls.push(String(url));assert.equal(init?.credentials,'same-origin');assert.equal(init?.cache,'no-store');if(String(url).endsWith('/attach')){assert.equal(init?.method,'POST');assert.equal(new Headers(init?.headers).get('X-CSRF-Token'),csrf);assert.equal(init?.body,'{}');return response({order_id:order.id})}return response(String(url).includes('?')?listing():confirmationFixture)})
 try{assert.deepEqual(await api.orders(),listing());assert.deepEqual(await api.order(order.id),confirmationFixture);await api.attachOrder(order.id);await assert.rejects(api.orders(-1));await assert.rejects(api.order('foreign/path'));assert.equal(calls.length,3)}finally{api.clear();Reflect.deleteProperty(globalThis,'document');Reflect.deleteProperty(globalThis,'location')}
})

test('order bridge rejects an asynchronous acknowledgement as a completed read',async()=>{
 const result=await customerBridge(new Request(shop+root+'/orders?offset=0'),config,async()=>Response.json(listing(),{status:202,headers:{'Cache-Control':'private, no-store'}}))
 assert.equal(result.status,503)
})
