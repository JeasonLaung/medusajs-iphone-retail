import test from 'node:test'
import assert from 'node:assert/strict'
import {CartClient,CartError,validPromotionCode,type CartContactDetail,type CartView,type CartPromotions,type PromotionInput} from '../src/lib/cart-client.ts'
import {createPromotionCommands} from '../src/lib/checkout-promotions.ts'
import {cartIntentKey,contactIntentKey,shippingIntentKey,promotionIntentKey} from '../src/lib/cart-commerce.ts'
import {paymentIntentKey,createPaymentCommands} from '../src/lib/checkout-payment.ts'
import {cartBridge} from '../src/lib/cart-bridge.ts'
const operation='12345678-1234-4234-8234-123456789012',csrf='a'.repeat(64)
const detail=(version='3'):CartPromotions=>({cart_id:'cart',version,promotions:[{id:'promo',code:'CODE',is_automatic:false}]})
const reply=(body:unknown,status=200)=>Response.json(body,{status,headers:{'Cache-Control':'private, no-store'}})
test('promotion client validates saved projection and exact version receipt without forwarding prices',async()=>{
 const client=new CartClient({readScopeCSRF:()=>undefined,fetcher:async(path,init)=>{
  assert.equal(init?.cache,'no-store');assert.equal(init?.credentials,'same-origin')
  if(init?.method==='GET')return reply(detail())
  assert.equal(path,'/api/store/carts/cart/promotions');assert.deepEqual(JSON.parse(String(init.body)),{version:'9007199254740993123',codes:['CODE']})
  assert.equal(new Headers(init.headers).get('X-CSRF-Token'),csrf);assert.equal(new Headers(init.headers).get('Idempotency-Key'),operation)
  return reply({cart_id:'cart',version:'9007199254740993124',replayed:false})
 }})
 assert.deepEqual(await client.retrievePromotions('cart'),detail())
 assert.equal((await client.savePromotions('cart',{version:'9007199254740993123',operationID:operation,codes:['CODE'],amount:'0'} as PromotionInput,{csrfToken:csrf})).version,'9007199254740993124')
 for(const bad of [{...detail(),customer_id:'private'},{...detail(),promotions:[...detail().promotions,...detail().promotions]},{...detail(),promotions:[{id:'p',code:'CODE',is_automatic:false,budget:'private'}]},{...detail(),promotions:[{id:'p',code:'\n',is_automatic:false}]}]){
  await assert.rejects(new CartClient({readScopeCSRF:()=>undefined,fetcher:async()=>reply(bad)}).retrievePromotions('cart'))
 }
 for(const code of ['', '\ud800', 'a\n', '中'.repeat(64)])assert.equal(validPromotionCode(code),false)
 assert.equal(validPromotionCode('折扣😀'),true)
 const uncertain=new CartClient({readScopeCSRF:()=>undefined,fetcher:async()=>reply({cart_id:'cart',version:'5',replayed:false})})
 await assert.rejects(uncertain.savePromotions('cart',{version:'2',operationID:operation,codes:[]},{csrfToken:csrf}),e=>e instanceof CartError&&e.uncertain)
 assert.equal(await new CartClient({readScopeCSRF:()=>undefined,fetcher:async()=>reply({},401)}).retrievePromotions('cart'),null)
})
function fixture(){
 const data=new Map<string,string>(),calls:PromotionInput[]=[];let lose=false,readFailure=false,mismatch=false,reject=false
 const storage={getItem:(k:string)=>data.get(k)??null,setItem:(k:string,v:string)=>{data.set(k,v)},removeItem:(k:string)=>{data.delete(k)}}
 const client={
  savePromotions:async(_id:string,input:PromotionInput)=>{calls.push(input);if(lose)throw new CartError('unavailable',null,true);if(reject)throw new CartError('input',400,false);return {cart_id:'cart',version:'3',replayed:calls.length>1}},
  retrievePromotions:async()=>{if(readFailure)throw new CartError('unavailable');return detail()},
  retrieveContacts:async()=>({cart_id:'cart',version:'3'}) as CartContactDetail,
  retrieveView:async()=>({amounts:{cart_id:'cart',version:mismatch?'4':'3'},items:[]}) as unknown as CartView,
 }
 const commands=createPromotionCommands({client,storage,uuid:()=>operation,readCSRF:()=>csrf,exclusive:async action=>action()})
 return {commands,client,storage,data,calls,lose:(v:boolean)=>{lose=v},failRead:(v:boolean)=>{readFailure=v},mismatch:()=>{mismatch=true},reject:()=>{reject=true}}
}
test('lost promotion response preserves the original operation and codes across refresh recovery',async()=>{
 const f=fixture();f.lose(true);await assert.rejects(f.commands.submit('cart','2',['CODE']))
 assert.equal(f.commands.pending()?.kind,'save');assert.equal(f.data.get(promotionIntentKey)?.includes(csrf),false)
 await assert.rejects(f.commands.submit('cart','3',['DIFFERENT']));assert.equal(f.calls.length,1)
 f.lose(false);await f.commands.recover();assert.deepEqual(f.calls[0],f.calls[1]);assert.equal(f.data.size,0)
})
test('acknowledged promotion writes recover reads only and refuse mixed snapshot versions',async()=>{
 const f=fixture();f.failRead(true);await assert.rejects(f.commands.submit('cart','2',[]));assert.equal(f.commands.pending()?.kind,'refresh')
 f.failRead(false);f.mismatch();await assert.rejects(f.commands.recover());assert.equal(f.calls.length,1)
 const g=fixture();g.failRead(true);await assert.rejects(g.commands.submit('cart','2',[]));g.failRead(false);await g.commands.recover();assert.equal(g.calls.length,1);assert.equal(g.data.size,0)
})
test('other checkout journals block discounts; uncertain discounts block payment; known rejection clears only its intent',async()=>{
 const oversized=fixture();await assert.rejects(oversized.commands.submit('cart','2',Array(100).fill('a'.repeat(191))));assert.equal(oversized.data.size,0);assert.equal(oversized.calls.length,0)
 for(const key of [cartIntentKey,contactIntentKey,shippingIntentKey,paymentIntentKey]){const f=fixture();f.data.set(key,'pending');await assert.rejects(f.commands.submit('cart','2',[]));assert.equal(f.calls.length,0)}
 const f=fixture();f.lose(true);await assert.rejects(f.commands.submit('cart','2',[]))
 let payments=0
 const payment=createPaymentCommands({client:{startPayment:async()=>{payments++;throw Error()}},storage:f.storage,readCSRF:()=>csrf,uuid:()=>operation,exclusive:async action=>action()})
 await assert.rejects(payment.submit('cart','2'));assert.equal(payments,0)
 const g=fixture();g.reject();await assert.rejects(g.commands.submit('cart','2',['UNKNOWN']));assert.equal(g.commands.pending(),null)
 g.data.set(promotionIntentKey,JSON.stringify({cartID:'cart',version:'2',operationID:operation,codes:[],kind:'save',customer_id:'foreign'}));assert.throws(()=>g.commands.pending())
})
test('promotion bridge permits only fixed private paths with cart authority and no query',async()=>{
 const shop='http://127.0.0.1:19375',config={ginOrigin:'http://127.0.0.1:19100',storefrontOrigin:shop}
 const fetcher=async(_url:unknown,init?:RequestInit)=>{assert.equal(new Headers(init?.headers).get('Cookie'),`retail_cart_possession=${csrf}`);return reply(detail())}
 assert.equal((await cartBridge(new Request(shop+'/api/store/carts/cart/promotions',{headers:{Cookie:`retail_cart_possession=${csrf}; retail_customer_session=${csrf}`}}),config,fetcher as typeof fetch)).status,200)
 assert.equal((await cartBridge(new Request(shop+'/api/store/carts/cart/promotions?amount=0'),config,fetcher as typeof fetch)).status,404)
 assert.equal((await cartBridge(new Request(shop+'/api/store/carts/cart/promotions',{method:'POST',headers:{Origin:'https://foreign.example','Content-Type':'application/json'},body:'{}'}),config,fetcher as typeof fetch)).status,403)
})
