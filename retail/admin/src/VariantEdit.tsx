import {useEffect,useRef,useState} from 'react'
import {Alert,Button,Drawer,Form,Input,InputNumber,Select,Space,Spin,Switch,Typography} from 'antd'
import {APIError,readVariantEdit,saveVariantEdit} from './api'
import type {VariantEditContext,VariantEditInput,VariantFields} from './api'
import type {Copy} from './i18n'

export default function VariantEdit({productID,variantID,copy,onClose,onSaved,onExpired}:{productID:string;variantID:string;copy:Copy;onClose():void;onSaved():void;onExpired():void}){
 const text=copy.language==='语言'?{title:'编辑 SKU',material:'材质',origin:'原产国代码',weight:'重量（g）',length:'长度（mm）',height:'高度（mm）',width:'宽度（mm）',manage:'管理库存',backorder:'允许缺货销售',inventory:'库存设置',shipping:'运输与海关',disabled:'SKU 编辑暂未启用。',conflict:'SKU、选项或图片已变更，或选项组合已存在。请重新加载后核对。',code:'SKU、条形码、EAN 或 UPC 已被使用。',tracked:'序列号 SKU 必须保留库存管理及原有库存关联。',inventoryCode:'该编码已被现有库存项使用，不能自动重新分配其库存。请核对库存关联或使用新的唯一 SKU。',intro:'更新颜色会同步当前机型对应的图片。清空已有 ERP 编码会保留原编号。',detach:'关闭库存管理会解除现有库存关联，但保留库存记录。重新启用不会自动恢复原库存。',invalid:'请检查 SKU 字段和完整选项组合。',reload:'重新加载 SKU',barcode:'条形码'}:{title:'Edit SKU',material:'Material',origin:'Origin country code',weight:'Weight (g)',length:'Length (mm)',height:'Height (mm)',width:'Width (mm)',manage:'Manage inventory',backorder:'Allow backorders',inventory:'Inventory settings',shipping:'Shipping and customs',disabled:'SKU editing is currently disabled.',conflict:'SKU, options or images changed, or this option combination already exists. Reload and review before saving.',code:'SKU, barcode, EAN or UPC is already in use.',tracked:'A device-tracked SKU must retain inventory management and its original inventory identity.',inventoryCode:'An existing inventory item uses this code. Its stock cannot be reassigned automatically. Review inventory links or use a new unique SKU.',intro:'Updating the color synchronizes images for this model. Clearing an existing ERP code retains its number.',detach:'Turning off inventory management removes the current inventory links and preserves inventory records. Turning it back on does not automatically restore the old stock.',invalid:'Check SKU fields and the complete option combination.',reload:'Reload SKU',barcode:'Barcode'}
 const [context,setContext]=useState<VariantEditContext>(),[loadError,setLoadError]=useState(false),[reload,setReload]=useState(0),[saving,setSaving]=useState(false),[attempt,setAttempt]=useState<VariantEditInput>(),[error,setError]=useState<string>()
 const active=useRef(false)
 useEffect(()=>{const abort=new AbortController();setContext(undefined);setLoadError(false);setAttempt(undefined);setError(undefined);readVariantEdit(productID,variantID,abort.signal).then(d=>{if(!abort.signal.aborted)setContext(d)}).catch(e=>{if(!abort.signal.aborted){if(e instanceof APIError&&e.status===401)onExpired();else setLoadError(true)}});return()=>abort.abort()},[productID,variantID,reload,onExpired])
 const save=async(body:VariantEditInput)=>{if(active.current)return;active.current=true;setSaving(true);setAttempt(body);setError(undefined);try{await saveVariantEdit(productID,variantID,body);onSaved()}catch(e){if(e instanceof APIError&&e.status===401)onExpired();else if(e instanceof APIError&&e.status===409)setError(e.code==='catalog.tracked_inventory'?'tracked':e.code==='catalog.variant_code_conflict'?'code':e.code==='catalog.inventory_code_conflict'?'inventoryCode':'conflict');else if(e instanceof APIError&&e.status===400){setAttempt(undefined);setError('invalid')}else if(e instanceof APIError&&(e.status===403||e.status===404))setError('denied');else setError('uncertain')}finally{active.current=false;setSaving(false)}}
 const optionalKeys=['sku','barcode','ean','upc','material','hs_code','mid_code','origin_country'] as const
 const numberKeys=['weight','length','height','width'] as const
 return <Drawer open title={text.title} width="min(600px,100vw)" closable={!saving} maskClosable={!saving} keyboard={!saving} onClose={()=>{if(!active.current)onClose()}}><Space direction="vertical" size={24} className="full-width">
  <Alert type="info" showIcon message={text.intro}/>
  {loadError?<><Alert type="error" message={copy.unavailable}/><Button onClick={()=>setReload(n=>n+1)}>{copy.retry}</Button></>:!context?<Spin/>:<>
   {!context.enabled&&<Alert type="warning" message={text.disabled}/>}
   {error&&<Alert type={error==='uncertain'?'warning':'error'} showIcon message={error==='uncertain'?copy.saveUncertain:error==='denied'?copy.accessDenied:error==='code'?text.code:error==='tracked'?text.tracked:error==='inventoryCode'?text.inventoryCode:error==='invalid'?text.invalid:text.conflict}/>}
   <Form key={context.revision} name="variant-edit" layout="vertical" disabled={saving||!!attempt||!context.enabled} initialValues={{...context.variant,...Object.fromEntries(optionalKeys.map(k=>[k,context.variant[k]??''])),options:Object.fromEntries(context.options.map((o,i)=>[String(i),o.values.find(v=>context.selected_ids.includes(v.id))?.id]))}} onFinish={(values:VariantFields&{options?:Record<string,string>})=>{
    const fields:VariantFields={...context.variant,...values};delete (fields as VariantFields&{options?:unknown}).options
    for(const k of optionalKeys)fields[k]=values[k]||null
    for(const k of numberKeys)fields[k]=values[k]??null
    void save({operation_id:crypto.randomUUID(),expected_revision:context.revision,...fields,option_value_ids:context.options.map((_,i)=>values.options?.[String(i)]??'')})
   }}>
    <Form.Item name="title" label={copy.variantTitle} rules={[{required:true,whitespace:true,message:copy.required},{max:500,message:text.invalid}]}><Input maxLength={500}/></Form.Item>
    {context.options.map((o,i)=><Form.Item key={o.id} label={o.title} name={['options',String(i)]} rules={[{required:true,message:copy.required}]}><Select showSearch optionFilterProp="label" options={o.values.map(v=>({value:v.id,label:v.value}))}/></Form.Item>)}
    <Form.Item name="material" label={text.material}><Input maxLength={512}/></Form.Item>
    <Typography.Title level={5}>{text.inventory}</Typography.Title>
    {(['sku','ean','upc','barcode'] as const).map(k=><Form.Item name={k} key={k} label={k==='barcode'?text.barcode:k.toUpperCase()}><Input maxLength={512} autoComplete="off"/></Form.Item>)}
    <Form.Item name="manage_inventory" label={text.manage} valuePropName="checked"><Switch aria-label={text.manage}/></Form.Item>
    <Form.Item name="allow_backorder" label={text.backorder} valuePropName="checked"><Switch aria-label={text.backorder}/></Form.Item>
    {context.inventory_links>0&&<Alert type="info" showIcon message={text.detach} style={{marginBottom:24}}/>}
    <Typography.Title level={5}>{text.shipping}</Typography.Title>
    {numberKeys.map(k=><Form.Item name={k} key={k} label={text[k]}><InputNumber style={{width:'100%'}}/></Form.Item>)}
    <Form.Item name="origin_country" label={text.origin}><Input maxLength={512}/></Form.Item>
    <Form.Item name="hs_code" label="HS code"><Input maxLength={512}/></Form.Item>
    <Form.Item name="mid_code" label="MID code"><Input maxLength={512}/></Form.Item>
    {!attempt&&context.enabled&&<Button type="primary" htmlType="submit" loading={saving}>{copy.save}</Button>}
   </Form>
   {attempt&&error==='uncertain'&&<Button type="primary" loading={saving} onClick={()=>void save(attempt)}>{copy.retrySave}</Button>}
   {error&&error!=='uncertain'&&<Button disabled={saving} onClick={()=>setReload(n=>n+1)}>{text.reload}</Button>}
  </>}
  <Button disabled={saving} onClick={onClose}>{copy.cancel}</Button>
 </Space></Drawer>
}
