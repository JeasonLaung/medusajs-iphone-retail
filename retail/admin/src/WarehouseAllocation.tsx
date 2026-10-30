import {useEffect,useRef,useState} from 'react'
import {Alert,Button,Input,Space,Spin,Table,Typography} from 'antd'
import {APIError,type AllocationInput} from './api'
import {warehouseAllocation,saveWarehouseAllocation,type WarehouseAllocation as Context} from './warehouse-fulfillments-api'
import type {Copy} from './i18n'

export default function WarehouseAllocation({location,order,id,item,title,copy,onExpired,onSaved,onBusy,blocked=false}:{location:string;order:string;id:string;item:string;title:string;copy:Copy;onExpired():void;onSaved():void;onBusy(busy:boolean):void;blocked?:boolean}){
 const [search,setSearch]=useState(''),[data,setData]=useState<Context>(),[selected,setSelected]=useState<string[]>([]),[reload,setReload]=useState(0),[loadError,setLoadError]=useState(false),[saving,setSaving]=useState(false),[attempt,setAttempt]=useState<AllocationInput>(),[error,setError]=useState<'uncertain'|'conflict'|'denied'|'invalid'>(),active=useRef(false)
 const zh=copy.language==='语言',label=zh?'包裹设备配货':'Parcel device allocation'
 useEffect(()=>{const abort=new AbortController();setData(undefined);setLoadError(false);warehouseAllocation(location,order,id,item,abort.signal).then(d=>{if(!abort.signal.aborted){setData(d);setSelected(d.assigned.map(v=>v.id))}}).catch(e=>{if(!abort.signal.aborted){if(e instanceof APIError&&e.status===401)onExpired();else setLoadError(true)}});return()=>abort.abort()},[location,order,id,item,reload,onExpired])
 const save=async()=>{
  if(active.current||blocked||!data)return;active.current=true;setSaving(true);onBusy(true);setError(undefined)
  const body=attempt??{operation_id:crypto.randomUUID(),expected_revision:data.revision,action:'reserve',ids:[...selected]} satisfies AllocationInput
  setAttempt(body)
  try{await saveWarehouseAllocation(location,order,id,item,body);setAttempt(undefined);setReload(n=>n+1);onSaved()}
  catch(e){if(e instanceof APIError&&e.status===401)onExpired();else if(e instanceof APIError&&[403,404].includes(e.status))setError('denied');else if(e instanceof APIError&&e.status===409)setError('conflict');else if(e instanceof APIError&&e.status===400){setAttempt(undefined);setError('invalid')}else setError('uncertain')}
  finally{active.current=false;setSaving(false);onBusy(false)}
 }
 if(loadError)return <Alert type="error" message={copy.unavailable} action={<Button onClick={()=>setReload(n=>n+1)}>{copy.retry}</Button>}/>
 if(!data)return <Spin/>
 const locked=blocked||saving||!!attempt||!data.enabled||!data.editable
 return <Space direction="vertical" className="full-width" data-testid="warehouse-allocation">
  <Typography.Title level={5}>{label} · {title}</Typography.Title>
  <Typography.Paragraph type="secondary">{zh?'选择此包裹商品的设备；替换设备不重复扣减库存。减少已配数量需要先取消包裹。':'Select devices for this parcel item. Replacement keeps quantity stock unchanged. Reducing allocation requires parcel cancellation.'}</Typography.Paragraph>
  <Input.Search aria-label={label} placeholder={label} allowClear onSearch={setSearch}/>
  <Table rowKey="id" size="small" scroll={{x:360}} pagination={{pageSize:10,showSizeChanger:false}} dataSource={[...data.assigned,...data.candidates].filter(v=>v.serial_number.toLowerCase().includes(search.trim().toLowerCase()))} rowSelection={{selectedRowKeys:selected,preserveSelectedRowKeys:true,onChange:keys=>setSelected(keys.map(String)),getCheckboxProps:()=>({disabled:locked})}} columns={[{title:'SN',dataIndex:'serial_number'}]}/>

  <Typography.Text>{selected.length} / {data.quantity}</Typography.Text>
  {error&&<Alert type={error==='uncertain'?'warning':'error'} message={error==='uncertain'?copy.saveUncertain:error==='conflict'?copy.parcelConflict:error==='invalid'?copy.permissionInvalid:copy.accessDenied}/>}
  {error==='uncertain'&&<Button loading={saving} onClick={()=>void save()}>{copy.retrySave}</Button>}
  <Button type="primary" disabled={locked||selected.length<Math.max(1,data.assigned.length)||selected.length>data.quantity} loading={saving} onClick={()=>void save()}>{copy.save}</Button>
 </Space>
}
