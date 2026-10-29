import {createContext,useContext,useRef,useEffect,type ReactNode} from 'react'
import {customerClient,type AuthState} from '../../lib/customer-client'
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
