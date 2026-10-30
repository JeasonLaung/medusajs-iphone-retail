import test from 'node:test'
import assert from 'node:assert/strict'
import {buybackClient,estimateProjection} from '../src/lib/buyback-client.ts'
import {buybackBridge} from '../src/lib/buyback-bridge.ts'
const origin='http://127.0.0.1:9100',shop='http://127.0.0.1:19383',path='/api/store/buyback/estimate'
const assessment={battery:'79',checks:[true,true,true,true,true],faults:['bright'],glass:['cracked'],screen:'',frame:'none',back:'none'}
const result={amount:152.02,status:'estimated',visual:'LIKE_NEW',faults:['BROKEN_SCREEN','BATTERY_ISSUE'],provisional:false,currency_code:'eur'}
test('buyback client keeps main currency amount and nullable missing estimate',async()=>{
 const api=buybackClient(async(url,init)=>{assert.equal(url,path);assert.equal(init?.credentials,'omit');assert.equal(init?.cache,'no-store');assert.deepEqual(JSON.parse(String(init?.body)),{product_id:'phone',capacity:'256GB',assessment});return Response.json(result)})
 assert.deepEqual(await api.estimate('phone','256GB',assessment),{estimate:result,error:null})
 const absent={amount:null,status:'unconfigured',visual:null,faults:[],provisional:false,currency_code:'eur'}
 assert.deepEqual(estimateProjection(absent),absent)
 for(const raw of [{...result,amount:'152.02'},{...result,amount:-1},{...result,amount:Infinity},{...result,status:'unknown'},{...result,visual:null},{...result,private:'secret'},{...result,faults:['BROKEN','BROKEN']},{...absent,amount:0},{...absent,provisional:true}])assert.throws(()=>estimateProjection(raw))
 const failed=await buybackClient(async()=>new Response('private',{status:503})).estimate('phone','256GB',assessment)
 assert.deepEqual(failed,{estimate:null,error:'BUYBACK_REQUEST_FAILED'})
})
test('buyback bridge preserves bytes and strips incoming and upstream credentials',async()=>{
 const body='{"product_id":"one","product_id":"two"}',request=new Request(shop+path,{method:'POST',headers:{'Content-Type':'application/json',Cookie:'PRIVATE',Authorization:'PRIVATE'},body})
 let calls=0
 const response=await buybackBridge(request,origin,async(url,init)=>{
  calls++;assert.equal(String(url),origin+path);assert.equal(new TextDecoder().decode(init?.body as Uint8Array),body)
  assert.equal(new Headers(init?.headers).get('Cookie'),null);assert.equal(new Headers(init?.headers).get('Authorization'),null);assert.equal(init?.redirect,'error');assert.equal(init?.credentials,'omit')
  return new Response('private upstream validation',{status:400,headers:{'Set-Cookie':'PRIVATE'}})
 })
 assert.equal(response.status,400);assert.equal(calls,1);assert.equal(response.headers.get('set-cookie'),null);assert.equal(response.headers.get('cache-control'),'no-store');assert.equal((await response.text()).includes('private upstream'),false)
 const valid=await buybackBridge(new Request(shop+path,{method:'POST',headers:{'Content-Type':'application/json'},body:'{}'}),origin,async()=>Response.json(result))
 assert.deepEqual(await valid.json(),result)
})
test('buyback bridge rejects target/query/response drift and projects prices',async()=>{
 let calls=0;const noCall:typeof fetch=async()=>{calls++;throw new Error()}
 for(const uri of ['/api/store/buyback/prices?store=foreign','/api/store/buyback/prices?product_id=a&product_id=b'])assert.equal((await buybackBridge(new Request(shop+uri),origin,noCall)).status,400)
 assert.equal((await buybackBridge(new Request(shop+'/api/store/buyback/prices'),'http://upstream.invalid/path',noCall)).status,503);assert.equal(calls,0)
 const row={product_id:'phone',capacity:'256GB',amount:294,currency_code:'eur'}
 const priceRequest=()=>new Request(shop+'/api/store/buyback/prices?product_id=phone')
 const good=await buybackBridge(priceRequest(),origin,async()=>Response.json({prices:[row]}));assert.deepEqual(await good.json(),{prices:[row]})
 for(const payload of [{prices:[{...row,product_id:'other'}]},{prices:[{...row,raw_amount:'PRIVATE'}]},{prices:[{...row,amount:null}]}])assert.equal((await buybackBridge(priceRequest(),origin,async()=>Response.json(payload))).status,503)
 const oversized=new Request(shop+path,{method:'POST',headers:{'Content-Type':'application/json'},body:'x'.repeat(16385)})
 assert.equal((await buybackBridge(oversized,origin,noCall)).status,400);assert.equal(calls,0)
})
