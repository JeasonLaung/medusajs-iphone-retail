import test from 'node:test'
import assert from 'node:assert/strict'
import {transferCustomerCart,customerCartIntentKey} from '../src/lib/customer-cart-transfer.ts'
import {contextIntentKey} from '../src/lib/cart-commerce.ts'
import {paymentIntentKey} from '../src/lib/checkout-payment.ts'
import {customerClient} from '../src/lib/customer-client.ts'
import {customerBridge} from '../src/lib/customer-bridge.ts'

test('queued recovery cannot create a second operation after the first recovery clears its journal',async()=>{
 const saved=storage();let reads=0,sends=0
 await transferCustomerCart({account:'cust_new',resumeOnly:true,storage:saved,uuid:()=>{throw Error('must not mint')},exclusive:async work=>work(),current:async()=>{reads++;return cart},send:async()=>{sends++;throw Error('must not send')}})
 assert.equal(reads,0);assert.equal(sends,0)
})

test('payment transfer clears or rebinds CREATE only after checking the server completion and fresh cart',async()=>{
 for(const restoring of [false,true]){
  const saved=storage();saved.setItem(paymentIntentKey,JSON.stringify({cartID:cart.id,version:'3',operationID:op}));saved.setItem(customerCartIntentKey,JSON.stringify({account:'cust_new',cart:cart.id,command:{operation_id:op,version:'4'}}))
  let posted=false
  await transferCustomerCart({account:'cust_new',restore:restoring,storage:saved,uuid:()=>{throw Error('must not mint')},exclusive:async work=>work(),current:async()=>({...cart,version:posted?'5':'4'}),paymentOptions:async()=>({cart_id:cart.id,version:posted?'5':'4',providers:[],attempt_started:!posted||restoring}),send:async(command,restore)=>{assert.equal(restore,restoring?true:undefined);posted=true;return {state:restoring?'restored':'saved',completion:{cart:{...cart,version:'5'},payment:restoring?{operation_id:op,version:'4'}:null}}}})
  assert.equal(saved.getItem(customerCartIntentKey),null)
  assert.deepEqual(saved.getItem(paymentIntentKey)===null?null:JSON.parse(saved.getItem(paymentIntentKey)!),restoring?{cartID:cart.id,version:'4',operationID:op}:null)
 }
})
const op='a8230181-a32c-4127-838b-92b7c033ef01'
test('coordinated login preserves both PayPal methods and rejects a mixed provider projection',async()=>{
 for(const providerID of ['pp_paypal_paypal','pp_paypal_card_paypal_card'] as const)for(const mixed of [false,true]){
  const saved=storage(),payment={cartID:cart.id,version:'3',operationID:op,providerID}
  saved.setItem(paymentIntentKey,JSON.stringify(payment));let posted=false
  const options={account:'cust_new',storage:saved,uuid:()=>op,exclusive:async<T>(work:()=>Promise<T>)=>work(),current:async()=>({...cart,version:posted?'5':'4'}),paymentOptions:async()=>({cart_id:cart.id,version:posted?'5':'4',providers:[],attempt_started:true,...(!posted||!mixed?{active_provider:{id:providerID,paypal:{client_id:'artificial_client',currency:'EUR',intent:'capture' as const,environment:'sandbox' as const}}}:{})}),send:async()=>{posted=true;return {state:'saved',completion:{cart:{...cart,version:'5'},payment:{operation_id:op,version:'3'}}}}}
  if(mixed){await assert.rejects(transferCustomerCart(options));assert(saved.getItem(customerCartIntentKey))}
  else{await transferCustomerCart(options);assert.equal(saved.getItem(customerCartIntentKey),null)}
  assert.deepEqual(JSON.parse(saved.getItem(paymentIntentKey)!),payment)
 }
})
const cart={id:'cart_test',region_id:'region',sales_channel_id:'channel',currency_code:'eur',locale:'en',version:'4'}
function storage(){const values=new Map<string,string>();return {getItem:(k:string)=>values.get(k)??null,setItem:(k:string,v:string)=>{values.set(k,v)},removeItem:(k:string)=>{values.delete(k)}}}
test('lost transfer response keeps original operation/version, refreshes current cart and removes only its journal',async()=>{
 const saved=storage(),commands:unknown[]=[];let lost=true,version='4',uuids=0
 const options={account:'cust_new',storage:saved,uuid:()=>{uuids++;return op},exclusive:async<T>(work:()=>Promise<T>)=>work(),current:async()=>({...cart,version}),send:async(command:unknown)=>{commands.push(command);if(BigInt(version)<5n)version='5';if(lost)throw new Error('lost response');return {cart_id:cart.id,version:'5',replayed:true}}}
 await assert.rejects(transferCustomerCart(options));assert.ok(saved.getItem(customerCartIntentKey));assert.equal(uuids,1)
 lost=false;version='7';await transferCustomerCart(options)
 assert.equal(uuids,1);assert.deepEqual(commands,[{operation_id:op,version:'4'},{operation_id:op,version:'4'}]);assert.equal(saved.getItem(customerCartIntentKey),null)
})
test('pending transfer cannot switch account/cart, proceed alongside another command or discard uncertain state',async()=>{
 const saved=storage();let calls=0
 const options={account:'cust_new',storage:saved,uuid:()=>op,exclusive:async<T>(work:()=>Promise<T>)=>work(),current:async()=>cart,send:async()=>{calls++;throw new Error('lost')}}
 await assert.rejects(transferCustomerCart(options));assert.equal(calls,1)
 await assert.rejects(transferCustomerCart({...options,account:'cust_other'}))
 await assert.rejects(transferCustomerCart({...options,current:async()=>({...cart,id:'cart_other'})}))
 await assert.rejects(transferCustomerCart({...options,current:async()=>null}))
 saved.setItem('retail.cart.intent.v1','other pending command');await assert.rejects(transferCustomerCart(options))
 assert.equal(calls,1);assert.ok(saved.getItem(customerCartIntentKey))
})
test('definitive transfer rejection permits correction; malformed success keeps retry identity',async()=>{
 for(const status of [400,401,403,404,409,429,503]){
  const saved=storage();const options={account:'cust_new',storage:saved,uuid:()=>op,exclusive:async<T>(work:()=>Promise<T>)=>work(),current:async()=>cart,send:async()=>{throw {status}}}
  await assert.rejects(transferCustomerCart(options));assert.equal(saved.getItem(customerCartIntentKey)!==null,status===503)
 }
 for(const reply of [{},{cart_id:'cart_other',version:'5',replayed:false},{cart_id:cart.id,version:'8',replayed:true},{cart_id:cart.id,version:'5',replayed:false,private:'no'}]){
  const saved=storage();await assert.rejects(transferCustomerCart({account:'cust_new',storage:saved,uuid:()=>op,exclusive:async<T>(work:()=>Promise<T>)=>work(),current:async()=>cart,send:async()=>reply}));assert.ok(saved.getItem(customerCartIntentKey))
 }
 const saved=storage();let dispatched=false
 await transferCustomerCart({account:'cust_new',storage:saved,uuid:()=>op,exclusive:async<T>(work:()=>Promise<T>)=>work(),current:async()=>null,send:async()=>{dispatched=true}})
 assert.equal(dispatched,false);assert.equal(saved.getItem(customerCartIntentKey),null)
})
test('default login retains the deployed anonymous-cart behavior until transfer is explicitly enabled',async()=>{
 let calls=0
 const api=customerClient(async(path)=>{calls++;assert.ok(String(path).endsWith('/challenge')||String(path).endsWith('/login'));return Response.json(String(path).endsWith('/challenge')?{csrf_token:'a'.repeat(64)}:{customer:{id:'cust_test'}})})
 const form=new FormData();form.set('email','new@example.invalid');form.set('password','memory-password')
 assert.deepEqual(await api.login(null,form,'dk-en'),{state:'success'});assert.equal(calls,2)
})
test('actual customer login transfers current anonymous cart before success, logout refreshes cart observers',async()=>{
 const descriptors=Object.fromEntries(['window','document','location','navigator'].map(key=>[key,Object.getOwnPropertyDescriptor(globalThis,key)]))
 const saved=storage(),events:string[]=[],csrf='a'.repeat(64),calls:string[]=[];let version='4'
 const customer={id:'cust_test',email:'new@example.invalid',first_name:'New',last_name:'Customer',phone:'',verified_at:'2026-10-31T00:00:00Z'}
 Object.defineProperty(globalThis,'window',{value:{localStorage:saved,dispatchEvent:(e:Event)=>{events.push(e.type)}},configurable:true})
 Object.defineProperty(globalThis,'document',{value:{cookie:'retail_customer_csrf='+csrf},configurable:true})
 Object.defineProperty(globalThis,'location',{value:{protocol:'http:'},configurable:true})
 Object.defineProperty(globalThis,'navigator',{value:{locks:{request:async(_name:string,work:()=>Promise<unknown>)=>work()}},configurable:true})
 try{
  const api=customerClient(async function(this:unknown,path,init){
   assert.equal(this,undefined,'Native window fetch cannot use CartClient as its receiver')
   const url=String(path);calls.push(url)
   if(url==='/api/store/cart')return Response.json({cart:{...cart,version},capabilities:{items:false,checkout:false}},{headers:{'Cache-Control':'private, no-store'}})
   if(url.endsWith('/challenge'))return Response.json({csrf_token:csrf})
   if(url.endsWith('/login'))return Response.json({customer})
   assert.equal(new Headers(init?.headers).get('X-CSRF-Token'),csrf)
   if(url.endsWith('/cart-transfer')){const input=JSON.parse(String(init?.body));assert.deepEqual(Object.keys(input).sort(),['operation_id','version']);assert.equal(input.version,'4');version='5';return Response.json({cart_id:cart.id,version,replayed:false})}
   assert.ok(url.endsWith('/logout'));return Response.json({success:true})
  },true)
  const form=new FormData();form.set('email',customer.email);form.set('password','private-memory-password')
  assert.deepEqual(await api.login(null,form,'dk-en'),{state:'success'},JSON.stringify(calls))
  assert.ok(calls.indexOf('/api/store/customer/auth/login')<calls.indexOf('/api/store/customer/cart-transfer'))
  assert.equal(saved.getItem(customerCartIntentKey),null)
  await api.logout();assert.equal(events.filter(e=>e==='retail:cart-changed').length,2)
 }finally{for(const [key,value] of Object.entries(descriptors)){if(value)Object.defineProperty(globalThis,key,value);else Reflect.deleteProperty(globalThis,key)}}
})
test('transfer bridge forwards only cart possession on fixed POST and permits no cookie changes',async()=>{
 const origin='http://127.0.0.1:9300',root='/api/store/customer',token='b'.repeat(64),config={ginOrigin:'http://127.0.0.1:9191',storefrontOrigin:origin}
 const request=(path='/cart-transfer',method='POST')=>new Request(origin+root+path,{method,headers:{Origin:origin,'Content-Type':'application/json','X-CSRF-Token':token,Cookie:`retail_customer_session=${token}; retail_cart_possession=${token}; retail_cart_scope=PRIVATE; retail_admin_session=PRIVATE`},...(method==='POST'?{body:JSON.stringify({operation_id:op,version:'4'})}:{})})
 const reply=(status=200,cookie?:string)=>{const headers=new Headers({'Content-Type':'application/json','Cache-Control':'private, no-store'});if(cookie)headers.append('Set-Cookie',cookie);return Response.json({cart_id:cart.id,version:'5',replayed:false},{status,headers})}
 const out=await customerBridge(request(),config,async(url,init)=>{assert.equal(String(url),config.ginOrigin+root+'/cart-transfer');assert.equal(new Headers(init?.headers).get('Cookie'),`retail_customer_session=${token}; retail_cart_possession=${token}`);return reply()})
 assert.equal(out.status,200)
 for(const req of [request('/cart-transfer?x=1'),request('/cart-transfer','GET')])assert.equal((await customerBridge(req,config,async()=>{throw new Error('must not transport')})).status,404)
 assert.equal((await customerBridge(request(),config,async()=>reply(202))).status,503)
 assert.equal((await customerBridge(request(),config,async()=>reply(200,`retail_cart_possession=; Path=/; Max-Age=0; HttpOnly; SameSite=Lax`))).status,503)
})

