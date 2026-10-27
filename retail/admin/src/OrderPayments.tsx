import {useEffect,useState} from 'react'
import {Alert,Button,Descriptions,Empty,Space,Spin,Table,Tag,Typography} from 'antd'
import {APIError,getOrderPayments} from './api'
import type {OrderPayments as Evidence,PaymentMovement,PaymentRecord} from './api'
import type {Copy} from './i18n'
import {amountText} from './VariantPrices'
export default function OrderPayments({id,version,copy,onExpired}:{id:string;version:number;copy:Copy;onExpired():void}) {
 const [attempt,setAttempt]=useState(0)
 const [state,setState]=useState<{loading:boolean;data?:Evidence;error?:unknown}>({loading:true})
 useEffect(()=>{
  const controller=new AbortController();setState({loading:true})
  getOrderPayments(id,controller.signal).then(data=>{
   if(controller.signal.aborted)return
   if(data.version!==version)throw new APIError(0,'orders.version_changed')
   setState({loading:false,data})
  }).catch(error=>{if(controller.signal.aborted)return;if(error instanceof APIError&&error.status===401)onExpired();else setState({loading:false,error})})
  return()=>controller.abort()
 },[id,version,attempt,onExpired])
 const amount=(v:string|null,currency:string)=><span className="price-rule-value">{v===null?'—':`${amountText(v)} ${currency}`}</span>
 const hold=(v:string)=><Tag color="warning">{v==='held'?copy.paymentHeld:copy.paymentUnverified}</Tag>
 const status=(v:string)=>({not_paid:copy.paymentNotPaid,awaiting:copy.paymentAwaiting,authorized:copy.paymentAuthorized,partially_authorized:copy.paymentPartAuthorized,canceled:copy.canceledOrder,failed:copy.paymentFailed,partially_captured:copy.paymentPartCaptured,completed:copy.completedOrder,captured:copy.paymentCaptured,pending:copy.pendingOrder,requires_more:copy.requiresActionOrder,error:copy.paymentFailed,pending_authorization:copy.paymentPendingAuthorization}[v]??v)
 const movements=(rows:PaymentMovement[],currency:string,label:string)=><Space direction="vertical" className="full-width"><Typography.Text strong>{label}</Typography.Text><Table rowKey="id" size="small" dataSource={rows} pagination={false} scroll={{x:500}} columns={[{title:copy.paymentID,dataIndex:'id',render:(v:string)=><span className="price-rule-value">{v}</span>},{title:copy.amount,render:(_,row)=>amount(row.amount,currency)},{title:copy.createdAt,dataIndex:'created_at',render:(v:string)=><time dateTime={v}>{new Date(v).toLocaleString(document.documentElement.lang)}</time>}]}/></Space>
 return <section aria-label={copy.orderPayments}><Space direction="vertical" size={16} className="full-width">
  <Typography.Title level={4}>{copy.orderPayments}</Typography.Title>
  {state.loading?<Spin/>:state.error!==undefined?<Space direction="vertical"><Alert type="error" showIcon message={state.error instanceof APIError&&state.error.code==='orders.version_changed'?copy.orderVersionChanged:copy.unavailable}/><Button onClick={()=>setAttempt(n=>n+1)}>{copy.retry}</Button></Space>:state.data&&<>
   <Alert type="info" showIcon message={copy.paymentEvidenceInfo}/>
   {state.data.collections.length===0?<Empty description={copy.noOrderPayments}/>:state.data.collections.map(c=><Space key={c.id} direction="vertical" size={16} className="full-width">
    <Descriptions column={{xs:1,sm:2,lg:3}} items={[{key:'id',label:copy.paymentCollection,children:<span className="price-rule-value">{c.id}</span>},{key:'status',label:copy.status,children:status(c.status)},{key:'amount',label:copy.amount,children:amount(c.amount,c.currency_code)},{key:'authorized',label:copy.authorizedAmount,children:amount(c.authorized_amount,c.currency_code)},{key:'captured',label:copy.capturedAmount,children:amount(c.captured_amount,c.currency_code)},{key:'refunded',label:copy.refundedAmount,children:amount(c.refunded_amount,c.currency_code)}]}/>
    <Table<PaymentRecord> rowKey="id" size="small" dataSource={c.payments} pagination={false} scroll={{x:800}} columns={[
     {title:copy.paymentID,dataIndex:'id',render:(v:string)=><span className="price-rule-value">{v}</span>},
     {title:copy.paymentProvider,dataIndex:'provider_id',render:(v:string)=><span className="price-rule-value">{v}</span>},
     {title:copy.amount,render:(_,row)=>amount(row.amount,row.currency_code)},
     {title:copy.capturedAt,dataIndex:'captured_at',render:(v:string|null)=>v?<time dateTime={v}>{new Date(v).toLocaleString(document.documentElement.lang)}</time>:'—'},
     {title:copy.paymentOperationHold,dataIndex:'operation_hold',render:hold},
    ]} expandable={{expandIcon:({expanded,onExpand,record})=><Button type="text" size="small" aria-label={expanded?copy.hidePaymentDetails:copy.showPaymentDetails} aria-expanded={expanded} onClick={event=>onExpand(record,event)}>{expanded?'−':'+'}</Button>,expandedRowRender:p=><Space direction="vertical" size={16} className="full-width"><Descriptions column={{xs:1,sm:2}} items={[{key:'session',label:copy.paymentSession,children:<span className="price-rule-value">{p.session_id}</span>},{key:'canceled',label:copy.paymentCanceledAt,children:p.canceled_at?<time dateTime={p.canceled_at}>{new Date(p.canceled_at).toLocaleString(document.documentElement.lang)}</time>:'—'}]}/>{movements(p.captures,p.currency_code,copy.paymentCaptures)}{movements(p.refunds,p.currency_code,copy.paymentRefunds)}</Space>}}/>
    <Typography.Text strong>{copy.paymentSessions}</Typography.Text>
    <Table rowKey="id" size="small" dataSource={c.sessions} pagination={false} scroll={{x:800}} columns={[{title:copy.paymentSession,dataIndex:'id',render:(v:string)=><span className="price-rule-value">{v}</span>},{title:copy.paymentProvider,dataIndex:'provider_id',render:(v:string)=><span className="price-rule-value">{v}</span>},{title:copy.amount,render:(_,row)=>amount(row.amount,row.currency_code)},{title:copy.status,dataIndex:'status',render:status},{title:copy.paymentOperationHold,dataIndex:'operation_hold',render:hold}]}/>
   </Space>)}
  </>}
 </Space></section>
}
