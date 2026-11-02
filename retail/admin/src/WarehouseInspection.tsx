import {useEffect,useRef,useState} from 'react'
import {Alert,Button,Drawer,Form,Input,InputNumber,Select,Space,Spin,Typography} from 'antd'
import {APIError,readWarehouseInspection,inspectWarehouseDevice,warehouseIntakeVariants,type WarehouseInspectionContext,type DeviceServiceInput,type DevicePhoto,type WarehouseIntakeVariant} from './api'
import type {Copy} from './i18n'
import DevicePhotos from './DevicePhotos'

type Fields={result:string;grade:string;battery_health:number;notes:string;photos:DevicePhoto[]}
export default function WarehouseInspection({location,id,copy,onClose,onExpired}:{location:string;id:string;copy:Copy;onClose():void;onExpired():void}){
 const zh=copy.language==='语言',[form]=Form.useForm<Fields>(),[data,setData]=useState<WarehouseInspectionContext>(),[attempt,setAttempt]=useState(0),[loadError,setLoadError]=useState(false),[error,setError]=useState<unknown>(),[pending,setPending]=useState<DeviceServiceInput>(),[saving,setSaving]=useState(false),[photoBusy,setPhotoBusy]=useState(false),busy=useRef(false)
 const [query,setQuery]=useState(''),[offset,setOffset]=useState(0),[variants,setVariants]=useState<WarehouseIntakeVariant[]>([]),[more,setMore]=useState(false),[variantError,setVariantError]=useState(false),[variant,setVariant]=useState('')
 const scope={locationID:location,category:'returned' as const}
 useEffect(()=>{const c=new AbortController();setData(undefined);setLoadError(false);readWarehouseInspection(location,id,c.signal).then(d=>{if(!c.signal.aborted){setData(d);setVariant(d.device.variant_id);form.setFieldsValue({result:'pass',grade:d.device.grade??'B',battery_health:d.device.battery_health??0,notes:'',photos:[]})}}).catch(e=>{if(!c.signal.aborted){if(e instanceof APIError&&e.status===401)onExpired();else setLoadError(true)}});return()=>c.abort()},[location,id,attempt,form,onExpired])
 useEffect(()=>{const c=new AbortController(),timer=setTimeout(()=>{setVariantError(false);warehouseIntakeVariants({locationID:location,category:'returned'},query,offset,c.signal).then(d=>{if(!c.signal.aborted){setVariants(d.variants);setMore(d.has_more)}}).catch(e=>{if(!c.signal.aborted){if(e instanceof APIError&&e.status===401)onExpired();else setVariantError(true)}})},250);return()=>{clearTimeout(timer);c.abort()}},[location,query,offset,attempt,onExpired])
 const send=async(body:DeviceServiceInput)=>{if(busy.current)return;busy.current=true;setPending(body);setSaving(true);setError(undefined);try{await inspectWarehouseDevice(location,id,body);onClose()}catch(e){if(e instanceof APIError&&e.status===401)onExpired();else setError(e)}finally{busy.current=false;setSaving(false)}}
 const uncertain=error!==undefined&&(!(error instanceof APIError)||error.status===0||error.status>=500),locked=saving||photoBusy||!!pending||!data?.enabled
 const reload=()=>{setPending(undefined);setError(undefined);setAttempt(n=>n+1)}
 const choices=variants.map(v=>({value:v.id,label:[v.product_title,v.sku,v.title].filter(Boolean).join(' / ')}));if(data&&!choices.some(v=>v.value===variant))choices.unshift({value:variant,label:variant===data.device.variant_id?(data.device.variant_title??variant):variant})
 return <Drawer open width="min(720px,100vw)" title={zh?'检测退回设备':'Inspect returned device'} closable={!saving&&!photoBusy} maskClosable={!saving&&!photoBusy} keyboard={!saving&&!photoBusy} onClose={()=>{if(!saving&&!photoBusy)onClose()}}><Space direction="vertical" size={16} className="full-width">
  <Alert type="info" message={zh?'记录实际检测结果、成色和电池；通过后普通退回设备转为销售可售，维修原机进入待返还。数量库存不变。':'Record the inspection result, grade and battery. Passing returns become available sale devices; an original repair becomes ready for return. Quantity stock is unchanged.'}/>
  {loadError?<><Alert type="error" message={copy.unavailable}/><Button onClick={reload}>{copy.retry}</Button></>:!data?<Spin/>:<>
   <Typography.Text>{data.device.serial_number} / {data.device.state}</Typography.Text>
   {data.repair&&<Alert type="info" message={zh?'此设备为原机维修，通过检测时必须保留原 SKU。':'This is an original repair. Keep its original SKU when passing inspection.'}/>}
   {!data.enabled&&<Alert type="warning" message={zh?'检测处理尚未启用':'Inspection is disabled'}/>}
   {error!==undefined&&<Alert type={uncertain?'warning':'error'} message={uncertain?copy.saveUncertain:zh?'设备、权限或检测条件已变化，请重新读取核对。':'The device, permission or inspection requirements changed. Reload and review.'}/>}
   <Form form={form} layout="vertical" disabled={locked} onFinish={v=>{if(!data||!variant)return;void send({operation_id:crypto.randomUUID(),expected_revision:data.device.revision,action:'inspect',result:v.result,variant_id:variant,grade:v.grade,battery_health:v.battery_health,notes:v.notes.trim(),photos:v.photos??[],tracking_number:''})}}>
    <Form.Item name="result" label={zh?'检测结果':'Inspection result'} rules={[{required:true}]}><Select options={[{value:'pass',label:zh?'检测通过':'Pass'},{value:'repair',label:zh?'继续维修':'Repair'},{value:'retire',label:zh?'报废':'Retire'},{value:'supplier_return',label:zh?'退回供应商':'Return to supplier'}]}/></Form.Item>
    <Form.Item label={zh?'检测后 SKU':'Inspected SKU'} required><Select aria-label={zh?'检测后 SKU':'Inspected SKU'} showSearch filterOption={false} onSearch={q=>{setQuery(q);setOffset(0)}} value={variant||undefined} onChange={setVariant} options={choices}/></Form.Item>
    {variantError&&<Alert type="error" message={copy.unavailable}/>}
    <Space><Button disabled={locked||!offset} onClick={()=>setOffset(n=>Math.max(0,n-20))}>{zh?'上一页 SKU':'Previous SKUs'}</Button><Button disabled={locked||!more} onClick={()=>setOffset(n=>n+20)}>{zh?'下一页 SKU':'Next SKUs'}</Button></Space>
    <Form.Item name="grade" label={zh?'实际成色':'Actual grade'} rules={[{required:true}]}><Select options={['S','A','B','C'].map(value=>({value,label:value}))}/></Form.Item>
    <Form.Item name="battery_health" label={zh?'实际电池健康度':'Actual battery health'} rules={[{required:true},{type:'integer',min:0,max:100}]}><InputNumber min={0} max={100} precision={0}/></Form.Item>
    <Form.Item name="notes" label={zh?'检测说明':'Inspection notes'} rules={[{required:true,whitespace:true},{max:2000}]}><Input.TextArea maxLength={2000} autoSize={{minRows:3,maxRows:8}}/></Form.Item>
    <Form.Item name="photos" label={zh?'检测照片':'Inspection photos'}><DevicePhotos scope={scope} copy={copy} disabled={locked} onBusy={setPhotoBusy} onExpired={onExpired}/></Form.Item>
    {!pending&&<Button type="primary" htmlType="submit" loading={saving} disabled={locked}>{zh?'保存检测结果':'Save inspection'}</Button>}
   </Form>
   {uncertain&&pending&&<Button type="primary" loading={saving} onClick={()=>void send(pending)}>{copy.retrySave}</Button>}
   {error!==undefined&&!uncertain&&<Button onClick={reload}>{zh?'重新读取':'Reload'}</Button>}
  </>}
  <Button disabled={saving||photoBusy} onClick={onClose}>{copy.cancel}</Button>
 </Space></Drawer>
}
