import {useEffect,useRef,useState} from 'react'
import {Alert,Button,Form,Input,InputNumber,Modal,Select,Space,Typography} from 'antd'
import {APIError} from './api'
import {buyback,inspectionDevices,type BuybackRequest,type InspectionCommand} from './buyback-api'

export default function BuybackInspection({request,write,zh,onSaved,onExpired}:{request:BuybackRequest;write:boolean;zh:boolean;onSaved(value:Awaited<ReturnType<typeof buyback>>):void;onExpired():void}){
 const text=(a:string,b:string)=>zh?a:b
 const [editor,setEditor]=useState<{action:'link_device';item:string}|{action:'final_offer'}>(),[devices,setDevices]=useState<string[]>(),[error,setError]=useState(''),[busy,setBusy]=useState(false),[attempt,setAttempt]=useState(0),[form]=Form.useForm(),lock=useRef(false)
 const fail=(e:unknown)=>{if(e instanceof APIError&&e.status===401)onExpired();return e instanceof APIError&&e.code==='BUYBACK_DEVICE_MISMATCH'?text('设备已变更或不匹配，请刷新后核对型号、容量、SN/IMEI和仓库。','Device changed or does not match. Refresh and check model, storage, SN/IMEI and warehouse.'):e instanceof APIError&&e.code==='BUYBACK_LINK_DEVICES_FIRST'?text('请先为每台手机关联检测设备。','Link every phone to an inspection device first.'):text('操作或加载失败，请刷新后核对当前状态。','Request unavailable. Refresh and check the current state.')}
 useEffect(()=>{
  if(editor?.action!=='link_device')return
  const c=new AbortController();setDevices(undefined);setError('')
  inspectionDevices(request.id,editor.item,c.signal).then(v=>{if(!c.signal.aborted)setDevices(v)}).catch(e=>{if(!c.signal.aborted)setError(fail(e))})
  return()=>c.abort()
 },[editor,request.id,attempt])
 const open=(value:NonNullable<typeof editor>)=>{form.resetFields();setError('');setEditor(value)}
 const save=async()=>{
  if(!editor||lock.current||!write)return
  let values:{device_id?:string;items?:{amount:number;notes:string}[]};try{values=await form.validateFields()}catch{return}
  const command:InspectionCommand=editor.action==='link_device'?{action:'link_device',item_id:editor.item,device_id:values.device_id!}:{action:'final_offer',items:request.items.map((item,i)=>({item_id:item.id,amount:values.items![i].amount,notes:values.items![i].notes.trim()}))}
  lock.current=true;setBusy(true);setError('')
  try{const current=await buyback(request.id,command);setEditor(undefined);onSaved(current)}catch(e){setError(fail(e))}finally{lock.current=false;setBusy(false)}
 }
 return <section aria-label={text('检测与最终报价','Inspection and final offer')}><Typography.Title level={4}>{text('检测与最终报价','Inspection and final offer')}</Typography.Title>
 <Space direction="vertical" size={16} className="full-width">{request.items.map(item=><div key={item.id}><Typography.Text strong>{item.model_name} · {item.capacity}</Typography.Text><Typography.Paragraph>{text('关联设备：','Linked device: ')}{item.device_id||'—'}</Typography.Paragraph>
 {item.final_amount!==null&&<Typography.Paragraph>{text('最终报价：','Final offer: ')}{request.currency_code.toUpperCase()} {item.final_amount.toFixed(2)} · {item.inspection_notes}</Typography.Paragraph>}
 {write&&request.status==='received'&&!item.device_id&&<Button onClick={()=>open({action:'link_device',item:item.id})}>{text('关联检测设备','Link inspection device')}</Button>}
 </div>)}
 {write&&request.status==='received'&&<Button type="primary" disabled={request.items.some(item=>!item.device_id)} onClick={()=>open({action:'final_offer'})}>{text('发送最终报价','Send final offer')}</Button>}
 </Space>
 <Modal key={editor?.action==='link_device'?editor.item:editor?.action||'closed'} open={!!editor} destroyOnClose title={editor?.action==='link_device'?text('关联检测设备','Link inspection device'):text('发送最终报价','Send final offer')} confirmLoading={busy} okText={text('确认','Confirm')} cancelText={text('取消','Cancel')} onOk={save} onCancel={()=>{if(!busy)setEditor(undefined)}}>
 {error&&<Alert type="error" showIcon message={error}/>}
 <Form form={form} layout="vertical" disabled={busy}>
 {editor?.action==='link_device'&&<><Typography.Paragraph>{text('先在商品 SKU 中登记回收设备。这里只显示型号、容量、SN/IMEI及仓库均匹配的待检测设备。','Register the recycled device under its product SKU first. Only inspection devices with matching model, storage, SN/IMEI and warehouse appear here.')}</Typography.Paragraph><Form.Item name="device_id" label={text('检测设备','Inspection device')} rules={[{required:true}]}><Select loading={!devices&&!error} disabled={!devices?.length||busy} options={devices?.map(value=>({value,label:value}))} /></Form.Item>{devices?.length===0&&<Alert type="info" message={text('暂无匹配的待检测设备','No matching inspection devices')}/>}<Button disabled={busy} onClick={()=>setAttempt(n=>n+1)}>{text('刷新设备','Refresh devices')}</Button></>}
 {editor?.action==='final_offer'&&<><Typography.Paragraph>{text('客户接受后才能登记打款；客户可拒绝报价并申请退回。','Payment can be recorded after the customer accepts. The customer can decline and request a return.')}</Typography.Paragraph>{request.items.map((item,i)=><section key={item.id}><Typography.Text strong>{item.model_name} · {item.capacity} · SN: {item.serial_number}</Typography.Text><Form.Item name={['items',i,'amount']} label={text('金额','Amount')+' ('+request.currency_code.toUpperCase()+')'} rules={[{required:true},{validator:(_,value)=>value===undefined||value===null||value===''?Promise.resolve():typeof value==='number'&&Number.isFinite(value)&&value>=0&&value<=1000000&&Math.abs(value*100-Math.round(value*100))<1e-7?Promise.resolve():Promise.reject(new Error(text('请输入精确到分的有效金额','Enter a valid amount in cents')))}]}><InputNumber min={0} max={1000000} step={0.01} style={{width:'100%'}}/></Form.Item><Form.Item name={['items',i,'notes']} label={text('检测说明','Inspection notes')} rules={[{required:true,whitespace:true,min:3,max:1000}]}><Input.TextArea maxLength={1000}/></Form.Item></section>)}</>}
 </Form></Modal></section>
}
