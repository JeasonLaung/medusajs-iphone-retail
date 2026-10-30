import {useEffect,useRef,useState} from 'react'
import {Alert,Button,Descriptions,Drawer,Form,Input,InputNumber,Select,Space,Spin,Typography} from 'antd'
import {APIError,readDeviceService,serviceDevice,type DeviceServiceContext,type DeviceServiceInput,type DevicePhoto} from './api'
import {copies} from './i18n'
import DevicePhotos from './DevicePhotos'
import ServiceVariantPicker from './ServiceVariantPicker'
type Fields={result:string;grade:string;battery_health:number;notes:string;photos:DevicePhoto[];tracking_number:string}
export default function DeviceService({order,requestID,assignment,onClose,onExpired}:{order:string;requestID:string;assignment:string;onClose():void;onExpired():void}){
 const zh=document.documentElement.lang.startsWith('zh'),copy=copies[zh?'zh-CN':'en'],[form]=Form.useForm<Fields>(),[data,setData]=useState<DeviceServiceContext>(),[attempt,setAttempt]=useState(0),[error,setError]=useState(false),[pending,setPending]=useState<DeviceServiceInput>(),[saving,setSaving]=useState(false),[failed,setFailed]=useState(false),[saved,setSaved]=useState(false),[photoBusy,setPhotoBusy]=useState(false),[picker,setPicker]=useState(false),[variant,setVariant]=useState<{id:string;label:string}>(),busy=useRef(false)
 useEffect(()=>{const c=new AbortController();setData(undefined);setError(false);readDeviceService(order,requestID,assignment,c.signal).then(d=>{if(!c.signal.aborted){setData(d);setVariant({id:d.variant_id,label:d.variant_id});form.setFieldsValue({result:'pass',grade:d.grade??'B',battery_health:d.battery_health??0,notes:'',photos:[],tracking_number:''})}}).catch(e=>{if(!c.signal.aborted){if(e instanceof APIError&&e.status===401)onExpired();else setError(true)}});return()=>c.abort()},[order,requestID,assignment,attempt,onExpired,form])
 const send=async(body:DeviceServiceInput)=>{if(busy.current)return;busy.current=true;setSaving(true);setFailed(false);setPending(body);try{await serviceDevice(order,requestID,assignment,body);setPending(undefined);setSaved(true);setAttempt(n=>n+1)}catch(e){if(e instanceof APIError&&e.status===401)onExpired();else setFailed(true)}finally{busy.current=false;setSaving(false)}}
 const reload=()=>{setPending(undefined);setFailed(false);setSaved(false);setAttempt(n=>n+1)}
 const repairShip=data?.state==='repair_ready'&&data.status==='returned'&&data.type==='repair'&&data.request_status==='processing',inspect=data?.status==='returned'&&['returned','recycled'].includes(data.category)&&['inspection','repair'].includes(data.state),locked=saving||!!pending||photoBusy||!data?.enabled
 const submit=(v:Fields)=>{if(!data||!variant)return;const body:DeviceServiceInput={operation_id:crypto.randomUUID(),expected_revision:data.revision,action:repairShip?'repair_ship':'inspect',result:'',variant_id:'',grade:'',battery_health:0,notes:'',photos:[],tracking_number:''};if(repairShip)body.tracking_number=v.tracking_number.trim();else Object.assign(body,{result:v.result,variant_id:variant.id,grade:v.grade,battery_health:v.battery_health,notes:v.notes.trim(),photos:v.photos??[]});void send(body)}
 return <Drawer open width={720} title={zh?'检测与维修返还':'Inspection and repair return'} onClose={()=>{if(!saving&&!photoBusy)onClose()}} closable={!saving&&!photoBusy} maskClosable={!saving&&!photoBusy} keyboard={!saving&&!photoBusy}><Space direction="vertical" size={16} className="full-width">
 <Alert type="info" message={zh?'检测记录设备实际成色、电池和处理结果。维修原机通过检测后需填写物流单号返还；这些操作不会增减数量库存。':'Record the actual condition, battery and inspection result. Dispatch the repaired original device with a tracking number after it passes inspection. Quantity stock is unchanged.'}/>
 {error?<><Alert type="error" message={zh?'设备记录暂不可用':'Device records unavailable'}/><Button onClick={reload}>{zh?'重新读取':'Reload'}</Button></>:!data?<Spin/>:<>
 <Descriptions column={1} items={[{key:'serial',label:zh?'序列号':'Serial number',children:data.serial_number},{key:'state',label:zh?'设备状态':'Device state',children:data.state},{key:'tracking',label:zh?'返还物流单号':'Return tracking number',children:data.tracking_number??'—'}]}/>
 {saved&&<Alert type="success" message={zh?'设备处理已保存':'Device operation saved'}/>}
 {!data.enabled&&<Alert type="warning" message={zh?'检测与返还操作尚未启用':'Inspection and repair return writes are disabled'}/>}
 {failed&&<Alert type="error" message={zh?'结果未确认或处理条件已变化，请重试原操作或重新读取核对。':'Result unconfirmed or prerequisites changed. Retry the same operation or reload to verify.'}/>}
 {(inspect||repairShip)&&<Form form={form} layout="vertical" disabled={locked} onFinish={submit}>
 {repairShip?<Form.Item name="tracking_number" label={zh?'返还物流单号':'Return tracking number'} rules={[{required:true,whitespace:true},{max:200}]}><Input aria-label={zh?'返还物流单号':'Return tracking number'} maxLength={200}/></Form.Item>:<>
 <Form.Item name="result" label={zh?'检测结果':'Inspection result'} rules={[{required:true}]}><Select options={[{value:'pass',label:zh?'检测通过':'Pass'},{value:'repair',label:zh?'继续维修':'Repair'},{value:'retire',label:zh?'报废':'Retire'},{value:'supplier_return',label:zh?'退回供应商':'Return to supplier'}]}/></Form.Item>
 <Typography.Paragraph className="identifier">SKU: {variant?.label}</Typography.Paragraph><Button disabled={locked} onClick={()=>setPicker(true)}>{zh?'选择检测后 SKU':'Select inspected SKU'}</Button>
 <Form.Item name="grade" label={zh?'实际成色':'Actual grade'} rules={[{required:true}]}><Select options={['S','A','B','C'].map(value=>({value,label:value}))}/></Form.Item>
 <Form.Item name="battery_health" label={zh?'实际电池健康度':'Actual battery health'} rules={[{required:true},{type:'integer',min:0,max:100}]}><InputNumber min={0} max={100} precision={0}/></Form.Item>
 <Form.Item name="notes" label={zh?'检测说明':'Inspection notes'} rules={[{required:true,whitespace:true},{max:2000}]}><Input.TextArea maxLength={2000} autoSize={{minRows:3,maxRows:8}}/></Form.Item>
 <Form.Item name="photos" label={zh?'检测照片':'Inspection photos'}><DevicePhotos copy={copy} disabled={saving||!!pending||!data.enabled} onBusy={setPhotoBusy} onExpired={onExpired}/></Form.Item>
 </>}
 {!pending&&<Button type="primary" htmlType="submit" disabled={locked||saved} loading={saving}>{repairShip?zh?'确认维修返还':'Dispatch repaired device':zh?'保存检测结果':'Save inspection'}</Button>}
 </Form>}
 {pending&&<Button type="primary" loading={saving} onClick={()=>void send(pending)}>{zh?'重试本次处理':'Retry this operation'}</Button>}
 <Button disabled={saving||photoBusy} onClick={reload}>{zh?'重新读取':'Reload'}</Button>
 </>}
 </Space>{picker&&<ServiceVariantPicker onExpired={onExpired} onClose={()=>setPicker(false)} onSelect={(id,label)=>{setVariant({id,label});setPicker(false)}}/>}</Drawer>
}
