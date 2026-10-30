import {useEffect,useRef,useState} from 'react'
import {Alert,Button,Descriptions,Drawer,Form,Select,Space,Spin,Typography} from 'antd'
import {APIError,readReplacement,replaceDevice,type ReplacementContext,type ReplacementInput,type ReplacementResult} from './api'
export default function DeviceReplacement({order,requestID,assignment,onClose,onExpired}:{order:string;requestID:string;assignment:string;onClose():void;onExpired():void}){
 const zh=document.documentElement.lang.startsWith('zh'),[data,setData]=useState<ReplacementContext>(),[reload,setReload]=useState(0),[error,setError]=useState(false),[pending,setPending]=useState<ReplacementInput>(),[failed,setFailed]=useState(false),[saving,setSaving]=useState(false),[result,setResult]=useState<ReplacementResult>(),busy=useRef(false)
 useEffect(()=>{const c=new AbortController();setData(undefined);setError(false);readReplacement(order,requestID,assignment,c.signal).then(d=>{if(!c.signal.aborted)setData(d)}).catch(e=>{if(!c.signal.aborted){if(e instanceof APIError&&e.status===401)onExpired();else setError(true)}});return()=>c.abort()},[order,requestID,assignment,reload,onExpired])
 const send=async(body:ReplacementInput)=>{if(busy.current)return;busy.current=true;setSaving(true);setPending(body);setFailed(false);try{const r=await replaceDevice(order,requestID,assignment,body);setResult(r);setPending(undefined);setReload(n=>n+1)}catch(e){if(e instanceof APIError&&e.status===401)onExpired();else setFailed(true)}finally{busy.current=false;setSaving(false)}}
 const locked=saving||!!pending||!!result||!data?.write_available||!data.eligible
 const reasons:Record<string,string>=zh?{native_return_exists:'原机已关联退货单，请核对替换设备记录。',replacement_exists:'原机已有活动替换设备。',payment_required:'订单付款尚未完成。',not_processing:'换机申请不在处理中。',original_not_returned:'请先收回原机。',order_inactive:'订单当前不可换机。'}:{native_return_exists:'The original device already has a return. Verify its replacement records.',replacement_exists:'An active replacement already exists.',payment_required:'Order payment is incomplete.',not_processing:'The exchange request is not processing.',original_not_returned:'Receive the original device first.',order_inactive:'The order is inactive.'}
 return <Drawer open width={720} title={zh?'分配替换设备':'Assign replacement device'} onClose={()=>{if(!saving)onClose()}} closable={!saving} maskClosable={!saving} keyboard={!saving}><Space direction="vertical" size={16} className="full-width">
 <Alert type="info" message={zh?'替换设备使用订单原 SKU 和原单价；当前税费与优惠可能形成差额。保存后继续配货发货，补款或退款须分别完成。':'The replacement uses the original order SKU and unit price. Current taxes and discounts may produce a difference. Continue packing and shipping after saving; collect or refund any difference separately.'}/>
 {result&&<><Alert type="success" message={zh?'替换设备已分配':'Replacement device assigned'}/><Descriptions column={1} items={[{key:'item',label:zh?'新商品行':'New order item',children:result.item_id},{key:'return',label:zh?'退货单':'Return',children:result.return_id},{key:'difference',label:zh?'订单待结差额':'Pending order difference',children:result.pending_difference}]}/></>}
 {error?<><Alert type="error" message={zh?'换机记录暂不可用':'Replacement records unavailable'}/><Button onClick={()=>setReload(n=>n+1)}>{zh?'重试读取':'Retry read'}</Button></>:!data?<Spin/>:<>
 <Typography.Paragraph className="identifier">SKU: {data.variant_id}</Typography.Paragraph>
 {!data.write_available&&<Alert type="warning" message={zh?'换机写入尚未启用或当前权限不足':'Replacement writes are disabled or current permission is insufficient'}/>}
 {!data.eligible&&<Alert type="warning" message={reasons[data.reason]??(zh?'当前不符合换机条件':'Replacement prerequisites are not met')}/>}
 {failed&&<Alert type="error" message={zh?'提交结果未确认或条件已变化。请重试原操作，或重新读取核对。':'Result unconfirmed or prerequisites changed. Retry the same operation or reload to verify.'}/>}
 <Form layout="vertical" disabled={locked} onFinish={(v:{device_id:string})=>{if(data)void send({operation_id:crypto.randomUUID(),expected_revision:data.revision,device_id:v.device_id})}}>
 <Form.Item name="device_id" label={zh?'替换设备':'Replacement device'} rules={[{required:true}]}><Select aria-label={zh?'替换设备':'Replacement device'} showSearch optionFilterProp="label" options={data.devices.map(d=>({value:d.id,label:`${d.serial_number} / ${d.source_name} / ${d.location_name}`}))}/></Form.Item>
 {!pending&&<Button type="primary" htmlType="submit" loading={saving} disabled={locked||!data.devices.length}>{zh?'确认分配替换设备':'Confirm replacement'}</Button>}
 </Form>
 {pending&&<Button type="primary" loading={saving} onClick={()=>void send(pending)}>{zh?'重试本次换机':'Retry this replacement'}</Button>}
 <Button disabled={saving} onClick={()=>{setPending(undefined);setFailed(false);setResult(undefined);setReload(n=>n+1)}}>{zh?'重新读取':'Reload'}</Button>
 </>}
 </Space></Drawer>
}
