import {useEffect,useRef,useState} from 'react'
import {Alert,Button,Descriptions,Drawer,Form,Input,Select,Space,Spin} from 'antd'
import {APIError,readPriceCreate,createBasePrice,stockQuantity,validPriceQuantityBounds as quantities} from './api'
import type {PriceCreateContext,PriceCreateInput} from './api'
import type {Copy} from './i18n'
type Fields={price_list_id?:string;amount:string;currency_code:string;region_id?:string;min_quantity?:string;max_quantity?:string}
export default function PriceCreate({productID,variantID,copy,onClose,onExpired,listPrice=false}:{productID:string;variantID:string;copy:Copy;listPrice?:boolean;onClose():void;onExpired():void}){
 const zh=copy.language==='语言'
 const [form]=Form.useForm<Fields>(),active=useRef(false)
 const [context,setContext]=useState<PriceCreateContext>(),[loadError,setLoadError]=useState(false),[reload,setReload]=useState(0),[saving,setSaving]=useState(false),[attempt,setAttempt]=useState<PriceCreateInput>(),[error,setError]=useState<'uncertain'|'conflict'|'denied'|'invalid'>()
 useEffect(()=>{const abort=new AbortController();setLoadError(false);setContext(undefined);readPriceCreate(productID,variantID,abort.signal).then(d=>{if(!abort.signal.aborted)setContext(d)}).catch(e=>{if(!abort.signal.aborted){if(e instanceof APIError&&e.status===401)onExpired();else setLoadError(true)}});return()=>abort.abort()},[productID,variantID,reload,onExpired])
 const submit=async(values:Fields)=>{
  if(active.current||!context?.enabled)return
  const amount=stockQuantity(values.amount);if(!amount||listPrice&&!values.price_list_id||!quantities(values.min_quantity,values.max_quantity)){setError('invalid');return}
  const body=attempt??{operation_id:crypto.randomUUID(),expected_revision:context.revision,currency_code:values.currency_code,amount,...(listPrice?{price_list_id:values.price_list_id}:{}),...(values.region_id?{region_id:values.region_id}:{}),...(values.min_quantity?{min_quantity:values.min_quantity}:{}),...(values.max_quantity?{max_quantity:values.max_quantity}:{})}
  active.current=true;setSaving(true);setAttempt(body);setError(undefined)
  try{await createBasePrice(productID,variantID,body);onClose()}
  catch(e){if(e instanceof APIError&&e.status===401)onExpired();else if(e instanceof APIError&&e.status===409)setError('conflict');else if(e instanceof APIError&&(e.status===403||e.status===404))setError('denied');else if(e instanceof APIError&&e.status===400){setAttempt(undefined);setError('invalid')}else setError('uncertain')}
  finally{active.current=false;setSaving(false)}
 }
 return <Drawer open title={listPrice?(zh?'新增价目表价格':'Add price-list price'):copy.createBasePrice} width="min(560px, 100vw)" onClose={()=>{if(!active.current)onClose()}} closable={!saving} maskClosable={!saving} keyboard={!saving}>
  <Space direction="vertical" size={24} className="full-width">
   <Alert type="info" showIcon message={listPrice?(zh?'为已有价目表添加 SKU 价格。价目表状态、生效时间与规则共同决定前台报价；共享价格集合仍影响关联 SKU。':'Add a SKU price to an existing price list. Its status, dates and rules determine the storefront quote; shared price sets affect all linked SKUs.') :zh?'基础价格适用于该币种；选择地区后仅用于该地区。数量上下限均包含，留空表示不限。共享价格集合仍影响关联SKU。':'Base prices apply to the currency. A regional price applies only to the selected region. Quantity bounds are inclusive; leave either blank for no limit. Shared price sets still affect their linked SKUs.'}/>
   {loadError?<Space direction="vertical"><Alert type="error" showIcon message={copy.unavailable}/><Button onClick={()=>setReload(n=>n+1)}>{copy.retry}</Button></Space>:!context?<Spin/>:<>
    <Descriptions column={1} items={[{key:'set',label:copy.priceSet,children:context.price_set_id??copy.newPriceSet}]}/>
    {context.shared_variants>1&&<Alert type="warning" showIcon message={copy.sharedPriceInfo+' '+context.shared_variants}/>}
    {!context.enabled&&<Alert type="warning" showIcon message={copy.priceEditingDisabled}/>}
    {error&&<Alert type={error==='uncertain'?'warning':'error'} showIcon message={error==='uncertain'?copy.saveUncertain:error==='invalid'?copy.stockInputInvalid:error==='conflict'?copy.priceEditConflict:copy.accessDenied}/>}
    <Form form={form} layout="vertical" disabled={saving||!!attempt||!context.enabled} onFinish={submit}>
     {listPrice&&<Form.Item label={copy.priceList} name="price_list_id" rules={[{required:true,message:copy.required}]}><Select showSearch optionFilterProp="label" options={(context.price_lists??[]).map(l=>({value:l.id,label:l.title+' · '+(l.status==='active'?copy.active:copy.draft)+' · '+(l.type==='sale'?copy.sale:copy.override)}))}/></Form.Item>}
     <Form.Item label={zh?'最小数量（可选）':'Minimum quantity (optional)'} name="min_quantity" dependencies={['max_quantity']} rules={[{validator:(_,v)=>quantities(v,form.getFieldValue('max_quantity'))?Promise.resolve():Promise.reject(new Error(zh?'请输入正整数，且最小数量不能超过最大数量。':'Use positive integers with minimum no greater than maximum.'))}]}><Input inputMode="numeric" maxLength={20} autoComplete="off"/></Form.Item>
     <Form.Item label={zh?'最大数量（可选）':'Maximum quantity (optional)'} name="max_quantity" dependencies={['min_quantity']} rules={[{validator:(_,v)=>quantities(form.getFieldValue('min_quantity'),v)?Promise.resolve():Promise.reject(new Error(zh?'请输入正整数，且最小数量不能超过最大数量。':'Use positive integers with minimum no greater than maximum.'))}]}><Input inputMode="numeric" maxLength={20} autoComplete="off"/></Form.Item>
     <Form.Item noStyle shouldUpdate>{()=>context.regions&&<Form.Item label={zh?'地区（可选）':'Region (optional)'} name="region_id"><Select allowClear showSearch optionFilterProp="label" onChange={id=>{const region=context.regions?.find(r=>r.id===id);if(region)form.setFieldValue('currency_code',region.currency_code)}} options={context.regions.map(r=>({value:r.id,label:r.name+' · '+r.currency_code.toUpperCase(),disabled:!listPrice&&!form.getFieldValue('min_quantity')&&!form.getFieldValue('max_quantity')&&context.existing_region_prices?.some(p=>p.region_id===r.id&&p.currency_code===r.currency_code)}))}/></Form.Item>}</Form.Item>
     <Form.Item noStyle shouldUpdate>{()=> <Form.Item label={copy.currency} name="currency_code" rules={[{required:true,message:copy.required}]}><Select showSearch options={context.currencies.map(code=>({value:code,label:code.toUpperCase(),disabled:form.getFieldValue('region_id')?context.regions?.find(r=>r.id===form.getFieldValue('region_id'))?.currency_code!==code:!listPrice&&!form.getFieldValue('min_quantity')&&!form.getFieldValue('max_quantity')&&context.existing_base_currencies.includes(code)}))}/></Form.Item>}</Form.Item>
     <Form.Item label={copy.amount} name="amount" rules={[{validator:(_,v)=>stockQuantity(v)?Promise.resolve():Promise.reject(new Error(copy.stockInputInvalid))}]}><Input inputMode="decimal" maxLength={39} autoComplete="off"/></Form.Item>
     {!attempt&&context.enabled&&<Button aria-label={copy.save} type="primary" htmlType="submit" loading={saving}>{copy.save}</Button>}
    </Form>
    {attempt&&error==='uncertain'&&<Button type="primary" loading={saving} onClick={()=>void submit({price_list_id:attempt.price_list_id,amount:attempt.amount,currency_code:attempt.currency_code,region_id:attempt.region_id,min_quantity:attempt.min_quantity,max_quantity:attempt.max_quantity})}>{copy.retrySave}</Button>}
   </>}
   <Button aria-label={copy.cancel} disabled={saving} onClick={onClose}>{copy.cancel}</Button>
  </Space>
 </Drawer>
}
