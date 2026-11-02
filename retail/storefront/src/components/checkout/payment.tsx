import {useEffect,useRef,useState} from 'react'
import {RadioGroup,Radio} from '@headlessui/react'
import {browserCartClient,type PayPalSDKConfig} from '../../lib/cart-client'
import PayPalWallet from './paypal-wallet'
import {browserPaymentCommands} from '../../lib/checkout-payment'
import {loadPaymentReview,type PaymentReviewState} from '../../lib/checkout-payment-review'
import type {ContactState} from '../../lib/checkout-contacts'
import {getCheckoutCopy,paymentTitle} from './i18n'
import styles from './styles.module.css'
type Props={state:ContactState;segment:string;step:'payment'|'review';onChange:(state:ContactState)=>void;onStep:(step:'payment'|'review')=>void}
const hosted='pp_mollie-hosted-checkout_mollie'
export default function Payment({state,segment,step,onChange,onStep}:Props){
 const copy=getCheckoutCopy(segment),zh=segment.endsWith('-zh'),retry=zh?'重试':'Try again'
 const [loaded,setLoaded]=useState<PaymentReviewState|null>(null),[error,setError]=useState<string|null>(null),[busy,setBusy]=useState(false),[pending,setPending]=useState(false),[readyURL,setReadyURL]=useState<string|null>(null),[selected,setSelected]=useState('')
 const [wallet,setWallet]=useState<{orderID:string;config:PayPalSDKConfig}|null>(null)
 const generation=useRef(0),live=useRef(false),active=useRef(false),api=useRef<ReturnType<typeof browserCartClient>|null>(null),commands=useRef<ReturnType<typeof browserPaymentCommands>|null>(null)
 const title=paymentTitle(wallet?'pp_paypal_paypal':hosted,segment,wallet?'PayPal':'Mollie'),resultURL=`/${segment}/payment-result?cart_id=${encodeURIComponent(state.contacts.cart_id)}`
 const accept=(next:PaymentReviewState)=>{if(next.view.amounts.currency_code!==state.view.amounts.currency_code)throw Error();if(live.current){setLoaded(next);onChange(next)}}
 const prepare=async()=>{
  if(active.current||!api.current||!commands.current)return
 const epoch=generation.current;active.current=true;setBusy(true);setError(null);setReadyURL(null);setWallet(null)
  try{
   const original=commands.current.pending(),provider=original?.providerID??(selected==='pp_paypal_paypal'?'pp_paypal_paypal':undefined)
   const receipt=await commands.current.submit(state.contacts.cart_id,state.contacts.version,provider)
   if(!live.current||epoch!==generation.current)return
   setPending(true)
   if(receipt.state==='ready'||receipt.state==='sdk_ready'){
    const next=await loadPaymentReview(api.current,state.contacts.cart_id,receipt.version,true)
    if(!next.options.attempt_started)throw Error()
    if(epoch!==generation.current)return
    accept(next);if(live.current){if(receipt.state==='sdk_ready'){const config=next.options.active_provider?.paypal;if(!config||next.options.active_provider?.id!==receipt.provider_id||!receipt.provider_order_id)throw Error();setWallet({orderID:receipt.provider_order_id,config})}else setReadyURL(receipt.checkout_url);onStep('review')}
   }else setError(receipt.state==='requires_review'?copy.paymentNeedsReview:copy.paymentPending)
  }catch{if(live.current&&epoch===generation.current){setError(copy.paymentError);try{setPending(commands.current.pending()!==null)}catch{setPending(true)}}}
  finally{if(epoch===generation.current){active.current=false;if(live.current)setBusy(false)}}
 }
 useEffect(()=>{
  generation.current++;live.current=true;let cancelled=false
  const timer=setTimeout(()=>void(async()=>{
   try{
    api.current=browserCartClient();commands.current=browserPaymentCommands(api.current)
    const intent=commands.current.pending();if(intent&&intent.cartID!==state.contacts.cart_id)throw Error()
    const next=await loadPaymentReview(api.current,state.contacts.cart_id,state.contacts.version)
    if(cancelled)return
    accept(next);setSelected(next.options.providers[0]?.id??'')
    if(intent){setPending(true);await prepare()}
    else if(next.options.attempt_started){setPending(true);setError(copy.paymentPending)}
    else if(step==='review')onStep('payment')
   }catch{if(!cancelled)setError(copy.paymentLoadError)}
  })(),0)
  return()=>{cancelled=true;generation.current++;active.current=false;live.current=false;clearTimeout(timer)}
 },[state.contacts.cart_id,segment])
 const prepared=!!readyURL||!!wallet,recovering=pending&&!prepared
 const methods=prepared?[{id:wallet?'pp_paypal_paypal':hosted}]:loaded?.options.providers??[]
 return <div className={styles.payment} data-testid="checkout-payment" data-status={busy?'loading':prepared?'ready':recovering?'pending':loaded?'loaded':'error'}>
  <section aria-labelledby="payment-heading"><div className="flex items-center justify-between gap-4 mb-6"><h2 id="payment-heading" style={{marginBottom:0}}>{copy.paymentMethods}</h2>{step==='review'&&readyURL&&<button type="button" onClick={()=>onStep('payment')} data-testid="edit-payment-button">{copy.edit}</button>}</div>
   {step==='payment'&&(!pending||prepared)?<><h3 className={styles.sectionLabel}>{copy.payNow}</h3>
    {loaded&&<RadioGroup value={prepared?(wallet?'pp_paypal_paypal':hosted):selected} onChange={setSelected} aria-label={copy.paymentMethods}>{methods.map(method=><Radio key={method.id} value={method.id} disabled={busy} className="flex flex-col gap-y-2 cursor-pointer py-4 border px-8 mb-2" data-testid="payment-method-option"><div className="flex items-center justify-between"><div className="flex items-center gap-x-4"><span className={styles.paymentIndicator} data-checked={prepared||selected===method.id} aria-hidden="true"><span/></span><span>{paymentTitle(method.id,segment,method.id==='pp_paypal_paypal'?'PayPal':'Mollie')}</span></div></div></Radio>)}</RadioGroup>}
    {loaded&&!prepared&&!loaded.options.providers.length&&<p role="alert" className={styles.error}>{copy.noPayment}</p>}
    <button type="button" className={styles.primary} disabled={!prepared&&(!selected||!loaded?.options.providers.length)||busy} onClick={()=>prepared?onStep('review'):void prepare()} data-testid="submit-payment-button">{copy.reviewPayment}</button>
   </>:<><p className="font-semibold mb-2" data-testid="payment-method-summary">{title}</p><p className={styles.muted}>{wallet?copy.paypalHelp:copy.mollieHelp}</p></>}
  </section>
  {error&&<p role="alert" className={styles.error} data-testid="payment-method-error-message">{error}</p>}
  {recovering&&<><button type="button" disabled={busy} onClick={()=>void prepare()} data-testid="payment-recover">{retry}</button><p><a href={resultURL}>{copy.paymentResultTitle}</a></p></>}
  {!loaded&&error&&!pending&&<button type="button" onClick={()=>location.reload()}>{retry}</button>}
  {readyURL&&step==='review'&&<section aria-label={copy.review}><p className={styles.terms}>{copy.terms}</p><button type="button" className={styles.primary} disabled={busy} onClick={()=>{if(!active.current&&readyURL)location.assign(readyURL)}} data-testid="submit-order-button">{copy.payNow}</button></section>}
  {wallet&&step==='review'&&<PayPalWallet cartID={state.contacts.cart_id} orderID={wallet.orderID} config={wallet.config} segment={segment}/>}
 </div>
}
