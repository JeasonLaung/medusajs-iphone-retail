import {useEffect,useRef,useState} from 'react'
import {Alert,App,Button,Checkbox,Drawer,Empty,Form,Input,Space,Spin,Table,Tag,Typography} from 'antd'
import {APIError,mailboxes,saveMailbox} from './api'
import type {Mailbox,MailboxCommand,MailboxDirectory} from './api'
import type {Copy} from './i18n'
import OrderMailJobs from './OrderMailJobs'
import {useAccess} from './Permissions'

type Fields=Pick<MailboxCommand,'name'|'email'|'reply_to'|'api_key'|'webhook_secret'|'enabled'|'registration'>
function Editor({copy,account,revision,onClose,onSaved,onExpired}:{copy:Copy;account?:Mailbox;revision:string;onClose():void;onSaved():void;onExpired():void}){
 const [form]=Form.useForm<Fields>(),[saving,setSaving]=useState(false),busy=useRef(false)
 const [attempt,setAttempt]=useState<MailboxCommand>(),[error,setError]=useState<'uncertain'|'conflict'|'forbidden'|'invalid'>()
 const enabled=Form.useWatch('enabled',form)
 const submit=async(values:Fields)=>{
  if(busy.current)return
  const body=attempt??{...values,id:account?.id??'',operation_id:crypto.randomUUID(),expected_revision:revision}
  busy.current=true;setSaving(true);setAttempt(body);setError(undefined)
  try{await saveMailbox(body);onSaved()}
  catch(e){
   if(e instanceof APIError&&e.status===401)onExpired()
   else if(e instanceof APIError&&e.status===409)setError('conflict')
   else if(e instanceof APIError&&e.status===403)setError('forbidden')
   else if(e instanceof APIError&&[400,413,415].includes(e.status)){setAttempt(undefined);setError('invalid')}
   else setError('uncertain')
  }finally{busy.current=false;setSaving(false)}
 }
 const close=()=>{if(!busy.current){form.resetFields();setAttempt(undefined);onClose()}}
 return <Drawer open title={account?copy.editMailbox:copy.newMailbox} width="min(560px, 100vw)" destroyOnHidden onClose={close} maskClosable={!saving} keyboard={!saving} closable={!saving} extra={<Space><Button onClick={close} disabled={saving}>{copy.cancel}</Button><Button aria-label={error==='uncertain'?copy.retrySave:copy.save} type="primary" loading={saving} disabled={error==='conflict'||error==='forbidden'} onClick={()=>form.submit()}>{error==='uncertain'?copy.retrySave:copy.save}</Button></Space>}>
  <Space direction="vertical" size={24} className="full-width">
   {error&&<Alert showIcon type={error==='uncertain'?'warning':'error'} message={error==='uncertain'?copy.mailUncertain:error==='conflict'?copy.mailConflict:error==='forbidden'?copy.editForbidden:copy.editInvalid}/>}
   <Form form={form} layout="vertical" onFinish={submit} disabled={saving||!!attempt} initialValues={{name:account?.name??'',email:account?.email??'',reply_to:account?.reply_to??'',api_key:'',webhook_secret:'',enabled:account?.enabled??true,registration:account?.registration??false}}>
    <Form.Item name="name" label={copy.senderName} rules={[{required:true,whitespace:true,message:copy.required},{max:160,message:copy.editTooLong}]}><Input maxLength={160}/></Form.Item>
    <Form.Item name="email" label={copy.senderEmail} rules={[{required:true,message:copy.required},{type:'email',message:copy.emailInvalid}]}><Input type="email" maxLength={254}/></Form.Item>
    <Form.Item name="reply_to" label={copy.replyTo} rules={[{type:'email',message:copy.emailInvalid}]}><Input type="email" maxLength={254}/></Form.Item>
    <Form.Item name="api_key" label={copy.apiKey} extra={account?.has_api_key?copy.mailKeyKeep:copy.mailSecretNew} rules={[{required:!account?.has_api_key,message:copy.required}]}><Input.Password autoComplete="new-password" maxLength={512}/></Form.Item>
    <Form.Item name="webhook_secret" label={copy.webhookSecret} extra={account?.has_webhook_secret?copy.mailKeyKeep:undefined}><Input.Password autoComplete="new-password" maxLength={512}/></Form.Item>
    <Form.Item name="enabled" valuePropName="checked"><Checkbox onChange={event=>{if(!event.target.checked)form.setFieldValue('registration',false)}}>{copy.mailEnabled}</Checkbox></Form.Item>
    <Form.Item name="registration" valuePropName="checked"><Checkbox disabled={!enabled||saving||!!attempt}>{copy.registrationSender}</Checkbox></Form.Item>
   </Form>
  </Space>
 </Drawer>
}
export default function Mailboxes({copy,onExpired}:{copy:Copy;onExpired():void}){
 const {message}=App.useApp(),access=useAccess(),[attempt,setAttempt]=useState(0)
 const [state,setState]=useState<{data?:MailboxDirectory;error?:unknown}>({}),[editor,setEditor]=useState<{account?:Mailbox}>()
 useEffect(()=>{
  const controller=new AbortController();setState({})
  mailboxes(controller.signal).then(data=>{if(!controller.signal.aborted)setState({data})}).catch(error=>{if(!controller.signal.aborted){setState({error});if(error instanceof APIError&&error.status===401)onExpired()}})
  return()=>controller.abort()
 },[attempt,onExpired])
 const reload=()=>setAttempt(n=>n+1)
 const close=()=>{setEditor(undefined);reload()}
 return <section className="account-panel"><Space direction="vertical" size={24} className="full-width">
  <Typography.Paragraph type="secondary">{copy.mailboxesInfo}</Typography.Paragraph>
  <Space wrap><Button onClick={reload} disabled={!!editor}>{copy.reload}</Button>{access.can('mail.write')&&<Button type="primary" disabled={!state.data||!!editor} onClick={()=>setEditor({})}>{copy.newMailbox}</Button>}</Space>
  {state.error!==undefined?<Alert showIcon type="error" message={state.error instanceof APIError&&state.error.status===403?copy.accessDenied:copy.unavailable} action={<Button onClick={reload}>{copy.retry}</Button>}/>:!state.data?<Spin/>:<>
   {!state.data.registration_account_id&&<Alert showIcon type="warning" message={copy.mailNoSender}/>}
   <Table<Mailbox> rowKey="id" dataSource={state.data.accounts} pagination={false} scroll={{x:760}} locale={{emptyText:<Empty description={copy.mailNoSender}/>}} columns={[
    {title:copy.senderName,dataIndex:'name'},
    {title:copy.senderEmail,dataIndex:'email'},
    {title:copy.replyTo,dataIndex:'reply_to',render:(v:string)=>v||'—'},
    {title:copy.status,render:(_,a)=><Space direction="vertical"><Tag color={a.enabled?'success':'default'}>{a.enabled?copy.mailOn:copy.mailOff}</Tag>{a.registration&&<Tag color="processing">{copy.registrationSender}</Tag>}</Space>},
    {title:copy.apiKey,render:(_,a)=>a.has_api_key?copy.configured:copy.unconfigured},
    {title:copy.webhookSecret,render:(_,a)=>a.has_webhook_secret?copy.configured:copy.unconfigured},
    ...(access.can('mail.write')?[{title:copy.editMailbox,render:(_:unknown,a:Mailbox)=><Button onClick={()=>setEditor({account:a})}>{copy.editMailbox}</Button>}]:[]),
   ]}/>
  </>}
  <OrderMailJobs copy={copy} onExpired={onExpired}/>
  {editor&&state.data&&<Editor key={editor.account?.id??'new'} copy={copy} account={editor.account} revision={state.data.revision} onClose={close} onExpired={onExpired} onSaved={()=>{message.success(copy.mailSaved);close()}}/>}
 </Space></section>
}
