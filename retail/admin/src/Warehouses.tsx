import {useEffect,useRef,useState} from 'react'
import {Alert,App,Button,Drawer,Form,Input,Result,Space,Spin,Table,Typography} from 'antd'
import {APIError,warehouses,saveWarehouse} from './api'
import type {Warehouse,WarehouseAddress,WarehouseCommand,WarehouseDirectory} from './api'
import type {Copy} from './i18n'
import {useAccess} from './Permissions'
import DeviceSources from './DeviceSources'

const empty:WarehouseAddress={address_1:'',address_2:'',company:'',city:'',country_code:'',phone:'',province:'',postal_code:''}
type Fields={name:string;address:WarehouseAddress}
export default function Warehouses({copy,onExpired}:{copy:Copy;onExpired():void}){
 const access=useAccess(),{message}=App.useApp(),[form]=Form.useForm<Fields>()
 const [sourcesOpen,setSourcesOpen]=useState(false)
 const [attempt,setAttempt]=useState(0),[data,setData]=useState<WarehouseDirectory>(),[loading,setLoading]=useState(true),[loadError,setLoadError]=useState<unknown>()
 const [editor,setEditor]=useState<{location?:Warehouse}>(),[command,setCommand]=useState<WarehouseCommand>(),[error,setError]=useState<unknown>(),[pending,setPending]=useState(false),busy=useRef(false)
 useEffect(()=>{
  const abort=new AbortController();setLoading(true);setLoadError(undefined)
  warehouses(abort.signal).then(value=>{if(!abort.signal.aborted)setData(value)}).catch(error=>{if(abort.signal.aborted)return;if(error instanceof APIError&&error.status===401)onExpired();else{setData(undefined);setLoadError(error)}}).finally(()=>{if(!abort.signal.aborted)setLoading(false)})
  return()=>abort.abort()
 },[attempt,onExpired])
 const open=(location?:Warehouse)=>{form.setFieldsValue({name:location?.name??'',address:{...empty,...location?.address}});setCommand(undefined);setError(undefined);setEditor({location})}
 const close=()=>{if(busy.current)return;setEditor(undefined);setCommand(undefined);setError(undefined);setAttempt(n=>n+1)}
 const send=async(input:WarehouseCommand)=>{
  if(busy.current)return;busy.current=true;setPending(true);setError(undefined);setCommand(input)
  try{await saveWarehouse(input);setEditor(undefined);setCommand(undefined);setAttempt(n=>n+1);void message.success(copy.warehouseSaved)}
  catch(error){if(error instanceof APIError&&error.status===401)onExpired();else setError(error)}
  finally{busy.current=false;setPending(false)}
 }
 const submit=(fields:Fields)=>{if(command||!editor||!data||!access.can('warehouse.read','warehouse.write'))return;void send({operation_id:crypto.randomUUID(),expected_revision:data.revision,id:editor.location?.id??'',name:fields.name,address:{...empty,...fields.address,country_code:fields.address.country_code.toLowerCase()}})}
 const e=error instanceof APIError?error:undefined,uncertain=error!==undefined&&(!e||e.status===0||e.status>=500)
 if(loading)return <Spin/>
 if(!data)return <Space direction="vertical">{loadError instanceof APIError&&loadError.status===403?<Result status="403" title={copy.accessDenied}/>:<Alert type="error" showIcon message={copy.unavailable}/>}<Button onClick={()=>setAttempt(n=>n+1)}>{copy.retry}</Button></Space>
 const write=access.can('warehouse.read','warehouse.write')
 const fields:[keyof WarehouseAddress,string,number,boolean][]=[['address_1',copy.addressLine1,500,true],['address_2',copy.addressLine2,500,false],['company',copy.company,160,false],['city',copy.city,160,false],['province',copy.province,160,false],['postal_code',copy.postalCode,40,false],['country_code',copy.country,2,true],['phone',copy.phone,80,false]]
 return <section className="account-panel"><Space direction="vertical" size={24} className="full-width">
  <Typography.Paragraph type="secondary">{copy.warehouseInfo}</Typography.Paragraph>
  <Space wrap>{access.can('device.read')&&<Button onClick={()=>setSourcesOpen(true)}>{copy.deviceSources}</Button>}<Button onClick={()=>setAttempt(n=>n+1)}>{copy.reload}</Button>{write&&<Button type="primary" onClick={()=>open()}>{copy.newWarehouse}</Button>}</Space>
  <Table<Warehouse> rowKey="id" dataSource={data.locations} pagination={{pageSize:20,showSizeChanger:false}} scroll={{x:700}} locale={{emptyText:copy.noWarehouses}} columns={[
   {title:copy.name,dataIndex:'name',render:(name:string,row)=>write?<Button type="link" onClick={()=>open(row)}>{name}</Button>:name},
   {title:copy.stockLocation,dataIndex:'id'},
   {title:copy.addressLine1,render:(_,row)=>row.address?[row.address.address_1,row.address.address_2,row.address.postal_code,row.address.city,row.address.country_code.toUpperCase()].filter(Boolean).join(', '):copy.warehouseAddressMissing},
   {title:copy.phone,render:(_,row)=>row.address?.phone||'—'},
  ]}/>
 </Space>
 {sourcesOpen&&access.can('device.read')&&<DeviceSources copy={copy} onClose={()=>setSourcesOpen(false)} onExpired={onExpired} onChanged={()=>setAttempt(n=>n+1)}/>}
 <Drawer open={!!editor} onClose={close} title={editor?.location?copy.editWarehouse:copy.newWarehouse} width={520}>
  {error!==undefined&&<Alert type="error" showIcon message={e?.status===409?copy.warehouseConflict:uncertain?copy.saveUncertain:e?.status===400?copy.editInvalid:copy.editForbidden}/>}
  <Form form={form} layout="vertical" disabled={pending||!!command} onFinish={submit} requiredMark={false}>
   <Form.Item name="name" label={copy.name} rules={[{required:true,whitespace:true,message:copy.required},{max:160,message:copy.editTooLong}]}><Input maxLength={160}/></Form.Item>
   {fields.map(([key,label,max,required])=><Form.Item key={key} name={['address',key]} label={label} extra={key==='country_code'?copy.warehouseCountryHelp:undefined} rules={[{required,whitespace:required,message:copy.required},{max,message:copy.editTooLong},...(key==='country_code'?[{pattern:/^[a-zA-Z]{2}$/,message:copy.editInvalid}]:[])]}><Input maxLength={max}/></Form.Item>)}
   {!command&&<Button type="primary" htmlType="submit" loading={pending}>{copy.save}</Button>}
  </Form>
  <Space className="mt-4"><Button onClick={close} disabled={pending}>{copy.cancel}</Button>{command&&uncertain&&<Button type="primary" disabled={pending} loading={pending} onClick={()=>void send(command)}>{copy.retrySave}</Button>}</Space>
 </Drawer></section>
}
