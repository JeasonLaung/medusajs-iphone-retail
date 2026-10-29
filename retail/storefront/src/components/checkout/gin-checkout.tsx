import {useEffect,useRef,useState} from 'react'
import {browserCartClient,CartError} from '../../lib/cart-client'
import {browserContactCommands,type ContactState,contactFields} from '../../lib/checkout-contacts'
import {cartIntentKey,shippingIntentKey} from '../../lib/cart-commerce'
import {customerClient,type Customer,type Address} from '../../lib/customer-client'
import type {Region} from '../../lib/regions'
import Addresses from './addresses'
import Summary from './summary'
import Shipping from './shipping'
import Progress from './checkout-progress'
import styles from './styles.module.css'

type Props={region:Region;segment:string;enabled:boolean;customerEnabled:boolean}
type Controller=ReturnType<typeof browserContactCommands>
export default function GinCheckout({region,segment,enabled,customerEnabled}:Props){
 const zh=segment.endsWith('-zh')
 const [state,setState]=useState<ContactState|null>(null),[step,setStep]=useState<'address'|'delivery'>('address'),[error,setError]=useState(false),[customer,setCustomer]=useState<(Customer&{addresses:Address[]})|null>(null),[recovery,setRecovery]=useState(false),[pendingSave,setPendingSave]=useState(false)
 const controller=useRef<Controller|null>(null),busy=useRef(false),live=useRef(false)
 const unavailable=zh?'结账暂时无法加载，请稍后重试。':'Checkout is temporarily unavailable. Please try again.'
 const retry=zh?'重试':'Try again'
 useEffect(()=>{
  live.current=true;let cancelled=false
  setState(null);setCustomer(null);setError(false)
  const accountPromise=customerEnabled?(async()=>{
   try{
    const api=customerClient((input,init)=>fetch(input,{...init,signal:AbortSignal.timeout(10000)})),current=await api.current()
    if(!current)return null
    return {...current,addresses:await api.addresses()}
   }catch{return null}
  })():Promise.resolve(null)
  const load=async()=>{
   try{
    if(!enabled||localStorage.getItem(cartIntentKey)!==null)throw Error()
    const api=browserCartClient(),commands=browserContactCommands(api),cart=await api.current()
    if(!cart){location.reload();return}
    if(cart.region_id!==region.id||cart.currency_code!==region.currency_code)throw Error()
    const [contacts,view,knownCustomer]=await Promise.all([api.retrieveContacts(cart.id),api.retrieveView(cart.id),accountPromise])
    if(!contacts||!view||contacts.version!==view.amounts.version||view.amounts.currency_code!==region.currency_code||BigInt(contacts.version)<BigInt(cart.version))throw Error()
    if(!view.items.length){location.replace(`/${segment}/cart`);return}
    if(cancelled)return
    const intent=commands.pending();if(intent&&intent.cartID!==cart.id)throw Error();setCustomer(knownCustomer);controller.current=commands;setState({contacts,view});setRecovery(intent?.kind==='refresh');setPendingSave(intent?.kind==='save')
    const currentStep=contacts.email&&contacts.shipping_address&&contacts.billing_address&&(localStorage.getItem(shippingIntentKey)!==null||new URL(location.href).searchParams.get('step')!=='address')&&intent?.kind!=='save'?'delivery':'address'
    setStep(currentStep);const url=new URL(location.href);url.searchParams.set('step',currentStep);history.replaceState(null,'',url)
   }catch{if(!cancelled)setError(true)}
  }
  void load()

  return()=>{cancelled=true;live.current=false}
 },[region.id,region.currency_code,segment,enabled,customerEnabled])
 const accept=(next:ContactState)=>{
  if(next.view.amounts.currency_code!==region.currency_code)throw Error()
  if(!live.current)return
  setState(next);setRecovery(false);setStep('delivery');const url=new URL(location.href);url.searchParams.set('step','delivery');location.assign(url.href)
 }
 const submit=async(_previous:string|null,form:FormData):Promise<string|null>=>{
  if(!state||!controller.current||busy.current)return unavailable
  busy.current=true
  try{accept(await controller.current.submit(state.contacts.cart_id,state.contacts.version,contactFields(form)));return null}
  catch(error){
   if(live.current){try{setRecovery(controller.current.pending()?.kind==='refresh');setPendingSave(controller.current.pending()?.kind==='save')}catch{setError(true)}}
   if(error instanceof CartError&&error.status===409)return zh?'购物车已更新，请刷新结账后重试。':'Your cart changed. Please refresh checkout and try again.'
   return unavailable
  }finally{busy.current=false}
 }
 const recover=async()=>{if(!controller.current||busy.current)return;busy.current=true;try{accept(await controller.current.recover())}catch{setError(true)}finally{busy.current=false}}
 if(!state)return <div className={styles.page}><p role={error?'alert':'status'} className={error?styles.error:styles.muted}>{error||!enabled?unavailable:(zh?'正在加载结账…':'Loading checkout…')}</p>{error&&<button type="button" onClick={()=>location.reload()}>{retry}</button>}</div>
 const cart={...state.contacts,region}
 return <div className={styles.page}><Progress segment={segment} step={step}/><div className={styles.columns}><div className={styles.card}>
  {recovery?<><p role="alert">{zh?'地址已提交，请重试读取结账信息。':'Your address was submitted. Retry loading checkout details.'}</p><button type="button" className={styles.primary} onClick={()=>void recover()} data-testid="contact-recover">{retry}</button></>:step==='address'?<>{pendingSave&&<p role="alert" className={styles.error}>{zh?'上次提交结果尚未确认，请保持原信息并重试。':'Your previous submission is not confirmed. Keep the original details and try again.'}</p>}<Addresses key={state.contacts.version} cart={cart} customer={customer} segment={segment} setAddresses={submit}/></>:<Shipping state={state} segment={segment} onChange={next=>setState(next)}/>}
 </div><Summary view={state.view} contacts={state.contacts} region={region} step={step} locale={segment}/></div></div>
}
