import { useRef, useState } from 'react'
import { Alert, Button, Drawer, Form, Input, Select, Space } from 'antd'
import { APIError, editProduct } from './api'
import type { ProductDetail, ProductEdit as Attempt } from './api'
import type { Copy } from './i18n'

type Fields={title:string;subtitle:string;description:string;status:ProductDetail['status']}
// Preserve an existing empty string when untouched; clearing nonempty text means NULL.
const nullable=(value:string,original:string|null)=>value===(original??'')?original:value||null
export default function ProductEdit({product,copy,onClose,onSaved,onExpired}:{product:ProductDetail;copy:Copy;onClose():void;onSaved():void;onExpired():void}) {
  const [form]=Form.useForm<Fields>()
  const [saving,setSaving]=useState(false)
  const active=useRef(false)
  const [attempt,setAttempt]=useState<Attempt>()
  const [error,setError]=useState<'uncertain'|'conflict'|'forbidden'|'missing'|'invalid'>()
  const submit=async(values:Fields)=>{
    if(active.current)return
    const body=attempt??{operation_id:crypto.randomUUID(),expected_revision:product.revision,title:values.title,subtitle:nullable(values.subtitle,product.subtitle),description:nullable(values.description,product.description),status:values.status}
    active.current=true;setSaving(true);setAttempt(body);setError(undefined)
    try{await editProduct(product.id,body);onSaved()}
    catch(err){
      if(err instanceof APIError&&err.status===401)onExpired()
      else if(err instanceof APIError&&err.status===409)setError('conflict')
      else if(err instanceof APIError&&err.status===403)setError('forbidden')
      else if(err instanceof APIError&&err.status===404)setError('missing')
      else if(err instanceof APIError&&err.status===400){setAttempt(undefined);setError('invalid')}
      else setError('uncertain')
    }finally{active.current=false;setSaving(false)}
  }
  return <Drawer open title={copy.editProduct} width="min(560px, 100vw)" destroyOnHidden maskClosable={!saving} closable={!saving} keyboard={!saving} onClose={()=>{if(!active.current)onClose()}} extra={<Space><Button aria-label={copy.cancel} disabled={saving} onClick={onClose}>{copy.cancel}</Button><Button aria-label={error==='uncertain'?copy.retrySave:copy.save} type="primary" loading={saving} disabled={error==='conflict'||error==='forbidden'||error==='missing'} onClick={()=>form.submit()}>{error==='uncertain'?copy.retrySave:copy.save}</Button></Space>}>
    <Space direction="vertical" size={24} className="full-width">
      {error&&<Alert showIcon type={error==='uncertain'?'warning':'error'} message={copy[error==='uncertain'?'saveUncertain':error==='conflict'?'editConflict':error==='missing'?'productNotFound':error==='invalid'?'editInvalid':'editForbidden']} action={error==='conflict'?<Button onClick={onClose}>{copy.reload}</Button>:undefined}/>}
      <Form form={form} layout="vertical" disabled={saving||!!attempt} initialValues={{title:product.title,subtitle:product.subtitle??'',description:product.description??'',status:product.status}} onFinish={submit}>
        <Form.Item name="title" label={copy.productTitle} rules={[{required:true,whitespace:true,message:copy.required},{max:500,message:copy.editTooLong}]}><Input maxLength={500}/></Form.Item>
        <Form.Item name="subtitle" label={copy.subtitle} rules={[{max:1000,message:copy.editTooLong}]}><Input maxLength={1000}/></Form.Item>
        <Form.Item name="description" label={copy.description} rules={[{max:100000,message:copy.editTooLong}]}><Input.TextArea autoSize={{minRows:5,maxRows:14}} maxLength={100000}/></Form.Item>
        <Form.Item name="status" label={copy.status} rules={[{required:true,message:copy.required}]}><Select options={(['draft','proposed','published','rejected'] as const).map(value=>({value,label:copy[value]}))}/></Form.Item>
      </Form>
    </Space>
  </Drawer>
}
