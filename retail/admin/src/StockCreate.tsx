import {useEffect,useRef,useState} from 'react'
import {Alert,Button,Drawer,Form,Input,Select,Space,Spin} from 'antd'
import {APIError,readCreateLevel,createInventoryLevel,stockQuantity} from './api'
import type {CreateLevelContext,CreateLevelInput} from './api'
import type {Copy} from './i18n'
type Fields={location_id:string;stocked_quantity:string;incoming_quantity:string}
export default function StockCreate({productID,variantID,itemID,copy,onClose,onExpired}:{productID:string;variantID:string;itemID:string;copy:Copy;onClose():void;onExpired():void}){
 const active=useRef(false)
 const [context,setContext]=useState<CreateLevelContext>(),[loadError,setLoadError]=useState(false),[reload,setReload]=useState(0),[saving,setSaving]=useState(false),[attempt,setAttempt]=useState<CreateLevelInput>(),[error,setError]=useState<'uncertain'|'conflict'|'denied'|'invalid'|'held'>()
 useEffect(()=>{const abort=new AbortController();setContext(undefined);setLoadError(false);readCreateLevel(productID,variantID,itemID,abort.signal).then(d=>{if(!abort.signal.aborted)setContext(d)}).catch(e=>{if(!abort.signal.aborted){if(e instanceof APIError&&e.status===401)onExpired();else setLoadError(true)}});return()=>abort.abort()},[productID,variantID,itemID,reload,onExpired])
 const save=async(values:Fields)=>{
  if(active.current||!context?.enabled)return
  const stocked=stockQuantity(values.stocked_quantity),incoming=stockQuantity(values.incoming_quantity);if(!attempt&&(!stocked||!incoming)){setError('invalid');return}
  const body=attempt??{operation_id:crypto.randomUUID(),expected_revision:context.revision,location_id:values.location_id,stocked_quantity:stocked!,incoming_quantity:incoming!}
  active.current=true;setSaving(true);setAttempt(body);setError(undefined)
  try{await createInventoryLevel(productID,variantID,itemID,body);onClose()}
  catch(e){if(e instanceof APIError&&e.status===401)onExpired();else if(e instanceof APIError&&e.code==='inventory.reconciliation_required')setError('held');else if(e instanceof APIError&&e.status===409)setError('conflict');else if(e instanceof APIError&&(e.status===403||e.status===404))setError('denied');else if(e instanceof APIError&&e.status===400){setAttempt(undefined);setError('invalid')}else setError('uncertain')}
  finally{active.current=false;setSaving(false)}
 }
 const validate={validator:(_:unknown,v:unknown)=>stockQuantity(v)?Promise.resolve():Promise.reject(new Error(copy.stockInputInvalid))}
 return <Drawer open title={copy.createStockLevel} width="min(560px, 100vw)" onClose={()=>{if(!active.current)onClose()}} closable={!saving} maskClosable={!saving} keyboard={!saving}>
  <Space direction="vertical" size={24} className="full-width">
   <Alert type="info" showIcon message={copy.createStockInfo}/>
   {loadError?<Space direction="vertical"><Alert type="error" showIcon message={copy.unavailable}/><Button onClick={()=>setReload(n=>n+1)}>{copy.retry}</Button></Space>:!context?<Spin/>:<>
    {!context.enabled&&<Alert type="warning" showIcon message={copy.stockCreationDisabled}/>}
    {context.shared_variants>1&&<Alert type="warning" showIcon message={copy.sharedStockInfo+' '+context.shared_variants}/>}
    {error&&<Alert type={error==='uncertain'?'warning':'error'} showIcon message={error==='uncertain'?copy.saveUncertain:error==='invalid'?copy.stockInputInvalid:error==='conflict'?copy.stockCreateConflict:error==='held'?copy.stockCreateHeld:copy.accessDenied}/>}
    <Form layout="vertical" initialValues={{stocked_quantity:'0',incoming_quantity:'0'}} disabled={saving||!!attempt||!context.enabled} onFinish={save}>
     <Form.Item label={copy.stockLocation} name="location_id" rules={[{required:true,message:copy.required}]}><Select showSearch optionFilterProp="label" options={context.locations.map(l=>({value:l.id,label:l.name+' ('+l.id+')',disabled:l.has_level}))}/></Form.Item>
     <Form.Item label={copy.stockedQuantity} name="stocked_quantity" rules={[validate]}><Input inputMode="decimal" maxLength={39} autoComplete="off"/></Form.Item>
     <Form.Item label={copy.incomingQuantity} name="incoming_quantity" rules={[validate]}><Input inputMode="decimal" maxLength={39} autoComplete="off"/></Form.Item>
     {!attempt&&context.enabled&&<Button aria-label={copy.save} type="primary" htmlType="submit" loading={saving}>{copy.save}</Button>}
    </Form>
    {attempt&&error==='uncertain'&&<Button type="primary" loading={saving} onClick={()=>void save(attempt)}>{copy.retrySave}</Button>}
   </>}
   <Button aria-label={copy.cancel} disabled={saving} onClick={onClose}>{copy.cancel}</Button>
  </Space>
 </Drawer>
}
