import {useEffect,useRef,useState} from 'react'
import {Alert,Button,Drawer,Form,Input,InputNumber,Select,Space,Spin,Typography} from 'antd'
import {APIError,readWarehouseDeviceEdit,editWarehouseDevice,warehouseIntakeVariants,type WarehouseDeviceEditContext,type WarehouseDeviceEditInput,type WarehouseDeviceFields,type WarehouseIntakeScope,type WarehouseIntakeVariant} from './api'
import type {Copy} from './i18n'
import DevicePhotos from './DevicePhotos'

export default function WarehouseDeviceEdit({scope,id,variantLabel,copy,onClose,onExpired}:{scope:WarehouseIntakeScope;id:string;variantLabel:string;copy:Copy;onClose():void;onExpired():void}){
 const zh=copy.language==='语言',[form]=Form.useForm<WarehouseDeviceFields>(),[data,setData]=useState<WarehouseDeviceEditContext>(),[attempt,setAttempt]=useState(0),[loadError,setLoadError]=useState(false),[pending,setPending]=useState<WarehouseDeviceEditInput>(),[saving,setSaving]=useState(false),[photoBusy,setPhotoBusy]=useState(false),[error,setError]=useState<unknown>(),busy=useRef(false)
 const [query,setQuery]=useState(''),[offset,setOffset]=useState(0),[variants,setVariants]=useState<WarehouseIntakeVariant[]>([]),[more,setMore]=useState(false),[variantError,setVariantError]=useState(false)
 useEffect(()=>{const c=new AbortController();setData(undefined);setLoadError(false);readWarehouseDeviceEdit(scope,id,c.signal).then(d=>{if(!c.signal.aborted){setData(d);form.setFieldsValue(d.fields)}}).catch(e=>{if(!c.signal.aborted){if(e instanceof APIError&&e.status===401)onExpired();else setLoadError(true)}});return()=>c.abort()},[scope.locationID,scope.category,id,attempt,form,onExpired])
 useEffect(()=>{const c=new AbortController(),timer=setTimeout(()=>{setVariantError(false);warehouseIntakeVariants(scope,query,offset,c.signal).then(d=>{if(!c.signal.aborted){setVariants(d.variants);setMore(d.has_more)}}).catch(e=>{if(!c.signal.aborted){if(e instanceof APIError&&e.status===401)onExpired();else setVariantError(true)}})},250);return()=>{clearTimeout(timer);c.abort()}},[scope.locationID,scope.category,query,offset,attempt,onExpired])
 const send=async(body:WarehouseDeviceEditInput)=>{if(busy.current)return;busy.current=true;setSaving(true);setPending(body);setError(undefined);try{await editWarehouseDevice(scope,id,body);onClose()}catch(e){if(e instanceof APIError&&e.status===401)onExpired();else setError(e)}finally{busy.current=false;setSaving(false)}}
 const reload=()=>{setPending(undefined);setError(undefined);setAttempt(n=>n+1)},locked=saving||photoBusy||!!pending,uncertain=error!==undefined&&(!(error instanceof APIError)||error.status===0||error.status>=500)
 const choices=variants.map(v=>({value:v.id,label:[v.product_title,v.sku,v.title].filter(Boolean).join(' / ')}));if(data&&!choices.some(v=>v.value===data.fields.variant_id))choices.unshift({value:data.fields.variant_id,label:variantLabel})
 const states=['pending','inspection','available','reserved','packed','shipped','repair','repair_ready','retired','supplier_returned']
 const stateNames=['待处理','待检测','可售','已预配','已装包','已发货','维修中','维修待返还','已退役','已退供应商']
 return <Drawer open width="min(720px,100vw)" title={zh?'编辑设备':'Edit device'} closable={!saving&&!photoBusy} maskClosable={!saving&&!photoBusy} keyboard={!saving&&!photoBusy} onClose={()=>{if(!saving&&!photoBusy)onClose()}}><Space direction="vertical" size={16} className="full-width">
  <Alert type="info" message={zh?'序列号保持不变；已分配设备的 SKU、来源、仓库、类别和状态不可改变。编辑不会增减数量库存。':'The serial number stays unchanged. Allocated devices retain their SKU, source, warehouse, category and state. Editing does not change quantity stock.'}/>
  {loadError?<><Alert type="error" message={copy.unavailable}/><Button onClick={reload}>{copy.retry}</Button></>:!data?<Spin/>:<>
   <Typography.Text>SN: {data.serial_number}</Typography.Text>
   {error!==undefined&&<Alert type={uncertain?'warning':'error'} message={uncertain?copy.saveUncertain:zh?'设备、号码、配置或权限已变化，请重新读取核对。':'The device, identity, configuration or permission changed. Reload and review.'}/>}
   <Form form={form} layout="vertical" disabled={locked} onFinish={v=>{if(!data)return;const fields={...v};for(const k of ['imei','imei2','eid'] as const)fields[k]=v[k]?.trim()||null;fields.grade=v.grade??null;fields.battery_health=v.battery_health??null;fields.bin=v.bin.trim();fields.notes=v.notes.trim();void send({operation_id:crypto.randomUUID(),expected_revision:data.revision,fields})}}>
    <Form.Item name="variant_id" label="SKU" rules={[{required:true}]}><Select showSearch filterOption={false} onSearch={q=>{setQuery(q);setOffset(0)}} options={choices}/></Form.Item>
    {variantError&&<Alert type="error" message={copy.unavailable}/>}
    <Space><Button disabled={locked||!offset} onClick={()=>setOffset(n=>Math.max(0,n-20))}>{zh?'上一页 SKU':'Previous SKUs'}</Button><Button disabled={locked||!more} onClick={()=>setOffset(n=>n+20)}>{zh?'下一页 SKU':'Next SKUs'}</Button></Space>
    <Form.Item name="source_id" label={zh?'设备来源':'Device source'} rules={[{required:true}]}><Select showSearch optionFilterProp="label" options={data.sources.map(s=>({value:s.id,label:s.name+' / '+(s.location_name??s.location_id)}))}/></Form.Item>
    <Form.Item name="location_id" label={zh?'当前仓库':'Current warehouse'} rules={[{required:true}]}><Select options={data.locations.map(l=>({value:l.id,label:l.name}))}/></Form.Item>
    <Form.Item name="category" label={zh?'设备类别':'Device category'} rules={[{required:true}]}><Select options={data.categories.map(value=>({value,label:value==='sale'?(zh?'销售':'Sale'):(zh?'退回':'Returned')}))}/></Form.Item>
    <Form.Item name="state" label={zh?'设备状态':'Device state'} rules={[{required:true}]}><Select options={states.map((value,i)=>({value,label:zh?stateNames[i]:value}))}/></Form.Item>
    {(['imei','imei2','eid'] as const).map(k=><Form.Item key={k} name={k} label={k==='imei2'?'IMEI 2':k.toUpperCase()} rules={[{pattern:k==='eid'?/^\d{32}$/:/^\d{15}$/,message:zh?'请核对完整号码':'Check the complete number'}]}><Input inputMode="numeric" maxLength={k==='eid'?32:15}/></Form.Item>)}
    <Form.Item name="grade" label={zh?'实际成色':'Actual grade'}><Select allowClear options={['S','A','B','C'].map(value=>({value,label:value}))}/></Form.Item>
    <Form.Item name="battery_health" label={zh?'实际电池健康度':'Actual battery health'}><InputNumber min={0} max={100} precision={0}/></Form.Item>
    <Form.Item name="cost_cents" label={zh?'成本（分）':'Cost (minor units)'} rules={[{required:true},{type:'integer',min:0,max:100000000}]}><InputNumber min={0} max={100000000} precision={0}/></Form.Item>
    <Form.Item name="currency_code" label={zh?'成本币种':'Cost currency'} rules={[{required:true},{pattern:/^[a-z]{3}$/}]}><Input maxLength={3}/></Form.Item>
    <Form.Item name="bin" label={zh?'库位':'Bin'}><Input maxLength={200}/></Form.Item>
    <Form.Item name="notes" label={zh?'备注':'Notes'}><Input.TextArea maxLength={2000} autoSize={{minRows:3,maxRows:8}}/></Form.Item>
    <Form.Item name="photos" label={zh?'设备照片':'Device photos'}><DevicePhotos scope={scope} copy={copy} disabled={locked} onBusy={setPhotoBusy} onExpired={onExpired}/></Form.Item>
    {!pending&&<Button htmlType="submit" type="primary" loading={saving} disabled={locked}>{copy.save}</Button>}
   </Form>
   {uncertain&&pending&&<Button type="primary" loading={saving} onClick={()=>void send(pending)}>{copy.retrySave}</Button>}
   {error!==undefined&&!uncertain&&<Button onClick={reload}>{zh?'重新读取':'Reload'}</Button>}
  </>}
  <Button disabled={saving||photoBusy} onClick={onClose}>{copy.cancel}</Button>
 </Space></Drawer>
}
