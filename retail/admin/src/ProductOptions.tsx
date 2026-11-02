import {useEffect,useRef,useState} from 'react'
import {Alert,Button,Drawer,Input,Select,Space,Spin,Tag,Typography} from 'antd'
import {APIError,readProductOptions,saveProductOptions} from './api'
import type {ManagedOption,ProductOptionsContext,ProductOptionsInput,OptionSelection} from './api'
import type {Copy} from './i18n'

export default function ProductOptions({productID,copy,onClose,onSaved,onExpired}:{productID:string;copy:Copy;onClose():void;onSaved():void;onExpired():void}){
 const text=copy.language==='语言'?{title:'管理选项',intro:'选用共享选项，或创建商品专属选项。已有 SKU 使用的选项值不能移除。共享选项的定义在此保持不变。',shared:'添加共享选项',local:'专属选项名称',add:'创建专属选项',values:'选项值',remove:'移除选项',used:'SKU 使用中',exclusive:'商品专属',global:'共享',disabled:'选项管理暂未启用。',conflict:'选项已变更，或已有 SKU 正在使用待移除的值。请重新加载后核对。',invalid:'请检查选项名称和值，避免重复或空值。',reload:'重新加载选项'}:{title:'Manage options',intro:'Choose shared options or create product-specific options. Values used by an existing SKU cannot be removed. Shared definitions stay unchanged here.',shared:'Add shared option',local:'Product-specific option name',add:'Create product-specific option',values:'Option values',remove:'Remove option',used:'Used by SKU',exclusive:'Product-specific',global:'Shared',disabled:'Option management is currently disabled.',conflict:'Options changed, or an active SKU uses a removed value. Reload and review before saving.',invalid:'Check option names and values for duplicates or empty values.',reload:'Reload options'}
 const [context,setContext]=useState<ProductOptionsContext>(),[selected,setSelected]=useState<OptionSelection[]>([]),[title,setTitle]=useState(''),[loadingError,setLoadingError]=useState(false),[reload,setReload]=useState(0),[error,setError]=useState<string>(),[saving,setSaving]=useState(false),[attempt,setAttempt]=useState<ProductOptionsInput>()
 const active=useRef(false)
 useEffect(()=>{const abort=new AbortController();setContext(undefined);setLoadingError(false);setError(undefined);setAttempt(undefined);setTitle('');readProductOptions(productID,abort.signal).then(d=>{if(!abort.signal.aborted){setContext(d);setSelected(d.options.filter(o=>o.selected).map(o=>({id:o.id,title:o.title,values:o.values.filter(v=>v.selected).map(v=>({id:v.id,value:v.value}))})))}}).catch(e=>{if(!abort.signal.aborted){if(e instanceof APIError&&e.status===401)onExpired();else setLoadingError(true)}});return()=>abort.abort()},[productID,reload,onExpired])
 const disabled=saving||!!attempt||!context?.enabled
 const save=async(body:ProductOptionsInput)=>{if(active.current)return;active.current=true;setSaving(true);setAttempt(body);setError(undefined);try{await saveProductOptions(productID,body);onSaved()}catch(e){if(e instanceof APIError&&e.status===401)onExpired();else if(e instanceof APIError&&e.status===409)setError('conflict');else if(e instanceof APIError&&e.status===400){setAttempt(undefined);setError('invalid')}else if(e instanceof APIError&&(e.status===403||e.status===404))setError('denied');else setError('uncertain')}finally{active.current=false;setSaving(false)}}
 const group=(id:string|null):ManagedOption|undefined=>context?.options.find(o=>o.id===id)
 return <Drawer open title={text.title} width="min(680px,100vw)" closable={!saving} maskClosable={!saving} keyboard={!saving} onClose={()=>{if(!active.current)onClose()}}><Space direction="vertical" size={24} className="full-width">
  <Alert type="info" showIcon message={text.intro}/>
  {loadingError?<><Alert type="error" message={copy.unavailable}/><Button onClick={()=>setReload(n=>n+1)}>{copy.retry}</Button></>:!context?<Spin/>:<>
   {!context.enabled&&<Alert type="warning" message={text.disabled}/>}
   {error&&<Alert type={error==='uncertain'?'warning':'error'} showIcon message={error==='conflict'?text.conflict:error==='invalid'?text.invalid:error==='denied'?copy.accessDenied:copy.saveUncertain}/>}
   {selected.map((o,index)=>{const definition=group(o.id),used=definition?.values.filter(v=>v.used&&v.selected)??[];return <Space key={o.id??'new-'+index} data-testid={'product-option-'+index} direction="vertical" className="full-width" size={8}>
    <Space wrap><Typography.Text strong>{o.title}</Typography.Text><Tag>{definition?.is_exclusive===false?text.global:text.exclusive}</Tag>{used.length>0&&<Tag>{text.used}</Tag>}<Button disabled={disabled||used.length>0} onClick={()=>setSelected(all=>all.filter((_,i)=>i!==index))}>{text.remove}</Button></Space>
    <Select aria-label={text.values+' '+o.title} showSearch mode={definition?.is_exclusive===false?'multiple':'tags'} className="full-width" disabled={disabled} value={o.values.map(v=>v.id??v.value)} optionFilterProp="label" maxCount={100} options={definition?.values.map(v=>({value:v.id,label:v.value,disabled:!definition.is_exclusive&&v.used&&v.selected}))} tagRender={({label,value,closable,onClose})=><Tag closable={closable&&!disabled&&!used.some(v=>v.id===value)} onClose={onClose} style={{margin:4}}>{label}</Tag>} onChange={(ids:string[])=>{
     const values=ids.map(id=>{const v=definition?.values.find(v=>v.id===id||v.value===id);return v?{id:v.id,value:v.value}:{id:null,value:id.trim()}})
     for(const v of used)if(!values.some(x=>x.id===v.id))values.push({id:v.id,value:v.value})
     setSelected(all=>all.map((x,i)=>i===index?{...x,values}:x))
    }}/>
   </Space>})}
   <Select aria-label={text.shared} placeholder={text.shared} className="full-width" showSearch optionFilterProp="label" disabled={disabled||selected.length>=8} value={undefined} options={context.options.filter(o=>!o.is_exclusive&&!selected.some(x=>x.id===o.id||x.title===o.title)).map(o=>({value:o.id,label:o.title}))} onChange={(id:string)=>{const o=group(id);if(o)setSelected(all=>[...all,{id:o.id,title:o.title,values:[]}])}}/>
   <Space direction="vertical" className="full-width"><Input aria-label={text.local} placeholder={text.local} disabled={disabled||selected.length>=8} maxLength={100} value={title} onChange={e=>setTitle(e.target.value)}/><Button disabled={disabled||selected.length>=8||!title.trim()||selected.some(o=>o.title===title.trim())} onClick={()=>{setSelected(all=>[...all,{id:null,title:title.trim(),values:[]}]);setTitle('')}}>{text.add}</Button></Space>
   {!attempt&&context.enabled&&<Button type="primary" disabled={disabled||selected.some(o=>o.values.length===0)} loading={saving} onClick={()=>void save({operation_id:crypto.randomUUID(),expected_revision:context.revision,options:selected})}>{copy.save}</Button>}
   {attempt&&error==='uncertain'&&<Button type="primary" loading={saving} onClick={()=>void save(attempt)}>{copy.retrySave}</Button>}
   {error&&error!=='uncertain'&&<Button disabled={saving} onClick={()=>setReload(n=>n+1)}>{text.reload}</Button>}
  </>}
  <Button disabled={saving} onClick={onClose}>{copy.cancel}</Button>
 </Space></Drawer>
}
