import {useEffect,useState} from 'react'
import {Alert,App,Button,Drawer,Form,Image,Input,Radio,Result,Space,Spin,Table,Typography} from 'antd'
import {APIError,getWarehouseAccess,getWarehouseDevices,commandWarehouseDevice} from './api'
import type {WarehouseAccess,WarehouseDevice,WarehouseDeviceAction,WarehouseDeviceCommand,WarehouseDevicePage} from './api'
import type {Copy} from './i18n'
import WarehouseIntakePicker from './WarehouseIntakePicker'

export default function WarehouseDevices({copy,onExpired}:{copy:Copy;onExpired():void}){
 const [form]=Form.useForm<{notes:string}>()
 const [intake,setIntake]=useState(false)
 const {message}=App.useApp(),[attempt,setAttempt]=useState(0),[access,setAccess]=useState<WarehouseAccess>(),[location,setLocation]=useState(''),[category,setCategory]=useState<'sale'|'returned'>('sale'),[q,setQ]=useState(''),[offset,setOffset]=useState(0),[data,setData]=useState<WarehouseDevicePage>(),[loading,setLoading]=useState(true),[error,setError]=useState<unknown>()
 const [editor,setEditor]=useState<{row:WarehouseDevice;action:WarehouseDeviceAction}>(),[command,setCommand]=useState<WarehouseDeviceCommand>(),[saving,setSaving]=useState(false),[saveError,setSaveError]=useState<unknown>()
 const reload=()=>setAttempt(n=>n+1)
 useEffect(()=>{
  const abort=new AbortController();setLoading(true);setError(undefined);setData(undefined)
  const run=async()=>{try{
   const current=await getWarehouseAccess(abort.signal);if(abort.signal.aborted)return;setAccess(current)
   const cats=(['sale','returned'] as const).filter(c=>current.actions.includes(c==='sale'?'warehouse.intake.read':'warehouse.return.read'))
   if(!cats.length)return
   const nextCategory=cats.includes(category)?category:cats[0],nextLocation=current.locations.some(l=>l.id===location)?location:current.locations[0]?.id??''
   if(nextCategory!==category||nextLocation!==location){setCategory(nextCategory);setLocation(nextLocation);setOffset(0);return}
   if(nextLocation){const page=await getWarehouseDevices(nextLocation,nextCategory,offset,q,abort.signal);if(!abort.signal.aborted)setData(page)}
  }catch(error){if(abort.signal.aborted)return;if(error instanceof APIError&&error.status===401)onExpired();else setError(error)}finally{if(!abort.signal.aborted)setLoading(false)}}
  void run();return()=>abort.abort()
 },[attempt,location,category,offset,q,onExpired])
 const send=async(input:WarehouseDeviceCommand)=>{if(!editor||saving)return;setSaving(true);setCommand(input);setSaveError(undefined);try{await commandWarehouseDevice(location,category,editor.row.id,input);setEditor(undefined);setCommand(undefined);void message.success(copy.deviceCommandSaved);reload()}catch(error){if(error instanceof APIError&&error.status===401)onExpired();else setSaveError(error)}finally{setSaving(false)}}
 const close=()=>{if(saving)return;setEditor(undefined);setCommand(undefined);setSaveError(undefined);reload()}
 if(loading)return <Spin/>
 if(error)return <Space direction="vertical">{error instanceof APIError&&error.status===403?<Result status="403" title={copy.accessDenied}/>:<Alert type="error" showIcon message={copy.unavailable}/>}<Button onClick={reload}>{copy.retry}</Button></Space>
 const categories=(['sale','returned'] as const).filter(c=>access?.actions.includes(c==='sale'?'warehouse.intake.read':'warehouse.return.read'))
 if(!categories.length)return <Result status="403" title={copy.accessDenied}/>
 const uncertain=saveError!==undefined&&(!(saveError instanceof APIError)||saveError.status===0||saveError.status>=500)
 return <section className="account-panel"><Space direction="vertical" size={24} className="full-width">
  <Typography.Paragraph type="secondary">{copy.scopedDevicesInfo}</Typography.Paragraph>
  <Space wrap>{location&&access?.actions.includes(category==='sale'?'warehouse.intake.write':'warehouse.return.write')&&<Button type="primary" onClick={()=>setIntake(true)}>{copy.deviceIntake}</Button>}<Button onClick={reload}>{copy.reload}</Button><Radio.Group aria-label={copy.stockLocation} value={location} onChange={e=>{setLocation(e.target.value);setOffset(0)}} options={access?.locations.map(l=>({value:l.id,label:l.name}))}/><Radio.Group aria-label={copy.deviceCategory} value={category} onChange={e=>{setCategory(e.target.value);setOffset(0)}} options={categories.map(c=>({value:c,label:c==='sale'?copy.deviceSale:copy.deviceReturned}))}/></Space>
  <Input.Search aria-label={copy.deviceExactSearch} placeholder={copy.deviceExactSearch} maxLength={100} allowClear onSearch={value=>{setQ(value.trim());setOffset(0)}}/>
  {!access?.locations.length?<Alert type="info" showIcon message={copy.noWarehouseScope}/>:data&&<>
   <Table<WarehouseDevice> rowKey="id" dataSource={data.devices} pagination={false} scroll={{x:1000}} columns={[
    {title:'SN',dataIndex:'serial_number'},{title:'IMEI',dataIndex:'imei'},{title:'IMEI 2',dataIndex:'imei2'},{title:'EID',dataIndex:'eid'},{title:'SKU',dataIndex:'variant_title'},
    {title:copy.status,dataIndex:'state'},{title:copy.deviceBin,dataIndex:'bin'},
    {title:copy.devicePhotos,render:(_,row)=><Image.PreviewGroup>{row.photos.map(p=><Image key={p.id} src={p.url} alt={copy.devicePhotos} width={64} height={64} style={{objectFit:'cover'}}/>)}</Image.PreviewGroup>},
    {title:copy.permissionActions,render:(_,row)=><Space wrap>{row.allowed_actions.map(action=><Button key={action} danger={action==='delete'||action==='retire'} onClick={()=>{form.resetFields();setEditor({row,action});setCommand(undefined);setSaveError(undefined)}}>{copy.deviceCommandNames[action]}</Button>)}</Space>},
   ]}/>
   <Space><Button disabled={!offset} onClick={()=>setOffset(n=>Math.max(0,n-50))}>{copy.previousWarehousePage}</Button><Typography.Text>{offset+1}–{offset+data.devices.length}</Typography.Text><Button disabled={!data.has_more} onClick={()=>setOffset(n=>n+50)}>{copy.nextWarehousePage}</Button></Space>
  </>}
 </Space><Drawer open={!!editor} title={editor?copy.deviceCommandNames[editor.action]:''} width={480} closable={!saving} maskClosable={!saving} keyboard={!saving} onClose={close}>
  {editor&&<Space direction="vertical" size={24} className="full-width"><Typography.Text>{editor.row.serial_number}</Typography.Text><Typography.Paragraph>{copy.deviceCommandConfirm}</Typography.Paragraph>
   {saveError!==undefined&&<Alert type="error" showIcon message={uncertain?copy.saveUncertain:copy.deviceIntakeConflict}/>}
   <Form form={form} layout="vertical" disabled={saving||!!command} onFinish={values=>void send({operation_id:crypto.randomUUID(),expected_revision:editor.row.revision,action:editor.action,...(editor.action==='transfer_return'?{notes:values.notes.trim()}:{})})}>
    {editor.action==='transfer_return'&&<><Alert type="info" showIcon message={copy.deviceTransferReturnInfo}/><Form.Item name="notes" label={copy.deviceTransferReturnReason} rules={[{required:true,whitespace:true,message:copy.required},{max:2000,message:copy.editTooLong}]}><Input.TextArea maxLength={2000} autoSize={{minRows:3,maxRows:8}}/></Form.Item></>}
    {!command&&<Button type="primary" htmlType="submit" danger={editor.action==='delete'||editor.action==='retire'} loading={saving}>{copy.save}</Button>}
   </Form>
   {command&&uncertain&&<Button type="primary" loading={saving} onClick={()=>void send(command)}>{copy.retrySave}</Button>}<Button disabled={saving} onClick={close}>{copy.cancel}</Button>
  </Space>}
 </Drawer>{intake&&<WarehouseIntakePicker scope={{locationID:location,category}} copy={copy} onExpired={onExpired} onClose={()=>{setIntake(false);reload()}}/>}</section>
}
