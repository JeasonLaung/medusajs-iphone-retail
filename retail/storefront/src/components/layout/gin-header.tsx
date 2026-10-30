import {useEffect,useState} from 'react'
import Header,{type HeaderProps} from './header'
import {browserCartClient} from '../../lib/cart-client'
import {customerClient} from '../../lib/customer-client'
import {watchCustomer} from '../../lib/customer-events'
import NotificationBell from '../notifications/notification-bell'
export default function GinHeader({commerceEnabled,customerEnabled=false,...props}:HeaderProps&{commerceEnabled:boolean;customerEnabled?:boolean}){
 const [count,setCount]=useState('0')
 const [signedIn,setSignedIn]=useState<string|null>(null)
 useEffect(()=>{
  if(!customerEnabled){setSignedIn(null);return}
  let live=true,generation=0
  const api=customerClient()
  const refresh=async()=>{
   const run=++generation
   try{const current=await api.current();if(live&&run===generation)setSignedIn(current?.id??null)}catch{/* Dependency failure does not assert logout. */}
  }
  const stop=watchCustomer(()=>void refresh());void refresh()
  return()=>{live=false;generation++;stop();api.clear()}
 },[customerEnabled,props.segment])
 useEffect(()=>{
  if(!commerceEnabled)return
  let live=true,run=0
  const refresh=async()=>{
   const request=++run
   try{
    const client=browserCartClient(),cart=await client.current()
    const amounts=cart?await client.retrieveAmounts(cart.id):null
    if(cart&&!amounts)return
    const count=amounts?amounts.lines.reduce((n,line)=>n+BigInt(line.quantity),0n).toString():'0'
    if(live&&request===run)setCount(count)
   }catch{/* Keep the last observed count; failures must not clear a cart. */}
  }
  const visible=()=>{if(document.visibilityState==='visible')void refresh()}
  const storage=(e:StorageEvent)=>{if(e.key==='retail.cart.intent.v1'||e.key===null)void refresh()}
  void refresh()
  window.addEventListener('retail:cart-changed',refresh)
  window.addEventListener('storage',storage)
  document.addEventListener('visibilitychange',visible)
  return()=>{live=false;window.removeEventListener('retail:cart-changed',refresh);window.removeEventListener('storage',storage);document.removeEventListener('visibilitychange',visible)}
 },[commerceEnabled,props.segment])
 const identity=customerEnabled?(signedIn?{isLoggedIn:true as const,notification:<NotificationBell key={signedIn} locale={props.locale} segment={props.segment}/>}:{isLoggedIn:false as const}):(props.isLoggedIn?{isLoggedIn:true as const,notification:props.notification}:{isLoggedIn:false as const})
 const {isLoggedIn:_isLoggedIn,notification:_notification,...base}=props
 return <Header {...base} {...identity} cartCount={commerceEnabled?count:props.cartCount}/>
}
