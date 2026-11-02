import {useRef,useState} from 'react'
import {Alert,Button,Drawer,Space,Table,Typography} from 'antd'
import {APIError,makeWarehouseBatchAvailable} from './api'
import type {WarehouseBatchInput,WarehouseDevice} from './api'
import type {Copy} from './i18n'

export default function WarehouseBatchAvailable({location,devices,copy,onExpired,onClose,onSaved}:{location:string;devices:WarehouseDevice[];copy:Copy;onExpired():void;onClose():void;onSaved():void}){
 const zh=copy.language==='语言',busy=useRef(false)
 const [pending,setPending]=useState<WarehouseBatchInput>(),[saving,setSaving]=useState(false),[error,setError]=useState<unknown>()
 const uncertain=error!==undefined&&(!(error instanceof APIError)||error.status===0||error.status>=500)
 const send=async()=>{
  if(busy.current)return
  const input=pending??{operation_id:crypto.randomUUID(),devices:devices.map(d=>({id:d.id,expected_revision:d.revision}))}
  busy.current=true;setSaving(true);setPending(input);setError(undefined)
  try{await makeWarehouseBatchAvailable(location,input);onSaved()}
  catch(e){if(e instanceof APIError&&e.status===401)onExpired();else setError(e)}
  finally{busy.current=false;setSaving(false)}
 }
 return <Drawer open width="min(680px,100vw)" title={zh?'批量设为可售':'Make selected devices available'} closable={!saving} maskClosable={!saving} keyboard={!saving} onClose={()=>{if(!busy.current)onClose()}}><Space direction="vertical" size={24} className="full-width">
  <Typography.Paragraph>{zh?'所选设备必须全部符合可售条件，否则整批不保存。此操作不修改数量库存。':'All selected devices must qualify. If any device fails validation, the entire batch remains unchanged. Quantity inventory is unchanged.'}</Typography.Paragraph>
  <Typography.Text>{zh?'已选设备':'Selected devices'}: {devices.length}</Typography.Text>
  <Table rowKey="id" dataSource={devices} pagination={false} scroll={{x:400,y:360}} columns={[{title:'SN',dataIndex:'serial_number'},{title:copy.status,dataIndex:'state'}]}/>
  {error!==undefined&&<Alert type={uncertain?'warning':'error'} showIcon message={uncertain?copy.saveUncertain:copy.deviceIntakeConflict}/>}
  {(!pending||uncertain)&&<Button type="primary" loading={saving} onClick={()=>void send()}>{pending?copy.retrySave:copy.save}</Button>}
  <Button disabled={saving} onClick={onClose}>{copy.cancel}</Button>
 </Space></Drawer>
}
