import {useEffect,useRef,useState} from 'react'
import {Alert,Button,Drawer,Form,Input,Select,Space,Spin} from 'antd'
import {APIError,readVariantCreate,createVariant} from './api'
import type {VariantCreateContext,VariantCreateInput} from './api'
import type {Copy} from './i18n'
export default function VariantCreate({productID,copy,onClose,onExpired}:{productID:string;copy:Copy;onClose():void;onExpired():void}){
 const active=useRef(false)
 const [context,setContext]=useState<VariantCreateContext>(),[loadError,setLoadError]=useState(false),[reload,setReload]=useState(0),[saving,setSaving]=useState(false),[attempt,setAttempt]=useState<VariantCreateInput>(),[error,setError]=useState<'uncertain'|'conflict'|'denied'|'invalid'>()
 useEffect(()=>{const abort=new AbortController();setContext(undefined);setLoadError(false);readVariantCreate(productID,abort.signal).then(d=>{if(!abort.signal.aborted)setContext(d)}).catch(e=>{if(!abort.signal.aborted){if(e instanceof APIError&&e.status===401)onExpired();else setLoadError(true)}});return()=>abort.abort()},[productID,reload,onExpired])
 const save=async(body:VariantCreateInput)=>{
  if(active.current||!context?.enabled)return
  active.current=true;setSaving(true);setAttempt(body);setError(undefined)
  try{await createVariant(productID,body);onClose()}
  catch(e){if(e instanceof APIError&&e.status===401)onExpired();else if(e instanceof APIError&&e.status===409)setError('conflict');else if(e instanceof APIError&&(e.status===403||e.status===404))setError('denied');else if(e instanceof APIError&&e.status===400){setAttempt(undefined);setError('invalid')}else setError('uncertain')}
  finally{active.current=false;setSaving(false)}
 }
 return <Drawer open title={copy.createVariant} width="min(560px, 100vw)" onClose={()=>{if(!active.current)onClose()}} closable={!saving} maskClosable={!saving} keyboard={!saving}>
  <Space direction="vertical" size={24} className="full-width">
   <Alert type="info" showIcon message={copy.createVariantInfo}/>
   {loadError?<Space direction="vertical"><Alert type="error" showIcon message={copy.unavailable}/><Button onClick={()=>setReload(n=>n+1)}>{copy.retry}</Button></Space>:!context?<Spin/>:<>
    {!context.enabled&&<Alert type="warning" showIcon message={copy.variantCreationDisabled}/>}
    {error&&<Alert type={error==='uncertain'?'warning':'error'} showIcon message={error==='uncertain'?copy.saveUncertain:error==='invalid'?copy.invalidVariant:error==='conflict'?copy.variantCreateConflict:copy.accessDenied}/>}
    <Form layout="vertical" disabled={saving||!!attempt||!context.enabled} onFinish={(values:{title:string;sku?:string;options?:Record<string,string>})=>void save(attempt??{operation_id:crypto.randomUUID(),expected_revision:context.revision,title:values.title,sku:values.sku??'',option_value_ids:context.options.map((_,i)=>values.options?.[String(i)]??'')})}>
     <Form.Item label={copy.variantTitle} name="title" rules={[{required:true,whitespace:true,message:copy.required},{max:500,message:copy.invalidVariant}]}><Input maxLength={500}/></Form.Item>
     <Form.Item label="SKU" name="sku" extra={copy.autoSKU}><Input maxLength={512} autoComplete="off"/></Form.Item>
     {context.options.map((option,i)=><Form.Item key={option.id} label={option.title} name={['options',String(i)]} rules={[{required:true,message:copy.required}]}><Select showSearch optionFilterProp="label" options={option.values.map(v=>({value:v.id,label:v.value}))}/></Form.Item>)}
     {!attempt&&context.enabled&&<Button aria-label={copy.save} type="primary" htmlType="submit" loading={saving}>{copy.save}</Button>}
    </Form>
    {attempt&&error==='uncertain'&&<Button type="primary" loading={saving} onClick={()=>void save(attempt)}>{copy.retrySave}</Button>}
   </>}
   <Button aria-label={copy.cancel} disabled={saving} onClick={onClose}>{copy.cancel}</Button>
  </Space>
 </Drawer>
}
