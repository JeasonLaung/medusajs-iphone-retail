import {paymentIntentKey,readPaymentIntent,bindPaymentIntent} from './checkout-payment.ts'
import {CartClient,type CartSnapshot,type CheckoutPaymentOptions} from './cart-client.ts'
import {contextIntentKey,cartIntentKey,contactIntentKey,shippingIntentKey,promotionIntentKey,cartLockName,customerCartIntentKey} from './cart-commerce.ts'

export {customerCartIntentKey} from './cart-commerce.ts'
type Command={operation_id:string;version:string}
type Intent={account:string;cart:string;command:Command;restoring?:true}
type Options={
 account:string
 current:()=>Promise<CartSnapshot|null>
 paymentOptions?:(id:string)=>Promise<CheckoutPaymentOptions|null>
 restore?:boolean
 resumeOnly?:boolean
 send:(command:Command,restore?:boolean)=>Promise<unknown>
 storage:Pick<Storage,'getItem'|'setItem'|'removeItem'>
 uuid:()=>string
 exclusive:<T>(work:()=>Promise<T>)=>Promise<T>
}
const id=/^[A-Za-z0-9][A-Za-z0-9_-]{0,190}$/,uuid=/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
const canonical=(value:unknown,pattern:RegExp):value is string=>typeof value==='string'&&!/[\r\n]/.test(value)&&pattern.test(value)
const version=(value:unknown):value is string=>canonical(value,/^[1-9][0-9]{0,19}$/)&&BigInt(value)<=18446744073709551615n
const fail=()=>new Error('Cart transfer unavailable')
function parse(raw:string):Intent {
 if(raw.length>1024)throw fail()
 const p=JSON.parse(raw)
 if(!p||Object.keys(p).some(k=>!['account','cart','command','restoring'].includes(k))||Object.keys(p).length!==(p.restoring===true?4:3)||!canonical(p.account,id)||!canonical(p.cart,id)||!p.command||Object.keys(p.command).length!==2||!canonical(p.command.operation_id,uuid)||!version(p.command.version)||BigInt(p.command.version)===18446744073709551615n)throw fail()
 return p
}

