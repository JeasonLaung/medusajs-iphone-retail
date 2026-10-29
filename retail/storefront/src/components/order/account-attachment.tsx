import {useEffect,useState} from 'react'
import {Alert} from 'antd'
import {customerClient,CustomerError} from '../../lib/customer-client'
import {watchCustomer} from '../../lib/customer-events'
import {Button} from '../account/ui'

// A CSRF cookie is only a login hint. Gin authenticates the customer and the
// independent cart capability; this component never infers ownership by email.
export function hasCustomerHint(){return document.cookie.split(';').some(p=>p.trim().startsWith((location.protocol==='https:'?'__Host-':'')+'retail_customer_csrf='))}
export default function AccountAttachment({orderID,segment}:{orderID:string;segment:string}){
 const [retry,setRetry]=useState(0),[failed,setFailed]=useState(false),[busy,setBusy]=useState(false)
 const zh=segment.endsWith('-zh')
 useEffect(()=>{
  let live=true
  const attach=async()=>{
   if(!hasCustomerHint())return
   setFailed(false);setBusy(true)
   const api=customerClient()
   try{
    const account=await api.current();if(!account)return
    try{await api.order(orderID);return}catch(error){if(!(error instanceof CustomerError)||error.status!==404)throw error}
    await api.attachOrder(orderID)
   }catch{if(live)setFailed(true)}finally{if(live)setBusy(false);api.clear()}
  }
  const stop=watchCustomer(()=>void attach());void attach()
  return()=>{live=false;stop()}
 },[orderID,retry])
 return failed?<Alert showIcon type="warning" data-testid="order-account-attachment-error" message={zh?'订单已创建，但尚未确认保存到当前账户。请重试。':'Your order was placed, but saving it to your account is not confirmed. Please retry.'} action={<Button disabled={busy} onClick={()=>setRetry(n=>n+1)}>{zh?'重试':'Retry'}</Button>}/>:null
}
