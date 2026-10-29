import {CartError,cartCSRFFromCookies,contactAddress,type CartClient,type CartContactInput,type CartContactDetail,type CartView} from './cart-client.ts'
import {cartIntentKey,cartLockName,contactIntentKey,shippingIntentKey} from './cart-commerce.ts'
export {contactIntentKey} from './cart-commerce.ts'
type Fields=Omit<CartContactInput,'operationID'|'version'>
type Intent={cartID:string;version:string;operationID:string;hash:string;kind:'save'|'refresh'}
type Options={client:Pick<CartClient,'saveContacts'|'retrieveContacts'|'retrieveView'>;storage:Pick<Storage,'getItem'|'setItem'|'removeItem'>;readCSRF:()=>string|undefined;uuid:()=>string;hash:(value:string)=>Promise<string>;exclusive:<T>(action:()=>Promise<T>)=>Promise<T>}
const id=/^[A-Za-z0-9][A-Za-z0-9_-]{0,190}$/,uuid=/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/,version=/^[1-9][0-9]{0,19}$/,hash=/^[0-9a-f]{64}$/
const valid=(v:unknown,re:RegExp):v is string=>typeof v==='string'&&!/[\r\n]/.test(v)&&re.test(v)
const fail=()=>new CartError('conflict')
export function contactFields(form:FormData):Fields {
 const email=String(form.get('email')??'')
 if(email.trim().toLowerCase()!==String(form.get('confirm_email')??'').trim().toLowerCase())throw new CartError('input')
 const address=(prefix:string)=>contactAddress(Object.fromEntries(['first_name','last_name','company','address_1','address_2','city','country_code','province','postal_code','phone'].map(k=>[k,String(form.get(prefix+'.'+k)??'')])))
 const shipping_address=address('shipping_address')
 return {email,shipping_address,billing_address:form.get('same_as_billing')==='on'?{...shipping_address}:address('billing_address')}
}
// A durable operation descriptor contains no contact values or credentials.
// Unknown outcomes retain the same operation/version/hash across reloads; a
// changed form cannot silently turn a retry into a second operation.
export function createContactCommands(o:Options){
 const pending=():Intent|null=>{
  const raw=o.storage.getItem(contactIntentKey);if(raw===null)return null
  if(raw.length>1024)throw fail();const v=JSON.parse(raw)
  if(!v||typeof v!=='object'||Array.isArray(v)||Object.keys(v).length!==5||!valid(v.cartID,id)||!valid(v.version,version)||BigInt(v.version)>18446744073709551615n||!valid(v.operationID,uuid)||!valid(v.hash,hash)||!['save','refresh'].includes(v.kind))throw fail()
  return v
 }
 const save=(v:Intent)=>{const raw=JSON.stringify(v);o.storage.setItem(contactIntentKey,raw);if(o.storage.getItem(contactIntentKey)!==raw)throw fail()}
 const refresh=async(cartID:string,minimum:string)=>{
  // Independent private reads must agree; a cross-tab mutation produces a
  // retryable conflict rather than displaying contacts beside stale totals.
  const [contacts,view]=await Promise.all([o.client.retrieveContacts(cartID),o.client.retrieveView(cartID)])
  if(!contacts||!view||contacts.version!==view.amounts.version||BigInt(contacts.version)<BigInt(minimum))throw fail()
  return {contacts,view}
 }
 const recover=()=>o.exclusive(async()=>{
  if((o.storage.getItem(cartIntentKey)!==null||o.storage.getItem(shippingIntentKey)!==null))throw fail()
  const intent=pending();if(!intent||intent.kind!=='refresh')throw fail()
  const result=await refresh(intent.cartID,intent.version);o.storage.removeItem(contactIntentKey);return result
 })
 const submit=(cartID:string,currentVersion:string,fields:Fields)=>o.exclusive(async()=>{
  if((o.storage.getItem(cartIntentKey)!==null||o.storage.getItem(shippingIntentKey)!==null)||!valid(cartID,id)||!valid(currentVersion,version)||BigInt(currentVersion)>=18446744073709551615n)throw fail()
  let intent=pending()
  if(intent&&(intent.cartID!==cartID||intent.kind!=='save'))throw fail()
  const expected=intent?.version??currentVersion
  const values={version:expected,email:fields.email,shipping_address:contactAddress(fields.shipping_address),billing_address:contactAddress(fields.billing_address)}
  const digest=await o.hash(JSON.stringify(values));if(!valid(digest,hash))throw fail()
  if(intent&&intent.hash!==digest)throw fail()
  if(!intent){intent={cartID,version:expected,operationID:o.uuid(),hash:digest,kind:'save'};if(!valid(intent.operationID,uuid))throw fail();save(intent)}
  const csrfToken=o.readCSRF();if(!valid(csrfToken,hash))throw new CartError('csrf')
  try{
   const receipt=await o.client.saveContacts(cartID,{...values,operationID:intent.operationID},{csrfToken})
   // Save the acknowledgement before reads; recovery after this point performs
   // reads only, even if page navigation or amount retrieval fails.
   intent={...intent,version:receipt.version,kind:'refresh'};save(intent)
  }catch(error){
   if(error instanceof CartError&&!error.uncertain&&[400,409,422].includes(error.status??0))o.storage.removeItem(contactIntentKey)
   throw error
  }
  const result=await refresh(cartID,intent.version);o.storage.removeItem(contactIntentKey);return result
 })
 return {pending,submit,recover}
}
export function browserContactCommands(client:CartClient){
 if(!navigator.locks)throw new CartError('unavailable')
 return createContactCommands({client,storage:localStorage,readCSRF:()=>cartCSRFFromCookies(document.cookie,location.protocol==='https:'),uuid:()=>crypto.randomUUID(),hash:async value=>Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256',new TextEncoder().encode(value)))).map(n=>n.toString(16).padStart(2,'0')).join(''),exclusive:action=>navigator.locks.request(cartLockName,{mode:'exclusive'},action)})
}
export type ContactState={contacts:CartContactDetail;view:CartView}
