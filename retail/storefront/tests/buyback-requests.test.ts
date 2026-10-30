import test from 'node:test'
import assert from 'node:assert/strict'
import {buybackRequestProjection,buybackListProjection} from '../src/lib/buyback-requests.ts'
import {customerClient} from '../src/lib/customer-client.ts'
import {requestFixture} from './buyback-fixture-data.ts'
test('buyback request projects original nested snapshots, metadata-free photos, and current history',()=>{
 const request=requestFixture()
 assert.deepEqual(buybackRequestProjection(request),request)
 for(const edit of [(v:any)=>v.customer_id='foreign',(v:any)=>v.deleted_at='2026-10-30T00:00:00Z',(v:any)=>v.items.push(v.items[0]),(v:any)=>v.final_amount=-1,(v:any)=>v.history[0].actor='admin',(v:any)=>v.photos=[{id:'bbp_fixture',request_id:v.id,item_id:'foreign',photo_type:'front',uploaded_at:v.created_at,review_status:'pending',review_note:'',mime_type:'image/jpeg'}]]){
  const v=structuredClone(request);edit(v);assert.throws(()=>buybackRequestProjection(v))
 }
 const body={requests:[request],count:1,offset:0,limit:20,summary:{recycled:2,processing:1,received:[{currency_code:'EUR',amount:125}]}}
 assert.deepEqual(buybackListProjection(body,0),body)
 for(const v of [{...body,offset:20},{...body,count:21},{...body,requests:[request,request]},{...body,summary:{...body.summary,processing:-1}},{...body,extra:true}])assert.throws(()=>buybackListProjection(v,0))
})
test('buyback list/detail/commands use owned paths, strict replies and CSRF without automatic retries',async()=>{
 const oldDocument=Object.getOwnPropertyDescriptor(globalThis,'document'),oldLocation=Object.getOwnPropertyDescriptor(globalThis,'location'),token='b'.repeat(64),request=requestFixture(),root='/api/store/customer/buyback/requests'
 Object.defineProperty(globalThis,'document',{value:{cookie:`retail_customer_csrf=${token}`},configurable:true});Object.defineProperty(globalThis,'location',{value:{protocol:'http:'},configurable:true})
 try{
  const calls:string[]=[]
  const api=customerClient(async(url,init)=>{
   calls.push(String(url));assert.equal(init?.credentials,'same-origin');assert.equal(init?.cache,'no-store');assert.equal(init?.redirect,'error')
   if(String(url).includes('?'))return Response.json({requests:[request],count:1,offset:0,limit:20,summary:{recycled:0,processing:1,received:[]}})
   if(init?.method==='POST'){assert.equal(new Headers(init.headers).get('X-CSRF-Token'),token);assert.deepEqual(JSON.parse(String(init.body)),{action:'cancel'})}
   return Response.json({request})
  })
  assert.ok((await api.buybackRequests()).data);assert.deepEqual((await api.buybackRequest(request.id)).data,{request});assert.deepEqual((await api.updateBuybackRequest(request.id,{action:'cancel'})).data,{request})
  assert.deepEqual(calls,[root+'?offset=0',root+'/'+request.id,root+'/'+request.id])
  assert.equal((await api.buybackRequests(-1)).error,'BUYBACK_REQUEST_FAILED');assert.equal((await api.buybackRequest('../foreign')).error,'BUYBACK_REQUEST_FAILED');assert.equal(calls.length,3)
  for(const status of [201,202])assert.equal((await customerClient(async()=>Response.json({request},{status})).buybackRequest(request.id)).error,'BUYBACK_REQUEST_FAILED')
  const foreign={...request,id:'bbr_foreign'};assert.equal((await customerClient(async()=>Response.json({request:foreign})).buybackRequest(request.id)).error,'BUYBACK_REQUEST_FAILED')
  assert.equal((await customerClient(async()=>Response.json({error:{code:'BUYBACK_OFFER_CHANGED'}},{status:409})).updateBuybackRequest(request.id,{action:'accept_offer',final_amount:125})).error,'BUYBACK_OFFER_CHANGED')
  let retries=0;const lost=customerClient(async()=>{retries++;throw Error('lost')});assert.equal((await lost.updateBuybackRequest(request.id,{action:'cancel'})).error,'BUYBACK_REQUEST_FAILED');assert.equal(retries,1)
 }finally{if(oldDocument)Object.defineProperty(globalThis,'document',oldDocument);else Reflect.deleteProperty(globalThis,'document');if(oldLocation)Object.defineProperty(globalThis,'location',oldLocation);else Reflect.deleteProperty(globalThis,'location')}
})
