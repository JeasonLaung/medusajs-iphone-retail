import {useEffect,useRef,useState} from 'react'
import {Alert,Button,Drawer,Space,Spin,Table,Typography} from 'antd'
import {APIError,readOrderPacking,saveOrderPacking} from './api'
import type {AllocationInput,PackingContext} from './api'
import type {Copy} from './i18n'

export default function OrderPacking({orderID,copy,onClose,onExpired}:{orderID:string;copy:Copy;onClose():void;onExpired():void}){
 const active=useRef(false)
 const [context,setContext]=useState<PackingContext>(),[loadingError,setLoadingError]=useState(false),[reload,setReload]=useState(0),[saving,setSaving]=useState(false),[attempt,setAttempt]=useState<AllocationInput>(),[error,setError]=useState<'uncertain'|'conflict'|'denied'|'invalid'>(),[selected,setSelected]=useState<string[]>([]),[created,setCreated]=useState<string>()
 useEffect(()=>{const abort=new AbortController();setContext(undefined);setLoadingError(false);readOrderPacking(orderID,abort.signal).then(d=>{if(!abort.signal.aborted)setContext(d)}).catch(e=>{if(!abort.signal.aborted){if(e instanceof APIError&&e.status===401)onExpired();else setLoadingError(true)}});return()=>abort.abort()},[orderID,reload,onExpired])
 const save=async()=>{
  if(active.current||!context?.enabled)return
  const body=attempt??{operation_id:crypto.randomUUID(),expected_revision:context.revision,action:'reserve' as const,ids:selected};if(!body.ids.length)return
  active.current=true;setSaving(true);setAttempt(body);setError(undefined)
  try{const id=await saveOrderPacking(orderID,body);setCreated(id);setAttempt(undefined);setSelected([]);setReload(n=>n+1)}catch(e){if(e instanceof APIError&&e.status===401)onExpired();else if(e instanceof APIError&&e.status===409)setError('conflict');else if(e instanceof APIError&&(e.status===403||e.status===404))setError('denied');else if(e instanceof APIError&&e.status===400){setAttempt(undefined);setError('invalid')}else setError('uncertain')}finally{active.current=false;setSaving(false)}
 }
 const chosen=context?.devices.filter(d=>selected.includes(d.assignment_id))??[]
 const oneSource=new Set(chosen.map(d=>d.source_id)).size<=1&&new Set(chosen.map(d=>d.location_id)).size<=1
 const locked=saving||!!attempt||!context?.enabled||!context.paid||!context.order_active||context.options.length!==1||!context.options[0]?.supported
 return <Drawer open title={copy.orderPacking} width="min(1000px, 100vw)" onClose={()=>{if(!active.current)onClose()}} closable={!saving} maskClosable={!saving} keyboard={!saving}>
  <Space direction="vertical" size={24} className="full-width">
   <Alert type="info" showIcon message={copy.packingInfo}/>
   {created&&<Alert type="success" showIcon message={copy.packingCreated} description={created}/>}
   {loadingError?<Space direction="vertical"><Alert type="error" showIcon message={copy.unavailable}/><Button onClick={()=>setReload(n=>n+1)}>{copy.retry}</Button></Space>:!context?<Spin/>:<>
    {!context.enabled&&<Alert type="warning" showIcon message={copy.allocationDisabled}/>}
    {(!context.paid||!context.order_active)&&<Alert type="warning" showIcon message={!context.order_active?copy.allocationOrderInactive:copy.allocationUnpaid}/>}
    {context.options.length!==1||!context.options[0].supported?<Alert type="warning" showIcon message={copy.packingShippingUnavailable}/>:<Typography.Text>{copy.packingShippingMethod}: {context.options[0].name}</Typography.Text>}
    {!oneSource&&<Alert type="warning" showIcon message={copy.packingSameSource}/>}
    {error&&<Alert type={error==='uncertain'?'warning':'error'} showIcon message={error==='uncertain'?copy.saveUncertain:error==='conflict'?copy.packingConflict:error==='invalid'?copy.allocationInvalid:copy.accessDenied}/>}
    {attempt&&error==='uncertain'&&<Button loading={saving} onClick={()=>void save()}>{copy.retrySave}</Button>}
    <Table rowKey="assignment_id" dataSource={context.devices} scroll={{x:850}} pagination={{pageSize:20}} rowSelection={{selectedRowKeys:selected,onChange:keys=>setSelected(keys.map(String)),getCheckboxProps:()=>({disabled:locked})}} columns={[
     {title:copy.productTitle,dataIndex:'title'},{title:'SN',dataIndex:'serial_number'},{title:copy.stockLocation,dataIndex:'location_name'},{title:copy.deviceSources,dataIndex:'source_name'},
    ]}/>
    <Button aria-label={copy.packDevices} type="primary" loading={saving} disabled={locked||!oneSource||!selected.length||selected.length>200} onClick={()=>void save()}>{copy.packDevices}</Button>
   </>}
   <Button disabled={saving} onClick={onClose}>{copy.cancel}</Button>
  </Space>
 </Drawer>
}
