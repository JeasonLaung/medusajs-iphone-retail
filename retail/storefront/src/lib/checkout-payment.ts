import {CartError,cartCSRFFromCookies,type CartClient} from './cart-client.ts'
import {contextIntentKey,cartIntentKey,contactIntentKey,shippingIntentKey,promotionIntentKey,cartLockName,customerCartIntentKey} from './cart-commerce.ts'
export const paymentIntentKey='retail:checkout-payment-intent:v1'
type Intent={cartID:string;version:string;operationID:string;providerID?:'pp_paypal_paypal'}
type Options={client:Pick<CartClient,'startPayment'>;storage:Pick<Storage,'getItem'|'setItem'>;readCSRF:()=>string|undefined;uuid:()=>string;exclusive:<T>(action:()=>Promise<T>)=>Promise<T>}
const id=/^[A-Za-z0-9][A-Za-z0-9_-]{0,190}$/,uuid=/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
const valid=(v:unknown,re:RegExp):v is string=>typeof v==='string'&&!/[\r\n]/.test(v)&&re.test(v)
const version=(v:unknown):v is string=>valid(v,/^[1-9][0-9]{0,19}$/)&&BigInt(v)<18446744073709551615n
const fail=()=>new CartError('conflict')
export function readPaymentIntent(storage:Pick<Storage,'getItem'>):Intent|null{
 const raw=storage.getItem(paymentIntentKey);if(raw===null)return null
 if(raw.length>1024)throw fail()
 let v:Intent;try{v=JSON.parse(raw)}catch{throw fail()}
 if(!v||typeof v!=='object'||Array.isArray(v)||Object.keys(v).some(k=>!['cartID','version','operationID','providerID'].includes(k))||Object.keys(v).length!==(v.providerID===undefined?3:4)||v.providerID!==undefined&&v.providerID!=='pp_paypal_paypal'||!valid(v.cartID,id)||!valid(v.operationID,uuid)||!version(v.version))throw fail()
 return v
}
// Persist only the original intention, before dispatch. Neither provider URLs,
// customer data nor credentials belong in this journal. A timed-out request can
// already have frozen the cart: retry its original version and operation ID.
export function createPaymentCommands(o:Options){
 const pending=()=>readPaymentIntent(o.storage)
 const submit=(cartID:string,currentVersion:string,providerID?:'pp_paypal_paypal')=>o.exclusive(async()=>{
  if(!valid(cartID,id)||!version(currentVersion)||[contextIntentKey,cartIntentKey,contactIntentKey,shippingIntentKey,promotionIntentKey,customerCartIntentKey].some(k=>o.storage.getItem(k)!==null))throw fail()
  let intent=pending()
  if(providerID!==undefined&&providerID!=='pp_paypal_paypal'||intent&&(intent.cartID!==cartID||intent.providerID!==providerID))throw fail()
  const csrfToken=o.readCSRF();if(!valid(csrfToken,/^[0-9a-f]{64}$/))throw new CartError('csrf')
  if(!intent){
   intent={cartID,version:currentVersion,operationID:o.uuid(),...(providerID?{providerID}:{})};if(!valid(intent.operationID,uuid))throw fail()
   const raw=JSON.stringify(intent);o.storage.setItem(paymentIntentKey,raw);if(o.storage.getItem(paymentIntentKey)!==raw)throw fail()
  }
  // Keep the descriptor after every response, including ready and review.
  // A navigation failure must not turn into a second payment initiation.
  return o.client.startPayment(cartID,{version:intent.version,operationID:intent.operationID,...(intent.providerID?{providerID:intent.providerID}:{})},{csrfToken})
 })
 return {pending,submit}
}
export function browserPaymentCommands(client:CartClient){
 if(!navigator.locks)throw new CartError('unavailable')
 return createPaymentCommands({client,storage:localStorage,readCSRF:()=>cartCSRFFromCookies(document.cookie,location.protocol==='https:'),uuid:()=>crypto.randomUUID(),exclusive:action=>navigator.locks.request(cartLockName,{mode:'exclusive'},action)})
}

// Call only after Gin has returned an authorized completion receipt. Cleanup is
// advisory: storage failures never change the server's completed order result.
export function clearCompletedPayment(storage:Pick<Storage,'getItem'|'removeItem'>,cartID:string):void{
 try{
  const raw=storage.getItem(paymentIntentKey);if(raw===null||raw.length>1024)return
  const v=readPaymentIntent(storage)
  if(!v||v.cartID!==cartID)return
  if(storage.getItem(paymentIntentKey)===raw)storage.removeItem(paymentIntentKey)
 }catch{ /* Receipt remains authoritative when browser storage is unavailable. */ }
}
