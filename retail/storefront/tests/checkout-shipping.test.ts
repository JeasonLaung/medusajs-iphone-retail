import test from 'node:test'
import assert from 'node:assert/strict'
import {CartClient,CartError,type CartContactDetail,type CartView,type DeliveryOptions,type ShippingInput} from '../src/lib/cart-client.ts'
import {createShippingCommands,shippingIntentKey} from '../src/lib/checkout-shipping.ts'
import {cartIntentKey,contactIntentKey} from '../src/lib/cart-commerce.ts'
import {cartBridge} from '../src/lib/cart-bridge.ts'
const key='12345678-1234-4234-8234-123456789012',csrf='a'.repeat(64)
const option={id:'delivery',profile_id:'default',name:'Delivery',description:'Tracked',kind:'shipping' as const,amount:'9007199254740993123.01',is_tax_inclusive:false,insufficient_inventory:false}
const options=(version='2'):DeliveryOptions=>({cart_id:'cart',version,currency_code:'eur',options:[option],selected_option_ids:['delivery'],complete:true})
const reply=(body:unknown,status=200)=>Response.json(body,{status,headers:{'Cache-Control':'private, no-store'}})
test('delivery private client verifies exact amounts, selection shape and next uint64 receipt',async()=>{
 const client=new CartClient({readScopeCSRF:()=>undefined,fetcher:async(path,init)=>{
  assert.equal(init?.cache,'no-store');assert.equal(init?.credentials,'same-origin')
  if(init?.method==='GET')return reply(options())
  assert.equal(path,'/api/store/carts/cart/shipping-methods');assert.deepEqual(JSON.parse(String(init.body)),{version:'9007199254740993123',option_id:'delivery'});assert.equal(new Headers(init.headers).get('Idempotency-Key'),key)
  return reply({cart_id:'cart',version:'9007199254740993124',option_id:'delivery',replayed:false})
 }})
 assert.equal((await client.retrieveDelivery('cart'))?.options[0].amount,option.amount)
 const input={operationID:key,version:'9007199254740993123',optionID:'delivery',amount:'0',provider_id:'foreign'}
 assert.equal((await client.saveShipping('cart',input,{csrfToken:csrf})).version,'9007199254740993124')
 for(const bad of [{...options(),selected_option_ids:['foreign']},{...options(),options:[option,option]},{...options(),options:[{...option,amount:1}]},{...options(),options:[{...option,provider_id:'private'}]}])await assert.rejects(new CartClient({readScopeCSRF:()=>undefined,fetcher:async()=>reply(bad)}).retrieveDelivery('cart'))
 await assert.rejects(new CartClient({readScopeCSRF:()=>undefined,fetcher:async()=>reply({cart_id:'cart',version:'2',option_id:'delivery',replayed:false})}).saveShipping('cart',input,{csrfToken:csrf}),e=>e instanceof CartError&&e.uncertain)
 assert.equal(await new CartClient({readScopeCSRF:()=>undefined,fetcher:async()=>reply({},401)}).retrieveDelivery('cart'),null)
 await assert.rejects(new CartClient({readScopeCSRF:()=>undefined,fetcher:async()=>reply({},503)}).retrieveDelivery('cart'))
})
function fixture(){
 const data=new Map<string,string>(),calls:ShippingInput[]=[];let failWrite=false,failRead=false,mismatch=false
 const commands=createShippingCommands({storage:{getItem:k=>data.get(k)??null,setItem:(k,v)=>{data.set(k,v)},removeItem:k=>{data.delete(k)}},readCSRF:()=>csrf,uuid:()=>key,exclusive:async action=>action(),client:{
  saveShipping:async(_id,input)=>{calls.push(input);if(failWrite)throw new CartError('unavailable',null,true);return {cart_id:'cart',version:'3',option_id:input.optionID,replayed:calls.length>1}},
  retrieveDelivery:async()=>{if(failRead)throw new CartError('unavailable');return options('3')},
  retrieveView:async()=>({amounts:{cart_id:'cart',version:mismatch?'4':'3',currency_code:'eur'},items:[]}) as unknown as CartView,
  retrieveContacts:async()=>({cart_id:'cart',version:'3'}) as CartContactDetail
 }})
 return {commands,data,calls,failWrite:(v:boolean)=>{failWrite=v},failRead:(v:boolean)=>{failRead=v},mismatch:()=>{mismatch=true}}
}
test('uncertain shipping keeps the exact operation and refuses a different choice',async()=>{
 const f=fixture();f.failWrite(true);await assert.rejects(f.commands.submit('cart','2','delivery'))
 assert.equal(f.commands.pending()?.kind,'save');assert.equal(f.data.get(shippingIntentKey)?.includes(csrf),false)
 await assert.rejects(f.commands.submit('cart','3','other'));assert.equal(f.calls.length,1)
 f.failWrite(false);await f.commands.submit('cart','3','delivery');assert.deepEqual(f.calls[0],f.calls[1]);assert.equal(f.data.size,0)
})
test('acknowledged shipping recovers reads only, and cross-tab contact/item intents block writes',async()=>{
 const f=fixture();f.failRead(true);await assert.rejects(f.commands.submit('cart','2','delivery'));assert.equal(f.commands.pending()?.kind,'refresh')
 f.failRead(false);await f.commands.recover();assert.equal(f.calls.length,1);assert.equal(f.data.size,0)
 for(const k of [cartIntentKey,contactIntentKey]){f.data.set(k,'pending');await assert.rejects(f.commands.submit('cart','3','delivery'));assert.equal(f.calls.length,1);f.data.delete(k)}
 f.mismatch();await assert.rejects(f.commands.submit('cart','3','delivery'));assert.equal(f.commands.pending()?.kind,'refresh')
})
test('malformed or foreign shipping journals send no request',async()=>{
 const f=fixture();f.data.set(shippingIntentKey,JSON.stringify({cartID:'other',version:'2',operationID:key,optionID:'delivery',kind:'save'}));await assert.rejects(f.commands.submit('cart','2','delivery'));assert.equal(f.calls.length,0)
 f.data.set(shippingIntentKey,'{');await assert.rejects(f.commands.submit('cart','2','delivery'));assert.equal(f.calls.length,0)
})
test('fixed private bridge allows delivery routes without forwarding provider fields or cookies',async()=>{
 const config={ginOrigin:'http://127.0.0.1:9100',storefrontOrigin:'http://127.0.0.1:9300'}
 const response=await cartBridge(new Request(config.storefrontOrigin+'/api/store/carts/cart/delivery-options',{headers:{Cookie:'retail_cart_possession='+csrf+'; admin_session=secret'}}),config,async(input,init)=>{
  assert.equal(new URL(String(input)).pathname,'/api/store/carts/cart/delivery-options');assert.equal(new Headers(init?.headers).get('Cookie'),'retail_cart_possession='+csrf);return reply(options())
 });assert.equal(response.status,200)
 const denied=await cartBridge(new Request(config.storefrontOrigin+'/api/store/carts/cart/delivery-options?provider_id=foreign'),config,async()=>{throw Error('must not forward')});assert.equal(denied.status,404)
})
