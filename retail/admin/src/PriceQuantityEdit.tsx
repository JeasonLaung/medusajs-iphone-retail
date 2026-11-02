import {useEffect,useRef,useState} from 'react'
import {Alert,Button,Descriptions,Drawer,Form,Input,Space,Spin} from 'antd'
import {APIError,readPriceEdit,editPriceQuantity,validPriceQuantityBounds} from './api'
import type {PriceEditContext,PriceQuantityInput} from './api'
import type {Copy} from './i18n'
import {amountText} from './VariantPrices'

export default function PriceQuantityEdit({productID,variantID,priceID,copy,onClose,onExpired}:{productID:string;variantID:string;priceID:string;copy:Copy;onClose():void;onExpired():void}){
 const zh=copy.language==='语言',title=zh?'编辑数量条件':'Edit quantity bounds',invalid=zh?'请输入正整数，且最小数量不能超过最大数量。':'Use positive integers with minimum no greater than maximum.'
 const [form]=Form.useForm<{min_quantity:string;max_quantity:string}>(),active=useRef(false)
 const [context,setContext]=useState<PriceEditContext>(),[loadError,setLoadError]=useState(false),[reload,setReload]=useState(0),[saving,setSaving]=useState(false),[attempt,setAttempt]=useState<PriceQuantityInput>(),[error,setError]=useState<'uncertain'|'conflict'|'denied'|'invalid'>()
 useEffect(()=>{const abort=new AbortController();setLoadError(false);setContext(undefined);readPriceEdit(productID,variantID,priceID,abort.signal).then(d=>{if(!abort.signal.aborted){if(d.effective_min_quantity===undefined||d.effective_max_quantity===undefined)throw new APIError(0,'request.invalid_response');setContext(d)}}).catch(e=>{if(!abort.signal.aborted){if(e instanceof APIError&&e.status===401)onExpired();else setLoadError(true)}});return()=>abort.abort()},[productID,variantID,priceID,reload,form,onExpired])
 useEffect(()=>{if(context)form.setFieldsValue({min_quantity:context.effective_min_quantity??'',max_quantity:context.effective_max_quantity??''})},[context,form])
 const submit=async(values:{min_quantity:string;max_quantity:string})=>{
  if(active.current||!context?.enabled)return
  if(!validPriceQuantityBounds(values.min_quantity,values.max_quantity)){setError('invalid');return}
  const body=attempt??{operation_id:crypto.randomUUID(),expected_revision:context.revision,min_quantity:values.min_quantity||null,max_quantity:values.max_quantity||null}
  active.current=true;setSaving(true);setAttempt(body);setError(undefined)
  try{await editPriceQuantity(productID,variantID,priceID,body);onClose()}
  catch(e){if(e instanceof APIError&&e.status===401)onExpired();else if(e instanceof APIError&&e.status===409)setError('conflict');else if(e instanceof APIError&&(e.status===403||e.status===404))setError('denied');else if(e instanceof APIError&&e.status===400){setAttempt(undefined);setError('invalid')}else setError('uncertain')}
  finally{active.current=false;setSaving(false)}
 }
 return <Drawer open title={title} width="min(560px, 100vw)" onClose={()=>{if(!active.current)onClose()}} closable={!saving} maskClosable={!saving} keyboard={!saving}>
  <Space direction="vertical" size={24} className="full-width">
   <Alert type="info" showIcon message={zh?'仅修改数量上下限；留空可清除该限制。金额、币种和适用规则保持原值。':'Edit quantity bounds only. Leave blank to remove a bound. Amount, currency and eligibility rules retain their current values.'}/>
   {loadError?<Space direction="vertical"><Alert type="error" showIcon message={copy.unavailable}/><Button onClick={()=>setReload(n=>n+1)}>{copy.retry}</Button></Space>:!context?<Spin/>:<>
    <Descriptions column={1} items={[{key:'id',label:copy.priceID,children:context.id},{key:'currency',label:copy.currency,children:context.currency_code},{key:'amount',label:copy.amount,children:amountText(context.effective_amount)},{key:'max',label:copy.maxQuantity,children:amountText(context.max_quantity)},{key:'list',label:copy.priceList,children:context.price_list_id??copy.basePrice}]}/>
    {context.shared_variants>1&&<Alert type="warning" showIcon message={copy.sharedPriceInfo+' '+context.shared_variants}/>}
    {!context.enabled&&<Alert type="warning" showIcon message={copy.priceEditingDisabled}/>}
    {error&&<Alert type={error==='uncertain'?'warning':'error'} showIcon message={error==='uncertain'?copy.saveUncertain:error==='invalid'?copy.stockInputInvalid:error==='conflict'?copy.priceEditConflict:copy.accessDenied}/>}
    <Form form={form} layout="vertical" disabled={saving||!!attempt||!context.enabled} onFinish={submit}>
     <Form.Item label={copy.minQuantity} name="min_quantity" dependencies={['max_quantity']} rules={[{validator:(_,v)=>validPriceQuantityBounds(v,form.getFieldValue('max_quantity'))?Promise.resolve():Promise.reject(new Error(invalid))}]}><Input inputMode="numeric" maxLength={20} autoComplete="off"/></Form.Item>
     <Form.Item label={copy.maxQuantity} name="max_quantity" dependencies={['min_quantity']} rules={[{validator:(_,v)=>validPriceQuantityBounds(form.getFieldValue('min_quantity'),v)?Promise.resolve():Promise.reject(new Error(invalid))}]}><Input inputMode="numeric" maxLength={20} autoComplete="off"/></Form.Item>
     {!attempt&&context.enabled&&<Button aria-label={copy.save} type="primary" htmlType="submit" loading={saving}>{copy.save}</Button>}
    </Form>
    {attempt&&error==='uncertain'&&<Button type="primary" loading={saving} onClick={()=>void submit({min_quantity:attempt.min_quantity??'',max_quantity:attempt.max_quantity??''})}>{copy.retrySave}</Button>}
   </>}
   <Button aria-label={copy.cancel} disabled={saving} onClick={onClose}>{copy.cancel}</Button>
  </Space>
 </Drawer>
}
