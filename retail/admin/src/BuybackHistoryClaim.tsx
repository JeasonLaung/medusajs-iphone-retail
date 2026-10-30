import {useRef,useState} from 'react'
import {Alert,Button,Form,Input,Modal} from 'antd'
import {claimBuyback,buyback} from './buyback-api'
import {APIError} from './api'
export default function BuybackHistoryClaim({id,zh,onSaved,onExpired,onBusy}:{id:string;zh:boolean;onSaved(value:Awaited<ReturnType<typeof buyback>>):void;onExpired():void;onBusy(value:boolean):void}){
 const text=(a:string,b:string)=>zh?a:b,[open,setOpen]=useState(false),[busy,setBusy]=useState(false),[error,setError]=useState(''),[form]=Form.useForm(),lock=useRef(false)
 const save=async()=>{
  if(lock.current)return
  let values:{customer_id:string;evidence_note:string};try{values=await form.validateFields()}catch{return}
  lock.current=true;setBusy(true);onBusy(true);setError('')
  try{await claimBuyback(id,values);const current=await buyback(id);onSaved(current);setOpen(false)}catch(e){if(e instanceof APIError&&e.status===401)onExpired();setError(text('绑定未确认，请刷新核对；原样重试不会重复绑定。','Binding was not confirmed. Refresh to check; retrying the same details is safe.'))}finally{lock.current=false;setBusy(false);onBusy(false)}
 }
 return <><Button onClick={()=>{form.resetFields();setError('');setOpen(true)}}>{text('核验并绑定新客户','Verify and link new customer')}</Button><Modal open={open} title={text('人工核验历史申请归属','Verify historical request ownership')} onOk={()=>void save()} onCancel={()=>{if(!busy)setOpen(false)}} confirmLoading={busy} closable={!busy} maskClosable={!busy} cancelButtonProps={{disabled:busy}} okText={text('确认已核验并绑定','Confirm verified ownership')}>
 <Alert type="warning" showIcon message={text('请先人工核验申请人的身份及其新账号。绑定会向该账号开放原联系方式、地址、照片和后续操作，不能仅凭同邮箱确认。','Verify the applicant’s identity and new account first. Linking grants access to original contact details, address, photos and actions. Matching email alone is insufficient.')}/>
 {error&&<Alert type="error" message={error}/>}<Form form={form} layout="vertical" disabled={busy}><Form.Item name="customer_id" label={text('已验证的新客户账号 ID','Verified new customer account ID')} rules={[{required:true,whitespace:true,max:80}]}><Input maxLength={80}/></Form.Item><Form.Item name="evidence_note" label={text('人工核验依据','Verification evidence')} rules={[{required:true,whitespace:true,min:10,max:1000}]}><Input.TextArea maxLength={1000}/></Form.Item></Form>
 </Modal></>
}
