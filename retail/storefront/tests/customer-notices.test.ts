import test from 'node:test'
import assert from 'node:assert/strict'
import {noticePage} from '../src/lib/customer-notices.ts'
import {customerClient} from '../src/lib/customer-client.ts'
import {customerBridge} from '../src/lib/customer-bridge.ts'
const id='notice_00000000-0000-4000-8000-000000000001'
const row={id,kind:'shipment.created',resource_id:'order-1',reference:'REF-1',action:'',read_at:null,created_at:'2026-10-30T00:00:00Z'}
const page={notifications:[row],count:1,unread:1,offset:0,limit:20}
test('notice projection validates pagination, routes and timestamps and omits private fields',()=>{
 assert.deepEqual(noticePage({...page,notifications:[{...row,customer_id:'PRIVATE'}]},0),page)
 for(const bad of [{...page,unread:2},{...page,offset:20},{...page,notifications:[{...row,resource_id:'https://evil.example'}]},{...page,notifications:[{...row,read_at:'bad'}]},{...page,notifications:[row,row]}])assert.throws(()=>noticePage(bad,0))
})
test('notification client and bridge keep authenticated list separate from CSRF writes',async()=>{
 const csrf='a'.repeat(64),shop='http://127.0.0.1:9300'
 const d=Object.getOwnPropertyDescriptor(globalThis,'document'),l=Object.getOwnPropertyDescriptor(globalThis,'location')
 Object.defineProperty(globalThis,'document',{value:{cookie:'retail_customer_csrf='+csrf},configurable:true});Object.defineProperty(globalThis,'location',{value:{protocol:'http:'},configurable:true})
 try{
  const api=customerClient(async(input,init)=>{if(init?.method==='POST'){assert.equal(new Headers(init.headers).get('X-CSRF-Token'),csrf);assert.deepEqual(JSON.parse(String(init.body)),{id});return Response.json({success:true})}assert.equal(String(input),'/api/store/customer/notifications?offset=0');return Response.json(page)})
  assert.deepEqual(await api.notifications(),page);await api.readNotifications(id)
  await assert.rejects(api.notifications(-1));await assert.rejects(api.readNotifications('foreign'))
  let calls=0
  const config={ginOrigin:'http://127.0.0.1:9191',storefrontOrigin:shop}
  const transport:typeof fetch=async()=>{calls++;return Response.json(page,{headers:{'Cache-Control':'private, no-store'}})}
  assert.equal((await customerBridge(new Request(shop+'/api/store/customer/notifications?offset=0'),config,transport)).status,200)
  for(const q of ['?','?offset=-1','?offset=100001','?offset=0&offset=20','?customer_id=other'])assert.equal((await customerBridge(new Request(shop+'/api/store/customer/notifications'+q),config,transport)).status,404)
  assert.equal(calls,1)
 }finally{if(d)Object.defineProperty(globalThis,'document',d);else Reflect.deleteProperty(globalThis,'document');if(l)Object.defineProperty(globalThis,'location',l);else Reflect.deleteProperty(globalThis,'location')}
})
