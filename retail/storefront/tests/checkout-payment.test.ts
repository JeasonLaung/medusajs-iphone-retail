import test from 'node:test'
import assert from 'node:assert/strict'
import {CartClient,CartError,type PaymentAttempt} from '../src/lib/cart-client.ts'
import {cartBridge} from '../src/lib/cart-bridge.ts'
import {createPaymentCommands,paymentIntentKey,clearCompletedPayment} from '../src/lib/checkout-payment.ts'
import {cartIntentKey,contactIntentKey,shippingIntentKey} from '../src/lib/cart-commerce.ts'
const operationID='12345678-1234-4234-8234-123456789012',csrf='a'.repeat(64)
const receipt=(extra:Record<string,unknown>={})=>({cart_id:'cart',version:'9007199254740993124',operation_id:operationID,state:'ready',checkout_url:'https://www.mollie.com/checkout/owned',...extra})
const reply=(v:unknown,status=200)=>Response.json(v,{status,headers:{'Cache-Control':'private, no-store'}})
test('payment initiation sends only saved version and operation, without price or provider authority',async()=>{
 const client=new CartClient({readScopeCSRF:()=>undefined,fetcher:async(path,init)=>{
  assert.equal(path,'/api/store/carts/cart/payment-attempts');assert.equal(init?.method,'POST');assert.equal(init?.body,'{"version":"9007199254740993123"}')
  const headers=new Headers(init?.headers);assert.equal(headers.get('Idempotency-Key'),operationID);assert.equal(headers.get('X-CSRF-Token'),csrf);assert.equal(init?.credentials,'same-origin');assert.equal(init?.redirect,'error')
  return reply(receipt())
 }})
 assert.equal((await client.startPayment('cart',{version:'9007199254740993123',operationID},{csrfToken:csrf})).state,'ready')
 for(const state of ['processing','requires_review'])assert.equal((await new CartClient({readScopeCSRF:()=>undefined,fetcher:async()=>reply(receipt({state,checkout_url:null}))}).startPayment('cart',{version:'9007199254740993123',operationID},{csrfToken:csrf})).state,state)
})
test('malformed, mixed-cart and unsafe payment responses remain uncertain',async()=>{
 const bodies=[receipt({cart_id:'foreign'}),receipt({operation_id:'foreign'}),receipt({version:'9007199254740993125'}),receipt({version:9007199254740992}),receipt({state:'paid'}),receipt({private_key:'private'}),receipt({state:'processing'}),receipt({checkout_url:null})]
 for(const checkout_url of ['http://mollie.com/','https://mollie.com.evil.test/','https://evil.test/?next=https://mollie.com','https://user:pass@www.mollie.com/','https://www.mollie.com:9443/','https://www.mollie.com/\n','https://www.mollie.com\\@evil.test/'])bodies.push(receipt({checkout_url}))
 for(const body of bodies)await assert.rejects(new CartClient({readScopeCSRF:()=>undefined,fetcher:async()=>reply(body)}).startPayment('cart',{version:'9007199254740993123',operationID},{csrfToken:csrf}),e=>e instanceof CartError&&e.uncertain)
 for(const fetcher of [async()=>reply({},503),async()=>{throw Error('network')}])await assert.rejects(new CartClient({readScopeCSRF:()=>undefined,fetcher}).startPayment('cart',{version:'2',operationID},{csrfToken:csrf}),e=>e instanceof CartError&&e.uncertain)
})
function fixture(){
 const data=new Map<string,string>(),calls:{version:string;operationID:string}[]=[];let failed=false,ids=0
 const commands=createPaymentCommands({storage:{getItem:k=>data.get(k)??null,setItem:(k,v)=>{data.set(k,v)}},readCSRF:()=>csrf,uuid:()=>{ids++;return operationID},exclusive:async action=>action(),client:{startPayment:async(_id,input)=>{assert.ok(data.has(paymentIntentKey));calls.push({...input});if(failed)throw new CartError('unavailable',null,true);return receipt() as PaymentAttempt}}})
 return {commands,data,calls,ids:()=>ids,fail:(v:boolean)=>{failed=v}}
}
test('network loss and repeated submissions reuse original pre-freeze payment version and operation',async()=>{
 const f=fixture();f.fail(true);await assert.rejects(f.commands.submit('cart','2'));f.fail(false)
 await f.commands.submit('cart','3');assert.deepEqual(f.calls,[{version:'2',operationID},{version:'2',operationID}]);assert.equal(f.ids(),1)
 assert.equal(f.data.get(paymentIntentKey),JSON.stringify({cartID:'cart',version:'2',operationID}));assert.ok(!f.data.get(paymentIntentKey)?.includes(csrf))
 await f.commands.submit('cart','3');assert.equal(f.ids(),1);assert.equal(f.calls.length,3)
 await assert.rejects(f.commands.submit('other','3'));assert.equal(f.calls.length,3)
})
test('pending contact, cart and shipping writes block initiation; malformed journals never dispatch',async()=>{
 const f=fixture()
 for(const key of [cartIntentKey,contactIntentKey,shippingIntentKey]){f.data.set(key,'pending');await assert.rejects(f.commands.submit('cart','2'));f.data.delete(key)}
 for(const raw of ['{','null','[]',JSON.stringify({cartID:'cart',version:'2',operationID,checkout_url:'https://evil.test'}),JSON.stringify({cartID:'cart',version:'18446744073709551615',operationID})]){f.data.set(paymentIntentKey,raw);await assert.rejects(f.commands.submit('cart','2'))}
 assert.equal(f.calls.length,0);assert.equal(f.ids(),0)
})
test('journal persistence failure prevents any payment request',async()=>{
 for(const storage of [{getItem:()=>null,setItem:()=>{throw Error('quota')}},{getItem:()=>null,setItem:()=>{}}]){
  let called=false
  const commands=createPaymentCommands({storage,readCSRF:()=>csrf,uuid:()=>operationID,exclusive:async a=>a(),client:{startPayment:async()=>{called=true;return receipt() as PaymentAttempt}}})
  await assert.rejects(commands.submit('cart','2'));assert.equal(called,false)
 }
})
test('payment bridge forwards only cart cookies and fixed initiation path',async()=>{
 const config={ginOrigin:'http://127.0.0.1:9100',storefrontOrigin:'http://127.0.0.1:9300'}
 const request=new Request(config.storefrontOrigin+'/api/store/carts/cart/payment-attempts',{method:'POST',headers:{Origin:config.storefrontOrigin,'Content-Type':'application/json','X-CSRF-Token':csrf,'Idempotency-Key':operationID,Cookie:'retail_cart_possession='+csrf+'; admin=secret'},body:'{"version":"2"}'})
 const response=await cartBridge(request,config,async(path,init)=>{assert.equal(new URL(String(path)).pathname,'/api/store/carts/cart/payment-attempts');const headers=new Headers(init?.headers);assert.equal(headers.get('Cookie'),'retail_cart_possession='+csrf);assert.equal(headers.get('Idempotency-Key'),operationID);assert.equal(headers.get('X-CSRF-Token'),csrf);return reply(receipt())});assert.equal(response.status,200)
 for(const suffix of ['?amount=0','/retry'])assert.equal((await cartBridge(new Request(request.url+suffix,{method:'POST'}),config,async()=>{throw Error('must not dispatch')})).status,404)
})

