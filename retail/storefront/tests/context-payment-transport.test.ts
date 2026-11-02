import test from 'node:test'
import assert from 'node:assert/strict'
import {CartClient,CartError} from '../src/lib/cart-client.ts'
import {cartBridge} from '../src/lib/cart-bridge.ts'

const csrf='a'.repeat(64),operationID='12345678-1234-4234-8234-123456789012'
const input={operationID,version:'9007199254740993123',regionID:'region',locale:'zh-CN'}
const receipt={cart_id:'cart',version:'9007199254740993124',replayed:false}
const completion={cart:{id:'cart',region_id:'region',sales_channel_id:'channel',currency_code:'eur',locale:'zh-CN',version:receipt.version},payment:null}
const reply=(v:unknown,status=200)=>Response.json(v,{status,headers:{'Cache-Control':'private, no-store'}})
const client=(value:unknown)=>new CartClient({readScopeCSRF:()=>undefined,fetcher:async()=>reply(value)})

test('context payment sends the exact target and retry identity through the private Astro bridge',async()=>{
 const config={ginOrigin:'http://127.0.0.1:9191',storefrontOrigin:'http://127.0.0.1:9300'}
 for(const restore of [false,true]){
  let calls=0
  const api=new CartClient({readScopeCSRF:()=>undefined,fetcher:async(path,init)=>{
   assert.equal(init?.credentials,'same-origin');assert.equal(init?.redirect,'error')
   const request=new Request(config.storefrontOrigin+path,{...init,headers:{...init?.headers,Origin:config.storefrontOrigin,Cookie:`retail_cart_possession=${csrf}; retail_admin_session=PRIVATE`}})
   return cartBridge(request,config,async(url,forwarded)=>{
    calls++;assert.equal(String(url),config.ginOrigin+'/api/store/carts/cart/context-payment'+(restore?'/restore':''))
    const headers=new Headers(forwarded?.headers)
    assert.equal(headers.get('Cookie'),`retail_cart_possession=${csrf}`)
    assert.equal(headers.get('X-CSRF-Token'),csrf);assert.equal(headers.get('Idempotency-Key'),operationID)
    assert.equal(headers.get('Authorization'),null)
    assert.deepEqual(JSON.parse(new TextDecoder().decode(forwarded?.body as Uint8Array)),{version:input.version,region_id:'region',locale:'zh-CN'})
    return reply(restore?{state:'restored',completion}:{state:'saved',receipt,completion})
   })
  }})
  assert.deepEqual(await api.contextPayment('cart',{...input,session_id:'injected'} as typeof input,{csrfToken:csrf},restore),restore?{state:'restored',completion}:{state:'saved',receipt,completion})
  assert.equal(calls,1)
 }
})

test('context progress distinguishes pending work and restoration from a target-save receipt',async()=>{
 for(const state of ['processing','requires_recovery'] as const)for(const restore of [false,true])assert.deepEqual(await client({state}).contextPayment('cart',input,{csrfToken:csrf},restore),{state})
 for(const [value,restore] of [
  [{state:'restored'},false],[{state:'saved',receipt,completion},true],
  [{state:'saved'},false],[{state:'saved',receipt:{...receipt,cart_id:'foreign'},completion},false],
  [{state:'saved',receipt:{...receipt,version:'9007199254740993125'},completion},false],
  [{state:'saved',receipt:{...receipt,version:9007199254740992},completion},false],
  [{state:'saved',receipt:{...receipt,replayed:'true'},completion},false],
  [{state:'processing',receipt},false],[{state:'requires_recovery',session_id:'private'},false],
  [{state:'paid'},false],[null,false],
  ...[{...completion,cart:{...completion.cart,id:'foreign'}},{...completion,cart:{...completion.cart,locale:'en'}},{...completion,payment:{operation_id:operationID,version:receipt.version}},{...completion,payment:{operation_id:operationID,version:'4',session_id:'private'}}].map(completion=>[{state:'saved',receipt,completion},false] as const),
 ] as const)await assert.rejects(client(value).contextPayment('cart',input,{csrfToken:csrf},restore),e=>e instanceof CartError&&e.kind==='protocol'&&e.uncertain)
})

