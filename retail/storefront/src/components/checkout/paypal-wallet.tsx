import {useEffect,useRef,useState} from 'react'
import {browserCartClient,cartCSRFFromCookies,type PayPalSDKConfig} from '../../lib/cart-client'
import {loadPayPalWallet,type WalletButtons} from '../../lib/paypal-sdk'
import {getCheckoutCopy} from './i18n'
import styles from './styles.module.css'

export default function PayPalWallet({cartID,orderID,config,segment}:{cartID:string;orderID:string;config:PayPalSDKConfig;segment:string}){
 const copy=getCheckoutCopy(segment),root=useRef<HTMLDivElement>(null),busy=useRef(false),[error,setError]=useState(false),[reload,setReload]=useState(0),[loading,setLoading]=useState(true),[settling,setSettling]=useState(false)
 const settle=async(observe=false)=>{
  if(busy.current)return
  busy.current=true;setSettling(true);setError(false)
  try{
   const csrfToken=cartCSRFFromCookies(document.cookie,location.protocol==='https:');if(!csrfToken)throw Error()
   const api=browserCartClient(),result=await (observe?api.paymentStatus(cartID,{csrfToken}):api.settlePayPal(cartID,{csrfToken}))
   if(result.state==='requires_review'||result.state==='failed')throw Error()
   location.assign(`/${segment}/payment-result?cart_id=${encodeURIComponent(cartID)}`)
  }catch{setError(true)}finally{busy.current=false;setSettling(false)}
 }
 useEffect(()=>{
  let stopped=false,buttons:WalletButtons|undefined
  setLoading(true);setError(false)
  void loadPayPalWallet(config).then(async sdk=>{
   if(stopped||!root.current)return
   buttons=sdk.Buttons({fundingSource:'paypal',style:{layout:'vertical',color:'blue',shape:'rect'},createOrder:async()=>{if(stopped||!/^([A-Z0-9]{17})$/.test(orderID))throw Error();return orderID},onApprove:async data=>{if(stopped)return;if(data.orderID!==orderID){setError(true);return}await settle()},onCancel:()=>{if(!stopped)setError(true)},onError:()=>{if(!stopped)setError(true)}})
   if(!buttons.isEligible())throw Error()
   await buttons.render(root.current);if(!stopped)setLoading(false)
  }).catch(()=>{if(!stopped){setError(true);setLoading(false)}})
  return()=>{stopped=true;void buttons?.close().catch(()=>{})}
 },[cartID,orderID,config.client_id,config.currency,config.intent,config.environment,reload])
 return <section data-testid="paypal-wallet" aria-busy={loading||settling}>
  <p className={styles.terms}>{copy.terms}</p>
  {loading&&<p role="status">{copy.paymentCheckingTitle}</p>}
  <div ref={root}/>
  {error&&<div><p role="alert" className={styles.error}>{copy.paymentError}</p><button type="button" disabled={settling} className={styles.primary} onClick={()=>setReload(n=>n+1)}>{segment.endsWith('-zh')?'重新加载 PayPal':'Reload PayPal'}</button></div>}
  <button type="button" disabled={settling} onClick={()=>void settle(true)} data-testid="paypal-reconcile">{segment.endsWith('-zh')?'已批准付款？查询并继续':'Already approved? Check and continue'}</button>
 </section>
}
