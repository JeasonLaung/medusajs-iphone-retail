import {useEffect,useState} from 'react'
import {Alert,Button,Drawer,Form,Input,InputNumber,Select,Space,Spin,Table,Typography} from 'antd'
import {APIError,deviceIntakeContext,intakeDevices} from './api'
import type {DeviceIntakeContext,DeviceIntakeInput,DeviceIntakeRecord,DeviceIntakeRow} from './api'
import type {Copy} from './i18n'
import DevicePhotos from './DevicePhotos'

const emptyRow=():DeviceIntakeRow=>({serial_number:'',imei:'',imei2:'',eid:'',source_id:'',category:'sale',grade:null,battery_health:null,cost_cents:0,currency_code:'eur',bin:'',notes:'',photos:[]})
export default function DeviceIntake({productID,variantID,copy,onExpired,onClose}:{productID:string;variantID:string;copy:Copy;onExpired():void;onClose():void}){
 const [busyPhotos,setBusyPhotos]=useState<Set<number>>(()=>new Set())
 const photoBusy=(key:number,busy:boolean)=>setBusyPhotos(prior=>{const next=new Set(prior);if(busy)next.add(key);else next.delete(key);return next})
 const [form]=Form.useForm<{devices:DeviceIntakeRow[]}>(),[attempt,setAttempt]=useState(0)
 const [state,setState]=useState<{loading:boolean;data?:DeviceIntakeContext;error?:unknown}>({loading:true})
 const [pending,setPending]=useState<DeviceIntakeInput>(),[saving,setSaving]=useState(false),[error,setError]=useState<unknown>(),[conflict,setConflict]=useState(false),[result,setResult]=useState<DeviceIntakeRecord[]>()
 useEffect(()=>{const controller=new AbortController();setState({loading:true});deviceIntakeContext(productID,variantID,controller.signal).then(data=>{if(!controller.signal.aborted)setState({loading:false,data})}).catch(error=>{if(controller.signal.aborted)return;if(error instanceof APIError&&error.status===401)onExpired();else setState({loading:false,error})});return()=>controller.abort()},[attempt,productID,variantID,onExpired])
 const reload=()=>{setError(undefined);setConflict(false);setPending(undefined);setAttempt(n=>n+1)}
 const send=async(command:DeviceIntakeInput)=>{if(saving)return;setPending(command);setSaving(true);setError(undefined);try{const records=await intakeDevices(productID,variantID,command);setPending(undefined);setResult(records)}catch(error){if(error instanceof APIError&&error.status===401)onExpired();else{setError(error);if(error instanceof APIError&&[400,403,404,409].includes(error.status)){setConflict(true);setPending(undefined)}}}finally{setSaving(false)}}
 const submit=({devices}:{devices:DeviceIntakeRow[]})=>{if(state.data&&!pending&&!conflict&&!busyPhotos.size)void send({operation_id:crypto.randomUUID(),expected_revision:state.data.revision,devices:devices.map(d=>({...d,serial_number:d.serial_number.trim(),imei:d.imei.trim(),imei2:d.imei2.trim(),eid:d.eid.trim(),bin:d.bin.trim(),notes:d.notes.trim(),photos:d.photos}))})}
 return <Drawer open title={copy.deviceIntake} width={760} onClose={()=>{if(!saving&&!busyPhotos.size)onClose()}} closable={!saving&&!busyPhotos.size} maskClosable={!saving&&!busyPhotos.size} keyboard={!saving&&!busyPhotos.size}>
  <Space direction="vertical" size={24} className="full-width">
   <Alert type="info" showIcon message={copy.deviceIntakeInfo}/><Alert type="info" showIcon message={copy.devicePhotosHelp}/>
   {state.loading?<Spin/>:state.error!==undefined?<><Alert type="error" showIcon message={copy.unavailable}/><Button onClick={reload}>{copy.retry}</Button></>:state.data&&<>
    {!!state.data.recent_devices.length&&<><Typography.Title level={5}>{copy.deviceRecent}</Typography.Title><Table rowKey="id" dataSource={state.data.recent_devices} pagination={false} scroll={{x:650}} columns={[{title:'SN',dataIndex:'serial_number'},{title:'IMEI',dataIndex:'imei'},{title:copy.stockLocation,dataIndex:'location_id'},{title:copy.status,dataIndex:'state'},{title:copy.devicePhotos,dataIndex:'photos',render:photos=><Space wrap>{photos.map((p:{id:string;url:string})=><a key={p.id} href={p.url} target="_blank" rel="noreferrer"><img src={p.url} alt={copy.devicePhotos} width={64} height={64} style={{objectFit:'cover'}}/></a>)}</Space>}]}/></>}
    <Typography.Paragraph>{state.data.options.map(o=>`${o.title}: ${o.value}`).join(' / ')}</Typography.Paragraph>
    {result?<><Alert type="success" showIcon message={copy.deviceIntakeSaved}/><Table<DeviceIntakeRecord> rowKey="id" dataSource={result} pagination={{pageSize:20,showSizeChanger:false}} scroll={{x:550}} columns={[{title:'SN',dataIndex:'serial_number'},{title:copy.stockLocation,dataIndex:'location_id'},{title:copy.status,dataIndex:'state',render:value=>value==='available'?copy.deviceAvailable:copy.deviceInspection}]}/></>:<>
     {!state.data.sources.length&&<Alert type="warning" showIcon message={copy.deviceIntakeNoSource}/>}
     {error!==undefined&&<Alert type="error" showIcon message={conflict?copy.deviceIntakeConflict:copy.deviceIntakeUncertain}/>}
     <Form form={form} initialValues={{devices:[emptyRow()]}} layout="vertical" requiredMark={false} disabled={saving||!!pending||conflict} onFinish={submit}>
      <Form.List name="devices">{(fields,{add,remove})=><Space direction="vertical" size={24} className="full-width">{fields.map(({key,name})=><div key={key}>
       <Space wrap><Typography.Title level={5}>{copy.deviceIntake} {name+1}</Typography.Title>{fields.length>1&&<Button disabled={!!busyPhotos.size} onClick={()=>remove(name)}>{copy.deviceRemoveRow}</Button>}</Space>
       <Form.Item label={copy.deviceSourceSelf} name={[name,'source_id']} rules={[{required:true,message:copy.required}]}><Select options={state.data!.sources.map(s=>({value:s.id,label:`${s.name} (${s.location_name})`}))}/></Form.Item>
       <Form.Item label={copy.deviceCategory} name={[name,'category']} rules={[{required:true,message:copy.required}]}><Select options={[{value:'sale',label:copy.deviceSale},{value:'returned',label:copy.deviceReturned},{value:'recycled',label:copy.deviceRecycled}]}/></Form.Item>
       {(['serial_number','imei','imei2','eid'] as const).map(field=><Form.Item key={field} label={{serial_number:'SN',imei:'IMEI',imei2:'IMEI 2',eid:'EID'}[field]} name={[name,field]} extra={field==='serial_number'&&state.data!.requires_radio?copy.deviceRadioRequired:undefined} rules={[...(field==='serial_number'?[{required:true,whitespace:true,message:copy.required}]:[]),{max:field==='serial_number'?100:field==='eid'?32:15,message:copy.editTooLong},...(field==='serial_number'?[]:[{pattern:field==='eid'?/^(?:[0-9]{32})?$/:/^(?:[0-9]{15})?$/,message:copy.editInvalid}])]}><Input maxLength={field==='serial_number'?100:field==='eid'?32:15}/></Form.Item>)}
       <Form.Item label={copy.deviceGrade} name={[name,'grade']}><Select allowClear options={['S','A','B','C'].map(value=>({value,label:value}))} onChange={v=>form.setFieldValue(['devices',name,'grade'],v??null)}/></Form.Item>
       <Form.Item label={copy.deviceBattery} name={[name,'battery_health']}><InputNumber min={0} max={100} precision={0}/></Form.Item>
       <Form.Item label={copy.deviceCostCents} name={[name,'cost_cents']} rules={[{required:true,message:copy.required}]}><InputNumber min={0} max={100000000} precision={0}/></Form.Item>
       <Form.Item label={copy.currency} name={[name,'currency_code']} rules={[{required:true,message:copy.required},{pattern:/^[a-z]{3}$/,message:copy.editInvalid}]}><Input maxLength={3}/></Form.Item>
       <Form.Item label={copy.deviceBin} name={[name,'bin']}><Input maxLength={200}/></Form.Item><Form.Item label={copy.deviceNotes} name={[name,'notes']}><Input.TextArea maxLength={2000} autoSize={{minRows:2,maxRows:6}}/></Form.Item>
       <Form.Item label={copy.devicePhotos} name={[name,'photos']}><DevicePhotos copy={copy} onExpired={onExpired} disabled={saving||!!pending||conflict} onBusy={busy=>photoBusy(key,busy)}/></Form.Item>
      </div>)}{fields.length<200&&<Button onClick={()=>add(emptyRow())}>{copy.deviceAddRow}</Button>}</Space>}</Form.List>
      {!pending&&!conflict&&<Button className="mt-4" type="primary" htmlType="submit" loading={saving} disabled={!state.data.sources.length||!!busyPhotos.size}>{copy.deviceIntakeSave}</Button>}
     </Form>
     <Space wrap>{pending&&<Button type="primary" loading={saving} onClick={()=>void send(pending)}>{copy.retrySave}</Button>}<Button disabled={saving||!!pending||!!busyPhotos.size} onClick={reload}>{copy.reload}</Button></Space>
    </>}
   </>}
  </Space>
 </Drawer>
}
