import {useEffect,useState} from 'react'
import {Alert,Button,Drawer,Form,Input,Space,Table,Tag,Typography} from 'antd'
import {APIError,csrfCookie} from './api'
import {useAccess} from './Permissions'
import type {Copy} from './i18n'
type Content={to:string[];cc:string[];bcc:string[];subject:string;text:string;attachments:{filename:string;content:string}[]}
type Row={id:string;account_id:string;revision:number;subject:string;status:string;delivery_status?:string;error_code?:string;updated_at:string}
type Values={to:string;cc:string;bcc:string;subject:string;text:string}
export type Reply={to:string;subject:string;text:string}
async function api<T>(path:string,body?:unknown):Promise<T>{
 const response=await fetch('/api/admin/notification-mail/compositions'+path,{credentials:'same-origin',cache:'no-store',...(body===undefined?{}:{method:'POST',headers:{'Content-Type':'application/json','X-CSRF-Token':csrfCookie()??''},body:JSON.stringify(body)})})
 if(!response.ok)throw new APIError(response.status,'mail.unavailable');return response.json()
}
const addresses=(value:string)=>value.split(/[;,\n]/).map(v=>v.trim()).filter(Boolean)
export default function MailCompose({accountID,copy,onExpired,reply,onReplyUsed}:{accountID:string;copy:Copy;onExpired():void;reply?:Reply;onReplyUsed():void}){
 const zh=copy.mailboxes==='通知邮箱',writable=useAccess().can('mail.write'),[form]=Form.useForm<Values>()
 const [rows,setRows]=useState<Row[]>([]),[reload,setReload]=useState(0),[editor,setEditor]=useState<{id:string;revision:number;status:string}>(),[attachments,setAttachments]=useState<Content['attachments']>([]),[busy,setBusy]=useState(false),[error,setError]=useState(false),[dirty,setDirty]=useState(false)
 const fail=(e:unknown)=>{setError(true);if(e instanceof APIError&&e.status===401)onExpired()}
 const create=(seed?:Reply)=>{form.resetFields();form.setFieldsValue({to:seed?.to??'',cc:'',bcc:'',subject:seed?.subject??'',text:seed?.text??''});setAttachments([]);setEditor({id:crypto.randomUUID(),revision:0,status:'draft'});setError(false)}
 useEffect(()=>{let active=true;api<{messages:Row[]}>(`?account_id=${encodeURIComponent(accountID)}`).then(r=>{if(active)setRows(r.messages)}).catch(fail);return()=>{active=false}},[accountID,reload])
 useEffect(()=>{if(reply&&writable){create(reply);onReplyUsed()}},[reply])
 const open=async(row:Row)=>{setBusy(true);try{const detail=await api<{message:Row;content:Content}>('/'+row.id);const c=detail.content;form.setFieldsValue({to:c.to.join('; '),cc:(c.cc??[]).join('; '),bcc:(c.bcc??[]).join('; '),subject:c.subject,text:c.text});setAttachments(c.attachments??[]);setEditor(detail.message);setDirty(false);setError(false)}catch(e){fail(e)}finally{setBusy(false)}}
 const save=async()=>{
  if(!editor)return
  const values=await form.validateFields();setBusy(true);setError(false)
  try{const row=await api<Row>('/'+editor.id,{account_id:accountID,revision:editor.revision,content:{to:addresses(values.to),cc:addresses(values.cc??''),bcc:addresses(values.bcc??''),subject:values.subject,text:values.text,attachments}});setEditor(row);setDirty(false);setReload(n=>n+1)}catch(e){fail(e)}finally{setBusy(false)}
 }
 const recover=async()=>{if(editor)await open({...editor,account_id:accountID,subject:'',updated_at:''})}
 const send=async()=>{
  if(!editor||editor.revision===0)return;setBusy(true);setError(false)
  try{const row=await api<Row>('/'+editor.id+'/send',{revision:editor.revision});setEditor(row);setDirty(false);setReload(n=>n+1)}catch(e){fail(e)}finally{setBusy(false)}
 }
 const pick=async(files:FileList|null)=>{
  if(!files)return
  const added:Content['attachments']=[]
  for(const file of Array.from(files)){
   if(file.size>512*1024){setError(true);return}
   const content=await new Promise<string>((resolve,reject)=>{const reader=new FileReader();reader.onload=()=>resolve(String(reader.result).split(',')[1]);reader.onerror=reject;reader.readAsDataURL(file)})
   added.push({filename:file.name,content})
  }
  const all=[...attachments,...added];if(all.length>10||all.reduce((n,a)=>n+atob(a.content).length,0)>512*1024){setError(true);return};setAttachments(all);setDirty(true)
 }
 return <section data-testid="mail-compose"><Space direction="vertical" size={16} className="full-width">
  <Typography.Title level={3}>{zh?'草稿与已发送':'Drafts and sent mail'}</Typography.Title>
  <Space><Button disabled={!writable||busy} onClick={()=>create()}>{zh?'写邮件':'Compose email'}</Button><Button onClick={()=>setReload(n=>n+1)}>{copy.reload}</Button></Space>
  {error&&!editor&&<Alert type="error" message={copy.unavailable}/>}
  <Table<Row> rowKey="id" dataSource={rows} pagination={{pageSize:10}} scroll={{x:600}} columns={[
   {title:zh?'主题':'Subject',render:(_,row)=><Button type="link" onClick={()=>void open(row)}>{row.subject}</Button>},
   {title:copy.status,render:(_,row)=><Tag>{({draft:zh?'草稿':'Draft',sent:zh?'服务已接收':'Accepted',sending:zh?'发送结果待核对':'Submission awaiting review',review_required:zh?'需核对渠道':'Channel review required'} as Record<string,string>)[row.status]??row.status}</Tag>},
   {title:zh?'更新时间':'Updated',render:(_,row)=>new Date(row.updated_at).toLocaleString()}
   ,{title:zh?'投递回执':'Delivery receipt',render:(_,row)=>row.delivery_status?({success:zh?'已投递':'Delivered',delayed:zh?'延迟':'Delayed',failed:zh?'投递失败':'Delivery failed'} as Record<string,string>)[row.delivery_status]??row.delivery_status:'—'}
  ]}/>
  {editor&&<Drawer open width="min(800px,100vw)" title={zh?'邮件':'Email'} onClose={()=>{if(!busy)setEditor(undefined)}} extra={<Space><Button onClick={()=>void recover()} disabled={busy}>{zh?'读取已保存版本':'Load saved version'}</Button><Button onClick={()=>void save()} disabled={busy||!writable||editor.status!=='draft'}>{zh?'保存草稿':'Save draft'}</Button><Button type="primary" onClick={()=>void send()} disabled={busy||!writable||!editor.revision||editor.status!=='draft'||dirty}>{zh?'发送已保存邮件':'Send saved email'}</Button></Space>}>
   {error&&<Alert type="error" showIcon message={zh?'操作未确认，请读取已保存版本后核对。':'Operation unconfirmed. Load the saved version to check its state.'}/>}
   {editor.status!=='draft'&&<Alert type="info" showIcon message={editor.status==='sent'?(zh?'发信服务已接收。':'The sending service accepted this email.'):(zh?'请核对渠道记录；系统不会自动重复提交。':'Check the sending service. This submission will not be automatically repeated.')}/>}
   <Form form={form} layout="vertical" disabled={busy||!writable||editor.status!=='draft'} onValuesChange={()=>setDirty(true)}>
    <Form.Item name="to" label={zh?'收件人':'To'} rules={[{required:true}]}><Input placeholder="name@example.com; second@example.com"/></Form.Item>
    <Form.Item name="cc" label="Cc"><Input/></Form.Item><Form.Item name="bcc" label="Bcc"><Input/></Form.Item>
    <Form.Item name="subject" label={zh?'主题':'Subject'} rules={[{required:true}]}><Input maxLength={1024}/></Form.Item>
    <Form.Item name="text" label={zh?'正文':'Message'} rules={[{required:true}]}><Input.TextArea rows={12} maxLength={65536}/></Form.Item>
    <Typography.Paragraph>{zh?'附件最多10个，总计512KiB。':'Up to 10 attachments, 512 KiB total.'}</Typography.Paragraph>
    <input type="file" multiple aria-label={zh?'添加附件':'Add attachments'} disabled={busy||!writable||editor.status!=='draft'} onChange={event=>void pick(event.target.files)}/>
    {attachments.map((a,index)=><Typography.Paragraph key={index}>{a.filename} <Button disabled={busy||!writable||editor.status!=='draft'} onClick={()=>{setAttachments(v=>v.filter((_,i)=>i!==index));setDirty(true)}}>{zh?'移除':'Remove'}</Button></Typography.Paragraph>)}
   </Form>
  </Drawer>}
 </Space></section>
}
