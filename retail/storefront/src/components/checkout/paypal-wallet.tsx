import {useEffect,useRef,useState} from 'react'
import {browserCartClient,cartCSRFFromCookies,type ContactAddress,type PayPalSDKConfig} from '../../lib/cart-client'
import {loadPayPalWallet,loadPayPalCard,type WalletButtons,type CardField,type CardForm} from '../../lib/paypal-sdk'
import {getCheckoutCopy} from './i18n'
import styles from './styles.module.css'

export default function PayPalWallet({cartID,orderID,config,segment,card=false,billingAddress}:{cartID:string;orderID:string;config:PayPalSDKConfig;segment:string;card?:boolean;billingAddress?:ContactAddress|null}){
 const copy=getCheckoutCopy(segment),root=useRef<HTMLDivElement>(null),busy=useRef(false),[error,setError]=useState(false),[reload,setReload]=useState(0),[loading,setLoading]=useState(true),[settling,setSettling]=useState(false)
 const form=useRef<CardForm|null>(null),submitting=useRef(false),[paying,setPaying]=useState(false),fieldsRoot=useRef<Record<string,HTMLDivElement|null>>({}),zh=segment.endsWith('-zh')
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
  let stopped=false,buttons:WalletButtons|undefined;const fields:CardField[]=[]
  setLoading(true);setError(false)
  const getSDK=async()=>{if(!card)return loadPayPalWallet(config);const csrfToken=cartCSRFFromCookies(document.cookie,location.protocol==='https:');if(!csrfToken)throw Error();const token=await browserCartClient().paypalClientToken(cartID,{csrfToken});if(stopped)throw Error();return loadPayPalCard(config,token)}
  void getSDK().then(async sdk=>{
   if(stopped||!root.current)return
   const callbacks={createOrder:async()=>{if(stopped||!/^([A-Z0-9]{17})$/.test(orderID))throw Error();return orderID},onApprove:async(data:{orderID:string})=>{if(stopped)return;if(data.orderID!==orderID){setError(true);return}await settle()},onCancel:()=>{if(!stopped){setError(true);setPaying(false);submitting.current=false}},onError:()=>{if(!stopped){setError(true);setPaying(false);submitting.current=false}}}
   if(card){
    const instance=sdk.CardFields({...callbacks,style:{input:{'font-size':'16px',color:'#000000E0'},'.invalid':{color:'#F5222D'}}});if(!instance.isEligible())throw Error()
    form.current=instance
    const definitions=[['name',instance.NameField({placeholder:zh?'持卡人姓名':'Cardholder name'})],['number',instance.NumberField({placeholder:zh?'卡号':'Card number'})],['expiry',instance.ExpiryField({placeholder:'MM/YY'})],['cvv',instance.CVVField({placeholder:'CVV'})]] as const
    for(const [name,field] of definitions){fields.push(field);const el=fieldsRoot.current[name];if(!el)throw Error();await field.render(el);if(stopped){await Promise.resolve(field.close());return}}
   }else{
    buttons=sdk.Buttons({...callbacks,fundingSource:'paypal',style:{layout:'vertical',color:'blue',shape:'rect'}})
    if(!buttons.isEligible())throw Error()
    await buttons.render(root.current)
   }
   if(!stopped)setLoading(false)
  }).catch(()=>{if(!stopped){setError(true);setLoading(false)}})
  return()=>{stopped=true;form.current=null;void buttons?.close().catch(()=>{});for(const field of fields)void Promise.resolve(field.close()).catch(()=>{})}
 },[cartID,orderID,config.client_id,config.currency,config.intent,config.environment,reload,card,segment])
 const submitCard=async()=>{
  if(submitting.current||!form.current||loading||settling)return
  submitting.current=true;setPaying(true);setError(false)
  try{
   if(!(await form.current.getState()).isFormValid)throw Error()
   const b=billingAddress
   await form.current.submit(b?{billingAddress:{addressLine1:b.address_1,addressLine2:b.address_2,adminArea1:b.province,adminArea2:b.city,postalCode:b.postal_code,countryCode:b.country_code.toUpperCase()}}:undefined)
  }catch{setError(true)}finally{submitting.current=false;setPaying(false)}
 }
 return <section data-testid={card?'paypal-card':'paypal-wallet'} aria-busy={loading||settling||paying}>
  <p className={styles.terms}>{copy.terms}</p>
  {loading&&<p role="status">{copy.paymentCheckingTitle}</p>}
  <div ref={root}/>
  {card&&<div className={styles.fields}>{(['name','number','expiry','cvv'] as const).map(name=><div key={name} className={styles.field} style={name==='name'||name==='number'?{gridColumn:'1 / -1'}:undefined}><span>{({name:zh?'持卡人姓名':'Cardholder name',number:zh?'卡号':'Card number',expiry:zh?'有效期':'Expiry date',cvv:zh?'安全码':'Security code'})[name]}</span><div style={{minHeight:48}} ref={el=>{fieldsRoot.current[name]=el}} data-testid={'paypal-card-'+name}/></div>)}</div>}
  {card&&<button type="button" disabled={loading||settling||paying||!form.current} className={styles.primary} onClick={()=>void submitCard()} data-testid="paypal-card-submit">{paying||settling?(zh?'处理中…':'Processing…'):(zh?'使用银行卡支付':'Pay by Card')}</button>}
  {error&&<div><p role="alert" className={styles.error}>{copy.paymentError}</p><button type="button" disabled={settling||paying} className={styles.primary} onClick={()=>setReload(n=>n+1)}>{segment.endsWith('-zh')?'重新加载 PayPal':'Reload PayPal'}</button></div>}
  <button type="button" disabled={settling||paying} onClick={()=>void settle(true)} data-testid="paypal-reconcile">{segment.endsWith('-zh')?'已批准付款？查询并继续':'Already approved? Check and continue'}</button>
 </section>
}
