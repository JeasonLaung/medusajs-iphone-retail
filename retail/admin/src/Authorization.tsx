import {useEffect,useRef,useState} from 'react'
import {Alert,App,Button,Checkbox,Drawer,Form,Input,Result,Select,Space,Spin,Switch,Table,Tabs,Tag,Typography} from 'antd'
import {APIError,getAuthorization,saveAuthorization} from './api'
import type {Action,AuthorizationCommand,AuthorizationDirectory,ManagedMember,ManagedRole} from './api'
import type {Copy} from './i18n'
import {useAccess} from './Permissions'

export default function Authorization({copy,onExpired}:{copy:Copy;onExpired():void}){
 const access=useAccess(),{message}=App.useApp()
 const [attempt,setAttempt]=useState(0)
 const [state,setState]=useState<{loading:boolean;data?:AuthorizationDirectory;error?:unknown}>({loading:true})
 const [editor,setEditor]=useState<{kind:'role';role:ManagedRole;fresh:boolean}|{kind:'member';member:ManagedMember}>()
 const [form]=Form.useForm<{id:string;name:string;enabled:boolean;actions:Action[];roles:string[]}>()
 const [pending,setPending]=useState(false),busy=useRef(false)
 const [command,setCommand]=useState<AuthorizationCommand>()
 const [error,setError]=useState<unknown>()
 useEffect(()=>{
  const controller=new AbortController();setState({loading:true})
  getAuthorization(controller.signal).then(data=>{if(!controller.signal.aborted)setState({loading:false,data})}).catch(error=>{
   if(controller.signal.aborted)return
   if(error instanceof APIError&&error.status===401)onExpired();else setState({loading:false,error})
  })
  return()=>controller.abort()
 },[attempt,onExpired])
 const close=()=>{if(busy.current)return;setEditor(undefined);setCommand(undefined);setError(undefined);setAttempt(n=>n+1)}
 const openRole=(role?:ManagedRole)=>{
  const value=role??{id:`role_${crypto.randomUUID()}`,name:'',enabled:true,actions:[]}
  form.setFieldsValue({...value,actions:[...value.actions]});setError(undefined);setCommand(undefined);setEditor({kind:'role',role:value,fresh:!role})
 }
 const openMember=(member:ManagedMember)=>{form.setFieldsValue({roles:[...member.roles]});setError(undefined);setCommand(undefined);setEditor({kind:'member',member})}
 const send=async(input:AuthorizationCommand)=>{
  if(busy.current)return;busy.current=true;setPending(true);setCommand(input);setError(undefined)
  try{await saveAuthorization(input);setEditor(undefined);setCommand(undefined);setAttempt(n=>n+1);void message.success(copy.permissionSaved);access.retry()}
  catch(error){if(error instanceof APIError&&error.status===401)onExpired();else setError(error)}
  finally{busy.current=false;setPending(false)}
 }
 const submit=(values:{id:string;name:string;enabled:boolean;actions:Action[];roles:string[]})=>{
  if(command||!editor||!state.data||!access.can('authorization.write'))return
  const base={operation_id:crypto.randomUUID(),expected_revision:state.data.revision}
  void send(editor.kind==='role'?{...base,kind:'role.save',role:{id:editor.role.id,name:values.name,enabled:values.enabled,actions:values.actions??[]},user_id:null,roles:null}:{...base,kind:'member.roles',role:null,user_id:editor.member.user_id,roles:values.roles??[]})
 }
 const e=error instanceof APIError?error:undefined
 const uncertain=!!error&&(!e||e.status===0||e.status>=500)
 const errorText=e?.code==='authorization.last_manager'?copy.lastPermissionManager:e?.status===409?copy.permissionConflict:e?.status===403?copy.permissionForbidden:e?.status===404?copy.permissionMissing:e?.status===400?copy.permissionInvalid:copy.permissionUncertain
 if(state.loading)return <Spin/>
 if(!state.data)return <Space direction="vertical">{state.error instanceof APIError&&state.error.status===403?<Result status="403" title={copy.accessDenied}/>:<Alert showIcon type="error" message={copy.unavailable}/>}<Button onClick={()=>{setAttempt(n=>n+1);access.retry()}}>{copy.retry}</Button></Space>
 const d=state.data,write=access.can('authorization.write'),names=new Map(d.roles.map(r=>[r.id,r.name]))
 const status=(enabled:boolean)=><Tag color={enabled?'success':undefined}>{enabled?copy.permissionEnabled:copy.permissionDisabled}</Tag>
 return <section className="account-panel"><Space direction="vertical" size={24} className="full-width">
  <Typography.Paragraph type="secondary">{copy.permissionInfo}</Typography.Paragraph>
  <Space wrap><Typography.Text type="secondary">{copy.permissionRevision}: {d.revision}</Typography.Text><Button onClick={()=>setAttempt(n=>n+1)}>{copy.reload}</Button>{write&&<Button type="primary" onClick={()=>openRole()}>{copy.newPermissionRole}</Button>}</Space>
  <Tabs items={[
   {key:'roles',label:copy.permissionRoles,children:<Table<ManagedRole> rowKey="id" dataSource={d.roles} pagination={{pageSize:20,showSizeChanger:false}} scroll={{x:700}} columns={[
    {title:copy.name,dataIndex:'name',render:(name:string,r)=>write?<Button type="link" onClick={()=>openRole(r)}>{name}</Button>:name},
    {title:copy.permissionRoleID,dataIndex:'id'},
    {title:copy.status,dataIndex:'enabled',render:status},
    {title:copy.permissionActions,render:(_,r)=><Space wrap>{r.actions.map(a=><Tag key={a}>{copy.actionNames[a]}</Tag>)}</Space>},
   ]}/>},
   {key:'members',label:copy.permissionMembers,children:<Table<ManagedMember> rowKey="user_id" dataSource={d.members} pagination={{pageSize:20,showSizeChanger:false}} scroll={{x:700}} columns={[
    {title:copy.name,render:(_,m)=><Space direction="vertical"><Typography.Text>{m.name||'—'}</Typography.Text><Typography.Text type="secondary">{m.user_id}</Typography.Text></Space>},
    {title:copy.permissionSubject,dataIndex:'kind',render:(kind:ManagedMember['kind'])=>copy.subjectNames[kind]},
    {title:copy.status,dataIndex:'enabled',render:status},
    {title:copy.permissionRoles,render:(_,m)=><Space wrap>{m.roles.map(id=><Tag key={id}>{names.get(id)}</Tag>)}</Space>},
    {title:copy.permissionAssignment,render:(_,m)=>write&&m.kind==='employee'?<Button onClick={()=>openMember(m)}>{copy.editPermissionMembership}</Button>:'—'},
   ]}/>},
  ]}/>
 </Space>
 <Drawer title={editor?.kind==='member'?copy.editPermissionMembership:editor?.fresh?copy.newPermissionRole:copy.editPermissionRole} open={!!editor} onClose={close} maskClosable={!pending} keyboard={!pending} closable={!pending} width={560} destroyOnHidden footer={<Space><Button onClick={close} aria-label={copy.cancel} disabled={pending}>{copy.cancel}</Button><Button type="primary" htmlType="submit" form="authorization-editor" aria-label={copy.save} loading={pending} disabled={pending||!!command||!write}>{copy.save}</Button></Space>}>
  {editor&&<Form id="authorization-editor" form={form} layout="vertical" onFinish={submit} disabled={pending||!!command||!write}>
   {editor.kind==='role'?<>
    <Typography.Paragraph type="secondary">{copy.permissionRoleID}: {editor.role.id}</Typography.Paragraph>
    <Form.Item name="name" label={copy.name} rules={[{required:true,whitespace:true,message:copy.required},{max:160,message:copy.editTooLong}]}><Input maxLength={160}/></Form.Item>
    <Form.Item name="enabled" label={copy.permissionEnabled} valuePropName="checked"><Switch/></Form.Item>
    <Form.Item name="actions" label={copy.permissionActions}><Checkbox.Group options={d.actions.map(a=>({label:copy.actionNames[a],value:a,disabled:!access.can(a)}))}/></Form.Item>
   </>:<>
    <Typography.Paragraph>{editor.member.name||editor.member.user_id}</Typography.Paragraph>
    <Form.Item name="roles" label={copy.permissionRoles}><Select mode="multiple" virtual={false} optionFilterProp="label" options={d.roles.map(r=>({value:r.id,label:r.name,disabled:!r.enabled||!r.actions.every(a=>access.can(a))}))}/></Form.Item>
   </>}
  </Form>}
  {error!==undefined&&<Space direction="vertical" className="full-width"><Alert showIcon type="error" message={errorText}/>{uncertain&&command&&write&&<Button onClick={()=>void send(command)} loading={pending} disabled={pending}>{copy.retrySave}</Button>}</Space>}
 </Drawer>
 </section>
}
