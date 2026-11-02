import {useEffect,useRef,useState} from 'react'
import {Alert,Button,Drawer,Form,Input,Popconfirm,Select,Space,Spin,Table,Tag,Typography} from 'antd'
import {APIError,getOrderPayments,type OrderPayments} from './api'
import {cancelRefund,prepareRefund,readRefunds,type RefundContext,type RefundInput,type RefundOperation} from './refundApi'
import {useAccess} from './Permissions'
export type RefundCommand={kind:'prepare';body:RefundInput}|{kind:'cancel';operation_id:string;original:string}
export default function OrderRefund({order,requestID,onClose,onExpired,draft,onDraft}:{order:string;requestID:string;onClose():void;onExpired():void;draft?:RefundCommand;onDraft(command:RefundCommand|undefined):void}){
 const zh=document.documentElement.lang.startsWith('zh'),access=useAccess(),[form]=Form.useForm<{payment_id:string;amount:string}>(),[data,setData]=useState<RefundContext>(),[payments,setPayments]=useState<OrderPayments>(),[error,setError]=useState(false),[reload,setReload]=useState(0),[pending,setPending]=useState<RefundCommand|undefined>(draft),[saving,setSaving]=useState(false),[failed,setFailed]=useState(false),[accepted,setAccepted]=useState(false),[previous,setPrevious]=useState<RefundOperation>(),busy=useRef(false)
 const allowed=access.can('order.refund.write','order.payment.read','order.after_sales.write')
 useEffect(()=>{const c=new AbortController();setError(false);Promise.all([readRefunds(order,requestID,c.signal),getOrderPayments(order,c.signal)]).then(([d,p])=>{if(!c.signal.aborted){setData(d);setPayments(p)}}).catch(e=>{if(!c.signal.aborted){if(e instanceof APIError&&e.status===401)onExpired();else setError(true)}});return()=>c.abort()},[order,requestID,reload,onExpired])
 // Poll local facts only. The durable worker owns provider calls; stop on errors or after five minutes.
 const watching=!!data?.operations.some(r=>!['failed','canceled'].includes(r.state)&&!r.applied)
 useEffect(()=>{if(!watching||error)return;const c=new AbortController();let count=0,inFlight=false;const timer=window.setInterval(async()=>{if(document.visibilityState!=='visible'||inFlight)return;if(++count>60){clearInterval(timer);return}inFlight=true;try{const d=await readRefunds(order,requestID,c.signal);if(!c.signal.aborted)setData(d)}catch(e){if(!c.signal.aborted){setError(true);if(e instanceof APIError&&e.status===401)onExpired()}}finally{inFlight=false}},5000);return()=>{clearInterval(timer);c.abort()}},[watching,error,order,requestID,reload,onExpired])
 const send=async(command:RefundCommand)=>{if(busy.current||!allowed)return;busy.current=true;setSaving(true);setFailed(false);setPending(command);onDraft(command);try{if(command.kind==='prepare')await prepareRefund(order,requestID,command.body);else await cancelRefund(order,requestID,command.original,command.operation_id);setPending(undefined);onDraft(undefined);setPrevious(undefined);setAccepted(true);form.resetFields();setReload(n=>n+1)}catch(e){setFailed(true);if(e instanceof APIError&&e.status===401)onExpired()}finally{busy.current=false;setSaving(false)}}
 const candidates=payments?.collections.flatMap(c=>c.payments).filter(p=>p.captured_at&&!p.canceled_at)??[]
 const active=data?.operations.some(r=>!['failed','canceled'].includes(r.state))??false
 const exchange=data?.request_type==='exchange'
 const creditAvailable=!exchange||!!data?.exchange_credit&&data.exchange_credit!=='0.00'
 const writable=!!data?.enabled&&allowed&&!error&&!saving&&!pending
 const blocked=saving
 const labels:Record<string,string>=zh?{pending:'等待发送',uncertain:'等待核实',queued:'已排队',processing:'渠道处理中',refunded:'渠道已退款',failed:'已失败',canceled:'已取消'}:{pending:'Awaiting dispatch',uncertain:'Awaiting reconciliation',queued:'Queued',processing:'Processing',refunded:'Provider refunded',failed:'Failed',canceled:'Canceled'}
 return <Drawer open width={760} title={exchange?(zh?'换机差额退款':'Exchange balance refund'):(zh?'退货退款':'Return refund')} onClose={()=>{if(!blocked)onClose()}} closable={!blocked} maskClosable={!blocked} keyboard={!blocked}>
 <Space direction="vertical" size={16} className="full-width">
 <Typography.Paragraph type="secondary">{zh?'核对原付款；选择了序列号设备的申请需先收回设备。退款不会自动收货、释放库存预留或取消发货。接受指令后由后台处理，只有“已记账”代表本地退款入账。':'Verify the original payment. Requests with selected serial-number devices require those devices to be received first. A refund does not receive goods, release stock reservations or cancel shipments. Accepted instructions are processed in the background; only “Posted” confirms the local refund ledger.'}</Typography.Paragraph>
 {error&&<Alert type="error" message={zh?'退款记录暂不可用，请重新读取并核对权限。':'Refund records unavailable. Reload and verify permissions.'}/>}
 {!data||!payments?error?null:<Spin/>:<>
 {exchange&&<Alert type="info" message={(zh?'当前可退换机差额：':'Available exchange credit: ')+(data.exchange_credit??'—')+(zh?'。仅可退回完整的当前差额。':'. Refund the full current credit.')} />}
 {!data.enabled&&<Alert type="warning" message={zh?'退款写入尚未启用':'Refund writes are disabled'}/>}
 {!allowed&&<Alert type="warning" message={zh?'当前退款权限不可用':'Refund permission is unavailable'}/>}
 {accepted&&<Alert type="success" message={zh?'指令已接受，请核对后续处理状态':'Instruction accepted. Check its processing status.'}/>}
 <Table rowKey="operation_id" size="small" pagination={false} scroll={{x:650}} dataSource={data.operations} columns={[
 {title:zh?'指令':'Instruction',dataIndex:'operation_id',render:(v:string)=><Typography.Text className="break-words">{v}</Typography.Text>},
 {title:zh?'退款金额':'Refund amount',render:(_,r)=>r.currency_code.toUpperCase()+' '+r.amount},
 {title:zh?'状态':'Status',render:(_,r)=><Space direction="vertical" size={4}><Tag>{labels[r.state]}</Tag><Typography.Text>{r.applied?(zh?'已记账':'Posted'):r.state==='refunded'?(zh?'等待记账':'Awaiting ledger posting'):null}</Typography.Text></Space>},
 {title:zh?'操作':'Actions',render:(_,r)=><Space wrap>{r.can_cancel&&<Popconfirm title={zh?'取消尚未发送的退款指令？':'Cancel this undispatched refund?'} okText={zh?'确认取消退款':'Confirm cancellation'} cancelText={zh?'保留':'Keep'} disabled={!writable} onConfirm={()=>send({kind:'cancel',operation_id:crypto.randomUUID(),original:r.operation_id})}><Button disabled={!writable}>{zh?'取消退款指令':'Cancel refund instruction'}</Button></Popconfirm>}{r.can_retry&&!active&&<Button disabled={!writable} onClick={()=>{setPrevious(r);form.setFieldsValue({payment_id:r.payment_id,amount:exchange&&data?.exchange_credit?data.exchange_credit:r.amount.includes('.')?r.amount.split('.')[0]+'.'+r.amount.split('.')[1].padEnd(2,'0'):r.amount+'.00'});setAccepted(false)}}>{zh?'明确发起新尝试':'Start a new attempt'}</Button>}</Space>}
 ]}/>
 {failed&&<Alert type="error" message={zh?'结果未确认。请重试同一指令；核对记录不会生成新指令。':'Result unconfirmed. Retry the same instruction; reloading does not create another instruction.'}/>}
 {pending&&<><Alert type="info" message={zh?'本次指令已锁定。关闭仅隐藏窗口；留在此订单页重新打开可重试同一指令。':'This instruction is locked. Closing hides this window; reopen it on this order page to retry the same instruction.'}/><Button type="primary" disabled={!allowed} loading={saving} onClick={()=>void send(pending)}>{zh?'重试同一退款操作':'Retry same refund operation'}</Button></>}
 {previous&&<Alert type="info" message={(zh?'新尝试的前驱：':'Previous instruction: ')+previous.operation_id}/>}
 <Form form={form} layout="vertical" disabled={!writable||!creditAvailable||active||data.operations.length>0&&!previous} onFinish={v=>void send({kind:'prepare',body:{operation_id:crypto.randomUUID(),payment_id:v.payment_id,amount:v.amount,...(previous?{previous_operation_id:previous.operation_id}:{})}})}>
 <Form.Item name="payment_id" label={zh?'原付款':'Original payment'} rules={[{required:true}]}><Select options={candidates.map(p=>({value:p.id,label:p.id+' · '+p.currency_code.toUpperCase()+' '+p.amount}))}/></Form.Item>
 <Form.Item name="amount" label={zh?'退款金额（两位小数）':'Refund amount (two decimal places)'} rules={[{required:true},{validator:(_,v)=>exchange&&v!==data.exchange_credit?Promise.reject(new Error(zh?'请输入完整的当前可退差额':'Enter the full current exchange credit')):typeof v==='string'&&/^(?:0|[1-9]\d{0,17})\.\d{2}$/.test(v)&&/[1-9]/.test(v)?Promise.resolve():Promise.reject(new Error(zh?'请输入大于零的精确金额，例如 10.00':'Enter a positive exact amount, e.g. 10.00'))}]}><Input inputMode="decimal" autoComplete="off" maxLength={21}/></Form.Item>
 <Button type="primary" htmlType="submit" loading={saving} disabled={!writable||!creditAvailable||active||!candidates.length||data.operations.length>0&&!previous}>{zh?'提交退款指令':'Submit refund instruction'}</Button>
 </Form></>}
 <Button disabled={saving} onClick={()=>setReload(n=>n+1)}>{zh?'重新读取退款记录':'Reload refund records'}</Button>
 </Space></Drawer>
}
