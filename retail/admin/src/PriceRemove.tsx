import {useEffect,useRef,useState} from 'react'
import {Alert,Button,Descriptions,Drawer,Space,Spin} from 'antd'
import {APIError,readPriceEdit,removePrice} from './api'
import type {PriceEditContext,PriceRemoveInput} from './api'
import type {Copy} from './i18n'
import {amountText} from './VariantPrices'

export default function PriceRemove({productID,variantID,priceID,copy,onClose,onExpired}:{productID:string;variantID:string;priceID:string;copy:Copy;onClose():void;onExpired():void}){
 const active=useRef(false)
 const [context,setContext]=useState<PriceEditContext>(),[loadError,setLoadError]=useState(false),[reload,setReload]=useState(0),[saving,setSaving]=useState(false),[attempt,setAttempt]=useState<PriceRemoveInput>(),[error,setError]=useState<'uncertain'|'conflict'|'denied'|'invalid'>()
 useEffect(()=>{const abort=new AbortController();setLoadError(false);setContext(undefined);readPriceEdit(productID,variantID,priceID,abort.signal).then(d=>{if(!abort.signal.aborted)setContext(d)}).catch(e=>{if(!abort.signal.aborted){if(e instanceof APIError&&e.status===401)onExpired();else setLoadError(true)}});return()=>abort.abort()},[productID,variantID,priceID,reload,onExpired])
 const submit=async()=>{
  if(active.current||!context?.enabled)return
  const body=attempt??{operation_id:crypto.randomUUID(),expected_revision:context.revision}
  active.current=true;setSaving(true);setAttempt(body);setError(undefined)
  try{await removePrice(productID,variantID,priceID,body);onClose()}
  catch(e){if(e instanceof APIError&&e.status===401)onExpired();else if(e instanceof APIError&&e.status===409)setError('conflict');else if(e instanceof APIError&&(e.status===403||e.status===404))setError('denied');else if(e instanceof APIError&&e.status===400)setError('invalid');else setError('uncertain')}
  finally{active.current=false;setSaving(false)}
 }
 return <Drawer open title={copy.removePrice} width="min(560px, 100vw)" onClose={()=>{if(!active.current)onClose()}} closable={!saving} maskClosable={!saving} keyboard={!saving}>
  <Space direction="vertical" size={24} className="full-width">
   <Alert type="warning" showIcon message={copy.removePriceInfo}/>
   {loadError?<Space direction="vertical"><Alert type="error" showIcon message={copy.unavailable}/><Button onClick={()=>setReload(n=>n+1)}>{copy.retry}</Button></Space>:!context?<Spin/>:<>
    <Descriptions column={1} items={[{key:'id',label:copy.priceID,children:context.id},{key:'currency',label:copy.currency,children:context.currency_code},{key:'amount',label:copy.amount,children:amountText(context.effective_amount)},{key:'min',label:copy.minQuantity,children:amountText(context.min_quantity)},{key:'max',label:copy.maxQuantity,children:amountText(context.max_quantity)},{key:'list',label:copy.priceList,children:context.price_list_id??copy.basePrice}]}/>
    {context.shared_variants>1&&<Alert type="warning" showIcon message={copy.sharedPriceRemoveInfo+' '+context.shared_variants}/>}
    {!context.enabled&&<Alert type="warning" showIcon message={copy.priceEditingDisabled}/>}
    {error&&<Alert type={error==='uncertain'?'warning':'error'} showIcon message={error==='uncertain'?copy.saveUncertain:error==='invalid'?copy.stockInputInvalid:error==='conflict'?copy.priceEditConflict:copy.accessDenied}/>}
    {!attempt&&context.enabled&&<Button danger type="primary" loading={saving} onClick={()=>void submit()}>{copy.removePrice}</Button>}
    {attempt&&error==='uncertain'&&<Button danger type="primary" loading={saving} onClick={()=>void submit()}>{copy.retrySave}</Button>}
   </>}
   <Button aria-label={copy.cancel} disabled={saving} onClick={onClose}>{copy.cancel}</Button>
  </Space>
 </Drawer>
}
