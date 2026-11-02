import {CartError,cartCSRFFromCookies,type CartClient,type CartSnapshot} from './cart-client.ts'
import {cartLockName,contextIntentKey,cartIntentKey,contactIntentKey,shippingIntentKey,promotionIntentKey,customerCartIntentKey} from './cart-commerce.ts'
import {paymentIntentKey} from './checkout-payment.ts'
import {createPaymentContextCommands,PaymentContextRestored} from './payment-context.ts'

type Choice={regionID:string;currency:string;locale:string}
type Intent=Choice&{cartID:string;version:string;operationID:string;kind:'save'|'refresh'}
type Options={
 client:Pick<CartClient,'current'|'saveContext'|'retrieveAmounts'>
 storage:Pick<Storage,'getItem'|'setItem'|'removeItem'>
 readCSRF:()=>string|undefined;uuid:()=>string
 exclusive:<T>(work:()=>Promise<T>)=>Promise<T>
}
const id=/^[A-Za-z0-9][A-Za-z0-9_-]{0,190}$/,locale=/^[A-Za-z]{2,8}(?:-[A-Za-z0-9]{1,8})*$/,uuid=/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
const canonical=(v:unknown,re:RegExp):v is string=>typeof v==='string'&&!/[\r\n]/.test(v)&&re.test(v)
const version=(v:unknown):v is string=>canonical(v,/^[1-9][0-9]{0,19}$/)&&BigInt(v)<=18446744073709551615n
const fail=()=>new CartError('conflict')
function validChoice(v:Choice){return canonical(v.regionID,id)&&canonical(v.currency,/^[a-z]{3}$/)&&canonical(v.locale,locale)&&v.locale.length<=35}
function decode(raw:string):Intent {
 if(raw.length>1024)throw fail()
 const v=JSON.parse(raw)
 if(!v||Array.isArray(v)||typeof v!=='object'||Object.keys(v).length!==7||!validChoice(v)||!canonical(v.cartID,id)||!version(v.version)||!canonical(v.operationID,uuid)||!['save','refresh'].includes(v.kind)||v.kind==='save'&&BigInt(v.version)===18446744073709551615n)throw fail()
 return v
}

// The journal stores only directory choices and retry identity. A lost success
// response reuses the exact operation/version; an acknowledged operation is
// followed by reads only. Cookies and navigation belong to the caller, after
// this resolves. No cart is created for a visitor without one.
export function createContextCommands(o:Options){
 const save=(v:Intent)=>{
  const raw=JSON.stringify(v);decode(raw);o.storage.setItem(contextIntentKey,raw)
  if(o.storage.getItem(contextIntentKey)!==raw)throw fail()
 }
 const pending=()=>{const raw=o.storage.getItem(contextIntentKey);return raw===null?null:decode(raw)}
 const confirmed=async(intent:Intent):Promise<CartSnapshot>=>{
  const [cart,amounts]=await Promise.all([o.client.current(),o.client.retrieveAmounts(intent.cartID)])
  if(!cart||!amounts||cart.id!==intent.cartID||cart.region_id!==intent.regionID||cart.currency_code!==intent.currency||cart.locale!==intent.locale||!version(cart.version)||BigInt(cart.version)<BigInt(intent.version)||amounts.cart_id!==cart.id||amounts.version!==cart.version||amounts.currency_code!==cart.currency_code)throw fail()
  return cart
 }
 const apply=(choice:Choice)=>o.exclusive(async()=>{
  if(!validChoice(choice)||[cartIntentKey,contactIntentKey,shippingIntentKey,promotionIntentKey,customerCartIntentKey,paymentIntentKey].some(k=>o.storage.getItem(k)!==null))throw fail()
  let intent=pending()
  if(intent&&(intent.regionID!==choice.regionID||intent.currency!==choice.currency||intent.locale!==choice.locale))throw fail()
  if(!intent){
   const cart=await o.client.current()
   if(!cart)return null
   if(!canonical(cart.id,id)||!version(cart.version)||BigInt(cart.version)===18446744073709551615n)throw fail()
   intent={regionID:choice.regionID,currency:choice.currency,locale:choice.locale,cartID:cart.id,version:cart.version,operationID:o.uuid(),kind:'save'}
   save(intent)
  }
  if(intent.kind==='save'){
   const csrfToken=o.readCSRF()
   if(!canonical(csrfToken,/^[0-9a-f]{64}$/))throw new CartError('csrf')
   let receipt
   try{receipt=await o.client.saveContext(intent.cartID,{operationID:intent.operationID,version:intent.version,regionID:intent.regionID,locale:intent.locale},{csrfToken})}
   catch(e){if(e instanceof CartError&&!e.uncertain&&[400,409,422].includes(e.status??0))o.storage.removeItem(contextIntentKey);throw e}
   // Persist acknowledgement before any further read can fail or unload.
   intent={...intent,kind:'refresh',version:receipt.version};save(intent)
  }
  const cart=await confirmed(intent)
  o.storage.removeItem(contextIntentKey)
  if(o.storage.getItem(contextIntentKey)!==null)throw fail()
  return cart
 })
 return {pending,apply}
}

export function browserContextCommands(client:CartClient){
 if(!navigator.locks)throw new CartError('unavailable')
 const options={client,storage:localStorage,readCSRF:()=>cartCSRFFromCookies(document.cookie,location.protocol==='https:'),uuid:()=>crypto.randomUUID(),exclusive:<T>(work:()=>Promise<T>)=>navigator.locks.request(cartLockName,{mode:'exclusive'},work)}
 const draft=createContextCommands(options),payment=createPaymentContextCommands(options)
 return {
  pending:()=>payment.available()?payment.pending():draft.pending(),
  canRestore:()=>payment.available(),
  restore:()=>payment.restore(),
  apply:async(choice:Choice)=>{
   if(payment.available()||localStorage.getItem(paymentIntentKey)!==null){
    const result=await payment.apply(choice)
    if(result.restored)throw new PaymentContextRestored()
    return result.cart
   }
   return draft.apply(choice)
  },
 }
}
