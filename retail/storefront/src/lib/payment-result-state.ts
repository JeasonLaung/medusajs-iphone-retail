import {CartError,type CartClient} from './cart-client.ts'
export type PaymentResultState={status:'pending'|'failed'|'invalid'|'error'|'confirmed';orderID?:string}
type PaymentAPI=Pick<CartClient,'current'|'paymentStatus'|'complete'>
// A return URL is only a routing hint. Every operation is authorized by the
// existing HttpOnly possession and CSRF. Closed carts recover via the durable
// completion receipt, never a local-storage flag or a provider URL parameter.
export async function checkPaymentResult(api:PaymentAPI,cartID:string|undefined,csrfToken:string|undefined):Promise<PaymentResultState>{
 try{
  if(!csrfToken)return {status:'invalid'}
  const cart=await api.current()
  if(cartID&&cart&&cartID!==cart.id)return {status:'invalid'}
  const id=cart?.id??cartID
  if(!id)return {status:'invalid'}
  const finish=async():Promise<PaymentResultState>=>{
   const receipt=await api.complete(id,{csrfToken})
   return {status:'confirmed',orderID:receipt.order_id}
  }
  if(!cart)return await finish()
  let state
  try{state=await api.paymentStatus(id,{csrfToken})}
  catch(error){
   // Another tab may have completed the order between current() and this
   // poll. Only the authenticated completion endpoint can settle that race.
   if(error instanceof CartError&&error.status===404)return await finish()
   throw error
  }
  if(state.state==='paid_pending_order')return await finish()
  if(state.state==='failed')return {status:'failed'}
  return {status:state.state==='requires_review'?'error':'pending'}
 }catch(error){
  return {status:error instanceof CartError&&(error.status===401||error.status===404||error.kind==='input')?'invalid':'error'}
 }
}
