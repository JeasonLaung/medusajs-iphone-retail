import {useEffect,useRef,useState} from 'react'
import {Alert,Progress,Spin,Statistic} from 'antd'
import type {Address,Customer} from '../../lib/customer-client'
import {watchCustomer} from '../../lib/customer-events'
import {useAccountContext,useAccountCopy} from './context'
import {Button} from './ui'

export default function Overview({customer}:{customer:Customer}){
 const {api}=useAccountContext(),{t}=useAccountCopy()
 const [addresses,setAddresses]=useState<Address[]|null>(null),[failed,setFailed]=useState(false)
 const generation=useRef(0),live=useRef(false)
 const observe=async()=>{
  const run=++generation.current
  try{const value=await api.addresses();if(live.current&&run===generation.current){setAddresses(value);setFailed(false)}}
  catch{if(live.current&&run===generation.current)setFailed(true)}
 }
 useEffect(()=>{
  live.current=true;setAddresses(null);setFailed(false)
  const stop=watchCustomer(()=>void observe());void observe()
  return()=>{live.current=false;generation.current++;stop()}
 },[customer.id])
 const completion=addresses===null?null:25*(Number(!!customer.email)+Number(!!(customer.first_name&&customer.last_name))+Number(!!customer.phone)+Number(addresses.some(address=>address.is_default_billing)))
 return <div data-testid="overview-page-wrapper"><div className="hidden small:block">
  <div className="text-xl-semi flex flex-wrap gap-3 justify-between items-center mb-4"><span data-testid="welcome-message" data-value={customer.first_name}>{t('hello',{name:customer.first_name})}</span><span className="text-small-regular text-ui-fg-base">{t('signedIn')} <span className="font-semibold" data-testid="customer-email" data-value={customer.email}>{customer.email}</span></span></div>
  <div className="flex flex-col py-8 border-t border-gray-200"><div className="flex flex-col gap-y-4 h-full col-span-1 row-span-2 flex-1">
   {failed?<Alert type="error" showIcon message={t('error')} data-testid="overview-address-error" action={<Button onClick={()=>void observe()}>{t('timelineRetry')}</Button>}/>:addresses===null?<div className="py-8 flex justify-center"><Spin/></div>:<div className="flex items-start gap-x-16 mb-6">
    <div className="flex flex-col gap-y-4"><h3 className="text-large-semi">{t('profile')}</h3><div className="flex items-end gap-x-2"><div data-testid="customer-profile-completion" data-value={completion}><Progress type="circle" size={72} percent={completion??0}/></div><span className="uppercase text-base-regular text-ui-fg-subtle">{t('completed')}</span></div></div>
    <div className="flex flex-col gap-y-4"><h3 className="text-large-semi">{t('addresses')}</h3><div className="flex items-end gap-x-2"><div data-testid="addresses-count" data-value={addresses.length}><Statistic value={addresses.length}/></div><span className="uppercase text-base-regular text-ui-fg-subtle">{t('saved')}</span></div></div>
   </div>}
   <div className="flex flex-col gap-y-4"><div className="flex items-center gap-x-2"><h3 className="text-large-semi">{t('recentOrders')}</h3></div><Alert type="info" message={t('transactionsUnavailable')} data-testid="overview-orders-unavailable"/></div>
  </div></div>
 </div></div>
}
