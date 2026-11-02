import {useRef,useState} from 'react'
import {Alert,Button,Drawer,Form,Input,Space,Typography} from 'antd'
import {APIError} from './api'
import {saveDefinitionMetadata,type DefinitionMetadataInput,type OptionDefinition} from './option-definitions-api'

type Row={key:string;value:string;locked?:boolean}
type Fields={rows:Row[]}
export default function OptionDefinitionMetadata({definition,zh,onClose,onSaved,onExpired}:{definition:OptionDefinition;zh:boolean;onClose():void;onSaved():void;onExpired():void}){
 const original:Record<string,unknown>=JSON.parse(definition.metadata_json??'{}')
 const initial=Object.entries(original).map(([key,value]):Row=>{
  const editable=['string','number','boolean'].includes(typeof value)&&!(typeof value==='number'&&(!Number.isFinite(value)||Math.abs(value)>Number.MAX_SAFE_INTEGER))
  return {key,value:typeof value==='string'?value:JSON.stringify(value),locked:!editable}
 })
 const text=zh?{title:'编辑元数据',key:'键',value:'值',add:'添加元数据',remove:'删除行',save:'保存元数据',cancel:'取消',retry:'重试同一操作',uncertain:'保存结果尚未确认，请重试同一操作。',conflict:'定义已变化或无法访问，请关闭并重新加载。',invalid:'请检查键是否重复，以及数值是否有效。',readonly:'对象、数组和超出安全范围的数字只读。原始 JSON 如下。'}:{title:'Edit metadata',key:'Key',value:'Value',add:'Add metadata',remove:'Remove row',save:'Save metadata',cancel:'Cancel',retry:'Retry same operation',uncertain:'The result is not confirmed. Retry the same operation.',conflict:'The definition changed or access is unavailable. Close and reload.',invalid:'Check for duplicate keys and invalid numbers.',readonly:'Objects, arrays and numbers outside the safe range are read-only. Original JSON follows.'}
 const [form]=Form.useForm<Fields>(),[attempt,setAttempt]=useState<DefinitionMetadataInput>(),[saving,setSaving]=useState(false),[error,setError]=useState(''),busy=useRef(false)
 const locked=saving||!!attempt
 async function execute(input:DefinitionMetadataInput){
  if(busy.current)return
  busy.current=true;setSaving(true);setAttempt(input);setError('')
  try{await saveDefinitionMetadata(definition.id,input);onSaved()}
  catch(e){
   if(e instanceof APIError&&[400,401,403,404,409].includes(e.status)){setAttempt(undefined);setError(e.status===400?text.invalid:text.conflict);if(e.status===401)onExpired()}
   else setError(text.uncertain)
  }finally{busy.current=false;setSaving(false)}
 }
 function submit({rows}:Fields){
  try{
   const patch:Record<string,unknown>=Object.create(null),keys=new Set<string>()
   for(const row of rows){
    const key=row.locked?row.key:row.key.trim();if(!key)continue
    if(keys.has(key))throw Error('duplicate');keys.add(key)
    if(row.locked)continue
    // Unchanged numbers stay on the server as raw JSON, including decimals
    // whose exact source spelling cannot round-trip through JavaScript.
    if(typeof original[key]==='number'&&initial.some(old=>old.key===key&&old.value===row.value))continue
    const value=row.value?.trim()??''
    if(value==='true'||value==='false')patch[key]=value==='true'
    else if(/^-?\d*\.?\d+$/.test(value)){
     const number=Number(value);if(!Number.isFinite(number)||Math.abs(number)>Number.MAX_SAFE_INTEGER)throw Error('number');patch[key]=number
    }else patch[key]=value
   }
   for(const key of Object.keys(original))if(!keys.has(key))patch[key]=''
   void execute({operation_id:crypto.randomUUID(),expected_revision:definition.revision,metadata:keys.size?patch:null})
  }catch{setError(text.invalid)}
 }
 return <Drawer open title={text.title} width={640} maskClosable={!locked} closable={!locked} onClose={onClose}>
  <Space direction="vertical" size={16} className="full-width">
   {error&&<Alert type={attempt?'warning':'error'} showIcon message={error} action={attempt?<Button loading={saving} onClick={()=>void execute(attempt)}>{text.retry}</Button>:undefined}/>}
   <Form name="option-definition-metadata" form={form} initialValues={{rows:initial.length?initial:[{key:'',value:''}]}} layout="vertical" onFinish={submit} disabled={locked}>
    <Form.List name="rows">{(fields,{add,remove})=><>
     {fields.map(field=><Space key={field.key} align="baseline" wrap>
      <Form.Item name={[field.name,'key']} label={text.key}><Input disabled={locked||!!form.getFieldValue(['rows',field.name,'locked'])}/></Form.Item>
      <Form.Item name={[field.name,'value']} label={text.value}><Input disabled={locked||!!form.getFieldValue(['rows',field.name,'locked'])}/></Form.Item>
      <Button disabled={locked||!!form.getFieldValue(['rows',field.name,'locked'])} onClick={()=>remove(field.name)}>{text.remove}</Button>
     </Space>)}
     <Form.Item><Button disabled={locked} onClick={()=>add({key:'',value:''})}>{text.add}</Button></Form.Item>
    </>}</Form.List>
    <Space><Button type="primary" htmlType="submit" loading={saving} disabled={locked}>{text.save}</Button><Button disabled={locked} onClick={onClose}>{text.cancel}</Button></Space>
   </Form>
   {definition.metadata_json&&<><Typography.Paragraph type="secondary">{text.readonly}</Typography.Paragraph><pre style={{whiteSpace:'pre-wrap',overflowWrap:'anywhere'}}>{definition.metadata_json}</pre></>}
  </Space>
 </Drawer>
}
