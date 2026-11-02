import {useEffect,useRef,useState} from 'react'
import {Alert,Button,Drawer,Form,InputNumber,Select,Space,Spin,Typography} from 'antd'
import {APIError,inventoryLinkChoices,readInventoryLinks,saveInventoryLinks} from './api'
import type {InventoryLinkChoice,InventoryLinksContext,InventoryLinksInput,InventoryRequirement} from './api'
import type {Copy} from './i18n'

export default function VariantInventoryItems({productID,variantID,copy,onClose,onSaved,onExpired}:{productID:string;variantID:string;copy:Copy;onClose():void;onSaved():void;onExpired():void}){
 const zh=copy.language==='语言',t=zh?{title:'管理库存关联',item:'库存项',quantity:'每件商品用量',add:'添加库存项',remove:'移除',search:'搜索名称、SKU 或编号',more:'加载更多',intro:'设置此 SKU 使用的库存项及每件商品消耗的数量。保存不会调整实物库存或预留数量。',tracked:'此 SKU 使用序列号设备，必须保留唯一库存项且用量为 1。',off:'此 SKU 尚未启用库存管理，关联将在启用后生效。',disabled:'库存关联编辑暂未启用。',unavailable:'库存项已不可用，请核对后明确移除或更换。',bound:'已用于其他序列号 SKU',conflict:'库存关联已变更，请重新加载后核对。',invalid:'请选择不同的库存项，并填写大于 0 的用量（最多 18 位小数）。',reload:'重新加载关联',empty:'尚未关联库存项'}:{title:'Manage inventory links',item:'Inventory item',quantity:'Required per product',add:'Add inventory item',remove:'Remove',search:'Search name, SKU or ID',more:'Load more',intro:'Choose the inventory items consumed by this SKU and the quantity required per product. Saving preserves physical stock and reservations.',tracked:'This SKU uses serial-numbered devices. Keep its unique inventory item with a required quantity of 1.',off:'Inventory management is disabled for this SKU. These links take effect when enabled.',disabled:'Inventory link editing is currently disabled.',unavailable:'An inventory item is unavailable. Review it and explicitly remove or replace it.',bound:'Used by another device-tracked SKU',conflict:'Inventory links changed. Reload and review before saving.',invalid:'Choose distinct inventory items and positive quantities with up to 18 decimal places.',reload:'Reload inventory links',empty:'No inventory items linked'}
 const [context,setContext]=useState<InventoryLinksContext>(),[loadError,setLoadError]=useState(false),[reload,setReload]=useState(0),[saving,setSaving]=useState(false),[pending,setPending]=useState<InventoryLinksInput>(),[error,setError]=useState<string>()
 const [query,setQuery]=useState(''),[offset,setOffset]=useState(0),[choices,setChoices]=useState<InventoryLinkChoice[]>([]),[more,setMore]=useState(false),[searching,setSearching]=useState(false),[choiceError,setChoiceError]=useState(false),[searchAttempt,setSearchAttempt]=useState(0)
 const [choiceCache,setChoiceCache]=useState<Record<string,InventoryLinkChoice>>({})
 const active=useRef(false),[form]=Form.useForm<{items:InventoryRequirement[]}>(),items=Form.useWatch('items',form)??[]
 useEffect(()=>{const abort=new AbortController();setContext(undefined);setLoadError(false);setPending(undefined);setError(undefined);readInventoryLinks(productID,variantID,abort.signal).then(d=>{if(!abort.signal.aborted){setContext(d);form.setFieldsValue({items:d.items.map(i=>({inventory_item_id:i.inventory_item_id,required_quantity:i.required_quantity}))})}}).catch(e=>{if(!abort.signal.aborted){if(e instanceof APIError&&e.status===401)onExpired();else setLoadError(true)}});return()=>abort.abort()},[productID,variantID,reload,onExpired,form])
 useEffect(()=>{const abort=new AbortController();setSearching(true);setChoiceError(false);const timer=setTimeout(()=>{inventoryLinkChoices(productID,variantID,query,offset,abort.signal).then(d=>{if(!abort.signal.aborted){setChoices(old=>offset?[...old,...d.items.filter(i=>!old.some(o=>o.id===i.id))]:d.items);setChoiceCache(old=>({...old,...Object.fromEntries(d.items.map(i=>[i.id,i]))}));setMore(d.has_more);setSearching(false)}}).catch(e=>{if(!abort.signal.aborted){setSearching(false);setChoiceError(true);if(e instanceof APIError&&e.status===401)onExpired()}})},250);return()=>{clearTimeout(timer);abort.abort()}},[productID,variantID,query,offset,searchAttempt,onExpired])
 const save=async(body:InventoryLinksInput)=>{if(active.current)return;active.current=true;setSaving(true);setPending(body);setError(undefined);try{await saveInventoryLinks(productID,variantID,body);onSaved()}catch(e){if(e instanceof APIError&&e.status===401)onExpired();else if(e instanceof APIError&&e.status===409)setError(e.code==='catalog.tracked_inventory'?'tracked':'conflict');else if(e instanceof APIError&&e.status===400){setPending(undefined);setError('invalid')}else if(e instanceof APIError&&(e.status===403||e.status===404))setError('denied');else setError('uncertain')}finally{active.current=false;setSaving(false)}}
 const options=[...(context?.items??[]).map(i=>({id:i.inventory_item_id,sku:i.sku,title:i.title,unit_of_measure:i.unit_of_measure,device_bound:false})),...Object.values({...Object.fromEntries(choices.map(i=>[i.id,i])),...Object.fromEntries(items.filter(i=>i?.inventory_item_id&&choiceCache[i.inventory_item_id]).map(i=>[i.inventory_item_id,choiceCache[i.inventory_item_id]]))}).filter(i=>!context?.items.some(o=>o.inventory_item_id===i.id))]
 const label=(i:InventoryLinkChoice)=>[i.title||i.sku||i.id,i.title&&i.sku?i.sku:null,i.unit_of_measure].filter(Boolean).join(' · ')
 return <Drawer open title={t.title} width="min(650px,100vw)" closable={!saving} maskClosable={!saving} keyboard={!saving} onClose={()=>{if(!active.current)onClose()}}><Space direction="vertical" size={24} className="full-width">
  <Alert type="info" showIcon message={t.intro}/>
  {loadError?<><Alert type="error" message={copy.unavailable}/><Button onClick={()=>setReload(n=>n+1)}>{copy.retry}</Button></>:!context?<Spin/>:<>
   {!context.enabled&&<Alert type="warning" message={t.disabled}/>}
   {!context.manage_inventory&&<Alert type="info" message={t.off}/>}
   {context.device_inventory_item_id&&<Alert type="info" message={t.tracked}/>}
   {context.items.some(i=>!i.available)&&<Alert type="warning" message={t.unavailable}/>}
   {error&&<Alert showIcon type={error==='uncertain'?'warning':'error'} message={error==='uncertain'?copy.saveUncertain:error==='denied'?copy.accessDenied:error==='tracked'?t.tracked:error==='invalid'?t.invalid:t.conflict}/>}
   <Form form={form} name="variant-inventory-links" layout="vertical" disabled={saving||!!pending||!context.enabled} onFinish={values=>void save({operation_id:crypto.randomUUID(),expected_revision:context.revision,items:values.items??[]})}>
    <Form.List name="items">{(fields,{add,remove})=><Space direction="vertical" size={16} className="full-width">
     {!fields.length&&<Typography.Text type="secondary">{t.empty}</Typography.Text>}
     {fields.map((field,index)=><div key={field.key}>
      <Form.Item label={`${t.item} ${index+1}`} name={[field.name,'inventory_item_id']} rules={[{required:true,message:copy.required}]}><Select aria-label={`${t.item} ${index+1}`} showSearch filterOption={false} placeholder={t.search} loading={searching} disabled={saving||!!pending||!context.enabled||!!context.device_inventory_item_id} onSearch={q=>{setQuery(q);setOffset(0);setChoices([]);setMore(false)}} options={options.map(i=>({value:i.id,label:label(i)+(i.device_bound?' · '+t.bound:''),disabled:i.device_bound||items.some((x,j)=>j!==index&&x?.inventory_item_id===i.id)}))}/></Form.Item>
      <Form.Item label={`${t.quantity} ${index+1}`} name={[field.name,'required_quantity']} rules={[{validator:(_,value)=>typeof value==='string'&&/^(0|[1-9]\d{0,19})(\.\d{1,18})?$/.test(value)&&/[1-9]/.test(value)?Promise.resolve():Promise.reject(new Error(t.invalid))}]}><InputNumber aria-label={`${t.quantity} ${index+1}`} stringMode controls={false} disabled={saving||!!pending||!context.enabled||!!context.device_inventory_item_id} style={{width:'100%'}}/></Form.Item>
      {!context.device_inventory_item_id&&<Button onClick={()=>remove(field.name)}>{t.remove} {index+1}</Button>}
     </div>)}
     {!context.device_inventory_item_id&&fields.length<1000&&<Button onClick={()=>add({inventory_item_id:undefined,required_quantity:'1'})}>{t.add}</Button>}
    </Space>}</Form.List>
    {choiceError&&<Alert style={{marginTop:16}} type="warning" message={copy.unavailable} action={<Button disabled={saving||!!pending} onClick={()=>setSearchAttempt(n=>n+1)}>{copy.retry}</Button>}/>}
    {more&&<Button style={{marginTop:16}} loading={searching} onClick={()=>setOffset(n=>n+50)}>{t.more}</Button>}
    {!pending&&context.enabled&&<Button style={{marginTop:24}} type="primary" htmlType="submit" loading={saving}>{copy.save}</Button>}
   </Form>
   {pending&&error==='uncertain'&&<Button type="primary" loading={saving} onClick={()=>void save(pending)}>{copy.retrySave}</Button>}
   {error&&error!=='uncertain'&&<Button disabled={saving} onClick={()=>setReload(n=>n+1)}>{t.reload}</Button>}
  </>}
  <Button disabled={saving} onClick={onClose}>{copy.cancel}</Button>
 </Space></Drawer>
}
