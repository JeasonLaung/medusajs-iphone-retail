import {test} from 'node:test'
import assert from 'node:assert/strict'
import {dhlDemo,loadShippingDemo} from '../src/lib/shipping-demo.ts'
import {PUBLIC_NOT_FOUND} from '../src/lib/gin-public.ts'

const sample={requested_tracking_number:'7777777770',fetched_at:'2026-11-01T00:00:00.000Z',source:'https://developer.dhl.com/tracking',shipment:{id:'official-sample',status:{timestamp:'original time',statusCode:'delivered',description:'Delivered'},events:[{timestamp:'original time',description:'Processed',location:{address:{addressLocality:'City',countryCode:'DK'}}}]}}
test('official envelope retains original event text and timezone',()=>{
 const d=dhlDemo(sample);assert.equal(d.shipment.events[0].timestamp,'original time');assert.equal(d.shipment.status?.description,'Delivered')
 for(const raw of [{...sample,source:'https://other.test'}, {...sample,requested_tracking_number:'real-order'}, {...sample,fetched_at:'bad'}, {...sample,shipment:{id:'x',events:null}}, {...sample,shipment:{id:'x',events:[{timestamp:3}]}}, {...sample,shipment:{id:'x',events:[{timestamp:'t',location:{address:{countryCode:1}}}]}}])assert.throws(()=>dhlDemo(raw))
})
test('server-owned read strips credentials and uses only fixed path',async()=>{
 let calls=0
 const fetcher:typeof fetch=async(input,init)=>{
  calls++;assert.equal(String(input),'http://127.0.0.1:9100/api/store/shipping-demo/dhl');assert.equal(init?.credentials,'omit');assert.equal(init?.cache,'no-store');assert.equal(init?.redirect,'error');assert.deepEqual(init?.headers,{Accept:'application/json'});assert.equal(init?.body,undefined)
  return Response.json(sample)
 }
 assert.equal((await loadShippingDemo('http://127.0.0.1:9100',fetcher) as ReturnType<typeof dhlDemo>).shipment.id,'official-sample')
 for(const origin of [undefined,'file:///tmp','http://user:pass@localhost','http://localhost/path','http://localhost?x=1'])await assert.rejects(loadShippingDemo(origin,fetcher))
 assert.equal(calls,1)
})
test('production absence and upstream failure stay distinct',async()=>{
 assert.equal(await loadShippingDemo('http://localhost',async()=>new Response('',{status:404})),PUBLIC_NOT_FOUND)
 for(const reply of [()=>new Response('private',{status:502}),()=>Response.json({...sample,source:'javascript:alert(1)'}),()=>new Response('{}',{headers:{'Content-Type':'text/plain'}}),()=>new Response('invalid',{headers:{'Content-Type':'application/json'}})])await assert.rejects(loadShippingDemo('http://localhost',async()=>reply()))
})
test('DHL bridge permits queries longer than the ordinary five-second read timeout',async()=>{
 const fetcher:typeof fetch=async(_input,init)=>{
  await new Promise<void>((resolve,reject)=>{
   const timer=setTimeout(resolve,5100)
   init?.signal?.addEventListener('abort',()=>{clearTimeout(timer);reject(new Error('aborted'))},{once:true})
  })
  return Response.json(sample)
 }
 const result=await loadShippingDemo('http://localhost',fetcher)
 assert.notEqual(result,PUBLIC_NOT_FOUND)
})
