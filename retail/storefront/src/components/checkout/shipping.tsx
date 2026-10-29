import {useEffect,useRef,useState} from 'react'
import {browserCartClient} from '../../lib/cart-client'
import {browserShippingCommands,type DeliveryState} from '../../lib/checkout-shipping'
import type {ContactState} from '../../lib/checkout-contacts'
import {formatCartMoney} from '../cart/types'
import {getCheckoutCopy} from './i18n'
import styles from './styles.module.css'
type Props={state:ContactState;segment:string;onChange:(state:ContactState)=>void}
export default function Shipping({state,segment,onChange}:Props){
 const copy=getCheckoutCopy(segment),zh=segment.endsWith('-zh'),retry=zh?'重试':'Try again'
 const [delivery,setDelivery]=useState<DeliveryState['delivery']|null>(null),[error,setError]=useState<string|null>(null),[saving,setSaving]=useState(false),[pending,setPending]=useState<'save'|'refresh'|null>(null),[optimistic,setOptimistic]=useState<string|null>(null)
 const controller=useRef<ReturnType<typeof browserShippingCommands>|null>(null),busy=useRef(false),live=useRef(false)
 const accept=(next:DeliveryState)=>{if(next.view.amounts.currency_code!==state.view.amounts.currency_code)throw Error();if(live.current){setDelivery(next.delivery);setPending(null);onChange(next)}}
 useEffect(()=>{
  live.current=true;let cancelled=false
  void(async()=>{try{
   const commands=browserShippingCommands(browserCartClient());controller.current=commands
   const intent=commands.pending();if(intent&&intent.cartID!==state.contacts.cart_id)throw Error()
   const next=await commands.load(state.contacts.cart_id,state.contacts.version)
   if(!cancelled){accept(next);setPending(intent?.kind??null)}
  }catch{if(!cancelled)setError(copy.deliveryError)}})()
  return()=>{cancelled=true;live.current=false}
 },[state.contacts.cart_id,segment])
 const choose=async(optionID?:string)=>{
  if(!controller.current||busy.current)return;busy.current=true;setSaving(true);setError(null);setOptimistic(optionID??null)
  try{
   const commands=controller.current,intent=commands.pending()
   accept(intent?.kind==='refresh'?await commands.recover():await commands.submit(state.contacts.cart_id,state.contacts.version,optionID??intent?.optionID??''))
  }catch{if(live.current){setError(copy.deliveryError);try{setPending(controller.current.pending()?.kind??null)}catch{setPending('save')}}}
  finally{busy.current=false;if(live.current){setSaving(false);setOptimistic(null)}}
 }
 return <section aria-labelledby="shipping-heading"><h2 id="shipping-heading">{copy.shipping}</h2><p className={styles.muted}>{copy.deliveryHelp}</p>
  {!delivery&&!error&&<p role="status">{zh?'正在加载配送方式…':'Loading delivery options…'}</p>}
  {delivery&&!delivery.options.length&&<p role="alert" className={styles.error}>{copy.noDelivery}</p>}
  {[false,true].map(pickup=>{
   const options=delivery?.options.filter(option=>(option.kind==='pickup')===pickup)??[]
   if(!options.length)return null
   return <div key={String(pickup)}><h3 className={styles.sectionLabel}>{pickup?copy.pickup:copy.homeDelivery}</h3>{options.map(option=>{
    const unavailable=saving||pending!==null||option.insufficient_inventory,checked=optimistic&&delivery?.options.find(o=>o.id===optimistic)?.profile_id===option.profile_id?optimistic===option.id:!!delivery?.selected_option_ids.includes(option.id)
    return <label key={option.id} className={styles.method} data-checked={checked} data-disabled={unavailable}><input type="radio" name={'delivery-method-'+option.profile_id} value={option.id} checked={checked} disabled={unavailable} onChange={()=>void choose(option.id)} data-testid="delivery-option-radio"/><div><strong>{option.name}</strong>{option.description&&<p className={styles.muted}>{option.description}</p>}</div><strong className={styles.methodPrice}>{formatCartMoney(option.amount,state.view.amounts.currency_code,segment)}</strong></label>
   })}</div>
  })}
  {error&&<p role="alert" className={styles.error}>{error}</p>}
  {pending&&<><p role="alert">{pending==='refresh'?(zh?'配送方式已提交，请重试读取结账信息。':'Your delivery method was submitted. Retry loading checkout details.'):(zh?'上次选择结果尚未确认，请重试原选择。':'Your previous selection is not confirmed. Retry the same selection.')}</p><button type="button" disabled={saving} onClick={()=>void choose()} data-testid="delivery-recover">{retry}</button></>}
  {!delivery&&error&&<button type="button" onClick={()=>location.reload()}>{retry}</button>}
  <button type="button" className={styles.primary} disabled data-testid="submit-delivery-option-button">{copy.continuePayment}</button>
  {delivery?.complete&&<p role="status" className={styles.muted}>{copy.paymentUnavailable}</p>}
 </section>
}
