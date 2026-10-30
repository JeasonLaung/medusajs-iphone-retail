import {useEffect,useRef,useState} from 'react'
import {Alert,Button,Modal,Space,Spin,Table,Typography} from 'antd'
import {APIError,type ParcelInput} from './api'
import {warehouseRelease,releaseWarehouseDevice,type WarehouseRelease as Context} from './warehouse-fulfillments-api'
import type {Copy} from './i18n'

export default function WarehouseRelease({location,order,id,copy,onExpired,onSaved,onBusy,blocked}:{location:string;order:string;id:string;copy:Copy;onExpired():void;onSaved():void;onBusy(busy:boolean):void;blocked:boolean}){
 const [data,setData]=useState<Context>(),[loadError,setLoadError]=useState(false),[reload,setReload]=useState(0),[selected,setSelected]=useState<Context['devices'][number]>(),[command,setCommand]=useState<{assignment:string;body:ParcelInput}>(),[saving,setSaving]=useState(false),[error,setError]=useState<'uncertain'|'conflict'|'denied'|'invalid'>(),active=useRef(false)
 const zh=copy.language==='语言',label=zh?'解除设备分配':'Release device allocation'
 useEffect(()=>{const abort=new AbortController();setData(undefined);setLoadError(false);warehouseRelease(location,order,id,abort.signal).then(d=>{if(!abort.signal.aborted)setData(d)}).catch(e=>{if(!abort.signal.aborted){if(e instanceof APIError&&e.status===401)onExpired();else setLoadError(true)}});return()=>abort.abort()},[location,order,id,reload,onExpired])
 const send=async()=>{
  if(active.current||blocked||!data||(!selected&&!command))return
  const c=command??{assignment:selected!.assignment_id,body:{operation_id:crypto.randomUUID(),expected_revision:data.revision,action:'cancel',tracking_number:'',tracking_url:''} satisfies ParcelInput}
  active.current=true;setSaving(true);onBusy(true);setCommand(c);setError(undefined)
  try{await releaseWarehouseDevice(location,order,id,c.assignment,c.body);onSaved()}
  catch(e){if(e instanceof APIError&&e.status===401)onExpired();else if(e instanceof APIError&&[403,404].includes(e.status))setError('denied');else if(e instanceof APIError&&e.status===409)setError('conflict');else if(e instanceof APIError&&e.status===400)setError('invalid');else setError('uncertain')}
  finally{active.current=false;setSaving(false);onBusy(false)}
 }
 if(loadError)return <Alert type="error" message={copy.unavailable} action={<Button onClick={()=>setReload(n=>n+1)}>{copy.retry}</Button>}/>
 if(!data)return <Spin/>
 const locked=blocked||saving||!!command||!data.enabled||!data.order_active||!data.supported||data.status!=='packed'
 return <Space direction="vertical" className="full-width" data-testid="warehouse-release">
  <Typography.Title level={5}>{label}</Typography.Title>
  <Table rowKey="assignment_id" size="small" pagination={false} scroll={{x:360}} dataSource={data.devices} columns={[{title:'SN',dataIndex:'serial_number'},{title:copy.status,render:(_,d)=><Button danger disabled={locked} onClick={()=>setSelected(d)}>{label}</Button>}]}/>
  <Modal open={!!selected} title={label} onCancel={()=>{if(!saving){setSelected(undefined);setCommand(undefined);setError(undefined);setReload(n=>n+1)}}} closable={!saving} maskClosable={!saving} keyboard={!saving} footer={<Space><Button disabled={saving} onClick={()=>{setSelected(undefined);setCommand(undefined);setError(undefined);setReload(n=>n+1)}}>{copy.cancel}</Button><Button danger type="primary" disabled={saving||!!command&&error!=='uncertain'} loading={saving} onClick={()=>void send()}>{command?copy.retrySave:copy.save}</Button></Space>}>
   <Typography.Paragraph>{zh?'这会取消整个包裹并恢复库存；所选设备恢复可售，其他设备保留订单预占。':'This cancels the entire parcel and restores stock. The selected device becomes available; other devices remain reserved for the order.'}</Typography.Paragraph>
   <Typography.Text>{selected?.serial_number} · {id}</Typography.Text>
   {error&&<Alert showIcon type={error==='uncertain'?'warning':'error'} message={error==='uncertain'?copy.saveUncertain:error==='conflict'?copy.parcelConflict:error==='invalid'?copy.permissionInvalid:copy.accessDenied}/>}
  </Modal>
 </Space>
}
