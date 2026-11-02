import {useEffect,useRef,useState} from 'react'
import {Alert,Button,Drawer,Form,Input,Select,Space,Spin,Steps,Table,Typography} from 'antd'
import {APIError,listProducts,stockQuantity,validPriceQuantityBounds} from './api'
import type {ProductSummary} from './api'
import {readPriceListBatch,batchPriceListPrices} from './price-lists-api'
import type {InitialListPrice,PriceListBatchCommand,PriceListBatchContext,PriceListBatchPrice} from './price-lists-api'
import type {Copy} from './i18n'
type Fields={existing:Record<string,{amount:string;min_quantity:string;max_quantity:string}>;create:InitialListPrice[]}
export default function PriceListBatch({id,copy,onClose,onExpired}:{id:string;copy:Copy;onClose():void;onExpired():void}){
 const zh=copy.language==='语言',[form]=Form.useForm<Fields>(),busy=useRef(false)
 const [step,setStep]=useState(0),[selected,setSelected]=useState<string[]>([]),[selectedRows,setSelectedRows]=useState<ProductSummary[]>([]),[products,setProducts]=useState<{products:ProductSummary[];count:number}>(),[page,setPage]=useState(0),[search,setSearch]=useState(''),[query,setQuery]=useState(''),[reload,setReload]=useState(0),[context,setContext]=useState<PriceListBatchContext>(),[removed,setRemoved]=useState<string[]>([]),[error,setError]=useState(''),[attempt,setAttempt]=useState<PriceListBatchCommand>(),[saving,setSaving]=useState(false)
 const fail=(e:unknown)=>{setError(e instanceof APIError&&e.status===409?copy.priceEditConflict:e instanceof APIError&&[401,403,404].includes(e.status)?copy.accessDenied:copy.unavailable);if(e instanceof APIError&&e.status===401)onExpired()}
 useEffect(()=>{const c=new AbortController();setProducts(undefined);const q=new URLSearchParams({limit:'20',offset:String(page*20)});if(query)q.set('q',query);listProducts(q,c.signal).then(d=>{if(!c.signal.aborted)setProducts(d)}).catch(e=>{if(!c.signal.aborted)fail(e)});return()=>c.abort()},[page,query,reload,onExpired])
 useEffect(()=>{const c=new AbortController();setContext(undefined);readPriceListBatch(id,selected,c.signal).then(d=>{if(!c.signal.aborted){setContext(d);setRemoved([]);form.setFieldsValue({existing:Object.fromEntries(d.prices.map(p=>[p.id,{amount:p.amount,min_quantity:p.min_quantity,max_quantity:p.max_quantity}])),create:[]})}}).catch(e=>{if(!c.signal.aborted)fail(e)});return()=>c.abort()},[id,selected,reload,onExpired,form])
 const locked=saving||!!attempt,disabled=locked||!context?.enabled
 async function execute(body:PriceListBatchCommand){if(busy.current)return;busy.current=true;setAttempt(body);setSaving(true);setError('');try{await batchPriceListPrices(id,body);onClose()}catch(e){if(e instanceof APIError&&[400,401,403,404,409].includes(e.status)){setAttempt(undefined);setContext(undefined);setError(e.status===400?copy.stockInputInvalid:e.status===409?copy.priceEditConflict:copy.accessDenied);if(e.status===401)onExpired()}else setError(copy.saveUncertain)}finally{busy.current=false;setSaving(false)}}
 const submit=()=>{
  if(disabled||step!==1||!context)return
  const values=form.getFieldsValue(true) as Fields,body:PriceListBatchCommand={operation_id:crypto.randomUUID(),expected_revision:context.revision,product_ids:selected,create:[],update:[],delete:[]}
  for(const p of context.prices){const v=values.existing?.[p.id];if(!v){setError(copy.priceEditConflict);return};const ref={id:p.id,product_id:p.product_id,variant_id:p.variant_id};if(removed.includes(p.id)||v.amount.trim()===''){body.delete.push(ref);continue};const amount=stockQuantity(v.amount);if(!amount||!validPriceQuantityBounds(v.min_quantity,v.max_quantity)){setError(copy.stockInputInvalid);return};if(amount!==stockQuantity(p.amount)||v.min_quantity!==p.min_quantity||v.max_quantity!==p.max_quantity)body.update.push({...ref,amount,min_quantity:v.min_quantity,max_quantity:v.max_quantity})}
  for(const p of values.create??[]){if(!p.amount?.trim())continue;const amount=stockQuantity(p.amount),product=context.products.find(product=>product.variants.some(v=>v.id===p.variant_id));if(!amount||!product||!p.currency_code||!validPriceQuantityBounds(p.min_quantity,p.max_quantity)){setError(copy.stockInputInvalid);return};body.create.push({product_id:product.id,variant_id:p.variant_id,currency_code:p.currency_code,amount,region_id:p.region_id??'',min_quantity:p.min_quantity??'',max_quantity:p.max_quantity??''})}
  const count=body.create.length+body.update.length+body.delete.length;if(!count){setError(zh?'请先修改价格。':'Change at least one price.');return};if(count>200){setError(zh?'每次最多提交 200 条变更。':'Save at most 200 changes at once.');return};void execute(body)
 }
 const label=(p:{product_id:string;variant_id:string})=>{const product=context?.products.find(v=>v.id===p.product_id),variant=product?.variants.find(v=>v.id===p.variant_id);return product?.title+' / '+variant?.title+(variant?.sku?' · '+variant.sku:'')+(variant&&variant.shared_variants>1?' · '+copy.sharedPriceInfo+' '+variant.shared_variants:'')}
 return <Drawer open title={zh?'批量编辑价格':'Edit prices'} width="min(1100px, 100vw)" onClose={()=>{if(!busy.current)onClose()}} closable={!saving} maskClosable={!saving} keyboard={!saving}>
  <Space direction="vertical" size={24} className="full-width">
   <Typography.Text>{copy.priceList}: {id}</Typography.Text>
   <Steps current={step} size="small" items={[{title:zh?'选择商品':'Products'},{title:zh?'编辑价格':'Prices'}]}/>
   <Alert type="info" showIcon message={zh?'选择商品后修改已有价格或添加新条件。同一价格集合只显示一次，变更影响所有关联 SKU。清空已有金额或点击移除将删除该价格；所有变更一次提交。':'Choose products, edit existing prices or add conditions. Each shared price appears once and affects all linked SKUs. Clear an existing amount or remove its row to delete it. All changes save together.'}/>
   {error&&<Alert showIcon type={attempt?'warning':'error'} message={error} action={attempt?<Button loading={saving} onClick={()=>void execute(attempt)}>{copy.retrySave}</Button>:<Button onClick={()=>{setReload(n=>n+1);setError('')}}>{copy.retry}</Button>}/>}
   {context&&!context.enabled&&<Alert showIcon type="warning" message={copy.priceEditingDisabled}/>}
   <Form form={form} layout="vertical" disabled={step===0?locked:disabled} onFinish={submit}>
   {step===0?<>
    <Input.Search aria-label={zh?'搜索商品':'Search products'} value={search} onChange={e=>setSearch(e.target.value)} onSearch={q=>{setQuery(q);setPage(0)}}/>
    <Typography.Text>{zh?'已选择':'Selected'} {selected.length} / 20</Typography.Text>
    {!products?<Spin/>:<Table<ProductSummary> rowKey="id" dataSource={products.products} scroll={{x:500}} rowSelection={{selectedRowKeys:selected,preserveSelectedRowKeys:true,onChange:(keys,rows)=>{if(keys.length>20)return;setSelected(keys.map(String));setSelectedRows(previous=>{const known=new Map([...previous,...rows.filter(Boolean)].map(p=>[p.id,p]));return keys.map(k=>known.get(String(k))).filter((p):p is ProductSummary=>!!p)})},getCheckboxProps:p=>({disabled:locked||selected.length>=20&&!selected.includes(p.id)})}} columns={[{title:zh?'商品':'Product',dataIndex:'title'},{title:'ID',dataIndex:'id'}]} pagination={{current:page+1,pageSize:20,total:products.count,showSizeChanger:false,onChange:n=>setPage(n-1)}}/>}
    <Typography.Paragraph>{selectedRows.map(p=>p.title).join(', ')}</Typography.Paragraph>
    <Button key="next" type="primary" disabled={locked||!selected.length||!context} onClick={()=>{setError('');setStep(1)}}>{zh?'下一步':'Next'}</Button>
   </>:!context?<Spin/>:<div>
    <Typography.Title level={5}>{zh?'已有价格':'Existing prices'}</Typography.Title>
    <Table<PriceListBatchPrice> rowKey="id" dataSource={context.prices} scroll={{x:1050}} pagination={{pageSize:20,showSizeChanger:false}} columns={[
     {title:zh?'商品 / SKU':'Product / SKU',width:260,render:(_,p)=><Space direction="vertical"><Typography.Text>{label(p)}</Typography.Text><Typography.Text type="secondary">{p.id}</Typography.Text></Space>},
     {title:copy.currency,width:80,dataIndex:'currency_code',render:v=>v.toUpperCase()},
     {title:copy.priceRules,width:210,render:(_,p)=>p.rules.length?<Space direction="vertical">{p.rules.map(r=><Typography.Text key={r.id}>{r.attribute} {r.operator} {r.value}</Typography.Text>)}</Space>:'—'},
     ...(['amount','min_quantity','max_quantity'] as const).map(key=>({title:key==='amount'?copy.amount:key==='min_quantity'?(zh?'最小数量':'Minimum quantity'):(zh?'最大数量':'Maximum quantity'),width:155,render:(_:unknown,p:PriceListBatchPrice)=><Form.Item name={['existing',p.id,key]} style={{marginBottom:0}}><Input aria-label={(key==='amount'?(zh?'金额':'Amount'):key==='min_quantity'?(zh?'最小数量':'Minimum quantity'):(zh?'最大数量':'Maximum quantity'))+' '+p.id} inputMode={key==='amount'?'decimal':'numeric'} maxLength={key==='amount'?39:20} disabled={disabled||removed.includes(p.id)}/></Form.Item>})),
     {title:zh?'操作':'Actions',width:100,render:(_,p)=><Button aria-label={removed.includes(p.id)?(zh?'撤销移除':'Undo removal'):(zh?'移除':'Remove')} danger={!removed.includes(p.id)} disabled={disabled} onClick={()=>{if(removed.includes(p.id)){form.setFieldValue(['existing',p.id],{amount:p.amount,min_quantity:p.min_quantity,max_quantity:p.max_quantity});setRemoved(v=>v.filter(id=>id!==p.id))}else setRemoved(v=>[...v,p.id])}}>{removed.includes(p.id)?(zh?'撤销移除':'Undo removal'):(zh?'移除':'Remove')}</Button>}
    ]}/>
    <Typography.Title level={5}>{zh?'新价格':'New prices'}</Typography.Title>
    <Form.List name="create">{(fields,{add,remove})=><Space direction="vertical" size={24} className="full-width">
     {fields.map(field=><div key={field.key}>
      <Form.Item name={[field.name,'variant_id']} label={zh?'商品 / SKU':'Product / SKU'}><Select showSearch optionFilterProp="label" options={context.products.flatMap(p=>p.variants.map(v=>({value:v.id,label:label({product_id:p.id,variant_id:v.id})})))}/></Form.Item>
      <Form.Item name={[field.name,'region_id']} label={zh?'地区（可选）':'Region (optional)'}><Select allowClear showSearch optionFilterProp="label" onChange={id=>{const r=context.regions.find(r=>r.id===id);if(r)form.setFieldValue(['create',field.name,'currency_code'],r.currency_code)}} options={context.regions.map(r=>({value:r.id,label:r.name+' · '+r.currency_code.toUpperCase()}))}/></Form.Item>
      <Form.Item noStyle shouldUpdate>{()=> <Form.Item name={[field.name,'currency_code']} label={copy.currency}><Select options={context.currencies.map(c=>({value:c,label:c.toUpperCase(),disabled:!!form.getFieldValue(['create',field.name,'region_id'])&&context.regions.find(r=>r.id===form.getFieldValue(['create',field.name,'region_id']))?.currency_code!==c}))}/></Form.Item>}</Form.Item>
      <Form.Item name={[field.name,'amount']} label={copy.amount}><Input inputMode="decimal" maxLength={39}/></Form.Item>
      <Form.Item name={[field.name,'min_quantity']} label={zh?'最小数量（可选）':'Minimum quantity (optional)'}><Input inputMode="numeric" maxLength={20}/></Form.Item>
      <Form.Item name={[field.name,'max_quantity']} label={zh?'最大数量（可选）':'Maximum quantity (optional)'}><Input inputMode="numeric" maxLength={20}/></Form.Item>
      <Button disabled={disabled} onClick={()=>remove(field.name)}>{zh?'移除新价格':'Remove new price'}</Button>
     </div>)}
     <Button disabled={disabled||fields.length>=200} onClick={()=>add({region_id:'',min_quantity:'',max_quantity:''})}>{zh?'添加新价格':'Add new price'}</Button>
    </Space>}</Form.List>
    <Space wrap style={{marginTop:24}}><Button disabled={locked} onClick={()=>{setStep(0);setError('')}}>{zh?'上一步':'Back'}</Button><Button key="save" aria-label={copy.save} type="primary" htmlType="submit" disabled={disabled} loading={saving}>{copy.save}</Button></Space>
   </div>}
   </Form>
   <Button aria-label={copy.cancel} disabled={saving} onClick={onClose}>{copy.cancel}</Button>
  </Space>
 </Drawer>
}
