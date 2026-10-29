import {useState} from 'react'
import {Alert,Button,Space,Upload} from 'antd'
import {APIError,uploadDevicePhoto} from './api'
import type {DevicePhoto,WarehouseIntakeScope} from './api'
import type {Copy} from './i18n'

// Match the original intake camera: keep aspect ratio, at most 1800px, JPEG 85%.
async function photoContent(file:File){
 const url=URL.createObjectURL(file)
 try{
  const image=new Image();image.src=url;await image.decode()
  const scale=Math.min(1,1800/Math.max(image.width,image.height)),canvas=document.createElement('canvas')
  canvas.width=Math.round(image.width*scale);canvas.height=Math.round(image.height*scale)
  const ctx=canvas.getContext('2d');if(!ctx)throw Error('canvas')
  ctx.fillStyle='#fff';ctx.fillRect(0,0,canvas.width,canvas.height);ctx.drawImage(image,0,0,canvas.width,canvas.height)
  const content=canvas.toDataURL('image/jpeg',0.85).split(',')[1]
  if(!content||content.length>4194304)throw Error('size')
  return {filename:file.name,mime_type:'image/jpeg',content}
 }finally{URL.revokeObjectURL(url)}
}
export default function DevicePhotos({value=[],onChange,disabled,copy,onBusy,onExpired,scope}:{value?:DevicePhoto[];onChange?(photos:DevicePhoto[]):void;disabled?:boolean;copy:Copy;onBusy(busy:boolean):void;onExpired():void;scope?:WarehouseIntakeScope}){
 const [uploading,setUploading]=useState(false),[error,setError]=useState(false)
 const [pending,setPending]=useState<Parameters<typeof uploadDevicePhoto>[0]>()
 const send=async(body:Parameters<typeof uploadDevicePhoto>[0])=>{
  setUploading(true);onBusy(true);setError(false);setPending(body)
  try{const photo=await uploadDevicePhoto(body,scope);onChange?.([...value,photo]);setPending(undefined);onBusy(false)}catch(error){if(error instanceof APIError&&error.status===401){setPending(undefined);onBusy(false);onExpired()}else setError(true)}finally{setUploading(false)}
 }
 const select=async(file:File)=>{
  if(disabled||uploading||pending||value.length>=8)return false
  if(!['image/jpeg','image/png','image/webp'].includes(file.type)){setError(true);return false}
  setUploading(true);onBusy(true);setError(false)
  try{const body={operation_id:crypto.randomUUID(),...await photoContent(file)};await send(body)}catch{setError(true);onBusy(false)}finally{setUploading(false)}
  return false
 }
 const locked=disabled||uploading||!!pending
 return <Space direction="vertical" className="full-width">
  <Upload accept="image/jpeg,image/png,image/webp" disabled={locked} beforeUpload={select} onRemove={file=>{onChange?.(value.filter(p=>p.id!==file.uid));return true}} fileList={value.map(p=>({uid:p.id,name:copy.devicePhotos,status:'done',url:p.url}))} listType="picture" openFileDialogOnClick={!locked&&value.length<8}>
   <Button disabled={locked||value.length>=8} loading={uploading}>{copy.devicePhotoAdd}</Button>
  </Upload>
  <Upload accept="image/jpeg,image/png,image/webp" capture="environment" showUploadList={false} disabled={locked||value.length>=8} beforeUpload={select}><Button disabled={locked||value.length>=8}>{copy.devicePhotoCamera}</Button></Upload>
  {error&&<Alert type="error" showIcon message={copy.devicePhotoError}/>}
  {pending&&!uploading&&<Space><Button disabled={disabled} onClick={()=>void send(pending)}>{copy.retrySave}</Button><Button disabled={disabled} onClick={()=>{setPending(undefined);setError(false);onBusy(false)}}>{copy.cancel}</Button></Space>}
 </Space>
}
