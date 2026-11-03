import {CartError,browserCartClient,cartCSRFFromCookies,scopeCSRFFromCookies,type CartClient,type CartSnapshot} from './cart-client.ts'
import {cartVariantQuantities} from './cart-quantities.ts'
import type {ProductCommerce,CartAddResult} from '../components/products/product-actions.tsx'
import {cartCustomerBinding,type CartCustomerBinding} from './cart-customer-binding.ts'

export const cartIntentKey='retail.cart.intent.v1'
export const contextIntentKey='retail:cart-context-intent:v1'
export const contactIntentKey='retail:checkout-contact-intent:v1'
export const shippingIntentKey='retail:checkout-shipping-intent:v1'
export const promotionIntentKey='retail:checkout-promotion-intent:v1'
export const customerCartIntentKey='retail:customer-cart-transfer:v1'
export const cartLockName='retail.cart.commerce.v1'
type Base={format:1;regionID:string;segment:string;variantID:string;quantity:'1'}
type Intent=Base & ({kind:'create';operationID:string;scopeBinding:string}|{kind:'prepare';cartID:string;version:string}|{kind:'bind';cartID:string;operationID:string;version:string;account:string}|{kind:'add';cartID:string;operationID:string;version:string}|{kind:'refresh';cartID:string;version:string})
type Options={
 client:Pick<CartClient,'current'|'openScope'|'create'|'addItem'|'retrieveAmounts'>
 regionID:string;currency:string;segment:string
 storage:Pick<Storage,'getItem'|'setItem'|'removeItem'>
 readCSRF:()=>string|undefined;readScopeCSRF:()=>string|undefined
 uuid:()=>string;bindScope:(csrf:string)=>Promise<string>
 exclusive:<T>(action:()=>Promise<T>)=>Promise<T>
 customer?:CartCustomerBinding
}
const id=/^[A-Za-z0-9][A-Za-z0-9_-]{0,190}$/,uuid=/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/,token=/^[0-9a-f]{64}$/
const canonical=(s:unknown,re:RegExp):s is string=>typeof s==='string'&&!/[\r\n]/.test(s)&&re.test(s)
const version=(s:unknown):s is string=>canonical(s,/^[1-9][0-9]{0,19}$/)&&BigInt(s)<=18446744073709551615n
const fail=()=>new CartError('input')
function readIntent(raw:string|null):Intent|null {
 if(raw===null)return null
 if(raw.length>2048)throw fail()
 const v=JSON.parse(raw)
 if(!v||typeof v!=='object'||Array.isArray(v)||v.format!==1||!canonical(v.regionID,id)||!canonical(v.segment,/^[a-z]{2}-(en|zh)$/)||!canonical(v.variantID,id)||v.quantity!=='1')throw fail()
 const fields=['format','regionID','segment','variantID','quantity','kind']
 if(v.kind==='create'){
  fields.push('operationID','scopeBinding')
  if(!canonical(v.operationID,uuid)||!canonical(v.scopeBinding,token))throw fail()
 }else if(v.kind==='add'||v.kind==='refresh'||v.kind==='prepare'||v.kind==='bind'){
  fields.push('cartID','version')
  if(!canonical(v.cartID,id)||!version(v.version))throw fail()
  if(v.kind==='add'||v.kind==='bind'){fields.push('operationID');if(!canonical(v.operationID,uuid)||v.version==='18446744073709551615')throw fail()}
  if(v.kind==='bind'){fields.push('account');if(!canonical(v.account,id))throw fail()}
 }else throw fail()
 if(Object.keys(v).length!==fields.length||fields.some(k=>!Object.hasOwn(v,k)))throw fail()
 return v as Intent
}

