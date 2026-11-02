import PriceListBatch from './PriceListBatch'
import PriceListCreate from './PriceListCreate'
import PriceListRemove from './PriceListRemove'
import {useEffect,useRef,useState} from 'react'
import {Alert,Button,DatePicker,Descriptions,Drawer,Form,Input,Select,Space,Spin,Table,Tag,Typography} from 'antd'
import dayjs from 'dayjs'
import utc from 'dayjs/plugin/utc'
import 'dayjs/locale/zh-cn'
import zhDate from 'antd/es/date-picker/locale/zh_CN'
import enDate from 'antd/es/date-picker/locale/en_GB'
dayjs.extend(utc)
import type {Dayjs} from 'dayjs'
import {APIError} from './api'
import {listPriceLists,readPriceList,editPriceList} from './price-lists-api'
import type {PriceListSummary,PriceListDetail,PriceListCommand} from './price-lists-api'
import {useAccess} from './Permissions'
import type {Copy} from './i18n'
export default function PriceLists({copy,onExpired}:{copy:Copy;onExpired():void}){
 const zh=copy.language==='语言',access=useAccess(),[page,setPage]=useState({limit:20,offset:0}),[reload,setReload]=useState(0),[data,setData]=useState<Awaited<ReturnType<typeof listPriceLists>>>(),[error,setError]=useState(false),[batching,setBatching]=useState<string>(),[adding,setAdding]=useState<string>(),[creating,setCreating]=useState(false),[editing,setEditing]=useState<string>(),[removing,setRemoving]=useState<string>()
 useEffect(()=>{const c=new AbortController();setData(undefined);setError(false);listPriceLists(page.limit,page.offset,c.signal).then(d=>{if(!c.signal.aborted)setData(d)}).catch(e=>{if(!c.signal.aborted){setError(true);if(e instanceof APIError&&e.status===401)onExpired()}});return()=>c.abort()},[page,reload,onExpired])
 const status=(p:PriceListSummary)=>p.starts_at&&Date.parse(p.starts_at)>Date.now()?<Tag color="warning">{zh?'尚未生效':'Scheduled'}</Tag>:p.ends_at&&Date.parse(p.ends_at)<Date.now()?<Tag color="error">{zh?'已过期':'Expired'}</Tag>:<Tag color={p.status==='active'?'success':undefined}>{p.status==='active'?copy.active:copy.draft}</Tag>
 return <Space direction="vertical" size={24} className="full-width">
  {access.can('price.write','product.read')&&<Button type="primary" onClick={()=>setCreating(true)}>{zh?'新建价目表':'Create price list'}</Button>}
  <Alert showIcon type="info" message={zh?'价目表设置会影响表内所有价格及关联商品。现有价格、客户组和其他规则保持原值。':'Price-list settings affect every price and linked product in the list. Existing prices, customer groups and other rules retain their values.'}/>
  {error?<Space><Alert type="error" showIcon message={copy.unavailable}/><Button onClick={()=>setReload(n=>n+1)}>{copy.retry}</Button></Space>:!data?<Spin/>:<Table<PriceListSummary> rowKey="id" dataSource={data.price_lists} scroll={{x:850}} columns={[{title:zh?'标题':'Title',dataIndex:'title'},{title:copy.status,render:(_,p)=>status(p)},{title:copy.priceListType,dataIndex:'type',render:v=>v==='sale'?copy.sale:copy.override},{title:copy.startsAt,dataIndex:'starts_at',render:v=>v??'—'},{title:copy.endsAt,dataIndex:'ends_at',render:v=>v??'—'},{title:zh?'详情':'Details',render:(_,p)=><Space><Button onClick={()=>setEditing(p.id)}>{zh?'查看价目表':'View price list'}</Button>{access.can('price.write','product.read')&&<Button onClick={()=>setBatching(p.id)}>{zh?'批量编辑价格':'Edit prices'}</Button>}{access.can('price.write','product.read')&&<Button onClick={()=>setAdding(p.id)}>{zh?'批量添加价格':'Add prices'}</Button>}{access.can('price.write')&&<Button danger onClick={()=>setRemoving(p.id)}>{zh?'删除价目表':'Remove price list'}</Button>}</Space>}]} pagination={{current:Math.floor(data.offset/data.limit)+1,pageSize:data.limit,total:data.count,showSizeChanger:true,pageSizeOptions:[20,50,100],onChange:(n,size)=>setPage({limit:size,offset:(n-1)*size})}}/>}
  {batching&&access.can('price.write','product.read')&&<PriceListBatch id={batching} copy={copy} onExpired={onExpired} onClose={()=>{setBatching(undefined);setReload(n=>n+1)}}/>}
  {adding&&access.can('price.write','product.read')&&<PriceListCreate listID={adding} copy={copy} onExpired={onExpired} onClose={()=>{setAdding(undefined);setReload(n=>n+1)}}/>}
  {creating&&access.can('price.write','product.read')&&<PriceListCreate copy={copy} onExpired={onExpired} onClose={()=>{setCreating(false);setReload(n=>n+1)}}/>}
  {removing&&access.can('price.write')&&<PriceListRemove id={removing} copy={copy} onExpired={onExpired} onClose={()=>{setRemoving(undefined);setReload(n=>n+1)}}/>}
  {editing&&<Editor id={editing} copy={copy} canWrite={access.can('price.write')} onExpired={onExpired} onClose={()=>{setEditing(undefined);setReload(n=>n+1)}}/>}
 </Space>
}
type Values={title:string;description:string;status:'active'|'draft';type:'sale'|'override';starts_at:Dayjs|null;ends_at:Dayjs|null}
function Editor({id,copy,canWrite,onClose,onExpired}:{id:string;copy:Copy;canWrite:boolean;onClose():void;onExpired():void}){
 const zh=copy.language==='语言',[form]=Form.useForm<Values>(),busy=useRef(false),changed=useRef(new Set<string>()),[reload,setReload]=useState(0),[data,setData]=useState<PriceListDetail>(),[error,setError]=useState(''),[attempt,setAttempt]=useState<PriceListCommand>(),[saving,setSaving]=useState(false)
 useEffect(()=>{const c=new AbortController();setData(undefined);setError('');readPriceList(id,c.signal).then(d=>{if(!c.signal.aborted){setData(d);changed.current.clear();form.setFieldsValue({...d,starts_at:d.starts_at?dayjs.utc(d.starts_at):null,ends_at:d.ends_at?dayjs.utc(d.ends_at):null})}}).catch(e=>{if(!c.signal.aborted){setError(copy.unavailable);if(e instanceof APIError&&e.status===401)onExpired()}});return()=>c.abort()},[id,reload,onExpired,form,copy.unavailable])
 async function execute(body:PriceListCommand){if(busy.current)return;busy.current=true;setSaving(true);setAttempt(body);setError('');try{await editPriceList(id,body);onClose()}catch(e){if(e instanceof APIError&&[400,401,403,404,409].includes(e.status)){setAttempt(undefined);setData(undefined);setError(e.status===409?copy.priceEditConflict:e.status===400?(zh?'请检查输入。':'Check the input.'):copy.accessDenied);if(e.status===401)onExpired()}else setError(copy.saveUncertain)}finally{busy.current=false;setSaving(false)}}
 const locked=saving||!!attempt||!data?.enabled||!canWrite
 return <Drawer open title={zh?'价目表设置':'Price list settings'} width="min(620px, 100vw)" onClose={()=>{if(!busy.current)onClose()}} closable={!saving} maskClosable={!saving} keyboard={!saving}>
  <Space direction="vertical" size={24} className="full-width">
   {error&&<Alert showIcon type={attempt?'warning':'error'} message={error} action={attempt?<Button loading={saving} onClick={()=>void execute(attempt)}>{copy.retrySave}</Button>:<Button onClick={()=>setReload(n=>n+1)}>{copy.retry}</Button>}/>}
   {!data&&!error&&<Spin/>}{data&&<>
    <Descriptions column={1} items={[{key:'id',label:'ID',children:data.id},{key:'count',label:zh?'价格数量':'Price count',children:data.price_count}]}/>
    <Alert showIcon type="warning" message={zh?'修改将影响价目表内所有商品报价。':'Changes affect quotes for every product in this price list.'}/>
    {!data.enabled&&<Alert showIcon type="info" message={copy.priceEditingDisabled}/>}
    <Form form={form} layout="vertical" disabled={locked} onValuesChange={v=>Object.keys(v).forEach(k=>changed.current.add(k))} onFinish={v=>void execute({operation_id:crypto.randomUUID(),expected_revision:data.revision,title:v.title,description:v.description,status:v.status,type:v.type,starts_at:changed.current.has('starts_at')?v.starts_at?v.starts_at.format('YYYY-MM-DDTHH:mm:ss.SSS')+'Z':null:data.starts_at,ends_at:changed.current.has('ends_at')?v.ends_at?v.ends_at.format('YYYY-MM-DDTHH:mm:ss.SSS')+'Z':null:data.ends_at})}>
     <Form.Item name="title" label={zh?'标题':'Title'} rules={[{required:true,whitespace:true,message:copy.required}]}><Input maxLength={4096}/></Form.Item>
     <Form.Item name="description" label={zh?'说明':'Description'} rules={[{required:true,whitespace:true,message:copy.required}]}><Input.TextArea rows={3} maxLength={65536}/></Form.Item>
     <Form.Item name="status" label={copy.status}><Select options={[{value:'active',label:copy.active},{value:'draft',label:copy.draft}]}/></Form.Item>
     <Form.Item name="type" label={copy.priceListType}><Select options={[{value:'sale',label:copy.sale},{value:'override',label:copy.override}]}/></Form.Item>
     <Form.Item name="starts_at" label={copy.startsAt}><DatePicker locale={zh?zhDate:enDate} placeholder={zh?'请选择日期':'Select date'} showTime format="YYYY-MM-DD HH:mm:ss" className="full-width"/></Form.Item>
     <Form.Item name="ends_at" label={copy.endsAt}><DatePicker locale={zh?zhDate:enDate} placeholder={zh?'请选择日期':'Select date'} showTime format="YYYY-MM-DD HH:mm:ss" className="full-width"/></Form.Item>
     {canWrite&&<Button aria-label={copy.save} type="primary" htmlType="submit" loading={saving} disabled={locked}>{copy.save}</Button>}
    </Form>
    <Typography.Title level={5}>{copy.priceRules}</Typography.Title>
    {data.rules_count!==data.rules.length&&<Alert showIcon type="warning" message={copy.ruleCountMismatch}/>}
    <Table rowKey="id" dataSource={data.rules} pagination={false} scroll={{x:450}} columns={[{title:copy.ruleAttribute,dataIndex:'attribute'},{title:copy.ruleValue,dataIndex:'value',render:v=><span className="price-rule-value">{v??'NULL'}</span>}]}/>
   </>}
   <Button disabled={saving} onClick={onClose}>{copy.cancel}</Button>
  </Space>
 </Drawer>
}
