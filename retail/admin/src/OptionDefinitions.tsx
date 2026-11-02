import {useEffect,useRef,useState} from 'react'
import {Alert,Button,DatePicker,Drawer,Form,Input,InputNumber,Modal,Select,Space,Table,Tag,Typography} from 'antd'
import {useSearchParams} from 'react-router-dom'
import dayjs,{type Dayjs} from 'dayjs'
import zhDate from 'antd/es/date-picker/locale/zh_CN'
import enDate from 'antd/es/date-picker/locale/en_GB'
import OptionDefinitionMetadata from './OptionDefinitionMetadata'
import {APIError} from './api'
import {useAccess} from './Permissions'
import type {Copy} from './i18n'
import {definitionFilterParams,optionDefinitions,removeOptionDefinition,saveOptionDefinition} from './option-definitions-api'
import type {DefinitionFilters,DefinitionInput,DefinitionRemoval,OptionDefinition,OptionDefinitionPage} from './option-definitions-api'

type FilterFields={q?:string;title?:string;id?:string;sort?:string;is_exclusive?:string;with_deleted?:string;deletedFrom?:Dayjs|null;deletedTo?:Dayjs|null;ids?:string[];titles?:string[];createdFrom?:Dayjs|null;createdTo?:Dayjs|null;updatedFrom?:Dayjs|null;updatedTo?:Dayjs|null}
type Editor={id?:string;revision:string;values:{value:string;rank:number|null;memberships:number}[];title:string}
type Attempt={kind:'save';id?:string;input:DefinitionInput}|{kind:'remove';id:string;valueID?:string;input:DefinitionRemoval}

