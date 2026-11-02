import {useEffect,useRef,useState} from 'react'
import {Alert,Button,Form,Input,InputNumber,Space,Spin} from 'antd'
import {APIError,csrfCookie,request} from './api'
import {useAccess} from './Permissions'
import type {Copy} from './i18n'

type Policies={warranty_months:number;return_days:number;warranty_note:{en:string;zh:string};return_note:{en:string;zh:string}}
type Context={policies:Policies;revision:string;enabled:boolean}
type Command={operation_id:string;expected_revision:string;policies:Policies}
const revision=(v:unknown)=>typeof v==='string'&&/^[a-f0-9]{64}$/.test(v)
function context(v:Context){
 const p=v?.policies
 if(!p||!revision(v.revision)||typeof v.enabled!=='boolean'||!Number.isInteger(p.warranty_months)||p.warranty_months<0||p.warranty_months>120||!Number.isInteger(p.return_days)||p.return_days<0||p.return_days>365||![p.warranty_note,p.return_note].every(n=>n&&typeof n.en==='string'&&n.en.length<=200&&typeof n.zh==='string'&&n.zh.length<=200))throw new APIError(0,'request.invalid_response')
 return v
}
export default function BuyPolicies({copy,onExpired}:{copy:Copy;onExpired():void}){
 const zh=copy.name==='姓名',access=useAccess()
 const text=zh?{months:'保修月数',days:'退货天数',warranty:'保修备注',returns:'退货备注',en:'英文',zh:'中文',save:'保存',retry:'重试同一操作',reload:'重新加载',closed:'购买保障编辑尚未启用。',uncertain:'结果尚未确认，请重试同一操作。',conflict:'设置已变化或权限不可用，请重新加载。',invalid:'请检查输入。',required:'必填',saved:'已保存'}:{months:'Warranty months',days:'Return days',warranty:'Warranty note',returns:'Return note',en:'English',zh:'Chinese',save:'Save',retry:'Retry same operation',reload:'Reload',closed:'Purchase policy editing is not enabled.',uncertain:'The result is not confirmed. Retry the same operation.',conflict:'Settings changed or access is unavailable. Reload before saving.',invalid:'Check the input.',required:'Required',saved:'Saved'}
 const [form]=Form.useForm<Policies>(),[data,setData]=useState<Context>(),[load,setLoad]=useState(0),[loading,setLoading]=useState(true),[error,setError]=useState(''),[attempt,setAttempt]=useState<Command>(),[saving,setSaving]=useState(false),[saved,setSaved]=useState(false),busy=useRef(false)
 useEffect(()=>{let live=true;const controller=new AbortController();setLoading(true);setError('');setData(undefined)
  request<Context>('buy-policies',{signal:controller.signal},'/api/admin/').then(context).then(d=>{if(live){setData(d);form.setFieldsValue(d.policies)}}).catch(e=>{if(live){setError(text.conflict);if(e instanceof APIError&&e.status===401)onExpired()}}).finally(()=>{if(live)setLoading(false)})
  return()=>{live=false;controller.abort()}
 },[load])
 async function execute(body:Command){
  if(busy.current)return;busy.current=true;setSaving(true);setAttempt(body);setError('');setSaved(false)
  try{
   const csrf=csrfCookie();if(!csrf)throw new APIError(403,'auth.invalid_csrf')
   const result=await request<{revision:string;replayed:boolean}>('buy-policies',{method:'POST',body,csrf},'/api/admin/')
   if(!revision(result?.revision)||typeof result.replayed!=='boolean')throw new APIError(0,'request.invalid_response')
   setAttempt(undefined);setSaved(true);setLoad(v=>v+1)
  }catch(e){
   if(e instanceof APIError&&[400,401,403,404,409].includes(e.status)){setAttempt(undefined);setError(e.status===400?text.invalid:text.conflict);if(e.status===401)onExpired()}
   else setError(text.uncertain)
  }finally{busy.current=false;setSaving(false)}
 }
 const locked=saving||!!attempt||!data?.enabled||!access.can('store.policies.write')
 return <Space direction="vertical" size={16} className="full-width">
  {loading&&<Spin/>}{saved&&<Alert type="success" showIcon message={text.saved}/>}
  {error&&<Alert type={attempt?'warning':'error'} showIcon message={error} action={attempt?<Button loading={saving} onClick={()=>void execute(attempt)}>{text.retry}</Button>:<Button onClick={()=>setLoad(v=>v+1)}>{text.reload}</Button>}/>}
  {data&&!data.enabled&&<Alert type="info" showIcon message={text.closed}/>}
  {data&&<Form name="buy-policies" form={form} layout="vertical" disabled={locked} onFinish={policies=>void execute({operation_id:crypto.randomUUID(),expected_revision:data.revision,policies})}>
   <Space wrap align="start">{(['warranty_months','return_days'] as const).map((key,i)=><Form.Item key={key} name={key} label={i?text.days:text.months} rules={[{required:true,message:text.required},{type:'integer',min:0,max:i?365:120,message:text.invalid}]}><InputNumber min={0} max={i?365:120} precision={0}/></Form.Item>)}</Space>
   {(['warranty_note','return_note'] as const).map((key,i)=><fieldset key={key} style={{border:0,padding:0,margin:0}}><legend>{i?text.returns:text.warranty}</legend>{(['en','zh'] as const).map(lang=><Form.Item key={lang} name={[key,lang]} label={lang==='en'?text.en:text.zh} rules={[{max:200,message:text.invalid}]}><Input.TextArea rows={2} maxLength={200} showCount/></Form.Item>)}</fieldset>)}
   <Button type="primary" htmlType="submit" loading={saving} disabled={locked}>{text.save}</Button>
  </Form>}
 </Space>
}
