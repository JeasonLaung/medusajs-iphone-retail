import {useEffect,useRef,useState} from 'react'
import {Alert,Button,Drawer,Form,Input,Popconfirm,Select,Space,Table,Tabs,Tag} from 'antd'
import {APIError,csrfCookie,request} from './api'
import {useAccess} from './Permissions'
import type {Copy} from './i18n'

type Resource='settings'|'pages'
type Setting={id:string;key:string;value:string;group:string;protected:boolean}
type Page={id:string;path:string;title:string;description:string;canonical:string;og_image:string;robots:string;hreflang_json:string;structured_data_json:string}
type Directory={revision:string;enabled:boolean;encryption_ready:boolean;settings:Setting[];pages:Page[];count:number;limit:number;offset:number}
type Command={operation_id:string;expected_revision:string;resource:Resource;action:'save'|'delete';id:string;setting?:Omit<Setting,'id'|'protected'>;page?:Omit<Page,'id'>}
const keys=['seo.site_url','seo.site_name','seo.home_path','seo.default_title','seo.default_description','seo.robots_txt','seo.llms_txt','seo.indexnow_enabled','seo.indexnow_key','bing.verification_enabled','bing.site_auth_xml','analytics.enabled','ga4.measurement_id','seo.google_search_console.enabled','gsc.verification_code']
const revision=(v:unknown)=>typeof v==='string'&&/^[a-f0-9]{64}$/.test(v)
function directory(d:Directory){
 if(!d||!revision(d.revision)||typeof d.enabled!=='boolean'||typeof d.encryption_ready!=='boolean'||!Array.isArray(d.settings)||!Array.isArray(d.pages)||!Number.isSafeInteger(d.count)||d.count<0||!Number.isSafeInteger(d.offset)||!Number.isSafeInteger(d.limit)||d.limit<1||d.limit>100||d.offset<0||d.offset>100000)throw new APIError(0,'request.invalid_response')
 if(d.settings.length>1000||d.pages.length>d.limit||d.settings.some(r=>!r||!r.id||!keys.includes(r.key)||typeof r.value!=='string'||typeof r.group!=='string'||typeof r.protected!=='boolean')||d.pages.some(p=>!p||!p.id||['path','title','description','canonical','og_image','robots','hreflang_json','structured_data_json'].some(k=>typeof p[k as keyof Page]!=='string')))throw new APIError(0,'request.invalid_response')
 if(new Set(d.settings.map(r=>r.id)).size!==d.settings.length||new Set(d.pages.map(p=>p.id)).size!==d.pages.length)throw new APIError(0,'request.invalid_response')
 return d
}
export default function SEO({copy,onExpired}:{copy:Copy;onExpired():void}){
 const zh=copy.language==='语言',access=useAccess(),[form]=Form.useForm<Record<string,string>>()
 const labels=zh?['站点网址','站点名称','首页路径','默认标题','默认描述','robots.txt','llms.txt','启用 IndexNow 验证','IndexNow 验证密钥','启用 Bing 验证','Bing 验证 XML','启用访问统计','GA4 Measurement ID','启用 Google 站点验证','Google 验证代码']:['Site URL','Site name','Home path','Default title','Default description','robots.txt','llms.txt','Enable IndexNow verification','IndexNow verification key','Enable Bing verification','Bing verification XML','Enable analytics','GA4 Measurement ID','Enable Google site verification','Google verification code']
 const t=zh?{settings:'站点设置',pages:'页面元数据',add:'新增',edit:'编辑',remove:'删除',confirm:'删除此配置？',save:'保存',cancel:'取消',reload:'重新加载',retry:'重试同一操作',closed:'SEO 编辑尚未启用。',encryption:'站点设置的加密配置尚未就绪。',protected:'待核对',protectedInfo:'受保护的历史设置需先完成核对。',saved:'已保存',uncertain:'结果尚未确认，请重试同一操作。',conflict:'配置或权限已变化，请重新加载后编辑。',invalid:'请检查输入。',required:'必填',key:'设置项',value:'值',group:'分组',path:'页面路径',title:'标题',description:'描述',canonical:'Canonical URL',image:'分享图片网址',robots:'Robots',languages:'语言链接 JSON',structured:'结构化数据 JSON',enabled:'启用',disabled:'关闭'}:{settings:'Site settings',pages:'Page metadata',add:'Add',edit:'Edit',remove:'Delete',confirm:'Delete this configuration?',save:'Save',cancel:'Cancel',reload:'Reload',retry:'Retry same operation',closed:'SEO editing is not enabled.',encryption:'Site setting encryption is not ready.',protected:'Reconciliation required',protectedInfo:'Protected historical settings require reconciliation.',saved:'Saved',uncertain:'The result is not confirmed. Retry the same operation.',conflict:'Configuration or access changed. Reload before editing.',invalid:'Check the input.',required:'Required',key:'Setting',value:'Value',group:'Group',path:'Page path',title:'Title',description:'Description',canonical:'Canonical URL',image:'Social image URL',robots:'Robots',languages:'Language links JSON',structured:'Structured data JSON',enabled:'Enabled',disabled:'Disabled'}
 const [resource,setResource]=useState<Resource>('settings'),[page,setPage]=useState(1),[load,setLoad]=useState(0),[data,setData]=useState<Directory>(),[loading,setLoading]=useState(true),[error,setError]=useState(''),[saved,setSaved]=useState(false),[editor,setEditor]=useState<{id:string}|null>(null),[attempt,setAttempt]=useState<Command>(),[saving,setSaving]=useState(false),busy=useRef(false)
 const settingKey=Form.useWatch('key',form)
 useEffect(()=>{
  let live=true;const controller=new AbortController();setLoading(true);setError('');setData(undefined)
  const path=`seo/${resource}${resource==='pages'?`?limit=50&offset=${(page-1)*50}`:''}`
  request<Directory>(path,{signal:controller.signal},'/api/admin/').then(directory).then(d=>{if(live)setData(d)}).catch(e=>{if(live){setError(t.conflict);if(e instanceof APIError&&e.status===401)onExpired()}}).finally(()=>{if(live)setLoading(false)})
  return()=>{live=false;controller.abort()}
 },[resource,page,load])
 const locked=saving||!!attempt,editable=!!data?.enabled&&access.can('seo.write'),settingEditable=editable&&!!data?.encryption_ready
 const reload=()=>{setEditor(null);setLoad(v=>v+1)}
 async function execute(body:Command){
  if(busy.current)return;busy.current=true;setSaving(true);setAttempt(body);setError('');setSaved(false)
  try{
   const csrf=csrfCookie();if(!csrf)throw new APIError(403,'auth.invalid_csrf')
   const result=await request<{id:string;revision:string;replayed:boolean}>(`seo/${body.resource}`,{method:'POST',body,csrf},'/api/admin/')
   if(!result?.id||!revision(result.revision)||typeof result.replayed!=='boolean')throw new APIError(0,'request.invalid_response')
   setAttempt(undefined);setEditor(null);setSaved(true);setLoad(v=>v+1)
  }catch(e){
   if(e instanceof APIError&&[400,401,403,404,409,413,415].includes(e.status)){setAttempt(undefined);setError(e.status===400||e.status===413?t.invalid:t.conflict);if(e.status===401)onExpired()}
   else setError(t.uncertain)
  }finally{busy.current=false;setSaving(false)}
 }
 function edit(row?:Setting|Page){
  form.resetFields()
  if(resource==='settings'){const r=row as Setting|undefined;form.setFieldsValue(r?{key:r.key,value:r.value,group:r.group}:{key:'seo.site_name',value:'',group:'site'})}
  else{const r=row as Page|undefined;form.setFieldsValue(r?{path:r.path,title:r.title,description:r.description,canonical:r.canonical,og_image:r.og_image,robots:r.robots,hreflang_json:r.hreflang_json,structured_data_json:r.structured_data_json}:{path:'',title:'',description:'',canonical:'',og_image:'',robots:'index,follow',hreflang_json:'{}',structured_data_json:'[]'})}
  setEditor({id:row?.id??''});setError('');setSaved(false)
 }
 function submit(values:Record<string,string>){
  if(!data||!editor)return
  const base={operation_id:crypto.randomUUID(),expected_revision:data.revision,resource,action:'save' as const,id:editor.id}
  // JSON text is sent as a string. Never parse/re-stringify JSON-LD numbers.
  void execute(resource==='settings'?{...base,setting:{key:values.key!,value:values.value??'',group:values.group??'site'}}:{...base,page:{path:values.path??'',title:values.title??'',description:values.description??'',canonical:values.canonical??'',og_image:values.og_image??'',robots:values.robots??'',hreflang_json:values.hreflang_json??'{}',structured_data_json:values.structured_data_json??'[]'}})
 }
 const actions=(row:Setting|Page)=><Space><Button disabled={locked||!editable||resource==='settings'&&(!settingEditable||(row as Setting).protected)} onClick={()=>edit(row)}>{t.edit}</Button><Popconfirm title={t.confirm} okText={t.remove} cancelText={t.cancel} onConfirm={()=>data&&execute({operation_id:crypto.randomUUID(),expected_revision:data.revision,resource,action:'delete',id:row.id})}><Button danger disabled={locked||!editable||resource==='settings'&&(row as Setting).protected}>{t.remove}</Button></Popconfirm></Space>
 const errorNotice=error?<Alert type={attempt?'warning':'error'} showIcon message={error} action={attempt?<Button loading={saving} onClick={()=>void execute(attempt)}>{t.retry}</Button>:<Button onClick={reload}>{t.reload}</Button>}/>:null
 return <Space direction="vertical" size={16} className="full-width">
  <Tabs activeKey={resource} onChange={key=>{setResource(key as Resource);setPage(1);setSaved(false)}} items={[{key:'settings',label:t.settings,disabled:locked||!!editor},{key:'pages',label:t.pages,disabled:locked||!!editor}]}/>
  {saved&&<Alert type="success" showIcon message={t.saved}/>}{!editor&&errorNotice}
  {data&&!data.enabled&&<Alert type="info" showIcon message={t.closed}/>}
  {data&&resource==='settings'&&!data.encryption_ready&&<Alert type="info" showIcon message={t.encryption}/>}
  <Space><Button type="primary" disabled={locked||loading||!editable||resource==='settings'&&!settingEditable} onClick={()=>edit()}>{t.add}</Button><Button disabled={locked||!!editor} onClick={reload}>{t.reload}</Button></Space>
  {resource==='settings'?<Table<Setting> rowKey="id" loading={loading} dataSource={data?.settings??[]} scroll={{x:720}} pagination={false} columns={[{title:t.key,dataIndex:'key',render:(key:string)=>labels[keys.indexOf(key)]??key},{title:t.value,dataIndex:'value',ellipsis:true,render:(value:string,row:Setting)=>row.protected?<Tag>{t.protected}</Tag>:value},{title:t.group,dataIndex:'group'},{title:'',key:'actions',render:(_,row)=>actions(row)}]}/>:<Table<Page> rowKey="id" loading={loading} dataSource={data?.pages??[]} scroll={{x:760}} pagination={{current:page,pageSize:50,total:data?.count??0,showSizeChanger:false,onChange:setPage,disabled:locked||!!editor}} columns={[{title:t.path,dataIndex:'path',ellipsis:true},{title:t.title,dataIndex:'title',ellipsis:true},{title:t.robots,dataIndex:'robots'},{title:'',key:'actions',render:(_,row)=>actions(row)}]}/>}
  <Drawer title={`${editor?.id?t.edit:t.add} ${resource==='settings'?t.settings:t.pages}`} open={!!editor} onClose={()=>!locked&&setEditor(null)} maskClosable={!locked} closable={!locked} keyboard={!locked} width={640} forceRender>
   {errorNotice}
   <Form form={form} name="seo-config" layout="vertical" disabled={locked||!editable||resource==='settings'&&!settingEditable} onFinish={submit}>
    {resource==='settings'?<>
     <Form.Item name="key" label={t.key} rules={[{required:true,message:t.required}]}><Select options={keys.map((value,i)=>({value,label:labels[i]}))}/></Form.Item>
     <Form.Item name="value" label={t.value}>{settingKey?.endsWith('enabled')?<Select options={[{value:'true',label:t.enabled},{value:'false',label:t.disabled}]}/>:<Input.TextArea rows={settingKey?.endsWith('_txt')||settingKey==='bing.site_auth_xml'?7:2} maxLength={65536}/>}</Form.Item>
     <Form.Item name="group" label={t.group} rules={[{max:32,message:t.invalid}]}><Input maxLength={32}/></Form.Item>
    </>:<>
     {([['path',t.path,2048],['title',t.title,512],['description',t.description,4000],['canonical',t.canonical,2048],['og_image',t.image,2048],['robots',t.robots,128]] as const).map(([name,label,max])=><Form.Item key={name} name={name} label={label} rules={[...(name==='path'?[{required:true,message:t.required}]:[]),{max,message:t.invalid}]}>{name==='description'?<Input.TextArea rows={3} maxLength={max}/>:<Input maxLength={max}/>}</Form.Item>)}
     {(['hreflang_json','structured_data_json'] as const).map((name,i)=><Form.Item key={name} name={name} label={i?t.structured:t.languages} rules={[{validator:(_,value:string)=>{try{const parsed:unknown=JSON.parse(value);if(!parsed||typeof parsed!=='object'||!i&&Array.isArray(parsed))throw Error();return Promise.resolve()}catch{return Promise.reject(new Error(t.invalid))}}}]}><Input.TextArea rows={6} maxLength={200000} spellCheck={false}/></Form.Item>)}
    </>}
    <Space><Button type="primary" htmlType="submit" loading={saving} disabled={locked||!editable||resource==='settings'&&!settingEditable}>{t.save}</Button><Button disabled={locked} onClick={()=>setEditor(null)}>{t.cancel}</Button></Space>
   </Form>
  </Drawer>
 </Space>
}
