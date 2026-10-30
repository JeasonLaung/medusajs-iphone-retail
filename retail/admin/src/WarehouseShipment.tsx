import {useEffect,useRef,useState} from 'react'
import {Alert,Button,Form,Input,Space,Spin} from 'antd'
import {APIError,type ParcelInput} from './api'
import {warehouseShipment,shipWarehouseParcel,type WarehouseShipment as Shipment} from './warehouse-fulfillments-api'
import type {Copy} from './i18n'

export default function WarehouseShipment({location,order,id,copy,onExpired,onSaved,onBusy,blocked=false}:{location:string;order:string;id:string;copy:Copy;onExpired():void;onSaved():void;onBusy(busy:boolean):void;blocked?:boolean}){
 const [data,setData]=useState<Shipment>(),[loadError,setLoadError]=useState(false),[reload,setReload]=useState(0),[error,setError]=useState<'uncertain'|'denied'|'conflict'|'invalid'>(),[attempt,setAttempt]=useState<ParcelInput>(),[saving,setSaving]=useState(false),[done,setDone]=useState(false),active=useRef(false)
 const [form]=Form.useForm<{tracking_number:string;tracking_url:string}>()
 useEffect(()=>{const abort=new AbortController();setData(undefined);setLoadError(false);warehouseShipment(location,order,id,abort.signal).then(d=>{if(!abort.signal.aborted)setData(d)}).catch(e=>{if(!abort.signal.aborted){if(e instanceof APIError&&e.status===401)onExpired();else setLoadError(true)}});return()=>abort.abort()},[location,order,id,reload,onExpired])
 const send=async()=>{
  if(active.current||blocked||!data)return
  active.current=true
  let body=attempt
  if(!body){try{const v=await form.validateFields();body={operation_id:crypto.randomUUID(),expected_revision:data.revision,action:'ship',tracking_number:v.tracking_number.trim(),tracking_url:(v.tracking_url??'').trim()}}catch{active.current=false;return}}
  setAttempt(body);setSaving(true);onBusy(true);setError(undefined)
  try{await shipWarehouseParcel(location,order,id,body);setAttempt(undefined);setDone(true);onSaved()}
  catch(e){if(e instanceof APIError&&e.status===401)onExpired();else if(e instanceof APIError&&[403,404].includes(e.status))setError('denied');else if(e instanceof APIError&&e.status===409)setError('conflict');else if(e instanceof APIError&&e.status===400){setAttempt(undefined);setError('invalid')}else setError('uncertain')}
  finally{active.current=false;setSaving(false);onBusy(false)}
 }
 const locked=blocked||saving||!!attempt||done||!data?.enabled||!data.order_active||!data.supported||data.status!=='packed'
 if(loadError)return <Alert type="error" message={copy.unavailable} action={<Button onClick={()=>setReload(n=>n+1)}>{copy.retry}</Button>}/>
 if(!data)return <Spin/>
 return <Space direction="vertical" className="full-width" data-testid="warehouse-shipment">
  {done?<Alert type="success" showIcon message={copy.parcelSaved}/>:<>
   <Alert type="info" showIcon message={copy.shipParcel} description={id}/>
   <Form form={form} layout="vertical" disabled={locked}>
    <Form.Item name="tracking_number" label={copy.trackingNumber} rules={[{required:true,whitespace:true,message:copy.required},{max:200,message:copy.parcelTrackingInvalid}]}><Input maxLength={200}/></Form.Item>
    <Form.Item name="tracking_url" label={copy.trackingURL} rules={[{validator:(_,v:string)=>{if(!v?.trim())return Promise.resolve();try{const u=new URL(v.trim());if(['http:','https:'].includes(u.protocol)&&!u.username&&!u.password&&v.trim().length<=2048)return Promise.resolve()}catch{/* show field error */}return Promise.reject(new Error(copy.parcelTrackingInvalid))}}]}><Input maxLength={2048}/></Form.Item>
   </Form>
   {error&&<Alert type={error==='uncertain'?'warning':'error'} message={error==='uncertain'?copy.saveUncertain:error==='conflict'?copy.parcelConflict:error==='invalid'?copy.parcelTrackingInvalid:copy.accessDenied}/>}
   {error==='uncertain'&&<Button loading={saving} onClick={()=>void send()}>{copy.retrySave}</Button>}
   <Button type="primary" disabled={locked} loading={saving} onClick={()=>void send()}>{copy.shipParcel}</Button>
  </>}
 </Space>
}
