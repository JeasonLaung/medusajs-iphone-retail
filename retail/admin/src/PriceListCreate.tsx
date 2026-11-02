import {useEffect,useRef,useState} from 'react'
import {Alert,Button,DatePicker,Drawer,Form,Input,Select,Space,Spin,Steps,Table,Typography} from 'antd'
import type {Dayjs} from 'dayjs'
import zhDate from 'antd/es/date-picker/locale/zh_CN'
import enDate from 'antd/es/date-picker/locale/en_GB'
import {APIError,listProducts,stockQuantity,validPriceQuantityBounds} from './api'
import type {ProductSummary} from './api'
import {createPriceList,readPriceListCreate,addPriceListPrices} from './price-lists-api'
import type {PriceListCreateContext,PriceListCreateCommand,InitialListPrice} from './price-lists-api'
import type {Copy} from './i18n'
type Values={title:string;description:string;status:'active'|'draft';type:'sale'|'override';starts_at:Dayjs|null;ends_at:Dayjs|null;customer_group_ids:string[];prices:InitialListPrice[]}
export default function PriceListCreate({copy,onClose,onExpired,listID}:{listID?:string;copy:Copy;onClose():void;onExpired():void}){
 const zh=copy.language==='语言',[form]=Form.useForm<Values>(),busy=useRef(false)
 const [step,setStep]=useState(listID?1:0),[selected,setSelected]=useState<string[]>([]),[selectedRows,setSelectedRows]=useState<ProductSummary[]>([]),[page,setPage]=useState(0),[search,setSearch]=useState(''),[query,setQuery]=useState(''),[reload,setReload]=useState(0)
 const [products,setProducts]=useState<{products:ProductSummary[];count:number}>(),[context,setContext]=useState<PriceListCreateContext>(),[error,setError]=useState(''),[attempt,setAttempt]=useState<PriceListCreateCommand>(),[saving,setSaving]=useState(false)
 const fail=(e:unknown)=>{setError(e instanceof APIError&&e.status===409?copy.priceEditConflict:e instanceof APIError&&[401,403,404].includes(e.status)?copy.accessDenied:copy.unavailable);if(e instanceof APIError&&e.status===401)onExpired()}
 useEffect(()=>{const c=new AbortController();setProducts(undefined);const q=new URLSearchParams({limit:'20',offset:String(page*20)});if(query)q.set('q',query);listProducts(q,c.signal).then(d=>{if(!c.signal.aborted)setProducts(d)}).catch(e=>{if(!c.signal.aborted)fail(e)});return()=>c.abort()},[page,query,reload,onExpired])
 useEffect(()=>{const c=new AbortController();setContext(undefined);readPriceListCreate(selected,c.signal,listID).then(d=>{if(!c.signal.aborted)setContext(d)}).catch(e=>{if(!c.signal.aborted)fail(e)});return()=>c.abort()},[selected,reload,onExpired,listID])
 const locked=saving||!!attempt
 async function execute(body:PriceListCreateCommand){if(busy.current)return;busy.current=true;setSaving(true);setAttempt(body);setError('');try{if(listID)await addPriceListPrices(listID,{operation_id:body.operation_id,expected_revision:body.expected_revision,product_ids:body.product_ids,prices:body.prices});else await createPriceList(body);onClose()}catch(e){if(e instanceof APIError&&[400,401,403,404,409].includes(e.status)){setAttempt(undefined);setContext(undefined);setError(e.status===400?(zh?'请检查价格、地区和数量条件。':'Check prices, regions and quantity bounds.'):e.status===409?copy.priceEditConflict:copy.accessDenied);if(e.status===401)onExpired()}else setError(copy.saveUncertain)}finally{busy.current=false;setSaving(false)}}
 const next=async()=>{if(locked)return;setError('');if(step===0){try{await form.validateFields(['title','description','status','type','starts_at','ends_at','customer_group_ids']);setStep(1)}catch{}}else if(selected.length&&context){setStep(2)}}
 const submit=(v:Values)=>{if(step!==2||!context?.enabled||!selected.length||locked)return;const prices:InitialListPrice[]=[];for(const p of v.prices??[]){const amount=stockQuantity(p.amount);if(!amount||!validPriceQuantityBounds(p.min_quantity,p.max_quantity)){setError(copy.stockInputInvalid);return};const product=context.products.find(product=>product.variants.some(variant=>variant.id===p.variant_id));if(!product){setError(copy.priceEditConflict);return};prices.push({product_id:product.id,variant_id:p.variant_id,currency_code:p.currency_code,amount,region_id:p.region_id??'',min_quantity:p.min_quantity??'',max_quantity:p.max_quantity??''})}void execute({operation_id:crypto.randomUUID(),expected_revision:context.revision,title:v.title,description:v.description,status:v.status,type:v.type,starts_at:v.starts_at?v.starts_at.format('YYYY-MM-DDTHH:mm:ss.SSS')+'Z':null,ends_at:v.ends_at?v.ends_at.format('YYYY-MM-DDTHH:mm:ss.SSS')+'Z':null,customer_group_ids:v.customer_group_ids??[],product_ids:selected,prices})}
 return <Drawer open title={listID?(zh?'批量添加价格':'Add prices'):(zh?'新建价目表':'Create price list')} width="min(900px, 100vw)" closable={!saving} maskClosable={!saving} keyboard={!saving} onClose={()=>{if(!busy.current)onClose()}}>
  <Space direction="vertical" size={24} className="full-width">
   {listID&&<Typography.Text>{copy.priceList}: {listID}</Typography.Text>}
   <Steps current={listID?step-1:step} size="small" items={[...(!listID?[{title:zh?'基本信息':'Details'}]:[]),{title:zh?'选择商品':'Products'},{title:zh?'初始价格':'Initial prices'}]}/>
   {error&&<Alert showIcon type={attempt?'warning':'error'} message={error} action={attempt?<Button loading={saving} onClick={()=>void execute(attempt)}>{copy.retrySave}</Button>:<Button onClick={()=>{setReload(n=>n+1);setError('')}}>{copy.retry}</Button>}/>}
   {context&&!context.enabled&&<Alert showIcon type="warning" message={copy.priceEditingDisabled}/>}
   <Form form={form} layout="vertical" disabled={locked} initialValues={{status:'active',type:'sale',starts_at:null,ends_at:null,customer_group_ids:[],prices:[]}} onFinish={submit}>
    {!listID&&<div hidden={step!==0}>
     <Form.Item name="title" label={zh?'标题':'Title'} rules={[{required:true,whitespace:true,message:copy.required}]}><Input maxLength={4096}/></Form.Item>
     <Form.Item name="description" label={zh?'说明':'Description'} rules={[{required:true,whitespace:true,message:copy.required}]}><Input.TextArea rows={3} maxLength={65536}/></Form.Item>
     <Form.Item name="status" label={copy.status} rules={[{required:true}]}><Select options={[{value:'active',label:copy.active},{value:'draft',label:copy.draft}]}/></Form.Item>
     <Form.Item name="type" label={copy.priceListType} rules={[{required:true}]}><Select options={[{value:'sale',label:copy.sale},{value:'override',label:copy.override}]}/></Form.Item>
     <Form.Item name="starts_at" label={copy.startsAt}><DatePicker locale={zh?zhDate:enDate} placeholder={zh?'请选择日期':'Select date'} showTime format="YYYY-MM-DD HH:mm:ss" className="full-width"/></Form.Item>
     <Form.Item name="ends_at" label={copy.endsAt}><DatePicker locale={zh?zhDate:enDate} placeholder={zh?'请选择日期':'Select date'} showTime format="YYYY-MM-DD HH:mm:ss" className="full-width"/></Form.Item>
     <Form.Item name="customer_group_ids" label={zh?'客户组 ID（可选）':'Customer group IDs (optional)'} extra={zh?'留空适用于所有客户；填写后仅匹配该客户组，公开匿名报价不会使用这些价格。':'Leave empty for all customers. Group prices only match the specified groups and do not apply to anonymous quotes.'} rules={[{validator:(_,v)=>Array.isArray(v)&&v.length<=100&&v.every(id=>typeof id==='string'&&!!id&&Array.from(id).length<=191&&!/[\x00-\x1f\x7f]/.test(id))?Promise.resolve():Promise.reject(new Error(copy.stockInputInvalid))}]}><Select mode="tags" open={false} tokenSeparators={[',']} maxCount={100}/></Form.Item>
    </div>}
    <div hidden={step!==1}>
     <Space wrap><Input.Search aria-label={zh?'搜索商品':'Search products'} placeholder={zh?'搜索商品':'Search products'} value={search} onChange={e=>setSearch(e.target.value)} onSearch={q=>{setQuery(q);setPage(0)}}/><Typography.Text>{zh?'已选择':'Selected'} {selected.length} / 20</Typography.Text></Space>
     {!products?<Spin/>:<Table<ProductSummary> rowKey="id" dataSource={products.products} scroll={{x:500}} rowSelection={{selectedRowKeys:selected,preserveSelectedRowKeys:true,onChange:(keys,rows)=>{if(keys.length>20)return;setSelected(keys.map(String));setSelectedRows(previous=>{const known=new Map([...previous,...rows.filter(Boolean)].map(p=>[p.id,p]));return keys.map(k=>known.get(String(k))).filter((p):p is ProductSummary=>!!p)});form.setFieldValue('prices',[])},getCheckboxProps:record=>({disabled:locked||selected.length>=20&&!selected.includes(record.id)})}} columns={[{title:zh?'商品':'Product',dataIndex:'title'},{title:'ID',dataIndex:'id'},{title:'SKU',dataIndex:'variant_count'}]} pagination={{current:page+1,pageSize:20,total:products.count,showSizeChanger:false,onChange:n=>setPage(n-1)}}/>}
     {selectedRows.length>0&&<Typography.Paragraph>{selectedRows.map(p=>p.title).join(', ')}</Typography.Paragraph>}
    </div>
    <div hidden={step!==2}>
     {!context?<Spin/>:<>
      <Alert showIcon type="info" message={zh?'可为多个 SKU 录入币种、地区和数量价格。共享价格集合会影响其关联 SKU；同一集合的相同价格条件只需录入一次。':'Add currency, regional and quantity prices for multiple SKUs. Shared price sets affect every linked SKU; enter each set and condition once.'}/>
      <Form.List name="prices">{(fields,{add,remove})=><Space direction="vertical" size={24} className="full-width">
       {fields.map(field=><div key={field.key}>
        <Typography.Title level={5}>{zh?'价格':'Price'} {field.name+1}</Typography.Title>
        <Form.Item name={[field.name,'variant_id']} label={zh?'商品 / SKU':'Product / SKU'} rules={[{required:true,message:copy.required}]}><Select showSearch optionFilterProp="label" options={context.products.flatMap(p=>p.variants.map(v=>({value:v.id,label:p.title+' / '+v.title+(v.sku?' · '+v.sku:'')+(v.shared_variants>1?' · '+(zh?'共享 SKU ':'Shared SKUs ')+v.shared_variants:'')})))}/></Form.Item>
        <Form.Item name={[field.name,'region_id']} label={zh?'地区（可选）':'Region (optional)'}><Select allowClear showSearch optionFilterProp="label" onChange={id=>{const r=context.regions.find(r=>r.id===id);if(r)form.setFieldValue(['prices',field.name,'currency_code'],r.currency_code)}} options={context.regions.map(r=>({value:r.id,label:r.name+' · '+r.currency_code.toUpperCase()}))}/></Form.Item>
        <Form.Item noStyle shouldUpdate>{()=> <Form.Item name={[field.name,'currency_code']} label={copy.currency} rules={[{required:true,message:copy.required}]}><Select options={context.currencies.map(c=>({value:c,label:c.toUpperCase(),disabled:!!form.getFieldValue(['prices',field.name,'region_id'])&&context.regions.find(r=>r.id===form.getFieldValue(['prices',field.name,'region_id']))?.currency_code!==c}))}/></Form.Item>}</Form.Item>
        <Form.Item name={[field.name,'amount']} label={copy.amount} rules={[{validator:(_,v)=>stockQuantity(v)?Promise.resolve():Promise.reject(new Error(copy.stockInputInvalid))}]}><Input inputMode="decimal" maxLength={39}/></Form.Item>
        <Form.Item name={[field.name,'min_quantity']} label={zh?'最小数量（可选）':'Minimum quantity (optional)'}><Input inputMode="numeric" maxLength={20}/></Form.Item>
        <Form.Item name={[field.name,'max_quantity']} label={zh?'最大数量（可选）':'Maximum quantity (optional)'}><Input inputMode="numeric" maxLength={20}/></Form.Item>
        <Button disabled={locked} onClick={()=>remove(field.name)}>{zh?'移除此价格':'Remove this price'}</Button>
       </div>)}
       <Button disabled={locked||fields.length>=200} onClick={()=>add({region_id:'',min_quantity:'',max_quantity:''})}>{zh?'添加初始价格':'Add initial price'}</Button>
       {!fields.length&&<Typography.Text type="secondary">{zh?'未配置初始价格；保存后可继续在 SKU 中录入。':'No initial prices. You can add SKU prices after saving.'}</Typography.Text>}
      </Space>}</Form.List>
     </>}
    </div>
    <Space wrap style={{marginTop:24}}>
     {step>(listID?1:0)&&<Button disabled={locked} onClick={()=>setStep(s=>s-1)}>{zh?'上一步':'Back'}</Button>}
     {step<2?<Button key="next" htmlType="button" type="primary" disabled={locked||step===1&&(!selected.length||!context)} onClick={()=>void next()}>{zh?'下一步':'Next'}</Button>:<Button key="save" aria-label={copy.save} type="primary" htmlType="submit" disabled={locked||!context?.enabled} loading={saving}>{copy.save}</Button>}
    </Space>
   </Form>
   <Button disabled={saving} onClick={onClose}>{copy.cancel}</Button>
  </Space>
 </Drawer>
}