// All writes pass a persisted intent before the network request. No credential,
// price, address, email or provider payload is stored. SQL/Redis remain the
// authority; editing this journal grants no ownership or pricing privileges.
export function createCartCommerce(o:Options):Extract<ProductCommerce,{enabled:true}> {
 if(!canonical(o.regionID,id)||!canonical(o.currency,/^[a-z]{3}$/)||!canonical(o.segment,/^[a-z]{2}-(en|zh)$/))throw fail()
 const validCart=(cart:CartSnapshot|null):cart is CartSnapshot=>!!cart&&cart.region_id===o.regionID&&cart.currency_code===o.currency
 const save=(intent:Intent)=>{
  const raw=JSON.stringify(intent)
  readIntent(raw)
  o.storage.setItem(cartIntentKey,raw)
  if(o.storage.getItem(cartIntentKey)!==raw)throw fail()
 }
 const blocked=()=>[contextIntentKey,contactIntentKey,shippingIntentKey,promotionIntentKey,customerCartIntentKey].some(key=>o.storage.getItem(key)!==null)
 const scopeMatches=async(pending:Extract<Intent,{kind:'create'}>)=>{
  const csrf=o.readScopeCSRF()
  return canonical(csrf,token)&&await o.bindScope(csrf)===pending.scopeBinding
 }
 let active:{variantID:string;recover:boolean;promise:Promise<CartAddResult>}|null=null
 const addToCart:Extract<ProductCommerce,{enabled:true}>['addToCart']=async input=>{
  if(input.countryCode!==o.segment||input.quantity!=='1'||!canonical(input.variantId,id))return {ok:false,error:'error'}
  try{return await o.exclusive(async():Promise<CartAddResult>=>{
   if(blocked())throw fail()
   let pending=readIntent(o.storage.getItem(cartIntentKey))
   let resuming=pending!==null
   if(pending?.kind==='create'&&(!await scopeMatches(pending)||pending.regionID!==o.regionID||pending.segment!==o.segment||pending.variantID!==input.variantId))return {ok:false,error:'creation_session'}
   if(pending&&(pending.regionID!==o.regionID||pending.segment!==o.segment||pending.variantID!==input.variantId))throw fail()
   const base:Base={format:1,regionID:o.regionID,segment:o.segment,variantID:input.variantId,quantity:'1'}
   if(!pending){
    const cart=await o.client.current()
    if(cart){
     if(!validCart(cart))throw fail()
     pending={...base,kind:'add',cartID:cart.id,version:cart.version,operationID:o.uuid()}
    }else{
     const scope=await o.client.openScope()
     pending={...base,kind:'create',operationID:o.uuid(),scopeBinding:await o.bindScope(scope.csrfToken)}
    }
    save(pending)
   }
   try{
    if(pending.kind==='create'){
     const csrf=o.readScopeCSRF()
     if(!canonical(csrf,token)||await o.bindScope(csrf)!==pending.scopeBinding)return {ok:false,error:'creation_session'}
     const created=await o.client.create({operationID:pending.operationID,regionID:o.regionID,locale:o.segment.endsWith('-zh')?'zh-CN':'en-GB'},{csrfToken:csrf})
     if(!validCart(created.cart))throw fail()
     pending=o.customer?{...base,kind:'prepare',cartID:created.cart.id,version:created.cart.version}:{...base,kind:'add',cartID:created.cart.id,version:created.cart.version,operationID:o.uuid()}
     save(pending)
     resuming=false
    }
    if(pending.kind==='prepare'){
     if(!o.customer)throw fail()
     const account=await o.customer.current()
     if(account!==null&&!canonical(account,id))throw fail()
     pending=account===null?{...base,kind:'add',cartID:pending.cartID,version:pending.version,operationID:o.uuid()}:{...base,kind:'bind',cartID:pending.cartID,version:pending.version,operationID:o.uuid(),account}
     save(pending)
    }
    if(pending.kind==='bind'){
     if(!o.customer)throw fail()
     const receipt=await o.customer.transfer(pending.cartID,pending.account,{operation_id:pending.operationID,version:pending.version})
     const fresh=await o.client.current()
     if(!validCart(fresh)||fresh.id!==pending.cartID||receipt.cart_id!==pending.cartID||!version(receipt.version)||BigInt(receipt.version)<BigInt(pending.version)||BigInt(receipt.version)>BigInt(pending.version)+1n||BigInt(fresh.version)!==BigInt(receipt.version))throw fail()
     pending={...base,kind:'add',cartID:fresh.id,version:fresh.version,operationID:o.uuid()}
     save(pending)
    }
    if(pending.kind==='add'){
     const csrf=o.readCSRF()
     if(!canonical(csrf,token))throw fail()
     const receipt=await o.client.addItem(pending.cartID,{operationID:pending.operationID,version:pending.version,variantID:pending.variantID,quantity:'1'},{csrfToken:csrf})
     pending={...base,kind:'refresh',cartID:pending.cartID,version:receipt.version}
     save(pending)
    }
   }catch(error){
    if(pending.kind==='create'&&error instanceof CartError&&error.status===401)return {ok:false,error:'creation_session'}
    // Only a definite HTTP rejection permits a later fresh user operation.
    // Missing cookies, storage faults, malformed replies or unknown errors keep
    // the original intent: never guess that its POST did not commit.
    if(pending.kind!=='prepare'&&pending.kind!=='bind'&&!resuming&&error instanceof CartError&&error.status!==null&&!error.uncertain)o.storage.removeItem(cartIntentKey)
    throw error
   }
   const current=await o.client.current()
   if(!validCart(current)||current.id!==pending.cartID||BigInt(current.version)<BigInt(pending.version))throw fail()
   const amounts=await o.client.retrieveAmounts(pending.cartID)
   if(!amounts||amounts.currency_code!==o.currency||BigInt(amounts.version)<BigInt(current.version))throw fail()
   const quantities=cartVariantQuantities(amounts.lines)
   o.storage.removeItem(cartIntentKey)
   if(o.storage.getItem(cartIntentKey)!==null)throw fail()
   return {ok:true,quantities}
  })}catch{return {ok:false,error:'error'}}
 }
 // Explicit recovery is limited to creation: that stage can only have created
 // an empty cart, never added an item or started a payment. Never discard an
 // uncertain add/bind/refresh. Read failures retain the original journal.
 const recoverCreation:NonNullable<Extract<ProductCommerce,{enabled:true}>['recoverCreation']>=async input=>{
  try{return await o.exclusive(async():Promise<CartAddResult>=>{
   if(input.countryCode!==o.segment||input.quantity!=='1'||!canonical(input.variantId,id)||blocked())throw fail()
   const pending=readIntent(o.storage.getItem(cartIntentKey))
   if(pending?.kind!=='create')throw fail()
   const base:Base={format:1,regionID:o.regionID,segment:o.segment,variantID:input.variantId,quantity:'1'}
   const cart=await o.client.current()
   if(cart){
    if(!validCart(cart))throw fail()
    const amounts=await o.client.retrieveAmounts(cart.id)
    if(!amounts||amounts.currency_code!==o.currency||amounts.version!==cart.version||amounts.lines.length)throw fail()
    save(o.customer?{...base,kind:'prepare',cartID:cart.id,version:cart.version}:{...base,kind:'add',cartID:cart.id,version:cart.version,operationID:o.uuid()})
   }else{
    const scope=await o.client.openScope(),scopeBinding=await o.bindScope(scope.csrfToken)
    // A still-valid scope must replay its original UUID and request body.
    // Only a new scope may start a new empty-cart creation operation.
    if(scopeBinding===pending.scopeBinding&&(pending.regionID!==o.regionID||pending.segment!==o.segment))throw fail()
    save({...base,kind:'create',scopeBinding,operationID:scopeBinding===pending.scopeBinding?pending.operationID:o.uuid()})
   }
   return {ok:true,quantities:{}}
  })}catch{return {ok:false,error:'creation_session'}}
 }
 const run=(input:Parameters<typeof addToCart>[0],recover:boolean):Promise<CartAddResult>=>{
  if(active)return active.recover===recover&&active.variantID===input.variantId&&input.countryCode===o.segment&&input.quantity==='1'?active.promise:Promise.resolve({ok:false,error:'error'})
  const promise=(recover?recoverCreation(input):addToCart(input)).finally(()=>{active=null})
  active={variantID:input.variantId,recover,promise}
  return promise
 }
 return {enabled:true,addToCart:input=>run(input,false),recoverCreation:input=>run(input,true)}
}

// Explicit opt-in only; catalog checkout=false must never turn this on.
// Requires Web Locks and durable intent storage before exposing mutations.
export function browserCartCommerce(context:{enabled:boolean;regionID:string;currency:string;segment:string}):ProductCommerce {
 if(!context.enabled||typeof window==='undefined'||!navigator.locks||!crypto.subtle)return {enabled:false}
 try {
  return createCartCommerce({...context,client:browserCartClient(),storage:localStorage,
   ...(import.meta.env.PUBLIC_CUSTOMER_CART_TRANSFER_ENABLED==='true'?{customer:cartCustomerBinding((input,init)=>fetch(input,init),()=>document.cookie,location.protocol==='https:')}:{}),
   readCSRF:()=>cartCSRFFromCookies(document.cookie,location.protocol==='https:'),
   readScopeCSRF:()=>scopeCSRFFromCookies(document.cookie,location.protocol==='https:'),
   uuid:()=>crypto.randomUUID(),bindScope:async csrf=>Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256',new TextEncoder().encode(csrf))),b=>b.toString(16).padStart(2,'0')).join(''),
   exclusive:action=>navigator.locks.request(cartLockName,{mode:'exclusive'},action)})
 }catch{return {enabled:false}}
}
