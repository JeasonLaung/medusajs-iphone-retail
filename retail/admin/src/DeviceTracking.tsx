import {useEffect,useState} from 'react'
import {Alert,Button,Descriptions,Drawer,Space,Spin,Tag} from 'antd'
import {APIError,deviceTracking,enrollDeviceTracking} from './api'
import type {DeviceTrackingState,DeviceEnrollInput,VariantDetail} from './api'
import type {Copy} from './i18n'
import {useAccess} from './Permissions'
import DeviceIntake from './DeviceIntake'

export default function DeviceTracking({productID,variant,copy,onExpired,onClose}:{productID:string;variant:VariantDetail;copy:Copy;onExpired():void;onClose():void}){
 const access=useAccess(),[attempt,setAttempt]=useState(0)
 const [state,setState]=useState<{loading:boolean;data?:DeviceTrackingState;error?:unknown}>({loading:true})
 const [intakeOpen,setIntakeOpen]=useState(false)
 const [pending,setPending]=useState<DeviceEnrollInput>(),[saving,setSaving]=useState(false),[error,setError]=useState<unknown>(),[conflict,setConflict]=useState(false)
 useEffect(()=>{
  const controller=new AbortController();setState({loading:true})
  deviceTracking(productID,variant.id,controller.signal).then(data=>{if(!controller.signal.aborted)setState({loading:false,data})}).catch(error=>{if(controller.signal.aborted)return;if(error instanceof APIError&&error.status===401)onExpired();else setState({loading:false,error})})
  return()=>controller.abort()
 },[productID,variant.id,attempt,onExpired])
 const reload=()=>{setPending(undefined);setError(undefined);setConflict(false);setAttempt(n=>n+1)}
 const save=async()=>{
  if(!state.data||saving||conflict)return
  const command=pending??{operation_id:crypto.randomUUID(),expected_revision:state.data.revision}
  setPending(command);setSaving(true);setError(undefined)
  try{await enrollDeviceTracking(productID,variant.id,command);setPending(undefined);setAttempt(n=>n+1)}
  catch(error){if(error instanceof APIError&&error.status===401)onExpired();else{setError(error);if(error instanceof APIError&&[400,403,404,409].includes(error.status)){setConflict(true);setPending(undefined)}}}
  finally{setSaving(false)}
 }
 const d=state.data
 return <Drawer open title={`${copy.deviceTracking} · ${variant.sku??variant.title}`} width={560} onClose={()=>{if(!saving)onClose()}} closable={!saving} maskClosable={!saving} keyboard={!saving}>
  <Space direction="vertical" size={24} className="full-width">
   <Alert type="info" showIcon message={copy.deviceTrackingInfo}/>
   {state.loading?<Spin/>:state.error!==undefined?<><Alert type="error" showIcon message={copy.unavailable}/><Button onClick={reload}>{copy.retry}</Button></>:d&&<>
    <Descriptions column={1} items={[{key:'sku',label:'SKU',children:variant.sku??'—'},{key:'item',label:copy.inventoryItemID,children:d.inventory_item_id||'—'},{key:'status',label:copy.status,children:<Tag color={d.enabled?'success':undefined}>{d.enabled?copy.deviceTrackingEnabled:copy.deviceTrackingDisabled}</Tag>}]}/>
    {!d.enabled&&!d.eligible&&<Alert type="warning" showIcon message={d.reason==='shared_inventory_item'?copy.deviceTrackingShared:copy.deviceTrackingInvalid}/>}
    {error!==undefined&&<Alert type="error" showIcon message={conflict?copy.deviceTrackingConflict:copy.deviceTrackingRetry}/>}
    <Space wrap>{(d.enabled||d.eligible)&&access.can('device.intake','warehouse.read')&&<Button type="primary" onClick={()=>setIntakeOpen(true)}>{copy.deviceIntake}</Button>}{access.can('device.enroll')&&!d.enabled&&d.eligible&&!conflict&&<Button type="primary" loading={saving} onClick={save}>{pending?copy.deviceTrackingRetryAction:copy.deviceTrackingEnable}</Button>}<Button disabled={saving||!!pending} onClick={reload}>{copy.reload}</Button></Space>
   </>}
  </Space>
 {intakeOpen&&<DeviceIntake productID={productID} variantID={variant.id} copy={copy} onExpired={onExpired} onClose={()=>{setIntakeOpen(false);setAttempt(n=>n+1)}}/>}
 </Drawer>
}
