import {useEffect,useMemo,useRef,useState} from 'react'
import {Alert,Spin,Tag} from 'antd'
import {Button} from '../account/ui'
import {customerClient,CustomerError} from '../../lib/customer-client'
import {paymentCommand,type PaymentContext,type PaymentInput} from '../../lib/order-payments'
import {formatCartMoney} from '../cart/types'

export default function SupplementalPayment({orderID,segment,onPaid,required=false}:{orderID:string;segment:string;onPaid?:()=>void;required?:boolean}){
 const zh=segment.endsWith('-zh'),api=useMemo(()=>customerClient(),[])
 const copy=zh?{title:'订单补款',due:'当前待补金额',pay:'确认补款',continue:'继续安全付款',cancel:'取消未发送的补款',refresh:'刷新付款状态',retry:'重试同一请求',waiting:'正在准备或核对付款，请稍后刷新。',unknown:'暂时无法确认请求结果。请重试同一请求或刷新状态。',error:'暂时无法加载补款信息。',paid:'补款已收妥',review:'当前差额需要客服核对，请联系商店。',login:'请重新登录后查看本人订单。',pending:'准备中',uncertain:'核对中',open:'待付款',authorized:'授权处理中',failed:'付款失败',canceled:'已取消',expired:'已过期'}:{title:'Additional payment',due:'Current amount due',pay:'Confirm additional payment',continue:'Continue to secure payment',cancel:'Cancel unsent payment',refresh:'Refresh payment status',retry:'Retry the same request',waiting:'Preparing or checking your payment. Please check again shortly.',unknown:'The request result could not be confirmed. Retry the same request or refresh its status.',error:'Additional payment details are unavailable.',paid:'Additional payment received',review:'This balance needs review. Please contact the store.',login:'Sign in again to view your order.',pending:'Preparing',uncertain:'Checking',open:'Awaiting payment',authorized:'Authorization pending',failed:'Payment failed',canceled:'Canceled',expired:'Expired'}
 const [data,setData]=useState<PaymentContext|null>(null),[error,setError]=useState(0),[busy,setBusy]=useState(false),[uncertain,setUncertain]=useState(false)
 const mounted=useRef(true),sequence=useRef(0),pending=useRef<PaymentInput|null>(null),cancelID=useRef<{payment:string;id:string}|null>(null),paid=useRef(new Set<string>()),initialized=useRef(false),onPaidRef=useRef(onPaid)
 onPaidRef.current=onPaid
 async function read(){
  const n=++sequence.current
  try{const next=await api.orderPayments(orderID);if(!mounted.current||n!==sequence.current)return
   setData(next);setError(0)
   if(pending.current&&next.payments.some(p=>p.operation_id===pending.current!.operation_id)){pending.current=null;setUncertain(false)}
   if(cancelID.current&&next.payments.some(p=>p.operation_id===cancelID.current!.payment&&!p.can_cancel)){cancelID.current=null;setUncertain(false)}
   for(const p of next.payments)if(p.applied&&!paid.current.has(p.operation_id)){paid.current.add(p.operation_id);if(initialized.current)onPaidRef.current?.()}
   initialized.current=true
   return next
  }catch(e){if(mounted.current&&n===sequence.current)setError(e instanceof CustomerError?e.status:503)}
 }
 useEffect(()=>{mounted.current=true;void read();return()=>{mounted.current=false;sequence.current++;api.clear()}},[api,orderID])
 const active=data?.payments.some(p=>!p.applied&&!['failed','canceled','expired'].includes(p.state))??false
 useEffect(()=>{if(!active)return;let times=0;const timer=setInterval(()=>{if(!document.hidden&&times++<60)void read()},3000);return()=>clearInterval(timer)},[active,orderID])
 async function prepare(){
  if(busy)return
  if(!pending.current){if(!data?.next||error)return;pending.current=paymentCommand(data.next,crypto.randomUUID())}
  setBusy(true);setUncertain(false)
  try{await api.prepareOrderPayment(orderID,pending.current);if(mounted.current){pending.current=null;await read()}}
  catch(e){if(mounted.current){if(e instanceof CustomerError&&e.status===409){const fresh=await read();if(fresh){pending.current=null;setUncertain(false)}else setUncertain(true)}else setUncertain(true)}}finally{if(mounted.current)setBusy(false)}
 }
 async function cancel(operation:string){
  if(busy)return
  if(cancelID.current?.payment!==operation)cancelID.current={payment:operation,id:crypto.randomUUID()}
  setBusy(true)
  try{await api.cancelOrderPayment(orderID,operation,cancelID.current.id);if(mounted.current){cancelID.current=null;setUncertain(false);await read()}}
  catch{if(mounted.current)setUncertain(true)}finally{if(mounted.current)setBusy(false)}
 }
 // A normally paid order with no supplemental history retains its layout.
 if(!required&&!data&&(error===0||error===503||error===404))return null
 if(data&&!data.payments.length&&!data.next&&data.pending_difference==='0'&&!error)return null
 const money=(amount:string)=>formatCartMoney(amount,data?.currency_code??'eur',segment)
 return <section id="order-supplemental-payment" className="min-w-0 space-y-4" data-testid="order-supplemental-payment" aria-label={copy.title}>
  <h2 className="text-lg font-semibold">{copy.title}</h2>
  {error?<Alert type={error===401?'warning':'error'} message={error===401?copy.login:copy.error} action={<Button variant="secondary" onClick={()=>void read()}>{copy.refresh}</Button>}/>:!data?<Spin/>:<>
   <p data-testid="supplemental-due">{copy.due}: <strong>{money(data.pending_difference)}</strong></p>
   {data.payments.map(p=><div key={p.operation_id} className="space-y-2" data-testid="supplemental-payment-receipt" data-operation={p.operation_id} data-state={p.state} data-applied={p.applied}>
    <div className="flex flex-wrap items-center justify-between gap-2"><span>{money(p.amount)}</span><Tag color={p.applied?'success':['failed','canceled','expired'].includes(p.state)?'default':'processing'}>{p.applied?copy.paid:p.state==='paid'?copy.uncertain:copy[p.state]}</Tag></div>
    <div className="flex flex-wrap gap-2">{p.checkout_url&&<a className="underline underline-offset-2 focus-visible:outline" href={p.checkout_url} rel="noreferrer" data-testid="supplemental-checkout-link">{copy.continue}</a>}{p.can_cancel&&<Button variant="secondary" disabled={busy} onClick={()=>void cancel(p.operation_id)}>{copy.cancel}</Button>}</div>
   </div>)}
   {data.next&&!uncertain&&<Button disabled={busy} isLoading={busy} onClick={()=>void prepare()} data-testid="supplemental-prepare">{copy.pay}: {money(data.next.amount)}</Button>}
   {active&&<p className="text-sm text-ui-fg-subtle" role="status">{copy.waiting}</p>}
   {!active&&!data.next&&data.pending_difference!=='0'&&<p>{copy.review}</p>}
   <Button variant="secondary" disabled={busy} onClick={()=>void read()}>{copy.refresh}</Button>
  </>}
  {uncertain&&<Alert type="warning" message={copy.unknown} action={pending.current?<Button disabled={busy} onClick={()=>void prepare()}>{copy.retry}</Button>:cancelID.current?<Button disabled={busy} onClick={()=>void cancel(cancelID.current!.payment)}>{copy.retry}</Button>:undefined}/>}
 </section>
}