test('context payment input validation cannot dispatch invalid versions, keys or targets',async()=>{
 let calls=0
 const api=new CartClient({readScopeCSRF:()=>undefined,fetcher:async()=>{calls++;return reply({state:'processing'})}})
 for(const change of [{version:'18446744073709551615'},{version:'01'},{version:1},{operationID:'bad'},{regionID:'../other'},{locale:null},{locale:'en\n'}])await assert.rejects(api.contextPayment('cart',{...input,...change} as typeof input,{csrfToken:csrf}),e=>e instanceof CartError&&e.kind==='input'&&!e.uncertain)
 await assert.rejects(api.contextPayment('cart',input,{csrfToken:''}))
 assert.equal(calls,0)
})

test('new bridge paths preserve origin/query/method boundaries',async()=>{
 const shop='http://127.0.0.1:9300',config={ginOrigin:'http://127.0.0.1:9191',storefrontOrigin:shop}
 let calls=0
 for(const [path,method,origin] of [
  ['/api/store/carts/cart/context-payment?session_id=bad','POST',shop],
  ['/api/store/carts/cart/context-payment/delete','POST',shop],
  ['/api/store/carts/cart/context-payment/restore/extra','POST',shop],
  ['/api/store/carts/cart/context-payment','GET',shop],
  ['/api/store/carts/cart/context-payment','POST','https://foreign.example'],
 ] as const){
  const r=await cartBridge(new Request(shop+path,{method,headers:{Origin:origin,'Content-Type':'application/json'},...(method==='POST'?{body:'{}'}:{})}),config,async()=>{calls++;return reply({})})
  assert.ok(r.status>=400)
 }
 assert.equal(calls,0)
})

test('payment-context client permits the bounded server operation and still aborts',async t=>{
 t.mock.timers.enable({apis:['setTimeout']})
 let signal:AbortSignal|undefined,entered!:()=>void
 const started=new Promise<void>(resolve=>{entered=resolve})
 const api=new CartClient({readScopeCSRF:()=>undefined,fetcher:async(_url,init)=>new Promise<Response>((_resolve,reject)=>{
  signal=init!.signal!;signal.addEventListener('abort',()=>reject(Error('timeout')));entered()
 })})
 const pending=api.contextPayment('cart',input,{csrfToken:csrf})
 const rejected=assert.rejects(pending,e=>e instanceof CartError&&e.uncertain)
 await started;t.mock.timers.tick(12000);assert.equal(signal!.aborted,false)
 t.mock.timers.tick(43000);await rejected;assert.equal(signal!.aborted,true)
})

test('Astro bridge allows the context operation its bound while ordinary requests retain their short bound',async t=>{
 t.mock.timers.enable({apis:['setTimeout']})
 for(const [path,bound] of [['context-payment',50000],['context',11000]] as const){
  let signal:AbortSignal|undefined,entered!:()=>void
  const started=new Promise<void>(resolve=>{entered=resolve})
  const shop='http://127.0.0.1:9300'
  const pending=cartBridge(new Request(shop+'/api/store/carts/cart/'+path,{method:'POST',headers:{Origin:shop,'Content-Type':'application/json'},body:'{}'}),{ginOrigin:'http://127.0.0.1:9191',storefrontOrigin:shop},async(_url,init)=>new Promise<Response>((_resolve,reject)=>{
   signal=init!.signal!;signal.addEventListener('abort',()=>reject(Error('timeout')));entered()
  }))
  await started;t.mock.timers.tick(bound-1);assert.equal(signal!.aborted,false)
  t.mock.timers.tick(1);assert.ok((await pending).status>=500);assert.equal(signal!.aborted,true)
 }
})
