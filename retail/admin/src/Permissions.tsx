import {createContext,useCallback,useContext,useEffect,useState} from 'react'
import type {ReactNode} from 'react'
import {Alert,Button,Result,Space,Spin} from 'antd'
import {APIError,getPermissions} from './api'
import type {Action,Permissions} from './api'
import type {Copy} from './i18n'

type Access={loading:boolean;scope?:Permissions['scope'];error?:unknown;can(...actions:Action[]):boolean;retry():void}
const AccessContext=createContext<Access>({loading:true,can:()=>false,retry:()=>{}})
export const useAccess=()=>useContext(AccessContext)
export function PermissionsProvider({children,onExpired}:{children:ReactNode;onExpired():void}){
 const [attempt,setAttempt]=useState(0)
 const [state,setState]=useState<{loading:boolean;data?:Permissions;error?:unknown}>({loading:true})
 useEffect(()=>{
  let live=true,controller:AbortController|undefined
  const check=()=>{
   if(document.visibilityState!=='visible')return
   controller?.abort();controller=new AbortController();const current=controller
   getPermissions(current.signal).then(data=>{if(live&&!current.signal.aborted)setState({loading:false,data})}).catch(error=>{
    if(!live||current.signal.aborted)return
    setState({loading:false,error});if(error instanceof APIError&&error.status===401)onExpired()
   })
  }
  check();const timer=window.setInterval(check,30_000)
  window.addEventListener('focus',check);document.addEventListener('visibilitychange',check)
  return()=>{live=false;controller?.abort();clearInterval(timer);window.removeEventListener('focus',check);document.removeEventListener('visibilitychange',check)}
 },[attempt,onExpired])
 const retry=useCallback(()=>{setState({loading:true});setAttempt(n=>n+1)},[])
 const can=(...actions:Action[])=>actions.length>0&&state.data?.scope==='employee'&&actions.every(a=>state.data!.actions.includes(a))
 return <AccessContext.Provider value={{loading:state.loading,scope:state.data?.scope,error:state.error,can,retry}}>{children}</AccessContext.Provider>
}
export function RequireAction({action,copy,children}:{action:Action;copy:Copy;children:ReactNode}){
 const access=useAccess()
 if(access.loading)return <Spin/>
 if(access.error!==undefined)return <Space direction="vertical"><Alert type="error" showIcon message={copy.unavailable}/><Button onClick={access.retry}>{copy.retry}</Button></Space>
 if(!access.can(action))return <Result status="403" title={copy.accessDenied}/>
 return children
}
