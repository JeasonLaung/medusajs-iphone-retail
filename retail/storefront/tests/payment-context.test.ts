import test from 'node:test'
import assert from 'node:assert/strict'
import {createPaymentContextCommands,PaymentContextWaiting} from '../src/lib/payment-context.ts'
import {CartError,type CartClient,type ContextPaymentProgress,type CartSnapshot} from '../src/lib/cart-client.ts'
import {paymentIntentKey} from '../src/lib/checkout-payment.ts'
import {contextIntentKey,contactIntentKey} from '../src/lib/cart-commerce.ts'

const originalID='12345678-1234-4234-8234-123456789012',operationID='22345678-1234-4234-8234-123456789012'
const choice={regionID:'region',currency:'eur',locale:'zh-CN'},csrf='a'.repeat(64)
function fixture(retain=false){
 const original={cartID:'cart',version:'4',operationID:originalID}
 const data=new Map<string,string>([[paymentIntentKey,JSON.stringify(original)]])
 let cart:CartSnapshot={id:'cart',region_id:'region',currency_code:'eur',locale:'en',sales_channel_id:'channel',version:'5'}
 let payment:{operation_id:string;version:string}|null={operation_id:originalID,version:'4'}
 let loseResponse=false,readFailure=false,mixedVersion=false,removeFailure=false,writeFailure=false,processing=false,ids=0,starts=0,mutations=0
 const calls:{version:string;operationID:string;restore:boolean}[]=[]
 const done=new Set<string>()
 const storage={getItem:(key:string)=>data.get(key)??null,setItem:(key:string,value:string)=>{if(writeFailure)throw Error('storage');data.set(key,value)},removeItem:(key:string)=>{if(removeFailure&&key===contextIntentKey)throw Error('storage');data.delete(key)}}
 const client:Pick<CartClient,'current'|'startPayment'|'contextPayment'|'retrieveAmounts'|'paymentOptions'>={
  current:async()=>{if(readFailure)throw Error('read');return {...cart}},
  startPayment:async(_id,input)=>{starts++;assert.deepEqual(input,{operationID:originalID,version:'4'});return {cart_id:'cart',version:cart.version,operation_id:originalID,state:'ready',checkout_url:'https://www.mollie.com/checkout/owned'}},
  contextPayment:async(_id,input,_auth,restore=false)=>{
   assert.ok(data.has(contextIntentKey));calls.push({version:input.version,operationID:input.operationID,restore})
   if(processing)return {state:'requires_recovery'}
   const key=input.operationID+':'+restore
   const replayed=done.has(key)
   if(!replayed){mutations++;done.add(key);cart={...cart,version:'6',locale:restore?'en':choice.locale};payment=restore?{operation_id:operationID,version:'5'}:retain?{operation_id:originalID,version:'4'}:null}
   if(loseResponse)throw new CartError('unavailable',null,true)
   const result:ContextPaymentProgress=restore?{state:'restored',completion:{cart:{...cart},payment}}:{state:'saved',receipt:{cart_id:'cart',version:'6',replayed},completion:{cart:{...cart},payment}}
   return result
  },
  retrieveAmounts:async()=>({cart_id:'cart',version:mixedVersion?'7':cart.version,currency_code:cart.currency_code,totals:{},lines:[],shipping_methods:[],pricing_recalculated:false,checkout:false}),
  paymentOptions:async()=>({cart_id:'cart',version:cart.version,attempt_started:payment!==null,providers:payment?[]:[{id:'pp_mollie-hosted-checkout_mollie'}]}),
 }
 const make=()=>createPaymentContextCommands({client,storage,readCSRF:()=>csrf,uuid:()=>{ids++;return operationID},exclusive:async work=>work()})
 return {data,storage,client,make,calls,original,ids:()=>ids,starts:()=>starts,mutations:()=>mutations,lose:(v:boolean)=>{loseResponse=v},readFail:(v:boolean)=>{readFailure=v},mixed:(v:boolean)=>{mixedVersion=v},removeFail:(v:boolean)=>{removeFailure=v},writeFail:(v:boolean)=>{writeFailure=v},processing:(v:boolean)=>{processing=v}}
}

