import {useEffect,useRef,useState} from 'react'
import {Alert,Button,Descriptions,Drawer,Form,Image,Input,Modal,Select,Space,Spin,Table,Tag,Timeline,Typography} from 'antd'
import {APIError} from './api'
import BuybackInspection from './BuybackInspection'
import {getFlowCopy} from '../../storefront/src/components/sell/flow-copy'
import {getBuybackCopy} from '../../storefront/src/components/sell/buyback-copy'
import {useAccess} from './Permissions'
import type {Copy} from './i18n'
import {buybacks,buyback,buybackPhoto,type BuybackRequest,type ReviewCommand} from './buyback-api'
const labels={en:{awaiting_photos:'Awaiting photos',photo_review:'Photos under review',ready_to_ship:'Approved for shipping',in_transit:'In transit',received:'Received / inspecting',final_offer:'Awaiting final-offer decision',accepted:'Accepted / awaiting payment',paid:'Paid',return_requested:'Return requested',returning:'Returning',returned:'Returned',canceled:'Canceled'},zh:{awaiting_photos:'待补充照片',photo_review:'待审核照片',ready_to_ship:'审核通过，待寄件',in_transit:'寄送中',received:'已收货，检测中',final_offer:'待客户确认最终报价',accepted:'已接受，待打款',paid:'已打款',return_requested:'待原路退回',returning:'退回途中',returned:'已退回',canceled:'已取消'}}
function Photo({id,photo,onExpired}:{id:string;photo:BuybackRequest['photos'][number];onExpired():void}){
 const [source,setSource]=useState(''),[failed,setFailed]=useState(false),[attempt,setAttempt]=useState(0)
 useEffect(()=>{const c=new AbortController();setSource('');setFailed(false);buybackPhoto(id,photo.id,c.signal).then(v=>{if(!c.signal.aborted)setSource('data:'+v.mime_type+';base64,'+v.content)}).catch(e=>{if(!c.signal.aborted){setFailed(true);if(e instanceof APIError&&e.status===401)onExpired()}});return()=>c.abort()},[id,photo.id,attempt,onExpired])
 return failed?<Button onClick={()=>setAttempt(n=>n+1)}>Retry / 重试</Button>:source?<Image src={source} width={144} height={144} style={{objectFit:'contain'}} alt={photo.photo_type}/>:<Spin/>
}
export default function Buybacks({copy,onExpired}:{copy:Copy;onExpired():void}){
 const zh=copy.language==='语言',text=(a:string,b:string)=>zh?a:b,statuses=zh?labels.zh:labels.en,access=useAccess()
 const flow=getFlowCopy(zh?'zh':'en'),selling=getBuybackCopy(zh?'zh':'en')
 const assessmentRows=(item:BuybackRequest['items'][number])=>{
  const a=item.assessment,unknown=text('未回答','Unanswered')
  const grade=(value:string)=>flow.grades[['damaged','visible','some','minimal','none'].indexOf(value)]||unknown
  return [
   {key:'battery',label:flow.titles[2],children:a.battery==='unknown'?flow.unsure:a.battery?`${a.battery}%`:unknown},
   ...a.checks.map((v,i)=>({key:'check'+i,label:i===2&&item.model_name.includes('SE')?flow.touchId[2]:flow.checks[i][2],children:v===null?unknown:v?flow.yes:flow.no})),
   {key:'faults',label:flow.titles[4],children:a.faults.map(v=>v==='none'?flow.allWorks:flow.faults[['bright','pixels','lines'].indexOf(v)]||v).join(', ')||unknown},
   {key:'glass',label:flow.titles[5],children:a.glass.map(v=>v==='none'?flow.none:flow.glass[['cracked','chipped','scratched'].indexOf(v)]?.[0]||v).join(', ')||unknown},
   ...(['screen','frame','back'] as const).map((key,i)=>({key,label:flow.titles[i+6],children:grade(a[key])}))
  ]
 }
 const [offset,setOffset]=useState(0),[q,setQ]=useState(''),[status,setStatus]=useState(''),[attempt,setAttempt]=useState(0)
 const [data,setData]=useState<Awaited<ReturnType<typeof buybacks>>>(),[error,setError]=useState('')
 const [id,setID]=useState(''),[detail,setDetail]=useState<Awaited<ReturnType<typeof buyback>>>(),[detailError,setDetailError]=useState('')
 const [editor,setEditor]=useState<ReviewCommand>(),[busy,setBusy]=useState(false),lock=useRef(false),[form]=Form.useForm()
 const fail=(e:unknown)=>{if(e instanceof APIError&&e.status===401)onExpired();return e instanceof APIError&&e.code==='BUYBACK_PHOTOS_REQUIRED'?text('必需照片须全部审核通过','Approve all required photos first'):e instanceof APIError&&e.code==='BUYBACK_REVIEW_NOTE_REQUIRED'?text('请填写至少三个字符的重拍原因','Enter a retake reason of at least three characters'):text('操作或加载失败，请刷新后核对当前状态。','Request unavailable. Refresh and check the current state.')}
 useEffect(()=>{const c=new AbortController();setData(undefined);setError('');buybacks(new URLSearchParams({offset:String(offset),...(q?{q}:{}),...(status?{status}:{})}),c.signal).then(v=>{if(!c.signal.aborted)setData(v)}).catch(e=>{if(!c.signal.aborted)setError(fail(e))});return()=>c.abort()},[offset,q,status,attempt])
 useEffect(()=>{if(!id){setDetail(undefined);return};const c=new AbortController();setDetail(undefined);setDetailError('');buyback(id,undefined,c.signal).then(v=>{if(!c.signal.aborted)setDetail(v)}).catch(e=>{if(!c.signal.aborted)setDetailError(fail(e))});return()=>c.abort()},[id,attempt])
 const open=(v:ReviewCommand)=>{form.resetFields();setDetailError('');setEditor(v)}
 const save=async()=>{
  if(!editor||lock.current)return
  let values:Record<string,string>;try{values=await form.validateFields()}catch{return};lock.current=true;setBusy(true);setDetailError('')
  try{const current=await buyback(id,{...editor,...values});setDetail(current);setEditor(undefined);setAttempt(n=>n+1)}catch(e){setDetailError(fail(e))}finally{lock.current=false;setBusy(false)}
 }
 const request=detail?.request,write=access.can('buyback.write')&&detail?.enabled
 return <section data-testid="buyback-admin"><Space direction="vertical" size={16} className="full-width">
 <Space wrap><Input.Search aria-label={text('搜索回收申请','Search selling requests')} allowClear maxLength={100} onSearch={v=>{setQ(v.trim());setOffset(0)}}/><Select aria-label={text('申请状态','Request status')} style={{minWidth:190}} value={status} onChange={v=>{setStatus(v);setOffset(0)}} options={[{value:'',label:text('全部状态','All states')},...Object.entries(statuses).map(([value,label])=>({value,label}))]}/><Button onClick={()=>setAttempt(n=>n+1)}>{text('刷新','Refresh')}</Button></Space>
 {error?<Alert showIcon type="error" message={error}/>:!data?<Spin/>:<Table rowKey="id" dataSource={data.requests} scroll={{x:750}} pagination={{current:offset/20+1,pageSize:20,total:data.count,showSizeChanger:false,onChange:p=>setOffset((p-1)*20)}} columns={[
 {title:text('申请','Request'),render:(_,r:BuybackRequest)=><Button type="link" onClick={()=>setID(r.id)}>{r.id}</Button>},
 {title:text('客户','Customer'),render:(_,r)=><>{r.contact_name}<br/>{r.email}</>},
 {title:text('状态','Status'),render:(_,r)=><Tag>{statuses[r.status as keyof typeof statuses]}</Tag>},
 {title:text('设备','Phones'),render:(_,r)=>r.items.map(v=>v.model_name+' · '+v.capacity).join(', ')},
 {title:text('预估回收价','Estimated value'),render:(_,r)=>r.reference_amount===null?'—':r.currency_code.toUpperCase()+' '+r.reference_amount},
 {title:text('创建时间','Created'),dataIndex:'created_at',render:v=>new Date(v).toLocaleString(zh?'zh-CN':'en-GB')}
 ]}/>}
 </Space>
 <Drawer title={text('回收申请详情','Selling request details')} open={!!id} onClose={()=>{if(!busy){setID('');setEditor(undefined)}}} width={880} destroyOnClose>
 <Space direction="vertical" size={16} className="full-width">{detailError&&<Alert type="error" showIcon message={detailError}/>}<Button disabled={busy} onClick={()=>setAttempt(n=>n+1)}>{text('刷新当前申请','Refresh request')}</Button>
 {!request?<Spin/>:<>
 {!detail.enabled&&<Alert type="info" message={text('回收处理尚未启用，仅可查看','Selling request commands are disabled')}/>}
 <Descriptions column={1} items={[{key:'id',label:text('申请','Request'),children:request.id},{key:'status',label:text('状态','Status'),children:statuses[request.status as keyof typeof statuses]},{key:'customer',label:text('客户','Customer'),children:request.contact_name+' · '+request.email+' · '+request.phone},{key:'address',label:text('退回地址','Return address'),children:Object.values(request.return_address).filter(Boolean).join(', ')},{key:'tracking',label:text('物流单号','Tracking'),children:request.tracking_number||'—'},{key:'note',label:text('审核说明','Review note'),children:request.review_note||'—'}]}/>
 {request.items.map(item=><section key={item.id}><Typography.Title level={4}>{item.model_name} · {item.capacity}</Typography.Title><Typography.Paragraph>SN: {item.serial_number||'—'} · IMEI: {item.imei||'—'}</Typography.Paragraph>
 <Descriptions size="small" column={2} items={assessmentRows(item)}/>
 <Space align="start" wrap>{request.photos.filter(p=>p.item_id===item.id).map(p=><Space direction="vertical" key={p.id}><Typography.Text>{selling.photoTypes[p.photo_type as keyof typeof selling.photoTypes]}</Typography.Text><Photo id={id} photo={p} onExpired={onExpired}/><Tag>{selling.reviews[p.review_status as keyof typeof selling.reviews]}</Tag>{p.review_note&&<Typography.Text>{p.review_note}</Typography.Text>}{write&&['awaiting_photos','photo_review'].includes(request.status)&&p.review_status!=='expired'&&<Space><Button disabled={busy} onClick={()=>open({action:'review_photo',photo_id:p.id,review_status:'approved',note:''})}>{text('通过','Approve')}</Button><Button disabled={busy} onClick={()=>open({action:'review_photo',photo_id:p.id,review_status:'retake',note:''})}>{text('要求重拍','Request retake')}</Button></Space>}</Space>)}</Space></section>)}
 <Space wrap>{write&&request.status==='photo_review'&&<Button type="primary" onClick={()=>open({action:'approve_shipping',shipping_instructions:''})}>{text('批准寄件','Approve shipping')}</Button>}{write&&['ready_to_ship','in_transit'].includes(request.status)&&<Button onClick={()=>open({action:'mark_received'})}>{text('确认收货','Mark received')}</Button>}{write&&['awaiting_photos','photo_review','ready_to_ship'].includes(request.status)&&<Button danger onClick={()=>open({action:'cancel',note:''})}>{text('取消申请','Cancel request')}</Button>}</Space>
 {['received','final_offer','accepted','paid','return_requested','returning','returned'].includes(request.status)&&<BuybackInspection key={request.id+request.updated_at} request={request} write={!!write} zh={zh} onExpired={onExpired} onSaved={current=>{setDetail(current);setAttempt(n=>n+1)}}/>}
 <Space wrap>{write&&request.status==='accepted'&&<Button type="primary" onClick={()=>open({action:'mark_paid',payment_reference:''})}>{text('登记已完成打款','Record completed payment')}</Button>}{write&&['received','final_offer'].includes(request.status)&&<Button onClick={()=>open({action:'request_return',note:''})}>{text('检测后退回','Return after inspection')}</Button>}{write&&request.status==='return_requested'&&<Button type="primary" onClick={()=>open({action:'mark_returning',return_tracking:''})}>{text('登记退回发货','Record return shipment')}</Button>}{write&&request.status==='returning'&&<Button type="primary" onClick={()=>open({action:'mark_returned'})}>{text('确认退回送达','Confirm return delivered')}</Button>}</Space>
 <Descriptions column={1} items={[...(request.payment_reference?[{key:'payment',label:text('付款凭证','Payment reference'),children:request.payment_reference}]:[]),...(request.return_tracking?[{key:'return',label:text('退回物流单号','Return tracking number'),children:request.return_tracking}]:[])]}/>
 <Timeline items={request.history.slice().reverse().map((v,i)=>({key:i,children:<>{statuses[v.status as keyof typeof statuses]} · {v.action} · {v.actor}<br/>{new Date(v.at).toLocaleString(zh?'zh-CN':'en-GB')}{v.note&&<p>{v.note}</p>}</>}))}/>
 </>}
 </Space></Drawer>
 <Modal key={editor?editor.action+(editor.action==='review_photo'?editor.photo_id+editor.review_status:''):'closed'} open={!!editor} title={text('确认回收操作','Confirm selling request action')} confirmLoading={busy} onOk={()=>void save()} onCancel={()=>{if(!busy)setEditor(undefined)}} maskClosable={!busy} closable={!busy} cancelButtonProps={{disabled:busy}} okText={text('确认','Confirm')} cancelText={text('取消','Cancel')}>
 {detailError&&<Alert type="error" message={detailError}/>}<Form form={form} layout="vertical" disabled={busy}>
 {editor&&(editor.action==='cancel'||editor.action==='request_return'||editor.action==='review_photo')&&<Form.Item name="note" label={text('说明','Note')} rules={[{required:editor.action!=='review_photo'||editor.review_status==='retake',whitespace:true,min:editor.action!=='review_photo'||editor.review_status==='retake'?3:0,max:editor.action==='review_photo'?500:1000}]}><Input.TextArea maxLength={editor.action==='review_photo'?500:1000}/></Form.Item>}
 {editor?.action==='mark_paid'&&<><Alert type="info" message={text('实际转账完成后再登记。本操作只保存付款凭证，不会发起转账。','Record this after the actual transfer is completed. This saves the payment reference and does not send money.')}/><Form.Item name="payment_reference" label={text('付款凭证','Payment reference')} rules={[{required:true,whitespace:true,min:3,max:200}]}><Input maxLength={200}/></Form.Item></>}
 {editor?.action==='mark_returning'&&<><Alert type="info" message={text('请按申请中的原退回地址寄出。本操作只登记退回物流，不会购买运单。','Ship to the original return address on this request. This records the return and does not purchase a shipping label.')}/><Form.Item name="return_tracking" label={text('退回物流单号','Return tracking number')} rules={[{required:true,whitespace:true,min:3,max:150}]}><Input maxLength={150}/></Form.Item></>}
 {editor?.action==='mark_returned'&&<Typography.Paragraph>{text('请核实包裹已送达客户原退回地址，再确认退回完成。','Verify delivery to the customer’s original return address before confirming the return.')}</Typography.Paragraph>}
 {editor?.action==='approve_shipping'&&<Form.Item name="shipping_instructions" label={text('寄件说明','Shipping instructions')} rules={[{required:true,whitespace:true,min:5,max:2000}]}><Input.TextArea maxLength={2000}/></Form.Item>}
 </Form></Modal>
 </section>
}
