import {CartError,browserCartClient,cartCSRFFromCookies,type CartClient,type CartSnapshot,type CartView} from './cart-client.ts'
import {cartIntentKey,cartLockName,contactIntentKey,shippingIntentKey,promotionIntentKey} from './cart-commerce.ts'

type Intent={format:2;kind:'set'|'refresh';regionID:string;currency:string;segment:string;cartID:string;lineID:string;quantity:string;version:string;operationID:string}
export type ItemChange={cartID:string;version:string;lineID:string;quantity:string}
export type PendingItemChange={kind:'none'}|{kind:'blocked'}|{kind:'set'|'refresh';lineID:string;quantity:string}
type Options={
 client:Pick<CartClient,'current'|'setItem'|'retrieveView'>
 regionID:string;currency:string;segment:string
 storage:Pick<Storage,'getItem'|'setItem'|'removeItem'>
 readCSRF:()=>string|undefined;uuid:()=>string
 exclusive:<T>(action:()=>Promise<T>)=>Promise<T>
}
const id=/^[A-Za-z0-9][A-Za-z0-9_-]{0,190}$/,uuid=/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
const canonical=(s:unknown,re:RegExp):s is string=>typeof s==='string'&&!/[\r\n]/.test(s)&&re.test(s)
const version=(s:unknown):s is string=>canonical(s,/^[1-9][0-9]{0,19}$/)&&BigInt(s)<18446744073709551615n
const quantity=(s:unknown):s is string=>canonical(s,/^(0|[1-9][0-9]{0,9})$/)&&BigInt(s)<=1000000000n
const fail=()=>new CartError('input')
function parse(raw:string):Intent{
 if(raw.length>2048)throw fail()
 const v=JSON.parse(raw)
 if(!v||typeof v!=='object'||Array.isArray(v)||Object.keys(v).length!==10||v.format!==2||!['set','refresh'].includes(v.kind)||!canonical(v.regionID,id)||!canonical(v.currency,/^[a-z]{3}$/)||!canonical(v.segment,/^[a-z]{2}-(en|zh)$/)||!canonical(v.cartID,id)||!canonical(v.lineID,id)||!quantity(v.quantity)||!canonical(v.operationID,uuid)||!canonical(v.version,/^[1-9][0-9]{0,19}$/)||BigInt(v.version)>18446744073709551615n||v.kind==='set'&&!version(v.version))throw fail()
 return v as Intent
}

// Uses the SAME journal and Web Lock as product additions. A pending creation,
// addition or corrupt journal blocks quantity changes without overwriting it.
export function createCartItemCommands(o:Options){
 if(!canonical(o.regionID,id)||!canonical(o.currency,/^[a-z]{3}$/)||!canonical(o.segment,/^[a-z]{2}-(en|zh)$/))throw fail()
 const context=(p:Intent)=>p.regionID===o.regionID&&p.currency===o.currency&&p.segment===o.segment
 const validCart=(cart:CartSnapshot|null,cartID:string):cart is CartSnapshot=>!!cart&&cart.id===cartID&&cart.region_id===o.regionID&&cart.currency_code===o.currency
 const pending=():PendingItemChange=>{
  try{const raw=o.storage.getItem(cartIntentKey);if(raw===null)return {kind:'none'};const p=parse(raw);return context(p)?{kind:p.kind,lineID:p.lineID,quantity:p.quantity}:{kind:'blocked'}}catch{return {kind:'blocked'}}
 }
 const save=(intent:Intent)=>{
  const raw=JSON.stringify(intent);parse(raw);o.storage.setItem(cartIntentKey,raw)
  if(o.storage.getItem(cartIntentKey)!==raw)throw fail()
 }
 let active:{key:string;promise:Promise<CartView>}|null=null
 const execute=async(input?:ItemChange):Promise<CartView>=>o.exclusive(async()=>{
  if(o.storage.getItem(contactIntentKey)!==null||o.storage.getItem(shippingIntentKey)!==null||o.storage.getItem(promotionIntentKey)!==null)throw fail()
  const raw=o.storage.getItem(cartIntentKey),resuming=raw!==null
  let intent=raw===null?null:parse(raw)
  if(intent&&(!context(intent)||input&&(intent.cartID!==input.cartID||intent.lineID!==input.lineID||intent.quantity!==input.quantity)))throw fail()
  if(!intent){
   if(!input)throw fail()
   const current=await o.client.current()
   if(!validCart(current,input.cartID)||BigInt(current.version)<BigInt(input.version))throw fail()
   // Keep the version the user actually saw. A newer current version must not
   // silently promote an old screen's edit into a new command.
   intent={format:2,kind:'set',regionID:o.regionID,currency:o.currency,segment:o.segment,...input,operationID:o.uuid()}
   save(intent)
  }
  const current=await o.client.current()
  if(!validCart(current,intent.cartID)||BigInt(current.version)<BigInt(intent.version))throw fail()
  if(intent.kind==='set'){
   const csrf=o.readCSRF()
   if(!canonical(csrf,/^[0-9a-f]{64}$/))throw fail()
   try{
    const receipt=await o.client.setItem(intent.cartID,intent.lineID,{operationID:intent.operationID,version:intent.version,quantity:intent.quantity},{csrfToken:csrf})
    intent={...intent,kind:'refresh',version:receipt.version};save(intent)
   }catch(error){
    if(!resuming&&error instanceof CartError&&error.status!==null&&!error.uncertain)o.storage.removeItem(cartIntentKey)
    throw error
   }
  }
  const view=await o.client.retrieveView(intent.cartID)
  if(!view||view.amounts.cart_id!==intent.cartID||view.amounts.currency_code!==o.currency||BigInt(view.amounts.version)<BigInt(intent.version)||BigInt(view.amounts.version)<BigInt(current.version))throw fail()
  o.storage.removeItem(cartIntentKey)
  if(o.storage.getItem(cartIntentKey)!==null)throw fail()
  return view
 })
 const run=(input?:ItemChange)=>{
  if(input&&(!canonical(input.cartID,id)||!canonical(input.lineID,id)||!version(input.version)||!quantity(input.quantity)))return Promise.reject(fail())
  const key=input?JSON.stringify([input.cartID,input.lineID,input.quantity]):'recover'
  if(active)return active.key===key?active.promise:Promise.reject(fail())
  const promise=execute(input).finally(()=>{active=null});active={key,promise};return promise
 }
 return {enabled:true as const,pending,set:(input:ItemChange)=>run(input),recover:()=>run()}
}

export function browserCartItemCommands(context:{enabled:boolean;regionID:string;currency:string;segment:string}){
 if(!context.enabled||typeof window==='undefined'||!navigator.locks)return {enabled:false as const}
 try{return createCartItemCommands({...context,client:browserCartClient(),storage:localStorage,
  readCSRF:()=>cartCSRFFromCookies(document.cookie,location.protocol==='https:'),uuid:()=>crypto.randomUUID(),
  exclusive:action=>navigator.locks.request(cartLockName,{mode:'exclusive'},action)})}
 catch{return {enabled:false as const}}
}
