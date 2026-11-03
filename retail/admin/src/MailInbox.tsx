import {useEffect,useRef,useState} from 'react'
import {Alert,App,Button,Drawer,Empty,Select,Space,Spin,Table,Tabs,Typography} from 'antd'
import {APIError,csrfCookie} from './api'
import type {Copy} from './i18n'
import {useAccess} from './Permissions'

type Message={id:string;account_id:string;subject:string;from_address:string;received_at:string;read_at:string|null;archived:boolean;deleted:boolean}
type Detail={message:Message;text:string;html:string;attachments:{index:number;filename:string}[]}
async function request<T>(path:string,body?:unknown):Promise<T>{
 const csrf=csrfCookie()
 const response=await fetch('/api/admin/notification-mail/'+path,{credentials:'same-origin',cache:'no-store',headers:{Accept:'application/json',...(body===undefined?{}:{'Content-Type':'application/json','X-CSRF-Token':csrf??''})},...(body===undefined?{}:{method:'POST',body:JSON.stringify(body)})})
 if(!response.ok)throw new APIError(response.status,'mail.unavailable')
 return response.json()
}
export default function MailInbox({accountID,enabled,copy,onExpired}:{accountID:string;enabled:boolean;copy:Copy;onExpired():void}){
 const zh=copy.mailboxes==='通知邮箱',{message}=App.useApp()
 const writable=useAccess().can('mail.write')
 const [folder,setFolder]=useState('inbox'),[offset,setOffset]=useState(0),[reload,setReload]=useState(0),[rows,setRows]=useState<Message[]>(),[detail,setDetail]=useState<Detail>(),[error,setError]=useState(false),[busy,setBusy]=useState(false),[view,setView]=useState('text')
 const generation=useRef(0)
 const fail=(e:unknown)=>{setError(true);if(e instanceof APIError&&e.status===401)onExpired()}
 useEffect(()=>{
  let active=true;setRows(undefined);setError(false)
  request<{messages:Message[]}>(`inbox?account_id=${encodeURIComponent(accountID)}&folder=${folder}&offset=${offset}`).then(result=>{if(active)setRows(result.messages)}).catch(e=>{if(active)fail(e)})
  return()=>{active=false}
 },[accountID,folder,offset,reload])
 const mutate=async(row:Message,values:{read:boolean;archived:boolean;deleted:boolean})=>{
  setBusy(true)
  try{await request('inbox/'+row.id+'/state',values);setDetail(undefined);setReload(n=>n+1)}catch(e){fail(e)}finally{setBusy(false)}
 }
 const open=async(row:Message)=>{
  const token=++generation.current;setBusy(true);setView('text')
  try{
   const loaded=await request<Detail>('inbox/'+row.id);if(token!==generation.current)return
   setDetail(loaded)
   if(!row.read_at&&writable){await request('inbox/'+row.id+'/state',{read:true,archived:row.archived,deleted:row.deleted});setReload(n=>n+1)}
  }catch(e){fail(e)}finally{setBusy(false)}
 }
 const sync=async()=>{
  setBusy(true);setError(false)
  try{const result=await request<{imported:number}>('accounts/'+accountID+'/sync',{});message.success((zh?'已同步邮件：':'Messages imported: ')+result.imported);setReload(n=>n+1)}catch(e){fail(e)}finally{setBusy(false)}
 }
 return <section data-testid="mail-inbox"><Space direction="vertical" size={16} className="full-width">
  <Typography.Title level={3}>{zh?'收件箱':'Inbox'}</Typography.Title>
  <Typography.Paragraph type="secondary">{zh?'同步读取 IMAP INBOX；归档、已读和垃圾箱操作保存在本系统。':'Synchronize the IMAP INBOX. Archive, read and trash states are saved in this application.'}</Typography.Paragraph>
  <Space wrap><Select aria-label={zh?'邮件文件夹':'Mail folder'} value={folder} onChange={value=>{setFolder(value);setOffset(0)}} options={[{value:'inbox',label:zh?'收件箱':'Inbox'},{value:'archive',label:zh?'归档':'Archive'},{value:'trash',label:zh?'垃圾箱':'Trash'}]}/><Button onClick={()=>setReload(n=>n+1)} disabled={busy}>{copy.reload}</Button><Button onClick={()=>void sync()} disabled={busy||!enabled||!writable} loading={busy}>{zh?'同步 IMAP 收件':'Sync IMAP inbox'}</Button></Space>
  {error&&<Alert type="error" showIcon message={copy.unavailable}/>}
  {!enabled&&<Alert type="info" showIcon message={zh?'请先配置 IMAP 服务器与凭据。':'Configure the IMAP server and credentials first.'}/>}
  {!rows?<Spin/>:<><Table<Message> rowKey="id" dataSource={rows} pagination={false} scroll={{x:700}} locale={{emptyText:<Empty/>}} columns={[
   {title:zh?'主题':'Subject',render:(_,row)=><Button type="link" onClick={()=>void open(row)} disabled={busy}><Typography.Text strong={!row.read_at}>{row.subject||'(No subject)'}</Typography.Text></Button>},
   {title:zh?'发件人':'From',dataIndex:'from_address'},
   {title:zh?'接收时间':'Received',dataIndex:'received_at',render:(value:string)=>new Date(value).toLocaleString()},
   {title:zh?'操作':'Actions',render:(_,row)=><Space><Button disabled={busy||!writable} onClick={()=>void mutate(row,{read:!!row.read_at,archived:!row.archived,deleted:false})}>{row.archived?(zh?'移回收件箱':'Move to inbox'):(zh?'归档':'Archive')}</Button><Button disabled={busy||!writable} onClick={()=>void mutate(row,{read:!!row.read_at,archived:row.archived,deleted:!row.deleted})}>{row.deleted?(zh?'恢复':'Restore'):(zh?'移入垃圾箱':'Move to trash')}</Button></Space>}
  ]}/><Space><Button disabled={offset===0||busy} onClick={()=>setOffset(n=>Math.max(0,n-50))}>{zh?'上一页':'Previous'}</Button><Button disabled={rows.length<50||busy} onClick={()=>setOffset(n=>n+50)}>{zh?'下一页':'Next'}</Button></Space></>}
  {detail&&<Drawer open width="min(900px,100vw)" title={detail.message.subject} onClose={()=>{generation.current++;setDetail(undefined)}} extra={<a href={'/api/admin/notification-mail/inbox/'+detail.message.id+'/original'}>{zh?'下载原始邮件':'Download original email'}</a>}>
   <Typography.Paragraph>{detail.message.from_address}</Typography.Paragraph>
   <Tabs activeKey={view} onChange={setView} items={[{key:'text',label:zh?'纯文本':'Plain text'},{key:'html',label:'HTML'}]}/>
   {view==='text'?<pre style={{whiteSpace:'pre-wrap',overflowWrap:'anywhere',fontFamily:'inherit'}}>{detail.text}</pre>:<iframe title={zh?'邮件预览':'Email preview'} sandbox="" referrerPolicy="no-referrer" style={{width:'100%',height:500,border:0}} srcDoc={'<!doctype html><meta http-equiv="Content-Security-Policy" content="default-src \'none\'; style-src \'unsafe-inline\'; img-src data:; font-src \'none\'; form-action \'none\'; base-uri \'none\'"><meta name="viewport" content="width=device-width">'+detail.html}/>}
   {detail.attachments.length>0&&<><Typography.Title level={5}>{zh?'附件':'Attachments'}</Typography.Title>{detail.attachments.map(a=><Typography.Paragraph key={a.index}><a href={'/api/admin/notification-mail/inbox/'+detail.message.id+'/attachments/'+a.index}>{a.filename||'attachment'}</a></Typography.Paragraph>)}</>}
  </Drawer>}
 </Space></section>
}
