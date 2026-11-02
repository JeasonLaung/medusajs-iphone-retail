import {CartError,type CartContactReceipt} from './cart-client.ts'

export type CartCustomerBinding={
 current():Promise<string|null>
 transfer(cart:string,account:string,command:{operation_id:string;version:string}):Promise<CartContactReceipt>
}
const id=/^[A-Za-z0-9][A-Za-z0-9_-]{0,190}$/
const unavailable=()=>new CartError('unavailable',null,true)

// Called under the existing commerce lock. Do not invoke customerClient.current:
// its recovery observer can acquire that same lock recursively.
export function cartCustomerBinding(fetcher:typeof fetch,readCookies:()=>string,secure:boolean):CartCustomerBinding {
 const current=async()=>{
  const response=await fetcher('/api/store/customer/me',{credentials:'same-origin',cache:'no-store',redirect:'error'})
  if(response.status===401)return null
  if(response.status!==200)throw unavailable()
  const data=await response.json(),account=data?.customer?.id
  if(typeof account!=='string'||!id.test(account)||/[\r\n]/.test(account))throw unavailable()
  return account
 }
 return {current,async transfer(cart,account,command){
  if(await current()!==account)throw unavailable()
  const name=(secure?'__Host-':'')+'retail_customer_csrf',values=readCookies().split(';').map(s=>s.trim()).filter(s=>s.startsWith(name+'=')).map(s=>s.slice(name.length+1))
  if(values.length!==1||!/^[0-9a-f]{64}$/.test(values[0]))throw unavailable()
  const response=await fetcher('/api/store/customer/cart-transfer',{method:'POST',credentials:'same-origin',cache:'no-store',redirect:'error',headers:{'Content-Type':'application/json','X-CSRF-Token':values[0]},body:JSON.stringify(command)})
  if(response.status!==200)throw unavailable()
  const receipt=await response.json()
  if(!receipt||Object.keys(receipt).length!==3||receipt.cart_id!==cart||typeof receipt.version!=='string'||!/^[1-9][0-9]{0,19}$/.test(receipt.version)||BigInt(receipt.version)>18446744073709551615n||typeof receipt.replayed!=='boolean'||BigInt(receipt.version)<BigInt(command.version)||BigInt(receipt.version)>BigInt(command.version)+1n)throw unavailable()
  return receipt
 }}
}
