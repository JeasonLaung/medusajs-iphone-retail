import {useEffect,useRef,useState} from 'react'
import {browserCartClient,type CartPromotions} from '../../lib/cart-client'
import {browserPromotionCommands,type PromotionState} from '../../lib/checkout-promotions'
import {paymentIntentKey} from '../../lib/checkout-payment'
import {getCheckoutCopy} from './i18n'
import styles from './styles.module.css'

type Props={cartID:string;version:string;segment:string;onChange:(state:PromotionState)=>void;onBlocked:(blocked:boolean)=>void}
export default function DiscountCode({cartID,version,segment,onChange,onBlocked}:Props){
 const copy=getCheckoutCopy(segment),zh=segment.endsWith('-zh')
 const [code,setCode]=useState(''),[loaded,setLoaded]=useState<CartPromotions|null>(null),[pending,setPending]=useState(false),[recovering,setRecovering]=useState(false),[error,setError]=useState<string|null>(null),[frozen,setFrozen]=useState(false)
 const commands=useRef<ReturnType<typeof browserPromotionCommands>|null>(null),active=useRef(false),live=useRef(false),generation=useRef(0)
 const load=async()=>{
  if(!commands.current||active.current)return
  const epoch=generation.current;active.current=true;setPending(true);setError(null)
  try{
   const intent=commands.current.pending()
   if(intent){if(intent.cartID!==cartID)throw Error();setRecovering(true);onBlocked(true);return}
   const result=await commands.current.load(cartID,version)
   if(!live.current||epoch!==generation.current)return
   setLoaded(result.promotions)
   if(result.contacts.version!==version)onChange(result)
  }catch{if(live.current&&epoch===generation.current)setError(copy.promoError)}
  finally{if(epoch===generation.current){active.current=false;if(live.current)setPending(false)}}
 }
 useEffect(()=>{
  generation.current++;live.current=true
  const timer=setTimeout(()=>{try{commands.current=browserPromotionCommands(browserCartClient());setFrozen(localStorage.getItem(paymentIntentKey)!==null);if(loaded?.version!==version){setLoaded(null);void load()}}catch{setError(copy.promoError)}},0)
  return()=>{generation.current++;active.current=false;live.current=false;clearTimeout(timer)}
 },[cartID,version,segment])
 const update=async(codes?:string[])=>{
  if(!commands.current||active.current)return
  const epoch=generation.current;active.current=true;setPending(true);setError(null);onBlocked(true)
  try{
   const intent=commands.current.pending()
   const requested=intent?.codes??codes??[]
   const result=intent?await commands.current.recover():await commands.current.submit(cartID,version,requested)
   if(!live.current||epoch!==generation.current)return
   setLoaded(result.promotions);setRecovering(false);onBlocked(false)
   if(requested.some(entry=>!result.promotions.promotions.some(p=>p.code.toLowerCase()===entry.toLowerCase())))setError(copy.promoInvalid)
   else setCode('')
   onChange(result)
  }catch{
   if(live.current&&epoch===generation.current){
    setError(copy.promoError)
    try{const intent=commands.current.pending();setRecovering(intent!==null);onBlocked(intent!==null)}catch{setRecovering(true);onBlocked(true)}
   }
  }finally{if(epoch===generation.current){active.current=false;if(live.current)setPending(false)}}
 }
 return <div className={styles.promo} data-testid="checkout-discount" aria-busy={pending}>
  <form className={styles.promoRow} onSubmit={event=>{event.preventDefault();if(code.trim()&&loaded&&!recovering)void update(Array.from(new Set([...loaded.promotions.map(p=>p.code),code.trim()])))}}>
   <label className="sr-only" htmlFor="checkout-discount-code">{copy.promotion}</label>
   <input id="checkout-discount-code" name="code" placeholder={copy.promotionPlaceholder} value={code} onChange={event=>setCode(event.target.value)} disabled={pending||recovering||frozen||!loaded} data-testid="discount-input"/>
   <button type="submit" disabled={pending||recovering||frozen||!loaded||!code.trim()} data-testid="discount-apply-button">{pending?'…':copy.apply}</button>
  </form>
  {error&&<p role="alert" className={styles.error} data-testid="discount-error-message">{error}</p>}
  {recovering?<><p role="alert">{zh?'优惠码提交结果尚未确认，请重试原操作后再继续结账。':'Your discount update is not confirmed. Retry the original update before continuing checkout.'}</p><button type="button" disabled={pending} onClick={()=>void update()} data-testid="discount-recover">{zh?'重试':'Try again'}</button></>:!loaded&&error?<button type="button" disabled={pending} onClick={()=>void load()} data-testid="discount-reload">{zh?'重试':'Try again'}</button>:null}
  {frozen&&<p className={styles.muted}>{zh?'付款已开始，暂时无法修改优惠码。':'Payment has started. Discount codes cannot be changed.'}</p>}
  {loaded?.promotions.map(p=><div key={p.id} className={styles.promoTag} data-testid="discount-code-tag"><span>{p.code}</span>{!p.is_automatic&&<button type="button" disabled={pending||recovering||frozen} aria-label={copy.removePromotion} onClick={()=>void update(loaded.promotions.filter(entry=>entry.id!==p.id).map(entry=>entry.code))}>{copy.removePromotion}</button>}</div>)}
 </div>
}
