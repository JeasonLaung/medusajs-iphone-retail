import test from 'node:test'
import assert from 'node:assert/strict'
import {customerBridge} from '../src/lib/customer-bridge.ts'
const origin='http://127.0.0.1:9300',root='/api/store/customer/buyback/requests',token='b'.repeat(64)
const config={ginOrigin:'http://127.0.0.1:9191',storefrontOrigin:origin,bridgeKey:'a'.repeat(64),clientAddress:'127.0.0.1'}
const request=(path:string)=>new Request(origin+root+path,{headers:{Cookie:`retail_customer_session=${token}; medusa_token=PRIVATE`}})
const response=(body:unknown,status=200)=>Response.json(body,{status,headers:{'Cache-Control':'private, no-store'}})
test('buyback reads preserve canonical list offset and detail ID with private signed transport',async()=>{
 let calls=0
 for(const path of ['', '?offset=20','?offset=100000','/bbr_00000000-0000-4000-8000-000000000001']){
  const result=await customerBridge(request(path),config,async(url,init)=>{
   calls++;assert.equal(String(url),config.ginOrigin+root+path)
   const headers=new Headers(init?.headers)
   assert.equal(headers.get('Cookie'),`retail_customer_session=${token}`)
   assert.match(headers.get('X-Retail-Bridge-Proof')??'',/^[a-f0-9]{64}$/)
   assert.equal(init?.cache,'no-store');assert.equal(init?.credentials,'omit');assert.equal(init?.method,'GET')
   return response(path.startsWith('/')?{request:{id:'fixture'}}:{requests:[],count:0,summary:{recycled:0,processing:0,received:[]},offset:20,limit:20})
  })
  assert.equal(result.status,200);assert.equal(result.headers.get('Cache-Control'),'private, no-store')
 }
 const transport:typeof fetch=async()=>{calls++;return response({})}
 for(const path of ['?','?offset=01','?offset=100001','?offset=0&offset=20','?customer_id=foreign','/id?offset=0','/id/photos','/id/extra','/%2fprivate'])assert.equal((await customerBridge(request(path),config,transport)).status,404)
 assert.equal(calls,4)
 for(const status of [201,202])assert.equal((await customerBridge(request('/id'),config,async()=>response({},status))).status,503)
 for(const status of [401,403,404,503]){
  const result=await customerBridge(request('/id'),config,async()=>response({private:'PRIVATE'},status))
  assert.equal(result.status,status);assert.ok(!(await result.text()).includes('PRIVATE'))
 }
})
