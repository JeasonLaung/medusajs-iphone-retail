import {useEffect,useRef,useState} from 'react'
import {Alert,Button,Drawer,Select,Space,Spin,Tag} from 'antd'
import {APIError,warehouseDelivery,linkWarehouseDelivery} from './api'
import type {Warehouse,WarehouseDelivery as Directory,WarehouseDeliveryCommand} from './api'
import type {Copy} from './i18n'
export default function WarehouseDelivery({location,copy,onClose,onExpired}:{location:Warehouse;copy:Copy;onClose():void;onExpired():void}){
 const zh=copy.language==='语言',busy=useRef(false),[data,setData]=useState<Directory>(),[selected,setSelected]=useState<string>(),[error,setError]=useState<unknown>(),[command,setCommand]=useState<WarehouseDeliveryCommand>(),[saving,setSaving]=useState(false),[reload,setReload]=useState(0)
 useEffect(()=>{const abort=new AbortController();setData(undefined);setError(undefined);warehouseDelivery(location.id,abort.signal).then(d=>{if(!abort.signal.aborted)setData(d)}).catch(e=>{if(!abort.signal.aborted){if(e instanceof APIError&&e.status===401)onExpired();else setError(e)}});return()=>abort.abort()},[location.id,reload,onExpired])
 const send=async()=>{if(busy.current||!data||!selected)return;busy.current=true;setSaving(true);const body=command??{operation_id:crypto.randomUUID(),expected_revision:data.revision,location_id:location.id,set_id:selected};setCommand(body);setError(undefined)
 try{await linkWarehouseDelivery(body);setCommand(undefined);setSelected(undefined);setReload(n=>n+1)}catch(e){if(e instanceof APIError&&e.status===401)onExpired();else setError(e)}finally{busy.current=false;setSaving(false)}}
 const terminal=error instanceof APIError&&[400,403,404,409].includes(error.status)
 return <Drawer open title={zh?'仓库配送方案':'Warehouse delivery sets'} width={560} onClose={()=>{if(!busy.current)onClose()}} closable={!saving} maskClosable={!saving} keyboard={!saving}>
 <Space direction="vertical" size={16} className="full-width"><strong>{location.name}</strong><Alert type="info" showIcon message={zh?'关联已有配送方案，使该仓库可按方案装包发货。配送服务与订单地址仍需匹配。':'Link an existing delivery set for packing from this warehouse. Provider and order address must still match.'}/>
 {error!==undefined&&<Alert type="error" showIcon message={terminal?(zh?'配置已变化或无权操作，请重新加载。':'Configuration changed or access denied. Reload to continue.'):(zh?'请求未确认，请重试同一操作。':'Request unconfirmed. Retry the same operation.')}/>}
 {!data&&!error?<Spin/>:data&&<><Space wrap>{data.sets.filter(s=>s.linked).map(s=><Tag key={s.id}>{s.name}</Tag>)}</Space><label htmlFor="warehouse-delivery-set">{zh?'配送方案':'Delivery set'}</label><Select id="warehouse-delivery-set" className="full-width" value={selected} disabled={saving||!!command} onChange={setSelected} options={data.sets.filter(s=>!s.linked).map(s=>({value:s.id,label:s.name}))}/>{!terminal&&<Button type="primary" disabled={!selected} loading={saving} onClick={()=>void send()}>{command?(zh?'重试本次保存':'Retry this save'):(zh?'关联配送方案':'Link delivery set')}</Button>}</>}
 <Button disabled={saving} onClick={()=>{setCommand(undefined);setSelected(undefined);setReload(n=>n+1)}}>{copy.reload}</Button></Space></Drawer>
}
