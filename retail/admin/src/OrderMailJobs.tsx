import {useEffect,useRef,useState} from 'react'
import {Alert,App,Button,Modal,Space,Spin,Table,Tag,Typography} from 'antd'
import {APIError,orderMailJobs,retryOrderMail,type OrderMailJob,type OrderMailJobs as Jobs,type OrderMailRetry} from './api'
import {useAccess} from './Permissions'
import type {Copy} from './i18n'
export default function OrderMailJobs({copy,onExpired}:{copy:Copy;onExpired():void}){
 const zh=copy.language==='语言',access=useAccess(),{message}=App.useApp(),busy=useRef(false)
 const [page,setPage]=useState(1),[revision,setRevision]=useState(0),[state,setState]=useState<{data?:Jobs;error?:unknown}>({}),[sending,setSending]=useState(false),[pending,setPending]=useState<{id:string;body:OrderMailRetry}>(),[error,setError]=useState<'uncertain'|'conflict'|'forbidden'>()
 const [confirmation,setConfirmation]=useState<OrderMailJob>()
 const words=zh?{title:'订单邮件任务',info:'发送成功表示邮件服务已接收，不代表收件箱最终投递。人工重试保留原正文与编号，超过恢复时限的任务不能重发。',order:'订单',attempts:'尝试次数',reason:'失败原因',created:'创建时间',sent:'发送时间',retry:'重新排队',confirm:'确认重新排队这封订单邮件？',uncertain:'重试结果尚未确认，请使用同一操作重试。',conflict:'任务状态或恢复时限已变化，请刷新查看。',queued:'已重新排队',pending:'待发送',sending:'发送中',success:'服务已接收',failed:'失败'}:{title:'Order email jobs',info:'Sent means the email provider accepted the message, not confirmed inbox delivery. Retry preserves the original message and key; expired recovery windows cannot be retried.',order:'Order',attempts:'Attempts',reason:'Failure reason',created:'Created',sent:'Sent',retry:'Queue again',confirm:'Queue this order email again?',uncertain:'The retry result is not confirmed. Retry this same operation.',conflict:'The task or recovery window changed. Reload its current state.',queued:'Queued again',pending:'Pending',sending:'Sending',success:'Provider accepted',failed:'Failed'}
 const reload=()=>setRevision(n=>n+1)
 useEffect(()=>{const controller=new AbortController();setState({});orderMailJobs((page-1)*50,controller.signal).then(data=>{if(!controller.signal.aborted)setState({data})}).catch(error=>{if(!controller.signal.aborted){setState({error});if(error instanceof APIError&&error.status===401)onExpired()}});return()=>controller.abort()},[page,revision,onExpired])
 const retry=async(job:OrderMailJob)=>{
  if(busy.current)return;const intent=pending??{id:job.id,body:{operation_id:crypto.randomUUID(),expected_attempts:job.attempts}};if(intent.id!==job.id)return
  busy.current=true;setSending(true);setPending(intent);setError(undefined)
  try{await retryOrderMail(intent.id,intent.body);setPending(undefined);message.success(words.queued);reload()}
  catch(e){if(e instanceof APIError&&e.status===401)onExpired();else if(e instanceof APIError&&e.status===403)setError('forbidden');else if(e instanceof APIError&&[400,409].includes(e.status))setError('conflict');else setError('uncertain')}
  finally{busy.current=false;setSending(false)}
 }
 const status=(j:OrderMailJob)=>({pending:words.pending,sending:words.sending,sent:words.success,failed:words.failed}[j.status])
 const date=(v:string|null)=>v?new Date(v).toLocaleString(zh?'zh-CN':'en-GB'):'—'
 return <section data-testid="order-mail-jobs"><Space direction="vertical" size={16} className="full-width">
  <Typography.Title level={3}>{words.title}</Typography.Title><Typography.Paragraph type="secondary">{words.info}</Typography.Paragraph>
  <Button onClick={()=>{if(error!=='uncertain'){setPending(undefined);setError(undefined)};reload()}} disabled={sending}>{copy.reload}</Button>
  {error&&<Alert showIcon type={error==='uncertain'?'warning':'error'} message={error==='uncertain'?words.uncertain:error==='conflict'?words.conflict:copy.accessDenied}/>}
  {state.error?<Alert showIcon type="error" message={state.error instanceof APIError&&state.error.status===403?copy.accessDenied:copy.unavailable}/>:!state.data?<Spin/>:<Table<OrderMailJob> rowKey="id" dataSource={state.data.jobs} scroll={{x:950}} pagination={{current:page,pageSize:50,total:state.data.count,showSizeChanger:false,onChange:setPage}} columns={[
   {title:words.order,dataIndex:'order_id'},
   {title:copy.status,render:(_,j)=><Tag color={j.status==='failed'?'error':j.status==='sent'?'success':'processing'}>{status(j)}</Tag>},
   {title:words.attempts,dataIndex:'attempts'},
   {title:words.reason,dataIndex:'error_code',render:(v:string|null)=>v??'—'},
   {title:words.created,dataIndex:'created_at',render:date},
   {title:words.sent,dataIndex:'sent_at',render:date},
   ...(access.can('mail.write')?[{title:words.retry,render:(_:unknown,j:OrderMailJob)=>pending?.id===j.id?<Button aria-label={words.retry} loading={sending} disabled={sending||error!=='uncertain'} onClick={()=>void retry(j)}>{words.retry}</Button>:<Button aria-label={words.retry} disabled={!j.retryable||!!pending||!!confirmation} onClick={()=>setConfirmation(j)}>{words.retry}</Button>}]:[])
  ]}/>}
  <Modal open={!!confirmation} title={words.confirm} okText={zh?'确定':'OK'} cancelText={copy.cancel} onCancel={()=>setConfirmation(undefined)} onOk={()=>{const job=confirmation;setConfirmation(undefined);if(job)void retry(job)}}><Typography.Paragraph>{confirmation?.order_id}</Typography.Paragraph></Modal>
 </Space></section>
}
