import {CartError,cartCSRFFromCookies,validPromotionCode,type CartClient,type CartPromotions} from './cart-client.ts'
import {cartIntentKey,cartLockName,contactIntentKey,shippingIntentKey,promotionIntentKey} from './cart-commerce.ts'
import {paymentIntentKey} from './checkout-payment.ts'
import type {ContactState} from './checkout-contacts.ts'
export type PromotionState=ContactState&{promotions:CartPromotions}
type Intent={cartID:string;version:string;operationID:string;kind:'save'|'refresh';codes:string[]}
type Options={client:Pick<CartClient,'retrieveContacts'|'retrieveView'|'retrievePromotions'|'savePromotions'>;storage:Pick<Storage,'getItem'|'setItem'|'removeItem'>;readCSRF:()=>string|undefined;uuid:()=>string;exclusive:<T>(action:()=>Promise<T>)=>Promise<T>}
const id=/^[A-Za-z0-9][A-Za-z0-9_-]{0,190}$/,uuid=/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
const valid=(v:unknown,re:RegExp):v is string=>typeof v==='string'&&!/[\r\n]/.test(v)&&re.test(v)
const version=(v:unknown):v is string=>valid(v,/^[1-9][0-9]{0,19}$/)&&BigInt(v)<=18446744073709551615n
const codes=(v:unknown):v is string[]=>Array.isArray(v)&&v.length<=100&&v.every(validPromotionCode)&&new TextEncoder().encode(JSON.stringify(v)).length<=16000
const fail=()=>new CartError('conflict')
export function createPromotionCommands(o:Options){
 const unblocked=()=>{if([cartIntentKey,contactIntentKey,shippingIntentKey,paymentIntentKey].some(key=>o.storage.getItem(key)!==null))throw fail()}
 const pending=():Intent|null=>{
  const raw=o.storage.getItem(promotionIntentKey);if(raw===null)return null
  if(raw.length>32768)throw fail()
  let v:Intent;try{v=JSON.parse(raw)}catch{throw fail()}
  if(!v||typeof v!=='object'||Array.isArray(v)||Object.keys(v).length!==5||!valid(v.cartID,id)||!version(v.version)||!valid(v.operationID,uuid)||!['save','refresh'].includes(v.kind)||!codes(v.codes))throw fail()
  return v
 }
 const save=(intent:Intent)=>{const raw=JSON.stringify(intent);o.storage.setItem(promotionIntentKey,raw);if(o.storage.getItem(promotionIntentKey)!==raw)throw fail()}
 const refresh=async(cartID:string,minimum:string):Promise<PromotionState>=>{
  const [contacts,view,promotions]=await Promise.all([o.client.retrieveContacts(cartID),o.client.retrieveView(cartID),o.client.retrievePromotions(cartID)])
  if(!contacts||!view||!promotions||contacts.cart_id!==cartID||view.amounts.cart_id!==cartID||promotions.cart_id!==cartID||contacts.version!==view.amounts.version||contacts.version!==promotions.version||BigInt(contacts.version)<BigInt(minimum))throw fail()
  return {contacts,view,promotions}
 }
 const dispatch=async(intent:Intent):Promise<PromotionState>=>{
  if(intent.kind==='save'){
   const csrfToken=o.readCSRF();if(!valid(csrfToken,/^[0-9a-f]{64}$/))throw new CartError('csrf')
   try{
    const receipt=await o.client.savePromotions(intent.cartID,{version:intent.version,operationID:intent.operationID,codes:intent.codes},{csrfToken})
    intent={...intent,version:receipt.version,kind:'refresh'};save(intent)
   }catch(error){if(error instanceof CartError&&!error.uncertain&&[400,409,413,415,422].includes(error.status??0))o.storage.removeItem(promotionIntentKey);throw error}
  }
  const result=await refresh(intent.cartID,intent.version);o.storage.removeItem(promotionIntentKey);return result
 }
 const load=(cartID:string,minimum:string)=>o.exclusive(async()=>{if(!valid(cartID,id)||!version(minimum)||pending())throw fail();return refresh(cartID,minimum)})
 const recover=()=>o.exclusive(async()=>{unblocked();const intent=pending();if(!intent)throw fail();return dispatch(intent)})
 const submit=(cartID:string,currentVersion:string,requested:string[])=>o.exclusive(async()=>{
  unblocked();if(!valid(cartID,id)||!version(currentVersion)||BigInt(currentVersion)>=18446744073709551615n||!codes(requested))throw fail()
  let intent=pending()
  if(intent&&(intent.cartID!==cartID||intent.kind!=='save'||JSON.stringify(intent.codes)!==JSON.stringify(requested)))throw fail()
  if(!intent){intent={cartID,version:currentVersion,operationID:o.uuid(),kind:'save',codes:[...requested]};if(!valid(intent.operationID,uuid))throw fail();save(intent)}
  return dispatch(intent)
 })
 return {pending,load,recover,submit}
}
export function browserPromotionCommands(client:CartClient){
 if(!navigator.locks)throw new CartError('unavailable')
 return createPromotionCommands({client,storage:localStorage,readCSRF:()=>cartCSRFFromCookies(document.cookie,location.protocol==='https:'),uuid:()=>crypto.randomUUID(),exclusive:action=>navigator.locks.request(cartLockName,{mode:'exclusive'},action)})
}