test('new controller after page reload reuses the persisted descriptor without generating an operation',async()=>{
 const f=fixture();await f.commands.submit('cart','2')
 const restored=createPaymentCommands({storage:{getItem:k=>f.data.get(k)??null,setItem:()=>{throw Error('must reuse')}},readCSRF:()=>csrf,uuid:()=>{throw Error('must reuse')},exclusive:async a=>a(),client:{startPayment:async(cart,input)=>{assert.equal(cart,'cart');assert.deepEqual(input,{version:'2',operationID});return receipt() as PaymentAttempt}}})
 assert.equal((await restored.submit('cart','3')).state,'ready')
})

test('private method availability binds current cart/version and never offers another method for a frozen attempt',async()=>{
 const available={cart_id:'cart',version:'9007199254740993123',providers:[{id:'pp_mollie-hosted-checkout_mollie'}],attempt_started:false}
 const api=(body:unknown,status=200)=>new CartClient({readScopeCSRF:()=>undefined,fetcher:async(path,init)=>{assert.equal(path,'/api/store/carts/cart/payment-options');assert.equal(init?.method,'GET');return reply(body,status)}})
 assert.deepEqual(await api(available).paymentOptions('cart'),available)
 assert.deepEqual((await api({...available,providers:[],attempt_started:true}).paymentOptions('cart'))?.providers,[])
 for(const body of [{...available,cart_id:'foreign'},{...available,version:1},{...available,attempt_started:true},{...available,providers:[...available.providers,...available.providers]},{...available,providers:[{id:'pp_paypal_paypal'}]},{...available,providers:[{id:'pp_mollie-hosted-checkout_mollie',api_key:'secret'}]},{...available,api_key:'secret'}])await assert.rejects(api(body).paymentOptions('cart'))
 assert.equal(await api({},404).paymentOptions('cart'),null);await assert.rejects(api({},503).paymentOptions('cart'))
 const config={ginOrigin:'http://127.0.0.1:9100',storefrontOrigin:'http://127.0.0.1:9300'}
 assert.equal((await cartBridge(new Request(config.storefrontOrigin+'/api/store/carts/cart/payment-options',{headers:{Cookie:'retail_cart_possession='+csrf+'; admin=private'}}),config,async(_path,init)=>{assert.equal(new Headers(init?.headers).get('Cookie'),'retail_cart_possession='+csrf);return reply(available)})).status,200)
})

test('completed-cart cleanup removes only the matching validated descriptor and tolerates storage failure',async()=>{
 const f=fixture();await f.commands.submit('cart','2');const storage={getItem:(k:string)=>f.data.get(k)??null,removeItem:(k:string)=>{f.data.delete(k)}}
 clearCompletedPayment(storage,'other');assert.ok(f.data.has(paymentIntentKey))
 clearCompletedPayment(storage,'cart');assert.equal(f.data.has(paymentIntentKey),false)
 f.data.set(paymentIntentKey,'{');clearCompletedPayment(storage,'cart');assert.equal(f.data.get(paymentIntentKey),'{')
 assert.doesNotThrow(()=>clearCompletedPayment({getItem:()=>{throw Error('disabled')},removeItem:()=>{throw Error('disabled')}},'cart'))
})
