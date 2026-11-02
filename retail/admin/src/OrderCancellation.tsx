import {useEffect,useRef,useState} from 'react'
import {Alert,Button,Descriptions,Drawer,Form,Input,Popconfirm,Space,Spin,Table,Tag} from 'antd'
import {APIError,csrfCookie,request} from './api'
import type {RefundOperation} from './refundApi'

type View={order_id:string;operation_id?:string;version:number;enabled:boolean;eligible:boolean;state:'not_started'|'processing'|'requires_action'|'canceled';reason?:string;completed_at?:string;refunds:RefundOperation[]}
type Command={path:string;body:Record<string,string>}

export default function OrderCancellation({id,onExpired,onChanged}:{id:string;onExpired():void;onChanged():void}){
 const zh=document.documentElement.lang.startsWith('zh')
 const [open,setOpen]=useState(false),[data,setData]=useState<View>(),[error,setError]=useState(''),[loading,setLoading]=useState(false),[saving,setSaving]=useState(false),[reload,setReload]=useState(0)
 const [pending,setPending]=useState<Command>(),[form]=Form.useForm<{reason:string}>()
 const busy=useRef(false)
 const path='orders/'+encodeURIComponent(id)+'/cancellation'
 const refundLabels:Record<string,string>=zh?{pending:'待处理',uncertain:'结果待确认',queued:'排队中',processing:'处理中',refunded:'已退款',failed:'失败',canceled:'已撤销'}:{pending:'Pending',uncertain:'Uncertain',queued:'Queued',processing:'Processing',refunded:'Refunded',failed:'Failed',canceled:'Canceled'}
 const labels:Record<string,string>=zh?{not_started:'尚未取消',processing:'取消处理中',requires_action:'退款需要处理',canceled:'订单已取消'}:{not_started:'Not started',processing:'Cancellation in progress',requires_action:'Refund needs attention',canceled:'Order canceled'}
 useEffect(()=>{
  if(!open)return
  const controller=new AbortController()
  setLoading(true)
  request<View>(path,{signal:controller.signal},'/api/admin/').then(v=>{
   if(controller.signal.aborted)return
   if(v.order_id!==id||!Array.isArray(v.refunds)||!Object.hasOwn(labels,v.state))throw new APIError(0,'request.invalid_response')
   setData(v);setError('')
  }).catch(e=>{if(controller.signal.aborted)return;if(e instanceof APIError&&e.status===401)onExpired();else setError(zh?'无法读取取消状态，请重试。':'Could not read cancellation status. Please retry.')}).finally(()=>{if(!controller.signal.aborted)setLoading(false)})
  return()=>controller.abort()
 },[open,id,reload,onExpired])
 useEffect(()=>{if(!open||data?.state!=='processing'||pending)return;const timer=setTimeout(()=>setReload(v=>v+1),3000);return()=>clearTimeout(timer)},[open,data,reload,pending])
 async function send(command:Command){
  if(busy.current)return
  busy.current=true;setSaving(true);setPending(command);setError('')
  try{
   const csrf=csrfCookie();if(!csrf)throw new APIError(403,'auth.invalid_csrf')
   const v=await request<View>(command.path,{method:'POST',body:command.body,csrf},'/api/admin/')
   if(v.order_id!==id||!Array.isArray(v.refunds)||!Object.hasOwn(labels,v.state))throw new APIError(0,'request.invalid_response')
   setData(v);setPending(undefined);setReload(v=>v+1)
  }catch(e){
   if(e instanceof APIError&&e.status===401)onExpired()
   if(e instanceof APIError&&[400,403,409].includes(e.status)){setPending(undefined);setReload(v=>v+1);setError(zh?'订单状态或权限已变化，请重新读取并检查包裹、付款及售后。':'Order state or permissions changed. Reload and check parcels, payments and after-sales.')}
   else setError(zh?'提交结果尚未确认。请重试同一操作，勿重复发起退款。':'The result is not confirmed. Retry the same operation to recover it.')
  }finally{busy.current=false;setSaving(false)}
 }
 return <>
 <Button danger onClick={()=>{setOpen(true);setReload(v=>v+1)}}>{zh?'取消整单':'Cancel order'}</Button>
 <Drawer open={open} onClose={()=>{setOpen(false);if(data?.state==='canceled')onChanged()}} width={620} title={zh?'取消整单':'Cancel order'} destroyOnClose={false}>
 <Space direction="vertical" size={16} className="full-width">
 <Alert showIcon type="warning" message={zh?'先取消所有包裹。系统将退回剩余实收金额，退款确认后释放预留库存并取消订单。':'Cancel all parcels first. The remaining captured amount will be refunded. Stock is released and the order canceled after refunds are confirmed.'}/>
 {loading&&!data&&<Spin/>}
 {error&&<Alert showIcon type="error" message={error}/>}
 {pending&&<Button loading={saving} onClick={()=>void send(pending)}>{zh?'重试同一取消操作':'Retry same cancellation operation'}</Button>}
 {data&&<>
 <Descriptions column={1} items={[{key:'state',label:zh?'状态':'Status',children:<Tag color={data.state==='canceled'?'success':data.state==='requires_action'?'error':undefined}>{labels[data.state]}</Tag>},...(data.reason?[{key:'reason',label:zh?'取消原因':'Cancellation reason',children:data.reason}]:[]),...(data.completed_at?[{key:'completed',label:zh?'取消时间':'Canceled at',children:new Date(data.completed_at).toLocaleString(document.documentElement.lang)}]:[])]}/>
 {!data.enabled&&<Alert type="info" message={zh?'整单取消当前未启用。':'Whole-order cancellation is currently disabled.'}/>}
 {data.state==='not_started'&&<>
 {!data.eligible&&data.enabled&&<Alert type="info" message={zh?'请先处理未取消的包裹、进行中的订单变更、补款或售后。已完成的订单请走退货流程。':'Resolve active parcels, order changes, payments or after-sales first. Completed orders require a return.'}/>}
 <Form form={form} layout="vertical" disabled={!data.enabled||!data.eligible||!!pending||saving}>
 <Form.Item name="reason" label={zh?'取消原因':'Cancellation reason'} rules={[{required:true,whitespace:true,max:1000}]}><Input.TextArea rows={4} maxLength={1000} showCount/></Form.Item>
 <Popconfirm title={zh?'确认取消整单并退回剩余实收金额？':'Cancel this order and refund the remaining captured amount?'} okText={zh?'确认取消':'Confirm cancellation'} cancelText={zh?'返回':'Back'} disabled={!data.enabled||!data.eligible||!!pending||saving} onConfirm={async()=>{try{const v=await form.validateFields();await send({path,body:{operation_id:crypto.randomUUID(),expected_version:String(data.version),reason:v.reason.trim()}})}catch{}}}>
 <Button danger type="primary" loading={saving} disabled={!data.enabled||!data.eligible||!!pending}>{zh?'提交整单取消':'Submit order cancellation'}</Button>
 </Popconfirm>
 </Form></>}
 {data.refunds.length>0&&<Table pagination={false} rowKey="operation_id" dataSource={data.refunds} scroll={{x:480}} columns={[
 {title:zh?'退款金额':'Refund amount',key:'amount',render:(_,r)=>r.amount+' '+r.currency_code.toUpperCase()},
 {title:zh?'状态':'Status',key:'state',render:(_,r)=>r.applied?(zh?'已退款':'Refunded'):(refundLabels[r.state]??r.state)},
 {title:zh?'操作':'Action',key:'action',render:(_,r)=>r.can_retry&&data.enabled?<Popconfirm title={zh?'为这笔失败的退款提交新尝试？':'Submit a new attempt for this failed refund?'} onConfirm={()=>send({path:path+'/refunds/'+encodeURIComponent(r.operation_id)+'/retry',body:{operation_id:crypto.randomUUID()}})}><Button disabled={!!pending||saving}>{zh?'重新发起退款':'Retry failed refund'}</Button></Popconfirm>:null},
 ]}/>}
 </>}
 <Button disabled={saving} loading={loading} onClick={()=>setReload(v=>v+1)}>{zh?'重新读取取消状态':'Reload cancellation status'}</Button>
 </Space></Drawer>
 </>
}