test('payment context persists exact command before dispatch and retires only an acknowledged payment',async()=>{
 const f=fixture();const result=await f.make().apply({...choice,location:'/dk-zh/store'} as typeof choice)
 assert.equal(result.restored,false);assert.equal(result.cart.version,'6')
 assert.equal(f.data.has(paymentIntentKey),false);assert.equal(f.data.has(contextIntentKey),false)
 assert.equal(f.ids(),1);assert.equal(f.starts(),1);assert.equal(f.mutations(),1)
})
test('lost committed context response survives reconstruction with same operation and no extra initial CREATE',async()=>{
 const f=fixture();f.lose(true)
 await assert.rejects(f.make().apply(choice))
 const journal=f.data.get(contextIntentKey)!
 assert.deepEqual(JSON.parse(journal),{...choice,cartID:'cart',version:'5',operationID,kind:'payment-save'})
 assert.ok(!journal.includes(csrf)&&!journal.includes('mollie'))
 assert.equal(f.data.get(paymentIntentKey),JSON.stringify(f.original))
 f.lose(false);await f.make().apply(choice)
 assert.deepEqual(f.calls,[{operationID,version:'5',restore:false},{operationID,version:'5',restore:false}])
 assert.equal(f.ids(),1);assert.equal(f.starts(),1);assert.equal(f.mutations(),1)
})
test('retained checkout keeps original CREATE version after context effective version advances',async()=>{
 const f=fixture(true);await f.make().apply(choice)
 assert.equal(f.data.get(paymentIntentKey),JSON.stringify(f.original));assert.equal(f.data.has(contextIntentKey),false)
})
test('mixed/current read failures retain both journals and retry the same saved operation',async()=>{
 for(const mixed of [true,false]){
  const f=fixture(),retrieve=f.client.retrieveAmounts;if(mixed)f.mixed(true)
  else f.client.retrieveAmounts=async()=>{throw Error('read')}
  await assert.rejects(f.make().apply(choice))
  assert.ok(f.data.has(contextIntentKey));assert.equal(f.data.get(paymentIntentKey),JSON.stringify(f.original))
  f.mixed(false);f.client.retrieveAmounts=retrieve;await f.make().apply(choice);assert.equal(f.mutations(),1)
 }
})
test('unknown provider result stays durable and restoring uses the original command after reload',async()=>{
 const f=fixture();f.processing(true)
 await assert.rejects(f.make().apply(choice),e=>e instanceof PaymentContextWaiting&&e.state==='requires_recovery')
 f.processing(false);f.lose(true)
 await assert.rejects(f.make().restore())
 assert.equal(f.make().pending()?.kind,'payment-restore')
 f.lose(false);const restored=await f.make().apply(choice)
 assert.equal(restored.restored,true);assert.equal(restored.cart.locale,'en')
 assert.equal(f.data.get(paymentIntentKey),JSON.stringify({cartID:'cart',version:'5',operationID}))
 assert.deepEqual(f.calls.map(v=>v.restore),[false,true,true]);assert.equal(f.ids(),1);assert.equal(f.starts(),1)
})
test('failed last storage removal recovers the new payment binding without rotating the restoration',async()=>{
 const f=fixture();f.processing(true);await assert.rejects(f.make().apply(choice));f.processing(false);f.removeFail(true)
 await assert.rejects(f.make().restore())
 assert.equal(f.make().pending()?.kind,'payment-restore')
 assert.equal(JSON.parse(f.data.get(paymentIntentKey)!).operationID,operationID)
 f.removeFail(false);await f.make().restore();assert.equal(f.mutations(),1);assert.equal(f.data.has(contextIntentKey),false)
})
test('binding persistence failure after committed restore retains its original command for replay',async()=>{
 const f=fixture();f.processing(true);await assert.rejects(f.make().apply(choice));f.processing(false)
 const write=f.storage.setItem
 f.storage.setItem=(key,value)=>{if(key===paymentIntentKey)throw Error('binding storage');write(key,value)}
 await assert.rejects(f.make().restore())
 assert.equal(f.make().pending()?.kind,'payment-restore')
 assert.equal(f.data.get(paymentIntentKey),JSON.stringify(f.original))
 f.storage.setItem=write;const result=await f.make().restore()
 assert.equal(result.restored,true);assert.equal(f.mutations(),1);assert.equal(f.starts(),1);assert.equal(f.ids(),1)
 assert.deepEqual(f.calls.map(call=>call.restore),[false,true,true])
 assert.equal(f.data.get(paymentIntentKey),JSON.stringify({cartID:'cart',version:'5',operationID}))
 assert.equal(f.data.has(contextIntentKey),false)
})
test('invalid storage, competing commands, changed target and non-durable writes cannot dispatch a new context',async()=>{
 for(const mode of ['malformed','competing','write','foreign'] as const){
  const f=fixture()
  if(mode==='malformed')f.data.set(contextIntentKey,'{')
  if(mode==='competing')f.data.set(contactIntentKey,'pending')
  if(mode==='write')f.writeFail(true)
  if(mode==='foreign')f.data.set(paymentIntentKey,JSON.stringify({...f.original,cartID:'foreign'}))
  await assert.rejects(f.make().apply(choice));assert.equal(f.calls.length,0)
 }
 const f=fixture();f.processing(true);await assert.rejects(f.make().apply(choice))
 const count=f.calls.length;await assert.rejects(f.make().apply({...choice,locale:'en'}));assert.equal(f.calls.length,count)
})
