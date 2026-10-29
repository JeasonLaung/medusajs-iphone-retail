import {useEffect,useState,useRef} from 'react'
import {Alert,Spin,Divider} from 'antd'
import ProfileName from './profile-name'
import ProfilePhone from './profile-phone'
import Theme from '../products/storefront-theme'
import {AccountContext,useAccountContext,useAccountCopy} from './context'
import LoginTemplate from './login-template'
import VerifyAccount from './verify-account'
import {Button} from './ui'
import Link from './localized-link'
import type {Customer} from '../../lib/customer-client'
import {watchCustomer} from '../../lib/customer-events'

function Account({enabled,profile}:{enabled:boolean;profile:boolean}){
 const {api,segment}=useAccountContext(),{t}=useAccountCopy()
 const [customer,setCustomer]=useState<Customer|null>(null),[loading,setLoading]=useState(true),[failed,setFailed]=useState(false)
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
  <div className="py-12"><div className="min-w-0 flex-1">
   {loading?<div className="w-full flex justify-center px-8 py-8"><Spin/></div>:failed?<Alert type="error" showIcon message={segment.endsWith('-zh')?'账户暂时无法加载，请稍后重试。':'Your account is temporarily unavailable. Please try again.'} action={enabled?<Button onClick={()=>void observe()}>{t('timelineRetry')}</Button>:undefined}/>:customer?<div data-testid="customer-authenticated">
    <div className="text-xl-semi flex flex-wrap gap-3 justify-between items-center mb-4"><span>{t('hello',{name:customer.first_name})}</span><span className="text-small-regular text-ui-fg-base">{t('signedIn')} <span className="font-semibold" data-testid="customer-email">{customer.email}</span></span></div>
    <Link href="/account/profile" className="underline" data-testid="profile-link">{t('profile')}</Link>
    {profile?<div className="w-full mt-6" data-testid="profile-page-wrapper"><div className="mb-8 flex flex-col gap-y-4"><h1 className="text-2xl-semi">{t('profile')}</h1><p className="text-base-regular">{t('profileDescription')}</p></div><div className="flex flex-col gap-y-8 w-full"><ProfileName customer={customer}/><Divider/><div><span>{t('email')}</span><div className="font-semibold" data-testid="profile-email-readonly">{customer.email}</div></div><Divider/><ProfilePhone customer={customer}/><Divider/></div></div>:null}
    <Alert type="info" message={segment.endsWith('-zh')?'账户资料与交易记录暂时无法加载。':'Account details and transaction records are temporarily unavailable.'}/>
    <Button className="mt-6" onClick={async()=>{try{await api.logout();setCustomer(null)}catch{setFailed(true)}}}>{t('logout')}</Button>
   </div>:<LoginTemplate onSuccess={()=>void observe()}/>}
  </div></div>
  <div className="flex flex-col small:flex-row items-end justify-between small:border-t border-gray-200 py-12 gap-8"><div><h3 className="text-xl-semi mb-4">{t('questions')}</h3><span className="txt-medium">{t('helpDescription')}</span></div><div><Link href="/help" className="underline">{t('customerService')}</Link></div></div>
 </div></div>
}
export default function GinAccount({segment,enabled,verify=false,profile=false}:{segment:string;enabled:boolean;verify?:boolean;profile?:boolean}){
 return <AccountContext segment={segment}><Theme locale={segment.endsWith('-zh')?'zh-CN':'en-GB'}>{verify?<div className="w-full flex justify-center px-8 py-12"><VerifyAccount/></div>:<Account enabled={enabled} profile={profile}/>}</Theme></AccountContext>
}
