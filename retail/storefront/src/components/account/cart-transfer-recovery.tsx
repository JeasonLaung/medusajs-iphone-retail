import {useState} from 'react'
import {useAccountContext} from './context'
import {customerCartIntentKey} from '../../lib/cart-commerce'
import {Button} from './ui'

export default function CartTransferRecovery({onSuccess}:{onSuccess?:()=>void}){
 const {api,segment}=useAccountContext(),zh=segment.endsWith('-zh')
 const [busy,setBusy]=useState(false),[failed,setFailed]=useState(false)
 let pending=false
 try{pending=typeof window!=='undefined'&&localStorage.getItem(customerCartIntentKey)!==null}catch{/* Retry remains available from the original login. */}
 if(!pending)return null
 return <div className="mt-4">
  <Button type="button" disabled={busy} data-testid="customer-cart-restore" onClick={()=>{
   if(busy)return
   setBusy(true);setFailed(false)
   void api.restoreCartTransfer().then(()=>onSuccess?.()).catch(()=>setFailed(true)).finally(()=>setBusy(false))
  }}>{zh?'恢复登录前的购物车':'Restore cart before sign in'}</Button>
  {failed&&<p role="alert">{zh?'付款状态仍待确认，请重试。':'Payment status still needs confirmation. Please try again.'}</p>}
 </div>
}
