import {useEffect,useRef,useState} from 'react'
import {Alert,Button,Descriptions,Drawer,Form,Input,Select,Space,Spin} from 'antd'
import {APIError,readPriceCreate,createBasePrice,stockQuantity} from './api'
import type {PriceCreateContext,PriceCreateInput} from './api'
import type {Copy} from './i18n'
export default function PriceCreate({productID,variantID,copy,onClose,onExpired}:{productID:string;variantID:string;copy:Copy;onClose():void;onExpired():void}){
 const [form]=Form.useForm<{amount:string;currency_code:string}>(),active=useRef(false)
 const [context,setContext]=useState<PriceCreateContext>(),[loadError,setLoadError]=useState(false),[reload,setReload]=useState(0),[saving,setSaving]=useState(false),[attempt,setAttempt]=useState<PriceCreateInput>(),[error,setError]=useState<'uncertain'|'conflict'|'denied'|'invalid'>()
 useEffect(()=>{const abort=new AbortController();setLoadError(false);setContext(undefined);readPriceCreate(productID,variantID,abort.signal).then(d=>{if(!abort.signal.aborted)setContext(d)}).catch(e=>{if(!abort.signal.aborted){if(e instanceof APIError&&e.status===401)onExpired();else setLoadError(true)}});return()=>abort.abort()},[productID,variantID,reload,onExpired])
 const submit=async(values:{amount:string;currency_code:string})=>{
  if(active.current||!context?.enabled)return
  const amount=stockQuantity(values.amount);if(!amount){setError('invalid');return}
  const body=attempt??{operation_id:crypto.randomUUID(),expected_revision:context.revision,currency_code:values.currency_code,amount}
  active.current=true;setSaving(true);setAttempt(body);setError(undefined)
  try{await createBasePrice(productID,variantID,body);onClose()}
  catch(e){if(e instanceof APIError&&e.status===401)onExpired();else if(e instanceof APIError&&e.status===409)setError('conflict');else if(e instanceof APIError&&(e.status===403||e.status===404))setError('denied');else if(e instanceof APIError&&e.status===400){setAttempt(undefined);setError('invalid')}else setError('uncertain')}
  finally{active.current=false;setSaving(false)}
 }
 return <Drawer open title={copy.createBasePrice} width="min(560px, 100vw)" onClose={()=>{if(!active.current)onClose()}} closable={!saving} maskClosable={!saving} keyboard={!saving}>
  <Space direction="vertical" size={24} className="full-width">
   <Alert type="info" showIcon message={copy.createBasePriceInfo}/>
   {loadError?<Space direction="vertical"><Alert type="error" showIcon message={copy.unavailable}/><Button onClick={()=>setReload(n=>n+1)}>{copy.retry}</Button></Space>:!context?<Spin/>:<>
    <Descriptions column={1} items={[{key:'set',label:copy.priceSet,children:context.price_set_id??copy.newPriceSet}]}/>
    {context.shared_variants>1&&<Alert type="warning" showIcon message={copy.sharedPriceInfo+' '+context.shared_variants}/>}
    {!context.enabled&&<Alert type="warning" showIcon message={copy.priceEditingDisabled}/>}
    {error&&<Alert type={error==='uncertain'?'warning':'error'} showIcon message={error==='uncertain'?copy.saveUncertain:error==='invalid'?copy.stockInputInvalid:error==='conflict'?copy.priceEditConflict:copy.accessDenied}/>}
    <Form form={form} layout="vertical" disabled={saving||!!attempt||!context.enabled} onFinish={submit}>
     <Form.Item label={copy.currency} name="currency_code" rules={[{required:true,message:copy.required}]}><Select showSearch options={context.currencies.map(code=>({value:code,label:code.toUpperCase(),disabled:context.existing_base_currencies.includes(code)}))}/></Form.Item>
     <Form.Item label={copy.amount} name="amount" rules={[{validator:(_,v)=>stockQuantity(v)?Promise.resolve():Promise.reject(new Error(copy.stockInputInvalid))}]}><Input inputMode="decimal" maxLength={39} autoComplete="off"/></Form.Item>
     {!attempt&&context.enabled&&<Button aria-label={copy.save} type="primary" htmlType="submit" loading={saving}>{copy.save}</Button>}
    </Form>
    {attempt&&error==='uncertain'&&<Button type="primary" loading={saving} onClick={()=>void submit({amount:attempt.amount,currency_code:attempt.currency_code})}>{copy.retrySave}</Button>}
   </>}
   <Button aria-label={copy.cancel} disabled={saving} onClick={onClose}>{copy.cancel}</Button>
  </Space>
 </Drawer>
}
