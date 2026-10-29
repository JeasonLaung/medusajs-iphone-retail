import {useEffect,useRef,useState} from 'react'
import {Alert,Button,Descriptions,Drawer,Form,Input,Popconfirm,Radio,Space,Spin,Table} from 'antd'
import {APIError,readOrderParcel,changeOrderParcel} from './api'
import type {ParcelContext,ParcelInput} from './api'
import type {Copy} from './i18n'

export default function OrderParcel({orderID,fulfillmentID,copy,onClose,onExpired}:{orderID:string;fulfillmentID:string;copy:Copy;onClose():void;onExpired():void}){
 const active=useRef(false)
 const [context,setContext]=useState<ParcelContext>(),[loadError,setLoadError]=useState(false),[reload,setReload]=useState(0),[saving,setSaving]=useState(false),[attempt,setAttempt]=useState<ParcelInput>(),[error,setError]=useState<'uncertain'|'conflict'|'denied'|'invalid'>(),[action,setAction]=useState<'ship'|'cancel'>('ship'),[done,setDone]=useState(false)
 const [form]=Form.useForm<{tracking_number:string;tracking_url:string}>()
 useEffect(()=>{const abort=new AbortController();setContext(undefined);setLoadError(false);readOrderParcel(orderID,fulfillmentID,abort.signal).then(d=>{if(!abort.signal.aborted)setContext(d)}).catch(e=>{if(!abort.signal.aborted){if(e instanceof APIError&&e.status===401)onExpired();else setLoadError(true)}});return()=>abort.abort()},[orderID,fulfillmentID,reload,onExpired])
 const save=async()=>{
  if(active.current||!context?.enabled)return
  active.current=true
  let body=attempt
  if(!body){let values={tracking_number:'',tracking_url:''};if(action==='ship'){try{values=await form.validateFields()}catch{active.current=false;return}}body={operation_id:crypto.randomUUID(),expected_revision:context.revision,action,tracking_number:values.tracking_number.trim(),tracking_url:values.tracking_url.trim()}}
  setSaving(true);setAttempt(body);setError(undefined)
  try{await changeOrderParcel(orderID,fulfillmentID,body);setAttempt(undefined);setDone(true);setReload(n=>n+1)}catch(e){if(e instanceof APIError&&e.status===401)onExpired();else if(e instanceof APIError&&e.status===409)setError('conflict');else if(e instanceof APIError&&(e.status===403||e.status===404))setError('denied');else if(e instanceof APIError&&e.status===400){setAttempt(undefined);setError('invalid')}else setError('uncertain')}finally{active.current=false;setSaving(false)}
 }
 const locked=saving||!!attempt||!context?.enabled||!context.order_active||!context.supported||context.status!=='packed'||context.devices.length===0
 const statuses:Record<string,string>={packed:copy.allocationStates.packed,shipped:copy.allocationStates.shipped,canceled:copy.parcelCanceled,delivered:copy.deliveredAt,unpacked:copy.parcelUnpacked}
 return <Drawer open title={copy.parcelOperations} width="min(900px, 100vw)" onClose={()=>{if(!active.current)onClose()}} closable={!saving} maskClosable={!saving} keyboard={!saving}>
  <Space direction="vertical" size={24} className="full-width">
   {done&&<Alert type="success" showIcon message={copy.parcelSaved}/>}
   {loadError?<Space direction="vertical"><Alert type="error" showIcon message={copy.unavailable}/><Button onClick={()=>setReload(n=>n+1)}>{copy.retry}</Button></Space>:!context?<Spin/>:<>
    <Descriptions column={1} items={[{key:'parcel',label:copy.allocationFulfillment,children:fulfillmentID},{key:'status',label:copy.status,children:statuses[context.status]??context.status}]}/>
    {!context.enabled&&<Alert type="warning" showIcon message={copy.parcelDisabled}/>}
    {(!context.order_active||!context.supported)&&<Alert type="warning" showIcon message={!context.order_active?copy.allocationOrderInactive:copy.parcelUnsupported}/>}
    <Table rowKey="assignment_id" dataSource={context.devices} scroll={{x:500}} pagination={false} columns={[{title:'SN',dataIndex:'serial_number'},{title:copy.status,dataIndex:'status',render:(v:string)=>copy.allocationStates[v as keyof typeof copy.allocationStates]??v}]}/>
    <Radio.Group aria-label={copy.parcelAction} value={action} disabled={locked} onChange={e=>setAction(e.target.value)} options={[{value:'ship',label:copy.shipParcel},{value:'cancel',label:copy.cancelParcel}]}/>
    <Alert type={action==='cancel'?'warning':'info'} showIcon message={action==='cancel'?copy.parcelCancelInfo:copy.parcelShipInfo}/>
    {action==='ship'&&<Form form={form} layout="vertical" initialValues={{tracking_number:'',tracking_url:''}} disabled={locked}>
     <Form.Item name="tracking_number" label={copy.trackingNumber} rules={[{required:true,whitespace:true,message:copy.parcelTrackingRequired},{max:200,message:copy.parcelTrackingRequired}]}><Input maxLength={200}/></Form.Item>
     <Form.Item name="tracking_url" label={copy.trackingURL} rules={[{validator:(_,value:string)=>{if(!value.trim())return Promise.resolve();try{const url=new URL(value.trim());if(['https:','http:'].includes(url.protocol)&&!url.username&&!url.password&&value.trim().length<=2048)return Promise.resolve()}catch{/* Show local field validation. */}return Promise.reject(new Error(copy.parcelTrackingInvalid))}}]}><Input maxLength={2048}/></Form.Item>
    </Form>}
    {error&&<Alert type={error==='uncertain'?'warning':'error'} showIcon message={error==='uncertain'?copy.saveUncertain:error==='conflict'?copy.parcelConflict:error==='invalid'?copy.parcelTrackingInvalid:copy.accessDenied}/>}
    {attempt&&error==='uncertain'&&<Button loading={saving} onClick={()=>void save()}>{copy.retrySave}</Button>}
    {action==='cancel'?<Popconfirm title={copy.cancelParcel} description={copy.parcelCancelInfo} okText={copy.confirmCancelParcel} cancelText={copy.cancel} disabled={locked} onConfirm={()=>save()}><Button aria-label={copy.cancelParcel} danger disabled={locked} loading={saving}>{copy.cancelParcel}</Button></Popconfirm>:<Button aria-label={copy.shipParcel} type="primary" disabled={locked} loading={saving} onClick={()=>void save()}>{copy.shipParcel}</Button>}
   </>}
   <Button disabled={saving} onClick={onClose}>{copy.cancel}</Button>
  </Space>
 </Drawer>
}
