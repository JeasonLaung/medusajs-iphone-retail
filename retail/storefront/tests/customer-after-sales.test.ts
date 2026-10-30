import test from 'node:test'
import assert from 'node:assert/strict'
import {afterSalesRequestProjection,afterSalesListProjection,orderDevicesProjection} from '../src/lib/customer-after-sales.ts'
import {customerClient} from '../src/lib/customer-client.ts'
import {customerBridge} from '../src/lib/customer-bridge.ts'
const request=()=>({id:'asr_created',type:'return',status:'pending',reason:'Please review this item.',items:[{item_id:'line',quantity:1,title:'Phone',device_ids:['device']}],created_at:'2026-10-30T00:00:00Z'})
test('after-sales projects original request and device fields without private data',()=>{
 assert.deepEqual(afterSalesRequestProjection(request()),request())
 for(const alter of [(v:any)=>v.customer_id='private',(v:any)=>v.status='refunded',(v:any)=>v.items[0].quantity='1',(v:any)=>v.items[0].quantity=1.1,(v:any)=>v.items[0].device_ids.push('device'),(v:any)=>v.items.push(v.items[0]),(v:any)=>v.items[0].private='cost',(v:any)=>v.created_at='invalid']){const v=request();alter(v);assert.throws(()=>afterSalesRequestProjection(v))}
 assert.throws(()=>afterSalesListProjection({requests:[request(),request()]}))
 const d={id:'device',assignment_id:'assignment',item_id:'line',serial_number:'SERIAL',imei:'123456789012345',status:'shipped',replacement_of:null,tracking_number:'0001234'}
 assert.deepEqual(orderDevicesProjection({devices:[d]}),[d])
 assert.throws(()=>orderDevicesProjection({devices:[{...d,status:'packed'}]}));assert.throws(()=>orderDevicesProjection({devices:[{...d,cost_cents:1}]}));assert.throws(()=>orderDevicesProjection({devices:[d,d]}))
})
test('after-sales private bridge accepts 201 only for the authorized mutation and forwards no cart cookie',async()=>{
 const shop='http://127.0.0.1:9300',root='/api/store/customer/orders/order_created/after-sales',token='a'.repeat(64),csrf='b'.repeat(64),body={operation_id:'923b53d8-efc7-4c97-9d55-e21e3400ea10',type:'return',reason:'Please review this item.',items:[{item_id:'line',quantity:1}]}
 const response=await customerBridge(new Request(shop+root,{method:'POST',headers:{Origin:shop,'Content-Type':'application/json','X-CSRF-Token':csrf,Cookie:`retail_customer_session=${token}; retail_customer_csrf=${csrf}; retail_cart_possession=${token}`},body:JSON.stringify(body)}),{ginOrigin:'http://127.0.0.1:9100',storefrontOrigin:shop},async(url,init)=>{assert.equal(String(url),'http://127.0.0.1:9100'+root);assert.equal(new Headers(init?.headers).get('Cookie'),`retail_customer_session=${token}; retail_customer_csrf=${csrf}`);assert.deepEqual(JSON.parse(new TextDecoder().decode(init?.body as Uint8Array)),body);return Response.json({request:request()},{status:201,headers:{'Cache-Control':'private, no-store'}})})
 assert.equal(response.status,201);assert.equal(response.headers.get('Cache-Control'),'private, no-store')
 Object.defineProperty(globalThis,'document',{value:{cookie:'retail_customer_csrf='+csrf},configurable:true});Object.defineProperty(globalThis,'location',{value:{protocol:'http:'},configurable:true})
 const api=customerClient(async(url,init)=>{assert.equal(url,root);assert.equal(new Headers(init?.headers).get('X-CSRF-Token'),csrf);assert.deepEqual(JSON.parse(init?.body as string),body);return Response.json({request:request()},{status:201})})
 try{assert.deepEqual(await api.createAfterSales('order_created',body as any),request())}finally{api.clear();Reflect.deleteProperty(globalThis,'document');Reflect.deleteProperty(globalThis,'location')}
})
