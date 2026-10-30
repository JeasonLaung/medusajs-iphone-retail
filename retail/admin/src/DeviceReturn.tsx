import {useEffect,useRef,useState} from 'react'
import {Alert,Button,Drawer,Form,Input,Select,Space,Spin,Table,Typography} from 'antd'
import {APIError,readDeviceReturn,receiveDeviceReturn,type DeviceReturnContext,type DeviceReturnInput} from './api'

export default function DeviceReturn({order,requestID,onClose,onExpired}:{order:string;requestID:string;onClose():void;onExpired():void}){
 const zh=document.documentElement.lang.startsWith('zh'),[form]=Form.useForm<{assignment_id:string;source_id:string;scan:string}>(),[data,setData]=useState<DeviceReturnContext>(),[attempt,setAttempt]=useState(0),[error,setError]=useState(false),[pending,setPending]=useState<DeviceReturnInput>(),[saving,setSaving]=useState(false),[failed,setFailed]=useState(false),[saved,setSaved]=useState(false),busy=useRef(false)
 useEffect(()=>{const c=new AbortController();setError(false);setData(undefined);readDeviceReturn(order,requestID,c.signal).then(d=>{if(!c.signal.aborted)setData(d)}).catch(e=>{if(!c.signal.aborted){if(e instanceof APIError&&e.status===401)onExpired();else setError(true)}});return()=>c.abort()},[order,requestID,attempt,onExpired])
 const send=async(body:DeviceReturnInput)=>{if(busy.current)return;busy.current=true;setSaving(true);setFailed(false);setPending(body);try{await receiveDeviceReturn(order,requestID,body);setSaved(true);setPending(undefined);setAttempt(n=>n+1);form.resetFields()}catch(e){if(e instanceof APIError&&e.status===401)onExpired();else setFailed(true)}finally{busy.current=false;setSaving(false)}}
 const reload=()=>{setPending(undefined);setFailed(false);setSaved(false);setAttempt(n=>n+1)}
 const eligible=data?.devices.filter(d=>d.status==='shipped'&&d.state==='shipped')??[]
 return <Drawer open width={720} title={zh?'收回售后设备':'Receive after-sales device'} onClose={()=>{if(!saving)onClose()}} closable={!saving} maskClosable={!saving} keyboard={!saving}>
 <Space direction="vertical" size={16} className="full-width">
 <Alert type="info" message={zh?'核对实际收到的机器，扫描原机序列号、IMEI 或 EID。收回后进入检测或维修，不直接增加可售数量库存。':'Verify the actual device using its serial number, IMEI or EID. Receipt quarantines it for inspection or repair without restocking.'}/>
 {error?<><Alert type="error" message={zh?'售后设备暂不可用':'Device records unavailable'}/><Button onClick={reload}>{zh?'重新读取':'Reload'}</Button></>:!data?<Spin/>:<>
 {saved&&<Alert type="success" message={zh?'设备已收回':'Device received'}/>}
 {(!data.enabled||data.status!=='processing')&&<Alert type="warning" message={zh?'需先将申请标记处理中，并启用收回操作':'The request must be processing and receipt operations enabled'}/>}
 <Table rowKey="assignment_id" dataSource={data.devices} pagination={false} scroll={{x:480}} columns={[{title:zh?'序列号':'Serial number',dataIndex:'serial_number'},{title:zh?'分配状态':'Assignment status',dataIndex:'status'},{title:zh?'设备状态':'Device state',dataIndex:'state'}]}/>
 {failed&&<Alert type="error" message={zh?'结果未确认或处理条件已变化，请重试原操作或重新读取核对。':'Result unconfirmed or prerequisites changed. Retry the same operation or reload to verify.'}/>}
 <Form form={form} layout="vertical" disabled={saving||!!pending||!data.enabled||data.status!=='processing'} onFinish={v=>void send({operation_id:crypto.randomUUID(),expected_revision:data.revision,assignment_id:v.assignment_id,source_id:v.source_id,scan:v.scan.trim()})}>
 <Form.Item name="assignment_id" label={zh?'原发货设备':'Originally shipped device'} rules={[{required:true}]}><Select options={eligible.map(d=>({value:d.assignment_id,label:d.serial_number}))}/></Form.Item>
 <Form.Item name="source_id" label={zh?'收货来源 / 仓库':'Receiving source / warehouse'} rules={[{required:true}]}><Select options={data.sources.map(s=>({value:s.id,label:s.name+' / '+(s.location_name??s.location_id)}))}/></Form.Item>
 <Form.Item name="scan" label={zh?'扫描序列号 / IMEI / EID':'Scan serial number / IMEI / EID'} rules={[{required:true,whitespace:true},{max:191}]}><Input autoComplete="off" maxLength={191}/></Form.Item>
 {!pending&&<Button type="primary" htmlType="submit" disabled={!eligible.length||saved} loading={saving}>{zh?'确认收回设备':'Confirm device receipt'}</Button>}
 </Form>
 {pending&&<Button type="primary" loading={saving} onClick={()=>void send(pending)}>{zh?'重试本次收回':'Retry this receipt'}</Button>}
 <Button disabled={saving} onClick={reload}>{zh?'重新读取':'Reload'}</Button>
 <Typography.Text type="secondary">{zh?'退款、检测和返还发货需分别处理。':'Refund, inspection and return dispatch are separate operations.'}</Typography.Text>
 </>}
 </Space></Drawer>
}