// Same cart lock as product/checkout edits. The journal holds only identifiers
// and retry identity; cookies, email and passwords never enter browser storage.
export async function transferCustomerCart(o:Options):Promise<void> {
 if(!canonical(o.account,id))throw fail()
 await o.exclusive(async()=>{
  for(const key of [contextIntentKey,cartIntentKey,contactIntentKey,shippingIntentKey,promotionIntentKey])if(o.storage.getItem(key)!==null)throw fail()
  const raw=o.storage.getItem(customerCartIntentKey)
  if(o.resumeOnly&&raw===null)return
  const current=await o.current(),paymentRaw=o.storage.getItem(paymentIntentKey)
  const payment=readPaymentIntent(o.storage)
  if(!current){if(raw!==null||payment!==null)throw fail();return}
  if(payment){
   if(payment.cartID!==current.id||BigInt(payment.version)+1n>BigInt(current.version)||!o.paymentOptions)throw fail()
   const options=await o.paymentOptions(current.id)
   if(!options||options.cart_id!==current.id||options.version!==current.version||!options.attempt_started&&raw===null)throw fail()
   // The server coordinates ownership and any change to the frozen amount.
  }
  const intent:Intent=raw===null?{account:o.account,cart:current.id,command:{operation_id:o.uuid(),version:current.version}}:parse(raw)
  if(intent.account!==o.account||intent.cart!==current.id||BigInt(current.version)<BigInt(intent.command.version))throw fail()
  if(o.restore){if(raw===null)throw fail();intent.restoring=true}
  const encoded=JSON.stringify(intent);parse(encoded)
  if(raw!==encoded){o.storage.setItem(customerCartIntentKey,encoded);if(o.storage.getItem(customerCartIntentKey)!==encoded)throw fail()}
  let receipt:unknown
  try {receipt=await o.send(intent.command,intent.restoring)}catch(error){
   const status=error&&typeof error==='object'&&'status' in error?error.status:null
   if(typeof status==='number'&&[400,401,403,404,409,413,415,429].includes(status))o.storage.removeItem(customerCartIntentKey)
   throw error
  }
  if(!receipt||typeof receipt!=='object'||Array.isArray(receipt))throw fail()
  const r=receipt as Record<string,unknown>
  if('state' in r){
   if(['processing','requires_recovery'].includes(String(r.state))&&Object.keys(r).length===1)throw fail()
   if(Object.keys(r).some(k=>!['state','receipt','completion'].includes(k))||!(r.state==='saved'||intent.restoring&&r.state==='restored')||!r.completion||typeof r.completion!=='object'||Array.isArray(r.completion))throw fail()
   const completion=r.completion as Record<string,any>,snapshot=completion.cart,binding=completion.payment
   if(Object.keys(completion).length!==2||!snapshot||snapshot.id!==intent.cart||!version(snapshot.version)||BigInt(snapshot.version)!==BigInt(intent.command.version)+1n||!(binding===null||binding&&Object.keys(binding).length===2&&canonical(binding.operation_id,uuid)&&version(binding.version)&&BigInt(binding.version)<BigInt(snapshot.version)))throw fail()
   const fresh=await o.current()
   if(!fresh||fresh.id!==intent.cart||fresh.version!==snapshot.version)throw fail()
   const methods=await o.paymentOptions?.(intent.cart)
   if(!methods||methods.cart_id!==intent.cart||methods.version!==fresh.version||methods.attempt_started!==(binding!==null))throw fail()
   if(o.storage.getItem(customerCartIntentKey)!==encoded||o.storage.getItem(paymentIntentKey)!==paymentRaw)throw fail()
   const nextPayment=binding===null?null:JSON.stringify(bindPaymentIntent(intent.cart,binding,methods,payment))
   if(nextPayment===null)o.storage.removeItem(paymentIntentKey)
   else o.storage.setItem(paymentIntentKey,nextPayment)
   if(o.storage.getItem(paymentIntentKey)!==nextPayment)throw fail()
   o.storage.removeItem(customerCartIntentKey)
   if(o.storage.getItem(customerCartIntentKey)!==null)throw fail()
   return
  }
  if(Object.keys(r).length!==3||r.cart_id!==intent.cart||!version(r.version)||typeof r.replayed!=='boolean'||BigInt(r.version)<BigInt(intent.command.version)||BigInt(r.version)>BigInt(intent.command.version)+1n)throw fail()
  const fresh=await o.current()
  if(!fresh||fresh.id!==intent.cart||BigInt(fresh.version)<BigInt(r.version)||BigInt(fresh.version)<BigInt(current.version))throw fail()
  o.storage.removeItem(customerCartIntentKey)
  if(o.storage.getItem(customerCartIntentKey)!==null)throw fail()
 })
}

export async function browserTransferCustomerCart(account:string,fetcher:typeof fetch,send:Options['send'],restore=false,resumeOnly=false) {
 // Keep the native Window.fetch receiver out of CartClient's method call.
 const client=new CartClient({fetcher:(input,init)=>fetcher(input,init),readScopeCSRF:()=>undefined})
 // Reading no cart needs neither storage nor the Web Locks API.
 if(!await client.current()){
  if(typeof window!=='undefined'&&window.localStorage.getItem(customerCartIntentKey)!==null)throw fail()
  return
 }
 if(typeof window==='undefined'||!navigator.locks)throw fail()
 await transferCustomerCart({account,current:()=>client.current(),paymentOptions:id=>client.paymentOptions(id),send,restore,resumeOnly,storage:window.localStorage,uuid:()=>crypto.randomUUID(),exclusive:work=>navigator.locks.request(cartLockName,work)})
 window.dispatchEvent(new Event('retail:cart-changed'))
}
