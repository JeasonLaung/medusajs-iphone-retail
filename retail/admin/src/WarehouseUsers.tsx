import {useEffect,useRef,useState} from 'react'
import {Alert,App,Button,Checkbox,Drawer,Form,Input,Radio,Result,Space,Spin,Switch,Table,Tag,Transfer,Typography} from 'antd'
import {APIError,warehouseUsers,saveWarehouseUser,WAREHOUSE_USER_ACTIONS} from './api'
import type {WarehouseUser,WarehouseUserCommand,WarehouseUserDirectory} from './api'
import type {Copy} from './i18n'
import {useAccess} from './Permissions'

type Fields=Omit<WarehouseUser,'id'>&{password:string}
export default function WarehouseUsers({copy,onExpired}:{copy:Copy;onExpired():void}){
 const access=useAccess(),{message}=App.useApp(),[form]=Form.useForm<Fields>()
 const [attempt,setAttempt]=useState(0),[data,setData]=useState<WarehouseUserDirectory>(),[loading,setLoading]=useState(true),[loadError,setLoadError]=useState<unknown>()
 const [editor,setEditor]=useState<{user?:WarehouseUser}>(),[command,setCommand]=useState<WarehouseUserCommand>(),[error,setError]=useState<unknown>(),[pending,setPending]=useState(false),busy=useRef(false)
 const mode=Form.useWatch('mode',form),selected:string[]=Form.useWatch('locations',form)??[],read=access.can('authorization.read','warehouse.read'),write=access.can('authorization.read','authorization.write','warehouse.read','warehouse.write')
 useEffect(()=>{
  if(!read)return
  const abort=new AbortController();setLoading(true);setLoadError(undefined)
  warehouseUsers(abort.signal).then(value=>{if(!abort.signal.aborted)setData(value)}).catch(error=>{if(abort.signal.aborted)return;if(error instanceof APIError&&error.status===401)onExpired();else{setData(undefined);setLoadError(error)}}).finally(()=>{if(!abort.signal.aborted)setLoading(false)})
  return()=>abort.abort()
 },[attempt,onExpired,read])
 const clear=()=>{form.resetFields();setCommand(undefined);setError(undefined);setEditor(undefined)}
 const close=()=>{if(busy.current)return;clear();setAttempt(n=>n+1)}
 const open=(user?:WarehouseUser)=>{form.resetFields();form.setFieldsValue(user?{...user,locations:[...user.locations],actions:[...user.actions],password:''}:{name:'',email:'',password:'',enabled:true,mode:'selected',locations:[],actions:[]});setCommand(undefined);setError(undefined);setEditor({user})}
 const send=async(input:WarehouseUserCommand)=>{
  if(busy.current)return;busy.current=true;setPending(true);setError(undefined);setCommand(input)
  try{await saveWarehouseUser(input);clear();setAttempt(n=>n+1);void message.success(copy.warehouseUserSaved)}
  catch(error){if(error instanceof APIError&&error.status===401)onExpired();else setError(error)}
  finally{busy.current=false;setPending(false)}
 }
 const submit=(fields:Fields)=>{if(command||!editor||!data||!write)return;void send({...fields,password:fields.password??'',actions:fields.actions??[],locations:fields.mode==='all'?[]:fields.locations??[],operation_id:crypto.randomUUID(),expected_revision:data.revision,id:editor.user?.id??''})}
 const e=error instanceof APIError?error:undefined,uncertain=error!==undefined&&(!e||e.status===0||e.status>=500)
 if(!read)return <Result status="403" title={copy.accessDenied}/>
 if(loading)return <Spin/>
 if(!data)return <Space direction="vertical">{loadError instanceof APIError&&loadError.status===403?<Result status="403" title={copy.accessDenied}/>:<Alert type="error" showIcon message={copy.unavailable}/>}<Button onClick={()=>setAttempt(n=>n+1)}>{copy.retry}</Button></Space>
 const names=new Map(data.locations.map(l=>[l.id,l.name]))
 const locationOptions=[...data.locations.map(l=>({key:l.id,title:`${l.name} (${l.id})`,disabled:false})),...(editor?.user?.locations??[]).filter(id=>!names.has(id)).map(id=>({key:id,title:`${id} (${copy.locationUnavailable})`,disabled:!selected.includes(id)}))]
 return <section className="account-panel"><Space direction="vertical" size={24} className="full-width">
  <Typography.Paragraph type="secondary">{copy.warehouseUsersInfo}</Typography.Paragraph>
  <Space wrap><Button onClick={()=>setAttempt(n=>n+1)}>{copy.reload}</Button>{write&&<Button type="primary" onClick={()=>open()}>{copy.newWarehouseUser}</Button>}</Space>
  <Table<WarehouseUser> rowKey="id" dataSource={data.users} pagination={{pageSize:20,showSizeChanger:false}} scroll={{x:800}} locale={{emptyText:copy.noWarehouseUsers}} columns={[
   {title:copy.name,dataIndex:'name',render:(name:string,row)=>write?<Button type="link" onClick={()=>open(row)}>{name}</Button>:name},
   {title:copy.email,dataIndex:'email'},
   {title:copy.permissionEnabled,render:(_,row)=><Tag color={row.enabled?'success':undefined}>{row.enabled?copy.permissionEnabled:copy.permissionDisabled}</Tag>},
   {title:copy.warehouseUserScope,render:(_,row)=>row.mode==='all'?copy.allWarehouses:row.locations.length?row.locations.map(id=>names.get(id)??id).join(', '):copy.noWarehouseScope},
   {title:copy.permissionActions,render:(_,row)=>row.actions.map(a=>copy.actionNames[a]).join(', ')||'—'},
  ]}/>
 </Space><Drawer open={!!editor} onClose={close} title={editor?.user?copy.editWarehouseUser:copy.newWarehouseUser} width={520}>
  {error!==undefined&&<Alert type="error" showIcon message={e?.status===409?copy.warehouseUserConflict:uncertain?copy.saveUncertain:e?.status===400?copy.permissionInvalid:copy.permissionForbidden}/>}
  <Form form={form} layout="vertical" disabled={pending||!!command} onFinish={submit} requiredMark={false}>
   <Form.Item name="name" label={copy.name} rules={[{required:true,whitespace:true,message:copy.required},{max:160,message:copy.editTooLong}]}><Input maxLength={160}/></Form.Item>
   <Form.Item name="email" label={copy.email} rules={[{required:true,message:copy.required},{type:'email',message:copy.emailInvalid}]}><Input type="email" maxLength={254} disabled={!!editor?.user||pending||!!command} autoComplete="off"/></Form.Item>
   <Form.Item name="password" label={copy.password} extra={editor?.user?copy.warehousePasswordKeep:copy.warehousePasswordHelp} rules={[{validator:(_,value:string)=>{const size=new TextEncoder().encode(value??'').length;return (!size&&!!editor?.user||size>=12&&size<=72)?Promise.resolve():Promise.reject(new Error(copy.warehousePasswordHelp))}}]}><Input.Password autoComplete="new-password" maxLength={72}/></Form.Item>
   <Form.Item name="enabled" label={copy.permissionEnabled} valuePropName="checked"><Switch/></Form.Item>
   <Form.Item name="mode" label={copy.warehouseUserScope}><Radio.Group options={[{value:'selected',label:copy.selectedWarehouses},{value:'all',label:copy.allWarehouses}]}/></Form.Item>
   {mode==='selected'&&<Form.Item name="locations" valuePropName="targetKeys" label={copy.warehouses} extra={copy.noWarehouseScope}><Transfer dataSource={locationOptions} render={item=>item.title} showSearch titles={[copy.warehouses,copy.selectedWarehouses]} listStyle={{width:'calc((100% - 48px) / 2)',height:220,minWidth:0}} style={{width:'100%'}}/></Form.Item>}
   <Form.Item name="actions" label={copy.permissionActions}><Checkbox.Group options={WAREHOUSE_USER_ACTIONS.map(value=>({value,label:copy.actionNames[value],disabled:!access.can(value)}))}/></Form.Item>
   {!command&&<Button type="primary" htmlType="submit" loading={pending}>{copy.save}</Button>}
  </Form><Space className="mt-4"><Button onClick={close} disabled={pending}>{copy.cancel}</Button>{command&&uncertain&&<Button type="primary" disabled={pending} loading={pending} onClick={()=>void send(command)}>{copy.retrySave}</Button>}</Space>
 </Drawer></section>
}
