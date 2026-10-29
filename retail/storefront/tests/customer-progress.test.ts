import test from 'node:test'
import assert from 'node:assert/strict'
import {customerProgressProjection} from '../src/lib/customer-progress.ts'
import {customerClient} from '../src/lib/customer-client.ts'
import {customerBridge} from '../src/lib/customer-bridge.ts'
const fixture=()=>({order_id:'order_created',currency_code:'eur',events:[{id:'order_created:placed',kind:'placed',occurred_at:'2026-10-30T00:00:00Z'},{id:'paid:capture',kind:'paid',occurred_at:'2026-10-30T00:01:00Z',amount:'9007199254740993.000000000000000001'}],shipments:[{id:'parcel',shipped_at:'2026-10-30T00:02:00Z',delivered_at:null,labels:[{tracking_number:'000012345678901234567890',tracking_url:'https://example.invalid/track'}]}]})
test('progress keeps exact money and tracking strings and rejects stale/mixed/private/unsafe records',()=>{
 assert.deepEqual(customerProgressProjection(fixture(),'order_created'),fixture())
 for(const mutate of [(v:any)=>v.order_id='foreign',(v:any)=>v.events.reverse(),(v:any)=>v.events.push(v.events[1]),(v:any)=>v.events[1].amount=158.75,(v:any)=>v.events[1].raw_amount='private',(v:any)=>v.events[1].kind='private_intake',(v:any)=>v.events[1].occurred_at='bad',(v:any)=>v.shipments.push(v.shipments[0]),(v:any)=>v.shipments[0].labels[0].label_url='private',(v:any)=>v.shipments[0].labels[0].tracking_url='javascript:alert(1)',(v:any)=>v.shipments[0].labels[0].tracking_url='https://secret:password@example.invalid/',(v:any)=>v.shipments[0].labels[0].tracking_number=123]){const v=fixture();mutate(v);assert.throws(()=>customerProgressProjection(v,'order_created'))}
})
test('private progress route forwards session only and client validates expected order',async()=>{
 const shop='http://127.0.0.1:9300',root='/api/store/customer/orders/order_created/progress',token='a'.repeat(64)
 const result=await customerBridge(new Request(shop+root,{headers:{Cookie:`retail_customer_session=${token}; retail_cart_possession=${token}; retail_admin_session=private`}}),{ginOrigin:'http://127.0.0.1:9100',storefrontOrigin:shop},async(url,init)=>{assert.equal(String(url),'http://127.0.0.1:9100'+root);assert.equal(new Headers(init?.headers).get('Cookie'),`retail_customer_session=${token}`);return Response.json(fixture(),{headers:{'Cache-Control':'private, no-store'}})})
 assert.equal(result.status,200);assert.equal(result.headers.get('Cache-Control'),'private, no-store')
 for(const suffix of ['?email=private','?','/extra'])assert.equal((await customerBridge(new Request(shop+root+suffix),{ginOrigin:'http://127.0.0.1:9100',storefrontOrigin:shop},async()=>{throw Error('must not call')})).status,404)
 const api=customerClient(async(path,init)=>{assert.equal(path,root);assert.equal(init?.credentials,'same-origin');assert.equal(init?.cache,'no-store');return Response.json(fixture())})
 try{assert.deepEqual(await api.orderProgress('order_created'),fixture());await assert.rejects(api.orderProgress('foreign/path'))}finally{api.clear()}
})