test('pending context prevents customer ownership transfer before any private request',async()=>{
 const saved=storage();saved.setItem(contextIntentKey,'pending');let reads=0,sends=0
 await assert.rejects(transferCustomerCart({account:'cust_new',storage:saved,uuid:()=>op,exclusive:async work=>work(),current:async()=>{reads++;return cart},send:async()=>{sends++;throw Error('must not send')}}))
 assert.equal(reads,0);assert.equal(sends,0);assert.equal(saved.getItem(contextIntentKey),'pending')
})

test('a frozen checkout permits same-account acknowledgment while retaining its original CREATE journal',async()=>{
 const saved=storage(),payment=JSON.stringify({cartID:cart.id,version:'4',operationID:op})
 saved.setItem(paymentIntentKey,payment);let sends=0
 const options={account:'cust_new',storage:saved,uuid:()=>op,exclusive:async<T>(work:()=>Promise<T>)=>work(),current:async()=>({...cart,version:'5'}),paymentOptions:async()=>({cart_id:cart.id,version:'5',providers:[],attempt_started:true}),send:async()=>{sends++;return {cart_id:cart.id,version:'5',replayed:false}}}
 await transferCustomerCart(options)
 assert.equal(sends,1);assert.equal(saved.getItem(paymentIntentKey),payment);assert.equal(saved.getItem(customerCartIntentKey),null)
 for(const paymentOptions of [async()=>null,async()=>({cart_id:cart.id,version:'5',providers:[],attempt_started:false}),async()=>({cart_id:cart.id,version:'6',providers:[],attempt_started:true})])await assert.rejects(transferCustomerCart({...options,paymentOptions}))
 assert.equal(sends,1)
 await assert.rejects(transferCustomerCart({...options,current:async()=>cart}))
 saved.setItem(paymentIntentKey,'invalid');await assert.rejects(transferCustomerCart(options));assert.equal(sends,1)
})

test('lost same-account acknowledgment keeps transfer UUID and the immutable payment journal through retry',async()=>{
 const saved=storage(),payment=JSON.stringify({cartID:cart.id,version:'4',operationID:op}),sent:unknown[]=[];let lost=true
 saved.setItem(paymentIntentKey,payment)
 const options={account:'cust_new',storage:saved,uuid:()=>op,exclusive:async<T>(work:()=>Promise<T>)=>work(),current:async()=>({...cart,version:'6'}),paymentOptions:async()=>({cart_id:cart.id,version:'6',providers:[],attempt_started:true}),send:async(command:unknown)=>{sent.push(command);if(lost)throw Error('lost');return {cart_id:cart.id,version:'6',replayed:true}}}
 await assert.rejects(transferCustomerCart(options));assert(saved.getItem(customerCartIntentKey));lost=false
 await transferCustomerCart(options);assert.deepEqual(sent,[{operation_id:op,version:'6'},{operation_id:op,version:'6'}]);assert.equal(saved.getItem(paymentIntentKey),payment)
})
