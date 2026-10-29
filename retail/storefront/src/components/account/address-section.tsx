import {useEffect,useRef,useState} from 'react'
import {Alert,Spin} from 'antd'
import type {Address,Customer} from '../../lib/customer-client'
import type {Region} from '../../lib/regions'
import {watchCustomer} from '../../lib/customer-events'
import {useAccountContext,useAccountCopy} from './context'
import AddressBook from './address-book'
import ProfileBillingAddress from './profile-billing-address'
import {Button} from './ui'

export default function AddressSection({customer,regions,billing=false}:{customer:Customer;regions:Region[];billing?:boolean}){
 const {api,segment}=useAccountContext(),{t}=useAccountCopy()
 const [addresses,setAddresses]=useState<Address[]>([]),[loading,setLoading]=useState(true),[failed,setFailed]=useState(false)
 const generation=useRef(0),live=useRef(false)
 const observe=async()=>{
  const run=++generation.current
  try{const value=await api.addresses();if(live.current&&generation.current===run){setAddresses(value);setFailed(false)}}
  catch{if(live.current&&generation.current===run)setFailed(true)}
  finally{if(live.current&&generation.current===run)setLoading(false)}
 }
 useEffect(()=>{
  live.current=true;setLoading(true)
  const stop=watchCustomer(()=>void observe());void observe()
  return()=>{live.current=false;generation.current++;stop()}
 },[customer.id])
 if(loading)return <div className="py-8 flex justify-center"><Spin/></div>
 if(failed)return <Alert type="error" showIcon message={t('error')} action={<Button onClick={()=>void observe()}>{t('timelineRetry')}</Button>}/>
 const owner={...customer,addresses},region=regions.find(region=>region.countries.some(country=>country.iso_2===segment.slice(0,2)))
 if(billing)return <ProfileBillingAddress customer={owner} regions={regions}/>
 return <div className="w-full" data-testid="addresses-page-wrapper"><div className="mb-8 flex flex-col gap-y-4"><h1 className="text-2xl-semi">{t('shippingAddresses')}</h1><p className="text-base-regular">{t('addressDescription')}</p></div>{region?<AddressBook customer={owner} region={region}/>:<Alert type="error" message={t('error')}/>}</div>
}
