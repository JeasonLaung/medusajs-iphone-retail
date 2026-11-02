import {CartError,type CartClient,type CartSnapshot} from './cart-client.ts'
import {contextIntentKey,cartIntentKey,contactIntentKey,shippingIntentKey,promotionIntentKey,customerCartIntentKey} from './cart-commerce.ts'
import {paymentIntentKey,readPaymentIntent,bindPaymentIntent} from './checkout-payment.ts'

type Choice={regionID:string;currency:string;locale:string}
type Intent=Choice&{cartID:string;version:string;operationID:string;kind:'payment-save'|'payment-restore'}
type Options={
 client:Pick<CartClient,'current'|'startPayment'|'contextPayment'|'retrieveAmounts'|'paymentOptions'>
 storage:Pick<Storage,'getItem'|'setItem'|'removeItem'>
 readCSRF:()=>string|undefined;uuid:()=>string
 exclusive:<T>(action:()=>Promise<T>)=>Promise<T>
}
const id=/^[A-Za-z0-9][A-Za-z0-9_-]{0,190}$/,uuid=/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
const valid=(v:unknown,re:RegExp):v is string=>typeof v==='string'&&!/[\r\n]/.test(v)&&re.test(v)
const version=(v:unknown):v is string=>valid(v,/^[1-9][0-9]{0,19}$/)&&BigInt(v)<18446744073709551615n
const fail=()=>new CartError('conflict')
const choiceOK=(v:Choice)=>valid(v.regionID,id)&&valid(v.currency,/^[a-z]{3}$/)&&valid(v.locale,/^[A-Za-z]{2,8}(?:-[A-Za-z0-9]{1,8})*$/)&&v.locale.length<=35
export class PaymentContextWaiting extends CartError{
 readonly state:'processing'|'requires_recovery'
 constructor(state:'processing'|'requires_recovery'){super('conflict',null,true);this.state=state}
}
export class PaymentContextRestored extends Error{}

// This shares the existing commerce lock and context journal. Persist only
// the target and exact retry identity, never customer/provider material. Even
// definitive HTTP errors may follow a committed provider effect: keep intent.
export function createPaymentContextCommands(o:Options){
 const pending=():Intent|null=>{
  const raw=o.storage.getItem(contextIntentKey);if(raw===null)return null
  let v:Intent
  try{if(raw.length>1024)throw fail();v=JSON.parse(raw)}catch{throw fail()}
  if(!v||typeof v!=='object'||Array.isArray(v)||Object.keys(v).length!==7||!choiceOK(v)||!valid(v.cartID,id)||!version(v.version)||!valid(v.operationID,uuid)||!['payment-save','payment-restore'].includes(v.kind))throw fail()
  return v
 }
 const available=()=>{
  const raw=o.storage.getItem(contextIntentKey);if(raw===null)return false
  try{const v=JSON.parse(raw);return v?.kind==='payment-save'||v?.kind==='payment-restore'}catch{return false}
 }
 const write=(intent:Intent)=>{
  const raw=JSON.stringify(intent);o.storage.setItem(contextIntentKey,raw)
  if(o.storage.getItem(contextIntentKey)!==raw)throw fail()
  pending()
 }
 const unblocked=()=>{
  if([cartIntentKey,contactIntentKey,shippingIntentKey,promotionIntentKey,customerCartIntentKey].some(k=>o.storage.getItem(k)!==null))throw fail()
 }
 const csrf=()=>{const token=o.readCSRF();if(!valid(token,/^[0-9a-f]{64}$/))throw new CartError('csrf');return token}
 const finish=async(intent:Intent):Promise<{restored:boolean;cart:CartSnapshot}>=>{
  const result=await o.client.contextPayment(intent.cartID,{operationID:intent.operationID,version:intent.version,regionID:intent.regionID,locale:intent.locale},{csrfToken:csrf()},intent.kind==='payment-restore')
  if(!('completion' in result))throw new PaymentContextWaiting(result.state)
  const {cart,payment}=result.completion
  // Pair three current reads with the server's reauthorized binding before
  // changing either local journal. A stale/mixed response cannot release it.
  const [current,amounts,options]=await Promise.all([o.client.current(),o.client.retrieveAmounts(intent.cartID),o.client.paymentOptions(intent.cartID)])
  if(!current||!amounts||!options||current.id!==intent.cartID||current.id!==cart.id||current.version!==cart.version||current.region_id!==cart.region_id||current.sales_channel_id!==cart.sales_channel_id||current.currency_code!==cart.currency_code||current.locale!==cart.locale||amounts.cart_id!==cart.id||amounts.version!==cart.version||amounts.currency_code!==cart.currency_code||options.cart_id!==cart.id||options.version!==cart.version||options.attempt_started!==(payment!==null))throw fail()
  if(result.state==='saved'&&(cart.region_id!==intent.regionID||cart.currency_code!==intent.currency||cart.locale!==intent.locale))throw fail()
  if(payment){
   const raw=JSON.stringify(bindPaymentIntent(cart.id,payment,options,readPaymentIntent(o.storage)))
   o.storage.setItem(paymentIntentKey,raw);if(o.storage.getItem(paymentIntentKey)!==raw)throw fail()
  }else{
   o.storage.removeItem(paymentIntentKey);if(o.storage.getItem(paymentIntentKey)!==null)throw fail()
  }
  // If this final removal fails, the original context command remains durable
  // and replays safely. Never generate a new operation to recover browser I/O.
  o.storage.removeItem(contextIntentKey);if(o.storage.getItem(contextIntentKey)!==null)throw fail()
  return {restored:result.state==='restored',cart}
 }
 const apply=(choice:Choice)=>o.exclusive(async()=>{
  unblocked();if(!choiceOK(choice))throw fail()
  let intent=pending()
  if(intent&&(intent.regionID!==choice.regionID||intent.currency!==choice.currency||intent.locale!==choice.locale))throw fail()
  if(!intent){
   const original=readPaymentIntent(o.storage);if(!original)throw fail()
   const before=await o.client.current();if(!before||before.id!==original.cartID)throw fail()
   // Resolve the already approved initial CREATE before starting another
   // operation. Its unknown result must not be replaced by a new CREATE.
   const payment=await o.client.startPayment(original.cartID,{operationID:original.operationID,version:original.version,...(original.providerID?{providerID:original.providerID}:{})},{csrfToken:csrf()})
   if(payment.state!=='ready'&&payment.state!=='sdk_ready')throw new PaymentContextWaiting(payment.state==='processing'?'processing':'requires_recovery')
   const cart=await o.client.current();if(!cart||cart.id!==original.cartID||!version(cart.version))throw fail()
   // Project Choice explicitly: the caller's navigation/location is not part
   // of the private operation journal or the server command.
   intent={regionID:choice.regionID,currency:choice.currency,locale:choice.locale,cartID:cart.id,version:cart.version,operationID:o.uuid(),kind:'payment-save'}
   if(!valid(intent.operationID,uuid))throw fail();write(intent)
  }
  return finish(intent)
 })
 const restore=()=>o.exclusive(async()=>{
  unblocked();let intent=pending();if(!intent)throw fail()
  if(intent.kind!=='payment-restore'){intent={...intent,kind:'payment-restore'};write(intent)}
  return finish(intent)
 })
 return {pending,available,apply,restore}
}
