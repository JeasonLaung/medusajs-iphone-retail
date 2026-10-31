import test from 'node:test'
import assert from 'node:assert/strict'
import {customerBridge} from '../src/lib/customer-bridge.ts'
const shop='http://127.0.0.1:9300',root='/api/store/customer/orders/order_fixture/payments',id='f9adc90d-1b76-46b3-8f1c-36c83c69d627',token='a'.repeat(64),config={ginOrigin:'http://127.0.0.1:9191',storefrontOrigin:shop}
const request=(method:string,path:string)=>new Request(shop+path,{method,headers:{Origin:shop,'Content-Type':'application/json','X-CSRF-Token':token,Cookie:`retail_customer_session=${token}; retail_cart_possession=PRIVATE; retail_admin_session=PRIVATE`},...(method==='POST'?{body:'{}'}:{})})
const response=(status:number,body:unknown={payment:{operation_id:id,state:'pending'}})=>Response.json(body,{status,headers:{'Cache-Control':'private, no-store'}})

test('order payment bridge forwards only current customer capability and fixed command/read paths',async()=>{
 for(const [method,path,status] of [['GET',root,200],['POST',root,202],['GET',root+'/'+id,200],['POST',root+'/'+id+'/cancel',200]] as const){
  const result=await customerBridge(request(method,path),config,async(url,init)=>{
   assert.equal(String(url),config.ginOrigin+path)
   const headers=new Headers(init?.headers)
   assert.equal(headers.get('Cookie'),`retail_customer_session=${token}`)
   assert.equal(headers.get('Host'),'127.0.0.1:9300')
   assert.equal(headers.get('X-CSRF-Token'),method==='POST'?token:null)
   assert.equal(init?.cache,'no-store');assert.equal(init?.redirect,'error')
   return response(status)
  })
  assert.equal(result.status,status);assert.equal(result.headers.get('Cache-Control'),'private, no-store');assert.equal(result.headers.get('Referrer-Policy'),'no-referrer')
 }
})
test('order payment bridge rejects unknown methods, noncanonical IDs, queries and remote actions before transport',async()=>{
 let calls=0
 for(const [method,path] of [['GET',root+'?status=paid'],['GET',root+'/'+id+'?status=paid'],['GET',root+'/'+id+'?'],['POST',root+'/'+id],['POST',root+'/'+id+'/run'],['GET',root+'/'+id+'/cancel'],['POST',root+'/'+id+'/cancel/'],['DELETE',root+'/'+id],['GET',root+'/'+id.toUpperCase()],['GET',root+'/00000000-0000-1000-8000-000000000001'],['POST',root+'/unknown/cancel']] as const){
  assert.equal((await customerBridge(request(method,path),config,async()=>{calls++;return response(200)})).status,404)
 }
 assert.equal(calls,0)
})
test('order payment success status is specific to staging and local receipt/cancel',async()=>{
 for(const [method,path,status] of [['GET',root,202],['POST',root,200],['GET',root+'/'+id,202],['POST',root+'/'+id+'/cancel',202]] as const){
  assert.equal((await customerBridge(request(method,path),config,async()=>response(status))).status,503)
 }
 for(const status of [400,401,403,404,409,503]) {
  const result=await customerBridge(request('POST',root),config,async()=>response(status,{error:{code:'customer.payment_conflict',message:'PRIVATE_SQL'}}))
  assert.equal(result.status,status);assert.ok(!(await result.text()).includes('PRIVATE_SQL'))
 }
})
