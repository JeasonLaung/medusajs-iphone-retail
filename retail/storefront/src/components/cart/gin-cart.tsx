import {useEffect,useRef,useState} from 'react'
import CartTemplate from './index'
import {cartDisplay} from './types'
import {getCartCopy} from './i18n'
import styles from './styles.module.css'
import {browserCartClient,type CartView} from '../../lib/cart-client.ts'
import {browserCartItemCommands} from '../../lib/cart-item-commands.ts'
import {cartIntentKey} from '../../lib/cart-commerce.ts'

type Props={regionID:string;currency:string;segment:string;commerceEnabled?:boolean}
type Controller=Extract<ReturnType<typeof browserCartItemCommands>,{enabled:true}>
type State={context:string;view:CartView|null;controller:Controller;pending:boolean;failed:boolean}|null

// Initial render/SSR contains no private cart and no active mutation. The
// explicit recovery button never silently submits a saved browser intent.
export default function GinCart({regionID,currency,segment,commerceEnabled=false}:Props){
 const context=JSON.stringify([regionID,currency,segment,commerceEnabled]),copy=getCartCopy(segment)
 const retryLabel=segment.endsWith('-zh')?'重试':'Try again'
 const [state,setState]=useState<State>(null),[error,setError]=useState(false),[recovering,setRecovering]=useState(false)
 const live=useRef(''),run=useRef(0),changing=useRef(false)
 useEffect(()=>{
  live.current=context
  setRecovering(false);setError(false)
  if(!commerceEnabled)return()=>{live.current=''}
  let disposed=false
  const initialize=async()=>{
   const request=++run.current
   try{
    const controller=browserCartItemCommands({enabled:true,regionID,currency,segment})
    if(!controller.enabled)throw Error('Cart unavailable')
    const client=browserCartClient(),cart=await client.current()
    let view:CartView|null=null
    if(cart){
     if(cart.region_id!==regionID||cart.currency_code!==currency)throw Error('Cart region mismatch')
     view=await client.retrieveView(cart.id)
     if(!view||view.amounts.currency_code!==currency||BigInt(view.amounts.version)<BigInt(cart.version))throw Error('Cart unavailable')
    }
    if(!disposed&&request===run.current){setState({context,view,controller,pending:controller.pending().kind!=='none',failed:false});setError(false)}
   }catch{if(!disposed&&request===run.current)setError(true)}
  }
  void initialize()
  const changed=(event:StorageEvent)=>{if(!changing.current&&(event.key===cartIntentKey||event.key===null))void initialize()}
  window.addEventListener('storage',changed)
  return()=>{disposed=true;live.current='';window.removeEventListener('storage',changed)}
 },[context,regionID,currency,segment,commerceEnabled])
 const current=state?.context===context?state:null
 const change=async(lineID:string,quantity:string)=>{
  if(!current?.view||current.pending||current.failed||changing.current||live.current!==context)throw Error('Cart unavailable')
  changing.current=true;const request=++run.current
  try{
   const view=await current.controller.set({cartID:current.view.amounts.cart_id,version:current.view.amounts.version,lineID,quantity})
   if(live.current===context&&request===run.current)setState({...current,view,pending:false,failed:false})
  }catch(error){
   if(live.current===context&&request===run.current)setState({...current,pending:current.controller.pending().kind!=='none',failed:true})
   throw error
  }finally{changing.current=false}
 }
 const recover=async()=>{
  if(!current||recovering||changing.current||live.current!==context)return
  changing.current=true;setRecovering(true);const request=++run.current
  try{
   let view:CartView|null
   if(current.controller.pending().kind!=='none')view=await current.controller.recover()
   else{
    const client=browserCartClient(),cart=await client.current()
    if(cart&&(cart.region_id!==regionID||cart.currency_code!==currency))throw Error('Cart region mismatch')
    view=cart?await client.retrieveView(cart.id):null
    if(cart&&(!view||view.amounts.currency_code!==currency||BigInt(view.amounts.version)<BigInt(cart.version)))throw Error('Cart unavailable')
   }
   if(live.current===context&&request===run.current){setState({...current,view,pending:false,failed:false});setError(false)}
  }catch{if(live.current===context&&request===run.current)setState({...current,failed:true,pending:current.controller.pending().kind!=='none'})}
  finally{changing.current=false;if(live.current===context)setRecovering(false)}
 }
 if(!current)return <div className={styles.page}><div className={styles.container}><p role={error?'alert':'status'} className={error?styles.error:styles.muted}>{!commerceEnabled?copy.unavailable:error?copy.updateError:copy.updating}</p>{commerceEnabled&&error&&<button type="button" className={styles.primary} style={{width:'auto',marginTop:8}} onClick={()=>window.location.reload()}>{retryLabel}</button>}</div></div>
 return <>
  {(current.pending||current.failed)&&<div className={styles.page} style={{minHeight:0,paddingBottom:0}}><div className={styles.container}><p role="alert" className={styles.error}>{copy.updateError}</p><button type="button" className={styles.primary} style={{width:'auto',marginTop:8}} disabled={recovering} onClick={()=>void recover()} data-testid="cart-recover-button">{recovering?copy.updating:retryLabel}</button></div></div>}
  <CartTemplate locale={segment} customer={false} cart={current.view?cartDisplay(current.view):null} commands={{enabled:!current.pending&&!current.failed&&!recovering,updateLineItem:({lineId,quantity})=>change(lineId,String(quantity)),deleteLineItem:lineID=>change(lineID,'0')}}/>
 </>
}
