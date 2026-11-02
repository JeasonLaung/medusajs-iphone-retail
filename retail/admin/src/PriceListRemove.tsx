import {useEffect,useRef,useState} from 'react'
import {Alert,Button,Descriptions,Drawer,Space,Spin} from 'antd'
import {APIError} from './api'
import {readPriceList,removePriceList} from './price-lists-api'
import type {PriceListDetail,PriceListRemoveCommand} from './price-lists-api'
import type {Copy} from './i18n'
export default function PriceListRemove({id,copy,onClose,onExpired}:{id:string;copy:Copy;onClose():void;onExpired():void}){
 const zh=copy.language==='语言',busy=useRef(false),[data,setData]=useState<PriceListDetail>(),[error,setError]=useState(''),[reload,setReload]=useState(0),[saving,setSaving]=useState(false),[attempt,setAttempt]=useState<PriceListRemoveCommand>(),[denied,setDenied]=useState(false)
 const title=zh?'删除价目表':'Remove price list'
 useEffect(()=>{const c=new AbortController();setData(undefined);setError('');setDenied(false);readPriceList(id,c.signal).then(d=>{if(!c.signal.aborted)setData(d)}).catch(e=>{if(!c.signal.aborted){setError(copy.unavailable);if(e instanceof APIError&&e.status===401)onExpired()}});return()=>c.abort()},[id,reload,onExpired,copy.unavailable])
 async function execute(){if(busy.current||!data?.enabled||denied)return;const body=attempt??{operation_id:crypto.randomUUID(),expected_revision:data.revision};busy.current=true;setSaving(true);setAttempt(body);setError('');try{await removePriceList(id,body);onClose()}catch(e){if(e instanceof APIError&&[400,401,403,404,409].includes(e.status)){setAttempt(undefined);setDenied(true);setError(e.status===409?copy.priceEditConflict:copy.accessDenied);if(e.status===401)onExpired()}else setError(copy.saveUncertain)}finally{busy.current=false;setSaving(false)}}
 return <Drawer open title={title} width="min(560px, 100vw)" onClose={()=>{if(!busy.current)onClose()}} closable={!saving} maskClosable={!saving} keyboard={!saving}>
  <Space direction="vertical" size={24} className="full-width">
   <Alert type="warning" showIcon message={zh?'删除整张价目表及其价格和规则？关联商品将使用其余有效价格，可能暂时无法报价。':'Remove this entire price list, its prices and rules? Linked products will use remaining eligible prices and may become unavailable.'}/>
   {!data&&!error&&<Spin/>}
   {error&&<Alert type={attempt?'warning':'error'} showIcon message={error}/>}
   {data&&<Descriptions column={1} items={[{key:'id',label:'ID',children:data.id},{key:'title',label:zh?'标题':'Title',children:data.title},{key:'prices',label:zh?'价格数量':'Price count',children:data.price_count},{key:'rules',label:zh?'价目表规则数量':'Price-list rule count',children:data.rules.length}]}/>}
   {data&&!data.enabled&&<Alert showIcon type="info" message={copy.priceEditingDisabled}/>}
   {data&&!denied&&<Button danger type="primary" aria-label={attempt?copy.retrySave:title} disabled={saving||!data.enabled} loading={saving} onClick={()=>void execute()}>{attempt?copy.retrySave:title}</Button>}
   {(!data&&!!error||denied)&&<Button onClick={()=>setReload(n=>n+1)}>{copy.retry}</Button>}
   <Button aria-label={copy.cancel} disabled={saving} onClick={onClose}>{copy.cancel}</Button>
  </Space>
 </Drawer>
}
