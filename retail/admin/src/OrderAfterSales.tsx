import {useEffect,useRef,useState} from 'react'
import {Alert,Button,Empty,Popconfirm,Select,Space,Spin,Table,Tag,Typography} from 'antd'
import {APIError,readAfterSales,reviewAfterSales,type AfterSalesReview,type AfterSalesReviewInput} from './api'
import DeviceReturn from './DeviceReturn'
import {useAccess} from './Permissions'

export default function OrderAfterSales({id,onExpired}:{id:string;onExpired():void}){
 const access=useAccess(),zh=document.documentElement.lang.startsWith('zh'),[data,setData]=useState<AfterSalesReview>(),[error,setError]=useState(false),[reload,setReload]=useState(0),[pending,setPending]=useState<AfterSalesReviewInput>(),[saving,setSaving]=useState(false),[failed,setFailed]=useState(false),busy=useRef(false)
 const [receiving,setReceiving]=useState<string>()
 const [targets,setTargets]=useState<Record<string,string>>({})
 const statuses:Record<string,string>=zh?{pending:'待审核',processing:'处理中',completed:'已完成',rejected:'已拒绝'}:{pending:'Pending',processing:'Processing',completed:'Completed',rejected:'Rejected'}
 const types:Record<string,string>=zh?{return:'退货退款',exchange:'换货',repair:'维修'}:{return:'Return & refund',exchange:'Exchange',repair:'Repair'}
 useEffect(()=>{const c=new AbortController();setError(false);readAfterSales(id,c.signal).then(r=>{if(!c.signal.aborted)setData(r)}).catch(e=>{if(!c.signal.aborted){if(e instanceof APIError&&e.status===401)onExpired();else setError(true)}});return()=>c.abort()},[id,reload,onExpired])
 const save=async(requestID:string)=>{
  if(busy.current||!data)return;const body=pending??{operation_id:crypto.randomUUID(),expected_revision:data.revision,request_id:requestID,status:targets[requestID]??'processing'};setPending(body);setSaving(true);setFailed(false);busy.current=true
  try{const r=await reviewAfterSales(id,body);setData(r);setPending(undefined);setReload(n=>n+1)}catch(e){if(e instanceof APIError&&e.status===401)onExpired();else setFailed(true)}finally{busy.current=false;setSaving(false)}
 }
 return <section className="order-after-sales" data-testid="order-after-sales"><Space direction="vertical" size={16} className="full-width"><Typography.Title level={4}>{zh?'售后申请':'After-sales requests'}</Typography.Title><Typography.Paragraph type="secondary">{zh?'完成前须核对设备收回、检测/维修、换机发货和退货退款记录；此操作仅更新审核状态。':'Verify device receipt, inspection/repair, replacement dispatch and return refund records before completing. This action updates the review status.'}</Typography.Paragraph>{error?<><Alert type="error" message={zh?'售后记录暂不可用':'After-sales records unavailable'}/><Button onClick={()=>setReload(n=>n+1)}>{zh?'重试':'Retry'}</Button></>:!data?<Spin/>:<>
 {!data.enabled&&<Alert type="warning" message={zh?'售后审核写入尚未启用':'After-sales review writes are disabled'}/>}
 {failed&&<Alert type="error" message={zh?'结果尚未确认，或记录/权限已变化。请重试原操作，或重新读取后核对处理条件。':'Result unconfirmed or records/permissions changed. Retry the same operation, or reload and verify the prerequisites.'} action={<Button disabled={saving} onClick={()=>{setPending(undefined);setFailed(false);setReload(n=>n+1)}}>{zh?'重新读取':'Reload'}</Button>}/>}
 <Table rowKey="id" dataSource={data.requests} pagination={false} scroll={{x:700}} locale={{emptyText:<Empty/>}} columns={[
 {title:zh?'类型':'Type',dataIndex:'type',render:(v:string)=>types[v]},
 {title:zh?'状态':'Status',dataIndex:'status',render:(v:string)=><Tag>{statuses[v]}</Tag>},
 {title:zh?'原因及商品':'Reason and items',render:(_,r)=><div className="break-words"><p style={{whiteSpace:'pre-wrap'}}>{r.reason}</p>{r.items.map(l=><p key={l.item_id}>{l.title} × {l.quantity}</p>)}</div>},
 {title:zh?'设备收回':'Device receipt',render:(_,r)=>access.can('device.intake','warehouse.read','order.after_sales.write')&&r.status==='processing'?<Button onClick={()=>setReceiving(r.id)}>{zh?'收回设备':'Receive device'}</Button>:null},
 {title:zh?'审核':'Review',render:(_,r)=>access.can('order.after_sales.write')&&['pending','processing'].includes(r.status)?<Space wrap><Select aria-label={zh?'售后目标状态':'After-sales target status'} value={targets[r.id]??'processing'} disabled={saving||!!pending||!data.enabled} onChange={v=>setTargets(s=>({...s,[r.id]:v}))} options={['processing','completed','rejected'].map(value=>({value,label:statuses[value]}))}/><Popconfirm okText={zh?'确认更新':'Confirm update'} cancelText={zh?'取消更新':'Cancel update'} title={zh?'确认更新售后状态？':'Update after-sales status?'} onConfirm={()=>save(r.id)} disabled={saving||!data.enabled||!!pending}><Button onClick={()=>{if(pending)void save(r.id)}} aria-label={pending?zh?'重试原操作':'Retry same operation':zh?'保存':'Save'} loading={saving} disabled={!data.enabled||!!pending&&pending.request_id!==r.id}>{pending?zh?'重试原操作':'Retry same operation':zh?'保存':'Save'}</Button></Popconfirm></Space>:null}
 ]}/></>}</Space>{receiving&&<DeviceReturn order={id} requestID={receiving} onExpired={onExpired} onClose={()=>{setReceiving(undefined);setReload(n=>n+1)}}/>}</section>
}
