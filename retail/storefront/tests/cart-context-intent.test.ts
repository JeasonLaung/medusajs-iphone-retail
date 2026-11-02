import test from 'node:test'
import assert from 'node:assert/strict'
import {createContextCommands} from '../src/lib/cart-context.ts'
import {CartError,type CartSnapshot,type CartAmounts,type CartContextInput} from '../src/lib/cart-client.ts'
import {contextIntentKey,cartIntentKey,contactIntentKey,shippingIntentKey,promotionIntentKey,customerCartIntentKey} from '../src/lib/cart-commerce.ts'
import {paymentIntentKey} from '../src/lib/checkout-payment.ts'

const operationID='f8230181-a32c-4127-838b-92b7c033ef01',csrfToken='a'.repeat(64)
const choice={regionID:'region_us',currency:'usd',locale:'zh-CN'}
function harness(){
 const data=new Map<string,string>(),posts:CartContextInput[]=[]
 const h={cart:{id:'cart_new',region_id:'region_eu',currency_code:'eur',locale:'en',sales_channel_id:'channel',version:'1'} as CartSnapshot|null,fresh:0,losePost:false,loseRead:false,readCalls:0,csrfReads:0}
 const storage={getItem:(k:string)=>data.get(k)??null,setItem:(k:string,v:string)=>{data.set(k,v)},removeItem:(k:string)=>{data.delete(k)}}
 const client={
  current:async()=>{h.readCalls++;return h.cart?{...h.cart}:null},
  saveContext:async(cartID:string,input:CartContextInput,auth:{csrfToken:string})=>{
   assert.equal(cartID,'cart_new');assert.equal(auth.csrfToken,csrfToken);posts.push({...input})
   if(h.fresh===0){h.fresh++;h.cart={...h.cart!,region_id:input.regionID,currency_code:choice.currency,locale:input.locale,version:'2'}}
   if(h.losePost){h.losePost=false;throw new CartError('unavailable',null,true)}
   return {cart_id:cartID,version:'2',replayed:posts.length>1}
  },
  retrieveAmounts:async()=>{if(h.loseRead){h.loseRead=false;throw new CartError('unavailable',503)};return {cart_id:'cart_new',version:h.cart!.version,currency_code:h.cart!.currency_code} as CartAmounts},
 }
 const options={client,storage,readCSRF:()=>{h.csrfReads++;return csrfToken},uuid:()=>operationID,exclusive:async<T>(work:()=>Promise<T>)=>work()}
 return {data,posts,h,storage,options,commands:()=>createContextCommands(options)}
}
test('lost context success retries the original persisted operation and version after reload',async()=>{
 const f=harness();f.h.losePost=true
 await assert.rejects(f.commands().apply(choice),e=>e instanceof CartError&&e.uncertain)
 assert.equal(JSON.parse(f.data.get(contextIntentKey)!).kind,'save')
 assert.equal((await f.commands().apply(choice))!.version,'2')
 assert.equal(f.h.fresh,1);assert.equal(f.posts.length,2);assert.deepEqual(f.posts[0],f.posts[1]);assert.equal(f.data.size,0)
})
test('acknowledged context recovers through reads only after amount refresh fails',async()=>{
 const f=harness();f.h.loseRead=true
 await assert.rejects(f.commands().apply(choice))
 const intent=JSON.parse(f.data.get(contextIntentKey)!)
 assert.equal(intent.kind,'refresh');assert.equal(intent.version,'2');assert.equal(intent.operationID,operationID)
 assert.equal((await f.commands().apply(choice))!.currency_code,'usd')
 assert.equal(f.posts.length,1);assert.equal(f.data.size,0)
})
test('no-cart preference preparation creates neither a cart nor a mutation journal',async()=>{
 const f=harness();f.h.cart=null
 assert.equal(await f.commands().apply(choice),null)
 assert.equal(f.posts.length,0);assert.equal(f.h.csrfReads,0);assert.equal(f.data.size,0)
 f.data.set(contextIntentKey,JSON.stringify({...choice,cartID:'cart_new',version:'1',operationID,kind:'save'}))
 f.options.client.saveContext=async()=>{throw new CartError('session',401)}
 await assert.rejects(f.commands().apply(choice));assert.ok(f.data.has(contextIntentKey))
})
test('uncertain context cannot be silently replaced by another language or region',async()=>{
 const f=harness();f.h.losePost=true;await assert.rejects(f.commands().apply(choice))
 const journal=f.data.get(contextIntentKey)
 for(const other of [{...choice,locale:'en'},{...choice,regionID:'other'},{...choice,currency:'eur'}])await assert.rejects(f.commands().apply(other))
 assert.equal(f.data.get(contextIntentKey),journal);assert.equal(f.posts.length,1)
})
test('context waits for other durable cart operations and rejects malformed stored identity',async()=>{
 for(const key of [cartIntentKey,contactIntentKey,shippingIntentKey,promotionIntentKey,customerCartIntentKey,paymentIntentKey]){
  const f=harness();f.data.set(key,'pending');await assert.rejects(f.commands().apply(choice));assert.equal(f.h.readCalls,0);assert.equal(f.posts.length,0)
 }
 for(const journal of ['null','{}','x'.repeat(1025),JSON.stringify({...choice,cartID:'cart_new',version:'01',operationID,kind:'save'}),JSON.stringify({...choice,cartID:'cart_new',version:'1',operationID,kind:'save',credential:'never'})]){
  const f=harness();f.data.set(contextIntentKey,journal);await assert.rejects(f.commands().apply(choice));assert.equal(f.posts.length,0);assert.equal(f.data.get(contextIntentKey),journal)
 }
})
test('mismatched current context cannot be treated as confirmed after an acknowledgement',async()=>{
 const f=harness();f.h.loseRead=true;await assert.rejects(f.commands().apply(choice));f.h.cart!.currency_code='eur'
 await assert.rejects(f.commands().apply(choice));assert.equal(f.posts.length,1);assert.equal(JSON.parse(f.data.get(contextIntentKey)!).kind,'refresh')
})
test('definite rejected mutation clears only its own intent; uncertain failures retain it',async()=>{
 for(const [status,uncertain] of [[409,false],[422,false],[503,true]] as const){
  const f=harness();f.options.client.saveContext=async()=>{throw new CartError(status===503?'unavailable':'conflict',status,uncertain)}
  await assert.rejects(f.commands().apply(choice));assert.equal(f.data.has(contextIntentKey),uncertain)
 }
})
