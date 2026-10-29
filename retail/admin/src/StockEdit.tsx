import { useRef, useState } from 'react'
import { Alert, Button, Descriptions, Drawer, Form, Input, Space } from 'antd'
import { APIError, editStock, stockQuantity } from './api'
import type { InventoryLevel, StockEdit as Attempt } from './api'
import type { Copy } from './i18n'
import { amountText } from './VariantPrices'

type Fields={stocked_quantity:string;incoming_quantity:string}
export type StockEditLevel=Pick<InventoryLevel,'id'|'revision'|'location_id'|'location_name'|'stocked_quantity'|'reserved_quantity'|'incoming_quantity'>
export default function StockEdit({level,copy,onClose,onSaved,onExpired,save=editStock}:{level:StockEditLevel;copy:Copy;onClose():void;onSaved():void;onExpired():void;save?:(id:string,body:Attempt)=>Promise<unknown>}){
  const [form]=Form.useForm<Fields>()
  const active=useRef(false)
  const [saving,setSaving]=useState(false)
  const [attempt,setAttempt]=useState<Attempt>()
  const [error,setError]=useState<'uncertain'|'conflict'|'held'|'forbidden'|'missing'|'invalid'>()
  const submit=async(values:Fields)=>{
    if(active.current)return
    const stocked=stockQuantity(values.stocked_quantity),incoming=stockQuantity(values.incoming_quantity)
    if(!attempt&&(!stocked||!incoming)){setError('invalid');return}
    const body=attempt??{operation_id:crypto.randomUUID(),expected_revision:level.revision,stocked_quantity:stocked!,incoming_quantity:incoming!}
    active.current=true;setSaving(true);setAttempt(body);setError(undefined)
    try{await save(level.id,body);onSaved()}
    catch(err){
      if(err instanceof APIError&&err.status===401)onExpired()
      else if(err instanceof APIError&&err.code==='inventory.reconciliation_required')setError('held')
      else if(err instanceof APIError&&err.status===409)setError('conflict')
      else if(err instanceof APIError&&err.status===403)setError('forbidden')
      else if(err instanceof APIError&&err.status===404)setError('missing')
      else if(err instanceof APIError&&err.status===400){setAttempt(undefined);setError('invalid')}
      else setError('uncertain')
    }finally{active.current=false;setSaving(false)}
  }
  const close=()=>{if(!active.current)onClose()}
  const validate={validator:(_:unknown,value:unknown)=>stockQuantity(value)?Promise.resolve():Promise.reject(new Error(copy.stockInputInvalid))}
  return <Drawer open title={copy.editStock} width="min(560px, 100vw)" destroyOnHidden maskClosable={!saving} closable={!saving} keyboard={!saving} onClose={close} extra={<Space><Button aria-label={copy.cancel} disabled={saving} onClick={close}>{copy.cancel}</Button><Button aria-label={error==='uncertain'?copy.retrySave:copy.save} type="primary" loading={saving} disabled={!!error&&error!=='uncertain'&&error!=='invalid'} onClick={()=>form.submit()}>{error==='uncertain'?copy.retrySave:copy.save}</Button></Space>}>
    <Space direction="vertical" size={24} className="full-width">
      <Alert showIcon type="info" message={copy.stockEditInfo}/>
      {error&&<Alert showIcon type={error==='uncertain'?'warning':'error'} message={copy[error==='uncertain'?'stockSaveUncertain':error==='conflict'?'stockConflict':error==='held'?'stockHeld':error==='missing'?'stockLevelMissing':error==='invalid'?'stockInputInvalid':'stockEditDenied']}/>}
      <Descriptions column={1} items={[{key:'location',label:copy.stockLocation,children:level.location_name??level.location_id},{key:'reserved',label:copy.reservedQuantity,children:amountText(level.reserved_quantity)}]}/>
      <Form form={form} layout="vertical" disabled={saving||!!attempt} initialValues={{stocked_quantity:level.stocked_quantity,incoming_quantity:level.incoming_quantity}} onFinish={submit}>
        <Form.Item name="stocked_quantity" label={copy.stockedQuantity} rules={[validate]}><Input inputMode="decimal" maxLength={39} autoComplete="off"/></Form.Item>
        <Form.Item name="incoming_quantity" label={copy.incomingQuantity} rules={[validate]}><Input inputMode="decimal" maxLength={39} autoComplete="off"/></Form.Item>
      </Form>
    </Space>
  </Drawer>
}
