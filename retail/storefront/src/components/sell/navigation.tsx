import {createContext,useContext,useEffect,useMemo,useState,type ReactNode} from 'react'
type Navigation={countryCode:string;router:{push:(path:string)=>void;replace:(path:string)=>void}}
const Context=createContext<Navigation|null>(null)
export function useSellNavigation(){const value=useContext(Context);if(!value)throw Error('Sell navigation unavailable');return value}
export function SellNavigation({segment,initialSteps,children}:{segment:string;initialSteps:string[];children:(steps:string[])=>ReactNode}){
 const [steps,setSteps]=useState(initialSteps)
 const router=useMemo(()=>{
  const navigate=(path:string,replace:boolean)=>{
   const root=`/${segment}/sell/iphone/`
   if(!path.startsWith(root)){window.location.assign(path);return}
   const next=path.slice(root.length).split('/')
   if(replace)history.replaceState(null,'',path);else history.pushState(null,'',path)
   setSteps(next);window.scrollTo(0,0)
  }
  return {push:(path:string)=>navigate(path,false),replace:(path:string)=>navigate(path,true)}
 },[segment])
 useEffect(()=>{
  const pop=()=>{const root=`/${segment}/sell/iphone/`;if(location.pathname.startsWith(root))setSteps(location.pathname.slice(root.length).split('/'))}
  window.addEventListener('popstate',pop);return()=>window.removeEventListener('popstate',pop)
 },[segment])
 return <Context.Provider value={{countryCode:segment,router}}>{children(steps)}</Context.Provider>
}
