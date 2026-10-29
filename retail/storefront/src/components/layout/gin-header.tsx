import {useEffect,useState} from 'react'
import Header,{type HeaderProps} from './header'
import {browserCartClient} from '../../lib/cart-client'
export default function GinHeader({commerceEnabled,...props}:HeaderProps&{commerceEnabled:boolean}){
 const [count,setCount]=useState('0')
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
 return <Header {...props} cartCount={commerceEnabled?count:props.cartCount}/>
}
