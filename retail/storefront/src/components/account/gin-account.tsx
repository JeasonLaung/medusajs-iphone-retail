import CustomerOrders from './orders'
import AccountOrderDetails from './order-details'
import {useEffect,useState,useRef} from 'react'
import {Alert,Spin,Divider} from 'antd'
import AccountNav from './account-nav'
import Overview from './overview'
import ProfileName from './profile-name'
import ProfilePhone from './profile-phone'
import AddressSection from './address-section'
import type {Region} from '../../lib/regions'
import Theme from '../products/storefront-theme'
import {AccountContext,useAccountContext,useAccountCopy} from './context'
import LoginTemplate from './login-template'
import VerifyAccount from './verify-account'
import {Button} from './ui'
import Link from './localized-link'
import type {Customer} from '../../lib/customer-client'
import {watchCustomer} from '../../lib/customer-events'

function Account({enabled,profile,addresses,orders,orderID,regions}:{enabled:boolean;profile:boolean;addresses:boolean;orders:boolean;orderID:string;regions:Region[]}){
 const {api,segment}=useAccountContext(),{t}=useAccountCopy()
 const [customer,setCustomer]=useState<Customer|null>(null),[loading,setLoading]=useState(true),[failed,setFailed]=useState(false)
 const [loggingOut,setLoggingOut]=useState(false)
 const generation=useRef(0),live=useRef(false)
 const observe=async()=>{
  const run=++generation.current
  try{const current=await api.current();if(live.current&&run===generation.current){setCustomer(current);setFailed(false)}}
  catch{if(live.current&&run===generation.current)setFailed(true)}
  finally{if(live.current&&run===generation.current)setLoading(false)}
 }
 useEffect(()=>{
  live.current=true
  if(!enabled){setLoading(false);setFailed(true);return()=>{live.current=false;generation.current++}}
  const stop=watchCustomer(()=>void observe());void observe()
  return()=>{live.current=false;generation.current++;stop()}
 },[enabled,segment])
 return <div className="flex-1 small:py-12" data-testid="account-page"><div className="flex-1 content-container h-full mx-auto bg-white flex flex-col">
  <div className={customer&&!loading&&!failed?'grid grid-cols-1 small:grid-cols-[240px_minmax(0,1fr)] py-12':'py-12'}>
   {customer&&!loading&&!failed?<div><AccountNav customer={customer} selected={orders||orderID?'/account/orders':profile?'/account/profile':addresses?'/account/addresses':'/account'} loggingOut={loggingOut} onLogout={()=>{
    if(loggingOut)return
    setLoggingOut(true)
    void api.logout().then(()=>{if(live.current){generation.current++;setCustomer(null)}}).catch(()=>{if(live.current)setFailed(true)}).finally(()=>{if(live.current)setLoggingOut(false)})
   }}/></div>:null}
   <div className="min-w-0 flex-1">
   {loading?<div className="w-full flex justify-center px-8 py-8"><Spin/></div>:failed?<Alert type="error" showIcon message={segment.endsWith('-zh')?'账户暂时无法加载，请稍后重试。':'Your account is temporarily unavailable. Please try again.'} action={enabled?<Button onClick={()=>void observe()}>{t('timelineRetry')}</Button>:undefined}/>:customer?<div data-testid="customer-authenticated">
    {profile?<div className="w-full mt-6" data-testid="profile-page-wrapper"><div className="mb-8 flex flex-col gap-y-4"><h1 className="text-2xl-semi">{t('profile')}</h1><p className="text-base-regular">{t('profileDescription')}</p></div><div className="flex flex-col gap-y-8 w-full"><ProfileName customer={customer}/><Divider/><div><span>{t('email')}</span><div data-testid="customer-email"><div className="font-semibold" data-testid="profile-email-readonly">{customer.email}</div></div></div><Divider/><ProfilePhone customer={customer}/><Divider/><AddressSection key={customer.id} customer={customer} regions={regions} billing/><Divider/></div></div>:null}
    {addresses?<AddressSection key={customer.id} customer={customer} regions={regions}/>:null}
    {orders?<CustomerOrders key={customer.id}/>:null}
    {orderID?<AccountOrderDetails key={customer.id+orderID} orderID={orderID}/>:null}
    {!profile&&!addresses&&!orders&&!orderID?<Overview key={customer.id} customer={customer}/>:null}
   </div>:<LoginTemplate onSuccess={()=>void observe()}/>}
  </div></div>
  <div className="flex flex-col small:flex-row items-end justify-between small:border-t border-gray-200 py-12 gap-8"><div><h3 className="text-xl-semi mb-4">{t('questions')}</h3><span className="txt-medium">{t('helpDescription')}</span></div><div><Link href="/help" className="underline">{t('customerService')}</Link></div></div>
 </div></div>
}
export default function GinAccount({segment,enabled,verify=false,profile=false,addresses=false,orders=false,orderID='',regions=[]}:{segment:string;enabled:boolean;verify?:boolean;profile?:boolean;addresses?:boolean;orders?:boolean;orderID?:string;regions?:Region[]}){
 return <AccountContext segment={segment}><Theme locale={segment.endsWith('-zh')?'zh-CN':'en-GB'}>{verify?<div className="w-full flex justify-center px-8 py-12"><VerifyAccount/></div>:<Account enabled={enabled} profile={profile} addresses={addresses} orders={orders} orderID={orderID} regions={regions}/>}</Theme></AccountContext>
}
