import test from 'node:test'
import assert from 'node:assert/strict'
import {orderPaymentReturn} from '../src/lib/order-payment-return.ts'
import {confirmationFixture} from './order-confirmation.fixture.ts'
const shop='http://127.0.0.1:9300',id='f9adc90d-1b76-46b3-8f1c-36c83c69d627',order='order_created',config={ginOrigin:'http://127.0.0.1:9191',storefrontOrigin:shop,bridgeKey:'71'.repeat(32),clientAddress:'127.0.0.1'}
const path='/api/mollie-return?cart_id=retired_cart&order_id='+order+'&order_payment_id='+id
const request=(path2=path)=>new Request(shop+path2,{headers:{Cookie:`retail_customer_session=${'a'.repeat(64)}; retail_cart_possession=PRIVATE; retail_admin_session=PRIVATE`,Origin:'https://www.mollie.com','Sec-Fetch-Site':'cross-site'}})
const view={operation_id:id,order_id:order,order_version:3,collection_id:'paycol_extra',amount:'6.00',currency_code:'eur',state:'uncertain',applied:false,can_cancel:false,can_retry:false,checkout_url:null}
const response=(body:unknown,status=200)=>Response.json(body,{status,headers:{'Cache-Control':'private, no-store'}})
test('supplemental return uses current customer reads and original order locale, without cart or payment writes',async()=>{
 for(const locale of ['en','zh-CN']){
  const calls:string[]=[]
  const result=await orderPaymentReturn(request(),config,async(url,init)=>{
   calls.push(String(url));assert.equal(init?.method,'GET');assert.equal(new Headers(init?.headers).get('Cookie'),`retail_customer_session=${'a'.repeat(64)}`);assert.equal(new Headers(init?.headers).get('Origin'),null)
   if(String(url).endsWith('/payments/'+id))return response({payment:view})
   assert.equal(String(url),config.ginOrigin+'/api/store/customer/orders/'+order)
   return response({...confirmationFixture,locale})
  })
  assert.equal(result.status,303);assert.equal(result.headers.get('Location'),`/dk-${locale.startsWith('zh')?'zh':'en'}/account/orders/details/${order}#order-supplemental-payment`)
  assert.equal(calls.length,2);assert.equal(result.headers.get('Cache-Control'),'private, no-store')
 }
})
test('return does not infer cash from browser status, missing/revoked sessions or foreign receipt',async()=>{
 let calls=0
 for(const bad of [path+'&status=paid',path+'&order_id=foreign',path.replace(id,'invalid'),path.replace('cart_id=retired_cart&','')])assert.equal((await orderPaymentReturn(request(bad),config,async()=>{calls++;return response({})})).status,404)
 assert.equal(calls,0)
 for(const status of [401,404,503])assert.equal((await orderPaymentReturn(request(),config,async()=>response({error:{}},status))).status,status)
 assert.equal((await orderPaymentReturn(request(),config,async()=>response({payment:{...view,order_id:'foreign'}}))).status,503)
 const spoof=request();spoof.headers.set('X-Forwarded-For','192.0.2.1');assert.equal((await orderPaymentReturn(spoof,config)).status,403)
})
