import {useEffect,useState} from 'react'
import {Alert,Button,Drawer,Form,Input,Radio,Space,Spin,Table,Tag} from 'antd'
import {APIError,deviceSources,createDeviceSource} from './api'
import type {DeviceSource,DeviceSourceInput,DeviceSourcesDirectory} from './api'
import type {Copy} from './i18n'
import {useAccess} from './Permissions'

export default function DeviceSources({copy,onClose,onExpired,onChanged}:{copy:Copy;onClose():void;onExpired():void;onChanged():void}){
 const access=useAccess(),[form]=Form.useForm<{name:string;type:'self'|'supplier'}>()
 const [attempt,setAttempt]=useState(0),[state,setState]=useState<{loading:boolean;data?:DeviceSourcesDirectory;error?:unknown}>({loading:true})
 const [editing,setEditing]=useState(false),[saving,setSaving]=useState(false),[pending,setPending]=useState<DeviceSourceInput>(),[error,setError]=useState<unknown>(),[conflict,setConflict]=useState(false)
 const write=access.can('device.source.write','warehouse.write')
 useEffect(()=>{
  const controller=new AbortController();setState({loading:true})
  deviceSources(controller.signal).then(data=>{if(!controller.signal.aborted)setState({loading:false,data})}).catch(error=>{if(controller.signal.aborted)return;if(error instanceof APIError&&error.status===401)onExpired();else setState({loading:false,error})})
  return()=>controller.abort()
 },[attempt,onExpired])
 const reset=()=>{setPending(undefined);setError(undefined);setConflict(false);setEditing(false);setAttempt(n=>n+1)}
 const send=async(command:DeviceSourceInput)=>{
  if(saving)return;setSaving(true);setError(undefined);setPending(command)
  try{await createDeviceSource(command);reset();onChanged()}
  catch(error){if(error instanceof APIError&&error.status===401)onExpired();else{setError(error);if(error instanceof APIError&&[400,403,404,409].includes(error.status)){setConflict(true);setPending(undefined)}}}
  finally{setSaving(false)}
 }
 const submit=({name,type}:{name:string;type:'self'|'supplier'})=>{if(state.data&&!pending&&!conflict)void send({operation_id:crypto.randomUUID(),expected_revision:state.data.revision,name:name.trim(),type})}
 return <Drawer open width={760} title={copy.deviceSources} onClose={()=>{if(!saving)onClose()}} closable={!saving} maskClosable={!saving} keyboard={!saving}>
  <Space direction="vertical" size={24} className="full-width">
   <Alert type="info" showIcon message={copy.deviceSourcesInfo}/>
   {state.loading?<Spin/>:state.error!==undefined?<><Alert type="error" showIcon message={copy.unavailable}/><Button onClick={reset}>{copy.retry}</Button></>:state.data&&<>
    <Space wrap><Button disabled={saving||!!pending} onClick={reset}>{copy.reload}</Button>{write&&!editing&&<Button type="primary" onClick={()=>{form.setFieldsValue({name:'',type:'self'});setEditing(true)}}>{copy.newDeviceSource}</Button>}</Space>
    <Form style={{display:editing?undefined:'none'}} form={form} layout="vertical" disabled={saving||!!pending||conflict} onFinish={submit} requiredMark={false}>
     <Form.Item label={copy.name} name="name" rules={[{required:true,whitespace:true,message:copy.required},{max:100,message:copy.editTooLong}]}><Input maxLength={100}/></Form.Item>
     <Form.Item label={copy.deviceSourceType} name="type" rules={[{required:true,message:copy.required}]}><Radio.Group options={[{label:copy.deviceSourceSelf,value:'self'},{label:copy.deviceSourceSupplier,value:'supplier'}]}/></Form.Item>
     {!pending&&!conflict&&<Button type="primary" htmlType="submit" loading={saving}>{copy.save}</Button>}
    </Form>
    {error!==undefined&&<Alert type="error" showIcon message={conflict?copy.deviceSourceConflict:copy.deviceSourceUncertain}/>}
    {!!pending&&<Button type="primary" loading={saving} onClick={()=>void send(pending)}>{copy.retrySave}</Button>}
    <Table<DeviceSource> rowKey="id" dataSource={state.data.sources} pagination={{pageSize:20,showSizeChanger:false}} scroll={{x:600}} locale={{emptyText:copy.noDeviceSources}} columns={[
     {title:copy.name,dataIndex:'name'},
     {title:copy.deviceSourceType,dataIndex:'type',render:(value:DeviceSource['type'])=>value==='self'?copy.deviceSourceSelf:copy.deviceSourceSupplier},
     {title:copy.stockLocation,key:'location',render:(_,row)=><Space direction="vertical">{row.location_name??<Tag color="warning">{copy.locationUnavailable}</Tag>}<span>{row.location_id}</span></Space>},
    ]}/>
   </>}
  </Space>
 </Drawer>
}
