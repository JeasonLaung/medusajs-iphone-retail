import {createContext,useContext,useRef,useEffect,type ReactNode} from 'react'
import {customerClient,type AuthState,type AddressFields} from '../../lib/customer-client'
import {getAccountCopy} from './i18n'
const Context=createContext<{segment:string;api:ReturnType<typeof customerClient>}|null>(null)
export function AccountContext({segment,children}:{segment:string;children:ReactNode}){
 const api=useRef<ReturnType<typeof customerClient>|null>(null)
 if(!api.current)api.current=customerClient()
 useEffect(()=>()=>api.current?.clear(),[])
 return <Context.Provider value={{segment,api:api.current}}>{children}</Context.Provider>
}
export function useAccountContext(){const context=useContext(Context);if(!context)throw new Error('Account context unavailable');return context}
export function useAccountCopy(){return getAccountCopy(useAccountContext().segment)}
export function useCustomerActions(){
 const {segment,api}=useAccountContext()
 return {login:(previous:AuthState,form:FormData)=>api.login(previous,form,segment),signup:(previous:AuthState,form:FormData)=>api.signup(previous,form,segment),resendVerificationEmail:()=>api.resend(),confirmEmailVerification:(token:string)=>api.verify(token)}
}
export function useAddressActions(billing=false){
 const {api}=useAccountContext()
 const save=async(_previous:unknown,form:FormData,id:string|null)=>{
  const fields:AddressFields={}
  for(const key of ['first_name','last_name','company','address_1','address_2','city','postal_code','province','country_code','phone'] as const){
   const value=form.get(key);if(typeof value==='string')fields[key]=value
  }
  if(billing)fields.is_default_billing=true
  else if(id===null){fields.is_default_billing=false;fields.is_default_shipping=false}
  try{await api.saveAddress(id,fields);return {success:true,error:null}}catch{return {success:false,error:'unavailable'}}
 }
 return {
  addCustomerAddress:(previous:unknown,form:FormData)=>save(previous,form,null),
  updateCustomerAddress:(previous:unknown,form:FormData)=>save(previous,form,String(form.get('addressId')??'')),
  deleteCustomerAddress:(id:string)=>api.deleteAddress(id),
 }
}