export default function OptionDefinitions({copy,onExpired}:{copy:Copy;onExpired():void}){
 const zh=copy.language==='语言',access=useAccess(),canWrite=access.can('product.read','product.write')
 const text=zh?{title:'共享选项',intro:'共享定义的名称和排序会影响所有使用它的商品。已有商品关联的选项值不能删除。',create:'新建共享选项',name:'名称',value:'选项值',rank:'排序',add:'添加值',remove:'删除',edit:'编辑',save:'保存',cancel:'取消',used:'已关联商品',products:'关联商品数',manageValues:'管理选项值',closeValues:'收起选项值',deleteValue:'删除选项值',noValues:'暂无选项值',empty:'暂无共享选项',closed:'共享选项维护尚未启用。',confirm:'确认删除？此操作影响共享定义。',uncertain:'保存结果尚未确认，请重试同一操作。',conflict:'定义已变化、名称重复或仍有关联，请重新加载核对。',reload:'重新加载',invalid:'请填写名称及不重复的选项值。'}:{title:'Shared options',intro:'Changes to shared names and ranks affect every product using the definition. Values associated with products cannot be deleted.',create:'Create shared option',name:'Name',value:'Option value',rank:'Rank',add:'Add value',remove:'Delete',edit:'Edit',save:'Save',cancel:'Cancel',used:'Associated with products',products:'Associated products',manageValues:'Manage option values',closeValues:'Hide option values',deleteValue:'Delete option value',noValues:'No option values',empty:'No shared options',closed:'Shared option maintenance is not enabled.',confirm:'Delete this shared definition or value?',uncertain:'The result is not confirmed. Retry the same operation.',conflict:'The definition changed, its title exists, or product associations remain. Reload and review.',reload:'Reload',invalid:'Enter a name and distinct option values.'}
 const [params,setParams]=useSearchParams(),key=params.toString(),offset=Number(params.get('offset')??0)
 const [filtersForm]=Form.useForm<FilterFields>()
 const [page,setPage]=useState<OptionDefinitionPage>(),[reload,setReload]=useState(0),[loading,setLoading]=useState(true),[loadError,setLoadError]=useState(false)
 const [editor,setEditor]=useState<Editor>(),[attempt,setAttempt]=useState<Attempt>(),[saving,setSaving]=useState(false),[error,setError]=useState<'uncertain'|'conflict'|'denied'|'invalid'|''>('')
 const [metadata,setMetadata]=useState<OptionDefinition>()
 const [modal,contextHolder]=Modal.useModal()
 const [form]=Form.useForm<Editor>(),busy=useRef(false),live=useRef(false)
 useEffect(()=>{live.current=true;return()=>{live.current=false}},[])
 useEffect(()=>{filtersForm.setFieldsValue({q:params.get('q')??'',title:params.get('title')??'',id:params.get('id')??'',sort:params.get('sort')??'title_asc',is_exclusive:params.get('is_exclusive')??'false',with_deleted:params.get('with_deleted')??'false',deletedFrom:params.has('deleted_at[$gte]')?dayjs(params.get('deleted_at[$gte]')):null,deletedTo:params.has('deleted_at[$lte]')?dayjs(params.get('deleted_at[$lte]')):null,ids:params.getAll('id[]'),titles:params.getAll('title[]'),
  createdFrom:params.has('created_at[$gte]')?dayjs(params.get('created_at[$gte]')):null,createdTo:params.has('created_at[$lte]')?dayjs(params.get('created_at[$lte]')):null,
  updatedFrom:params.has('updated_at[$gte]')?dayjs(params.get('updated_at[$gte]')):null,updatedTo:params.has('updated_at[$lte]')?dayjs(params.get('updated_at[$lte]')):null,
 })},[key,filtersForm,params])
 useEffect(()=>{
  const abort=new AbortController();setLoading(true);setLoadError(false)
  const query=new URLSearchParams(key),filters:DefinitionFilters={}
  let valid=true
  for(const [name,value] of query){
   if(name==='id[]'||name==='title[]'){filters[name==='id[]'?'ids':'titles']=query.getAll(name);continue}
   if(name.startsWith('$and[')||name.startsWith('$or[')){Object.assign(filters,{[name]:name.endsWith('[]')?query.getAll(name):value});continue}
   if(query.getAll(name).length!==1)valid=false
   if(name==='offset'){if(String(offset)!==value)valid=false}
   else if(['q','title','id','sort','is_exclusive','with_deleted'].includes(name)||/^(created_at|updated_at|deleted_at)\[\$(gt|gte|lt|lte|eq|ne)\]$/.test(name))Object.assign(filters,{[name]:value})
   else valid=false
  }
  if(!valid){setLoadError(true);setLoading(false);return()=>abort.abort()}
  optionDefinitions(offset,abort.signal,filters).then(d=>{if(!abort.signal.aborted)setPage(d)}).catch(e=>{if(abort.signal.aborted)return;if(e instanceof APIError&&e.status===401)onExpired();else setLoadError(true)}).finally(()=>{if(!abort.signal.aborted)setLoading(false)})
  return()=>abort.abort()
 },[key,offset,reload,onExpired])
 function open(d?:OptionDefinition){
  const next:Editor=d?{id:d.id,revision:d.revision,title:d.title,values:d.values.map(v=>({value:v.value,rank:v.rank,memberships:v.memberships}))}:{revision:'0'.repeat(64),title:'',values:[{value:'',rank:1,memberships:0}]}
  setEditor(next);form.setFieldsValue(next);setError('')
 }
 async function execute(command:Attempt){
  if(busy.current)return
  busy.current=true;setSaving(true);setAttempt(command);setError('')
  try{
   if(command.kind==='save')await saveOptionDefinition(command.id,command.input)
   else await removeOptionDefinition(command.id,command.valueID,command.input)
   if(live.current){setAttempt(undefined);setEditor(undefined);setReload(v=>v+1)}
  }catch(e){
   if(!live.current)return
   if(e instanceof APIError&&[400,401,403,404,409].includes(e.status)){
    setAttempt(undefined);setError(e.status===409||e.status===404?'conflict':e.status===401||e.status===403?'denied':'invalid');if(e.status===401)onExpired()
   }else setError('uncertain')
  }finally{busy.current=false;if(live.current)setSaving(false)}
 }
 const locked=saving||!!attempt||!!error,disabled=locked||!!metadata||loading||loadError||!canWrite||!page?.enabled
 function submit(values:Editor){
  if(!editor||disabled)return
  const ranks:Record<string,number>=Object.create(null)
  for(const value of values.values)if(value.rank!==null&&value.rank!==undefined)ranks[value.value]=value.rank
  void execute({kind:'save',id:editor.id,input:{operation_id:crypto.randomUUID(),expected_revision:editor.revision,title:values.title,values:values.values.map(v=>v.value),ranks}})
 }
 const alert=error&&<Alert type={error==='uncertain'?'warning':'error'} showIcon message={error==='uncertain'?text.uncertain:error==='conflict'?text.conflict:error==='denied'?copy.accessDenied:text.invalid} action={attempt?<Button loading={saving} onClick={()=>void execute(attempt)}>{copy.retrySave}</Button>:<Button onClick={()=>{setError('');setEditor(undefined);setReload(n=>n+1)}}>{text.reload}</Button>}/>
 return <section data-testid="option-definitions">{contextHolder}<Typography.Paragraph type="secondary">{text.intro}</Typography.Paragraph>
  <Space direction="vertical" size={16} className="full-width">
   {!editor&&alert}
   {loadError&&<Alert type="error" showIcon message={copy.unavailable} action={<Button disabled={locked} onClick={()=>setReload(n=>n+1)}>{copy.retry}</Button>}/>}
   {page&&!page.enabled&&<Alert type="info" showIcon message={text.closed}/>}
   <Form name="option-definition-filters" form={filtersForm} layout="inline" className="product-filters" onFinish={values=>{
    const filters:DefinitionFilters={}
    for(const name of ['q','title','id','sort','is_exclusive','with_deleted'] as const){const value=values[name]?.trim();if(value&&!(name==='sort'&&value==='title_asc'))filters[name]=value}
    if(values.ids?.length)filters.ids=values.ids
    if(values.titles?.length)filters.titles=values.titles
    // Preserve strict comparison predicates from a bookmarked URL; the four
    // picker controls manage the inclusive boundaries shown in this form.
    for(const [name,value] of params)if(name.startsWith('$and[')||name.startsWith('$or['))Object.assign(filters,{[name]:name.endsWith('[]')?params.getAll(name):value})
    for(const [name,value] of params)if(/^(created_at|updated_at|deleted_at)\[\$(gt|lt|eq|ne)\]$/.test(name))Object.assign(filters,{[name]:value})
    for(const [field,column,operator] of [['createdFrom','created_at','gte'],['createdTo','created_at','lte'],['updatedFrom','updated_at','gte'],['updatedTo','updated_at','lte'],['deletedFrom','deleted_at','gte'],['deletedTo','deleted_at','lte']] as const){const date=values[field];if(date){const name=`${column}[$${operator}]`,original=params.get(name);Object.assign(filters,{[name]:original&&dayjs(original).valueOf()===date.valueOf()?original:date.toISOString()})}}
    try{const next=definitionFilterParams(0,filters);next.delete('offset');setParams(next)}catch{setLoadError(true)}
   }}>
    <Form.Item name="is_exclusive" label={zh?'定义类型':'Definition type'}><Select disabled={locked} style={{minWidth:160}} options={[{value:'false',label:zh?'共享':'Shared'},{value:'true',label:zh?'商品专属':'Product-specific'}]}/></Form.Item>
    <Form.Item name="with_deleted" label={zh?'删除记录':'Deleted records'}><Select disabled={locked} style={{minWidth:160}} options={[{value:'false',label:zh?'隐藏':'Hidden'},{value:'true',label:zh?'包含':'Included'}]}/></Form.Item>
    <Form.Item name="q" label={zh?'搜索名称':'Search name'}><Input maxLength={200} allowClear disabled={locked}/></Form.Item>
    <Form.Item name="title" label={zh?'精确名称':'Exact name'}><Input maxLength={100} allowClear disabled={locked} onChange={()=>filtersForm.setFieldValue('titles',[])}/></Form.Item>
    <Form.Item name="id" label="ID"><Input maxLength={191} allowClear disabled={locked} onChange={()=>filtersForm.setFieldValue('ids',[])}/></Form.Item>
    <Form.Item name="ids" label={zh?'匹配任一ID':'Any ID'}><Select mode="tags" disabled={locked} style={{minWidth:180}} maxCount={100} aria-label={zh?'匹配任一ID':'Any ID'} onChange={()=>filtersForm.setFieldValue('id','')}/></Form.Item>
    <Form.Item name="titles" label={zh?'匹配任一名称':'Any name'}><Select mode="tags" disabled={locked} style={{minWidth:180}} maxCount={100} aria-label={zh?'匹配任一名称':'Any name'} onChange={()=>filtersForm.setFieldValue('title','')}/></Form.Item>
    {([
     ['createdFrom',zh?'创建时间起':'Created from'],['createdTo',zh?'创建时间止':'Created through'],
     ['updatedFrom',zh?'更新时间起':'Updated from'],['updatedTo',zh?'更新时间止':'Updated through'],
     ['deletedFrom',zh?'删除时间起':'Deleted from'],['deletedTo',zh?'删除时间止':'Deleted through'],
    ] as const).map(([name,label])=><Form.Item key={name} name={name} label={label}><DatePicker showTime locale={zh?zhDate:enDate} placeholder={zh?'选择日期和时间':'Select date and time'} format="YYYY-MM-DD HH:mm:ss.SSS" disabled={locked} aria-label={label}/></Form.Item>)}
    <Form.Item name="sort" label={zh?'排序方式':'Sort'}><Select disabled={locked} style={{minWidth:180}} options={[
     {value:'title_asc',label:zh?'名称升序':'Name ascending'},{value:'title_desc',label:zh?'名称降序':'Name descending'},
     {value:'created_at_desc',label:zh?'创建时间最新':'Newest created'},{value:'created_at_asc',label:zh?'创建时间最早':'Oldest created'},
     {value:'updated_at_desc',label:zh?'更新时间最新':'Newest updated'},{value:'updated_at_asc',label:zh?'更新时间最早':'Oldest updated'},
    ]}/></Form.Item>
    <Form.Item><Space><Button htmlType="submit" disabled={locked}>{zh?'应用筛选':'Apply filters'}</Button><Button disabled={locked} onClick={()=>setParams({})}>{zh?'重置':'Reset'}</Button></Space></Form.Item>
   </Form>
   {canWrite&&<Button type="primary" disabled={disabled} onClick={()=>open()}>{text.create}</Button>}
   <Table<OptionDefinition> rowKey="id" loading={loading} dataSource={page?.options??[]} locale={{emptyText:text.empty}} scroll={{x:640}} expandable={{
    expandIcon:({expanded,onExpand,record})=><Button type="link" disabled={locked} aria-expanded={expanded} onClick={e=>onExpand(record,e)}>{expanded?text.closeValues:text.manageValues}</Button>,
    expandedRowRender:d=><Table rowKey="id" size="small" dataSource={d.values} pagination={false} locale={{emptyText:text.noValues}} scroll={{x:480}} columns={[
     {title:text.value,dataIndex:'value'},
     {title:text.rank,dataIndex:'rank',render:rank=>rank??'—'},
     {title:text.products,dataIndex:'memberships'},
     {title:'',render:(_,value)=><Button danger disabled={disabled||d.deleted||d.is_exclusive||value.memberships>0} onClick={()=>modal.confirm({title:text.confirm,content:value.value,okText:text.deleteValue,cancelText:text.cancel,okButtonProps:{danger:true},onOk:()=>execute({kind:'remove',id:d.id,valueID:value.id,input:{operation_id:crypto.randomUUID(),expected_revision:d.revision}})})}>{text.deleteValue}</Button>},
    ]}/>,
   }} pagination={{current:offset/20+1,pageSize:20,total:page?.count??0,showSizeChanger:false,disabled:locked,onChange:p=>{const next=new URLSearchParams(key);next.set('offset',String((p-1)*20));setParams(next)}}} columns={[
    {title:text.name,dataIndex:'title'},
    {title:zh?'状态':'State',render:(_,d)=><Space>{d.is_exclusive&&<Tag>{zh?'商品专属':'Product-specific'}</Tag>}{d.deleted&&<Tag>{zh?'已删除':'Deleted'}</Tag>}</Space>},
    {title:text.value,render:(_,d)=><Space wrap>{d.values.map(v=><Tag key={v.id}>{v.value}</Tag>)}</Space>},
    {title:text.products,dataIndex:'active_products'},
    {title:'',render:(_,d)=><Space><Button disabled={disabled||d.deleted||d.is_exclusive} onClick={()=>setMetadata(d)}>{zh?'元数据':'Metadata'}</Button><Button disabled={disabled||d.deleted||d.is_exclusive} onClick={()=>open(d)}>{text.edit}</Button><Button danger disabled={disabled||d.deleted||d.is_exclusive||d.active_products>0} onClick={()=>modal.confirm({title:text.confirm,okText:text.remove,cancelText:text.cancel,okButtonProps:{danger:true},onOk:()=>execute({kind:'remove',id:d.id,input:{operation_id:crypto.randomUUID(),expected_revision:d.revision}})})}>{text.remove}</Button></Space>},
   ]}/>
  </Space>
  {metadata&&<OptionDefinitionMetadata definition={metadata} zh={zh} onExpired={onExpired} onClose={()=>{setMetadata(undefined);setReload(n=>n+1)}} onSaved={()=>{setMetadata(undefined);setReload(n=>n+1)}}/>}
  <Drawer title={editor?.id?text.edit:text.create} open={!!editor} width={640} closable={!saving&&!attempt} maskClosable={!saving&&!attempt} onClose={()=>{if(!saving&&!attempt){setEditor(undefined);setError('')}}} destroyOnHidden>
   <Space direction="vertical" size={16} className="full-width">{alert}
    <Form name="option-definition-editor" form={form} layout="vertical" onFinish={submit} disabled={disabled}>
     <Form.Item name="title" label={text.name} rules={[{required:true,whitespace:true,max:100}]}><Input maxLength={100}/></Form.Item>
     <Form.List name="values" rules={[{validator:async(_,values:{value:string}[])=>{if(!values?.length||values.length>100||new Set(values.map(v=>v?.value)).size!==values.length)throw new Error(text.invalid)}}]}>{(fields,{add,remove},meta)=><>
      {fields.map(field=><Space key={field.key} align="baseline" wrap>
       <Form.Item name={[field.name,'value']} label={text.value} rules={[{required:true,whitespace:true,max:500}]}><Input maxLength={500}/></Form.Item>
       <Form.Item name={[field.name,'rank']} label={text.rank}><InputNumber precision={0} min={-2147483648} max={2147483647}/></Form.Item>
       {Number(form.getFieldValue(['values',field.name,'memberships']))>0?<Tag>{text.used}</Tag>:<Button disabled={disabled||fields.length===1} onClick={()=>remove(field.name)}>{text.remove}</Button>}
      </Space>)}
      <Form.ErrorList errors={meta.errors}/><Form.Item><Button disabled={disabled||fields.length>=100} onClick={()=>add({value:'',rank:null,memberships:0})}>{text.add}</Button></Form.Item>
     </>}</Form.List>
     <Space><Button type="primary" htmlType="submit" loading={saving} disabled={disabled}>{text.save}</Button><Button disabled={saving||!!attempt} onClick={()=>{setEditor(undefined);setError('')}}>{text.cancel}</Button></Space>
    </Form>
   </Space>
  </Drawer>
 </section>
}
