import {useEffect,useRef,useState} from 'react'
import {Alert,Button,Drawer,Form,Input,Select,Space,Spin} from 'antd'
import {APIError,createProduct,readProductCreate} from './api'
import type {ProductCreateContext,ProductCreateInput} from './api'
import type {Copy} from './i18n'
const wording=(copy:Copy)=>copy.language==='语言'?{title:'新建商品',handle:'商品链接标识',channels:'销售渠道',profile:'配送配置',collection:'合集',type:'商品类型',categories:'商品分类',imageURLs:'图片地址（每行一个）',optionTitle:'选项名称',values:'选项值',addOption:'添加选项组',remove:'移除',info:'先创建商品与选项组，再在商品详情新增 SKU、价格和库存。',disabled:'商品创建暂未启用。',invalid:'请检查标识、关联配置、图片地址和重复选项值。',conflict:'商品标识或操作已被使用。请核对商品列表后重试。'}:{title:'Create product',handle:'Handle',channels:'Sales channels',profile:'Shipping profile',collection:'Collection',type:'Product type',categories:'Categories',imageURLs:'Image URLs (one per line)',optionTitle:'Option title',values:'Option values',addOption:'Add option group',remove:'Remove',info:'Create the product and options, then add SKUs, prices and stock from its detail page.',disabled:'Product creation is currently disabled.',invalid:'Check the handle, related configuration, image URLs and duplicate option values.',conflict:'The handle or operation is already in use. Review the product list before retrying.'}
type Fields={title:string;handle:string;subtitle?:string;description?:string;status:ProductCreateInput['status'];channels:string[];profile:string;collection?:string;type?:string;categories?:string[];images?:string;options?:{title:string;values:string[]}[]}
export default function ProductCreate({copy,onClose,onCreated,onExpired}:{copy:Copy;onClose():void;onCreated(id:string):void;onExpired():void}){
 const text=wording(copy),active=useRef(false)
 const [context,setContext]=useState<ProductCreateContext>(),[loadError,setLoadError]=useState(false),[reload,setReload]=useState(0),[saving,setSaving]=useState(false),[attempt,setAttempt]=useState<ProductCreateInput>(),[error,setError]=useState<'uncertain'|'conflict'|'denied'|'invalid'>()
 useEffect(()=>{const abort=new AbortController();setContext(undefined);setLoadError(false);readProductCreate(abort.signal).then(d=>{if(!abort.signal.aborted)setContext(d)}).catch(e=>{if(!abort.signal.aborted){if(e instanceof APIError&&e.status===401)onExpired();else setLoadError(true)}});return()=>abort.abort()},[reload,onExpired])
 const save=async(body:ProductCreateInput)=>{
  if(active.current)return
  active.current=true;setSaving(true);setAttempt(body);setError(undefined)
  try{const id=await createProduct(body);onCreated(id)}catch(e){if(e instanceof APIError&&e.status===401)onExpired();else if(e instanceof APIError&&e.status===409)setError('conflict');else if(e instanceof APIError&&e.status===403)setError('denied');else if(e instanceof APIError&&e.status===400){setAttempt(undefined);setError('invalid')}else setError('uncertain')}finally{active.current=false;setSaving(false)}
 }
 return <Drawer open title={text.title} width="min(720px, 100vw)" closable={!saving} maskClosable={!saving} keyboard={!saving} onClose={()=>{if(!active.current)onClose()}}>
  <Space direction="vertical" size={24} className="full-width">
   <Alert type="info" showIcon message={text.info}/>
   {loadError?<Space direction="vertical"><Alert type="error" showIcon message={copy.unavailable}/><Button onClick={()=>setReload(n=>n+1)}>{copy.retry}</Button></Space>:!context?<Spin/>:<>
    {!context.enabled&&<Alert type="warning" showIcon message={text.disabled}/>}
    {error&&<Alert type={error==='uncertain'?'warning':'error'} showIcon message={error==='uncertain'?copy.saveUncertain:error==='conflict'?text.conflict:error==='invalid'?text.invalid:copy.accessDenied}/>}
    <Form<Fields> name="product-create" layout="vertical" disabled={saving||!!attempt||!context.enabled} initialValues={{status:'draft',channels:context.channels.length===1?[context.channels[0].id]:[],profile:context.shipping_profiles.length===1?context.shipping_profiles[0].id:undefined,options:[]}} onFinish={v=>void save(attempt??{operation_id:crypto.randomUUID(),title:v.title,handle:v.handle,subtitle:v.subtitle||null,description:v.description||null,status:v.status,sales_channel_ids:v.channels,shipping_profile_id:v.profile,collection_id:v.collection||null,type_id:v.type||null,category_ids:v.categories??[],image_urls:(v.images??'').split('\n').map(s=>s.trim()).filter(Boolean),options:(v.options??[]).map(o=>({title:o.title.trim(),values:o.values.map(s=>s.trim())}))})}>
     <Form.Item name="title" label={copy.productTitle} rules={[{required:true,whitespace:true,message:copy.required},{max:500,message:copy.editTooLong}]}><Input maxLength={500}/></Form.Item>
     <Form.Item name="handle" label={text.handle} rules={[{required:true,whitespace:true,message:copy.required},{max:512,message:copy.editTooLong}]}><Input maxLength={512} autoComplete="off"/></Form.Item>
     <Form.Item name="subtitle" label={copy.subtitle}><Input maxLength={1000}/></Form.Item>
     <Form.Item name="description" label={copy.description}><Input.TextArea rows={4} maxLength={100000}/></Form.Item>
     <Form.Item name="status" label={copy.status} rules={[{required:true,message:copy.required}]}><Select options={(['draft','proposed','published','rejected'] as const).map(value=>({value,label:copy[value]}))}/></Form.Item>
     <Form.Item name="channels" label={text.channels} rules={[{required:true,type:'array',min:1,message:copy.required}]}><Select mode="multiple" options={context.channels.map(v=>({value:v.id,label:v.name}))}/></Form.Item>
     <Form.Item name="profile" label={text.profile} rules={[{required:true,message:copy.required}]}><Select options={context.shipping_profiles.map(v=>({value:v.id,label:v.name}))}/></Form.Item>
     <Form.Item name="collection" label={text.collection}><Select allowClear options={context.collections.map(v=>({value:v.id,label:v.name}))}/></Form.Item>
     <Form.Item name="type" label={text.type}><Select allowClear options={context.types.map(v=>({value:v.id,label:v.name}))}/></Form.Item>
     <Form.Item name="categories" label={text.categories}><Select mode="multiple" options={context.categories.map(v=>({value:v.id,label:v.name}))}/></Form.Item>
     <Form.Item name="images" label={text.imageURLs}><Input.TextArea rows={3}/></Form.Item>
     <Form.List name="options">{(fields,{add,remove})=><Space direction="vertical" size={16} className="full-width">{fields.map(field=><Space key={field.key} direction="vertical" className="full-width"><Form.Item name={[field.name,'title']} label={text.optionTitle} rules={[{required:true,whitespace:true,message:copy.required}]}><Input maxLength={100}/></Form.Item><Form.Item name={[field.name,'values']} label={text.values} rules={[{required:true,type:'array',min:1,message:copy.required}]}><Select mode="tags" tokenSeparators={[',']}/></Form.Item><Button disabled={saving||!!attempt||!context.enabled} onClick={()=>remove(field.name)}>{text.remove}</Button></Space>)}<Button disabled={saving||!!attempt||!context.enabled||fields.length>=8} onClick={()=>add({title:'',values:[]})}>{text.addOption}</Button></Space>}</Form.List>
     {!attempt&&context.enabled&&<Form.Item><Button type="primary" htmlType="submit" loading={saving}>{copy.save}</Button></Form.Item>}
    </Form>
    {attempt&&error==='uncertain'&&<Button type="primary" loading={saving} onClick={()=>void save(attempt)}>{copy.retrySave}</Button>}
   </>}
   <Button disabled={saving} onClick={onClose}>{copy.cancel}</Button>
  </Space>
 </Drawer>
}
