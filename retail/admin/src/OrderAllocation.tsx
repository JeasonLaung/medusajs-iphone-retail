import {useEffect,useRef,useState} from 'react'
import {Alert,Button,Descriptions,Drawer,Input,Space,Spin,Table,Typography} from 'antd'
import {APIError,readOrderAllocation,saveOrderAllocation} from './api'
import type {AllocationContext,AllocationInput} from './api'
import type {Copy} from './i18n'
export default function OrderAllocation({orderID,itemID,title,copy,onClose,onExpired}:{orderID:string;itemID:string;title:string;copy:Copy;onClose():void;onExpired():void}){
 const active=useRef(false)
 const [context,setContext]=useState<AllocationContext>(),[loadError,setLoadError]=useState(false),[reload,setReload]=useState(0),[saving,setSaving]=useState(false),[attempt,setAttempt]=useState<AllocationInput>(),[error,setError]=useState<'uncertain'|'conflict'|'denied'|'invalid'>(),[selected,setSelected]=useState<string[]>([]),[releasing,setReleasing]=useState<string[]>([]),[search,setSearch]=useState('')
 useEffect(()=>{const abort=new AbortController();setContext(undefined);setLoadError(false);readOrderAllocation(orderID,itemID,abort.signal).then(d=>{if(!abort.signal.aborted)setContext(d)}).catch(e=>{if(!abort.signal.aborted){if(e instanceof APIError&&e.status===401)onExpired();else setLoadError(true)}});return()=>abort.abort()},[orderID,itemID,reload,onExpired])
 const save=async(action:'reserve'|'release')=>{
  if(active.current||!context?.enabled)return
  const body=attempt??{operation_id:crypto.randomUUID(),expected_revision:context.revision,action,ids:action==='reserve'?selected:releasing};if(!body.ids.length)return
  active.current=true;setSaving(true);setAttempt(body);setError(undefined)
  try{await saveOrderAllocation(orderID,itemID,body);setAttempt(undefined);setSelected([]);setReleasing([]);setReload(n=>n+1)}
  catch(e){if(e instanceof APIError&&e.status===401)onExpired();else if(e instanceof APIError&&e.status===409)setError('conflict');else if(e instanceof APIError&&(e.status===403||e.status===404))setError('denied');else if(e instanceof APIError&&e.status===400){setAttempt(undefined);setError('invalid')}else setError('uncertain')}
  finally{active.current=false;setSaving(false)}
 }
 const locked=saving||!!attempt||!context?.enabled||!context.order_active
 const devices=context?.devices.filter(d=>[d.serial_number,d.imei,d.imei2,d.eid,d.location_name,d.source_name].some(s=>s?.toLowerCase().includes(search.toLowerCase())))
 return <Drawer open title={copy.orderAllocation} width="min(1100px, 100vw)" onClose={()=>{if(!active.current)onClose()}} closable={!saving} maskClosable={!saving} keyboard={!saving}>
  <Space direction="vertical" size={24} className="full-width">
   <Typography.Text strong>{title}</Typography.Text><Alert type="info" showIcon message={copy.allocationInfo}/>
   {loadError?<Space direction="vertical"><Alert type="error" showIcon message={copy.unavailable}/><Button onClick={()=>setReload(n=>n+1)}>{copy.retry}</Button></Space>:!context?<Spin/>:<>
    {!context.enabled&&<Alert type="warning" showIcon message={copy.allocationDisabled}/>}
    {(!context.paid||!context.order_active)&&<Alert type="warning" showIcon message={!context.order_active?copy.allocationOrderInactive:copy.allocationUnpaid}/>}
    <Descriptions column={{xs:1,sm:3}} items={[{key:'quantity',label:copy.orderedQuantity,children:context.quantity},{key:'fulfilled',label:copy.fulfilledQuantity,children:context.fulfilled},{key:'remaining',label:copy.allocationRemaining,children:context.remaining}]}/>
    {error&&<Alert type={error==='uncertain'?'warning':'error'} showIcon message={error==='uncertain'?copy.saveUncertain:error==='conflict'?copy.allocationConflict:error==='invalid'?copy.allocationInvalid:copy.accessDenied}/>}
    {attempt&&error==='uncertain'&&<Button loading={saving} onClick={()=>void save(attempt.action)}>{copy.retrySave}</Button>}
    <Typography.Title level={4}>{copy.allocationAvailable}</Typography.Title>
    <Input.Search aria-label={copy.allocationSearch} placeholder={copy.allocationSearch} value={search} onChange={e=>setSearch(e.target.value)} allowClear/>
    <Table rowKey="id" dataSource={devices} scroll={{x:850}} pagination={{pageSize:20,showSizeChanger:true}} rowSelection={{selectedRowKeys:selected,onChange:keys=>setSelected(keys.map(String)),getCheckboxProps:()=>({disabled:locked||!context.paid})}} columns={[
     {title:'SN',dataIndex:'serial_number'},{title:'IMEI',dataIndex:'imei',render:v=>v??'—'},{title:copy.stockLocation,dataIndex:'location_name'},{title:copy.deviceSources,dataIndex:'source_name'},
    ]}/>
    <Button aria-label={copy.reserveDevices} type="primary" loading={saving} disabled={locked||!context.paid||!selected.length||selected.length>Math.min(200,context.remaining)} onClick={()=>void save('reserve')}>{copy.reserveDevices}</Button>
    <Typography.Title level={4}>{copy.allocatedDevices}</Typography.Title>
    <Table rowKey="id" dataSource={context.assignments} scroll={{x:900}} pagination={{pageSize:20}} rowSelection={{selectedRowKeys:releasing,onChange:keys=>setReleasing(keys.map(String)),getCheckboxProps:a=>({disabled:locked||a.status!=='reserved'||a.fulfillment_id!==null})}} columns={[
     {title:copy.allocationDeviceID,dataIndex:'device_id'},{title:copy.status,dataIndex:'status',render:(s:string)=>(copy.allocationStates[s as keyof typeof copy.allocationStates]??s)},{title:copy.allocationFulfillment,dataIndex:'fulfillment_id',render:v=>v??'—'},
    ]}/>
    <Button aria-label={copy.releaseDevices} disabled={locked||!releasing.length||releasing.length>200} loading={saving} onClick={()=>void save('release')}>{copy.releaseDevices}</Button>
   </>}
   <Button aria-label={copy.cancel} disabled={saving} onClick={onClose}>{copy.cancel}</Button>
  </Space>
 </Drawer>
}
