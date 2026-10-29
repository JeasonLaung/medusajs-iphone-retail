import {CartError,cartCSRFFromCookies,type CartClient,type DeliveryOptions} from './cart-client.ts'
import {cartIntentKey,cartLockName,contactIntentKey,shippingIntentKey} from './cart-commerce.ts'
import type {ContactState} from './checkout-contacts.ts'
export {shippingIntentKey} from './cart-commerce.ts'
export type DeliveryState=ContactState&{delivery:DeliveryOptions}
type Intent={cartID:string;version:string;operationID:string;optionID:string;kind:'save'|'refresh'}
type Options={client:Pick<CartClient,'saveShipping'|'retrieveDelivery'|'retrieveView'|'retrieveContacts'>;storage:Pick<Storage,'getItem'|'setItem'|'removeItem'>;readCSRF:()=>string|undefined;uuid:()=>string;exclusive:<T>(action:()=>Promise<T>)=>Promise<T>}
const id=/^[A-Za-z0-9][A-Za-z0-9_-]{0,190}$/,uuid=/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/,version=/^[1-9][0-9]{0,19}$/
const valid=(v:unknown,re:RegExp):v is string=>typeof v==='string'&&!/[\r\n]/.test(v)&&re.test(v)
const fail=()=>new CartError('conflict')
// Only operation descriptors survive navigation. An acknowledged write is
// journaled before reads; recovery of that stage never repeats the POST.
export function createShippingCommands(o:Options){
 const unblocked=()=>{if(o.storage.getItem(cartIntentKey)!==null||o.storage.getItem(contactIntentKey)!==null)throw fail()}
 const pending=():Intent|null=>{
  const raw=o.storage.getItem(shippingIntentKey);if(raw===null)return null
  if(raw.length>1024)throw fail();const v=JSON.parse(raw)
  if(!v||typeof v!=='object'||Array.isArray(v)||Object.keys(v).length!==5||!valid(v.cartID,id)||!valid(v.version,version)||BigInt(v.version)>18446744073709551615n||!valid(v.operationID,uuid)||!valid(v.optionID,id)||!['save','refresh'].includes(v.kind))throw fail()
  return v
 }
 const save=(v:Intent)=>{const raw=JSON.stringify(v);o.storage.setItem(shippingIntentKey,raw);if(o.storage.getItem(shippingIntentKey)!==raw)throw fail()}
 const refresh=async(cartID:string,minimum:string):Promise<DeliveryState>=>{
  const [contacts,view,delivery]=await Promise.all([o.client.retrieveContacts(cartID),o.client.retrieveView(cartID),o.client.retrieveDelivery(cartID)])
  if(!contacts||!view||!delivery||contacts.version!==view.amounts.version||delivery.version!==contacts.version||delivery.currency_code!==view.amounts.currency_code||BigInt(delivery.version)<BigInt(minimum))throw fail()
  return {contacts,view,delivery}
 }
 const load=(cartID:string,minimum:string)=>o.exclusive(async()=>{unblocked();return refresh(cartID,minimum)})
 const recover=()=>o.exclusive(async()=>{
  unblocked();const intent=pending();if(!intent||intent.kind!=='refresh')throw fail()
  const result=await refresh(intent.cartID,intent.version);o.storage.removeItem(shippingIntentKey);return result
 })
 const submit=(cartID:string,currentVersion:string,optionID:string)=>o.exclusive(async()=>{
  unblocked();if(!valid(cartID,id)||!valid(currentVersion,version)||BigInt(currentVersion)>=18446744073709551615n||!valid(optionID,id))throw fail()
  let intent=pending()
  if(intent&&(intent.cartID!==cartID||intent.kind!=='save'||intent.optionID!==optionID))throw fail()
  if(!intent){intent={cartID,version:currentVersion,operationID:o.uuid(),optionID,kind:'save'};if(!valid(intent.operationID,uuid))throw fail();save(intent)}
  const csrfToken=o.readCSRF();if(!valid(csrfToken,/^[0-9a-f]{64}$/))throw new CartError('csrf')
  try{
   const receipt=await o.client.saveShipping(cartID,{version:intent.version,operationID:intent.operationID,optionID:intent.optionID},{csrfToken})
   intent={...intent,version:receipt.version,kind:'refresh'};save(intent)
  }catch(error){
   if(error instanceof CartError&&!error.uncertain&&[400,409,422].includes(error.status??0))o.storage.removeItem(shippingIntentKey)
   throw error
  }
  const result=await refresh(cartID,intent.version);o.storage.removeItem(shippingIntentKey);return result
 })
 return {pending,load,submit,recover}
}
export function browserShippingCommands(client:CartClient){
 if(!navigator.locks)throw new CartError('unavailable')
 return createShippingCommands({client,storage:localStorage,readCSRF:()=>cartCSRFFromCookies(document.cookie,location.protocol==='https:'),uuid:()=>crypto.randomUUID(),exclusive:action=>navigator.locks.request(cartLockName,{mode:'exclusive'},action)})
}
