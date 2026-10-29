import {useAccess} from './Permissions'
import PriceEdit from './PriceEdit'
import { useEffect, useState } from 'react'
import { Alert, Button, Descriptions, Drawer, Empty, Space, Spin, Table, Tag, Typography } from 'antd'
import { APIError, variantPrices } from './api'
import type { ConfiguredPrice, VariantDetail, VariantPrices as Prices } from './api'
import type { Copy } from './i18n'

// Presentation only: trim insignificant zeros with strings, never Number/float.
export function amountText(value:string|null){return value===null?'—':value.includes('.')?value.replace(/0+$/,'').replace(/\.$/,''):value}
export default function VariantPrices({productID,variant,copy,onClose,onExpired}:{productID:string;variant:VariantDetail;copy:Copy;onClose():void;onExpired():void}){
  const access=useAccess(),[editing,setEditing]=useState<string>()
  const [page,setPage]=useState({limit:20,offset:0})
  const [attempt,setAttempt]=useState(0)
  const [state,setState]=useState<{loading:boolean;data?:Prices;error?:unknown}>({loading:true})
  useEffect(()=>{
    const controller=new AbortController();setState({loading:true})
    variantPrices(productID,variant.id,page.limit,page.offset,controller.signal).then(data=>{if(!controller.signal.aborted)setState({loading:false,data})}).catch(error=>{
      if(controller.signal.aborted)return
      if(error instanceof APIError&&error.status===401)onExpired()
      else setState({loading:false,error})
    })
    return()=>controller.abort()
  },[productID,variant.id,page.limit,page.offset,attempt,onExpired])
  const d=state.data
  const expanded=(p:ConfiguredPrice)=><Space direction="vertical" size={16} className="full-width">
    <Descriptions column={1} items={[{key:'id',label:copy.priceID,children:p.id},{key:'set',label:copy.priceSet,children:p.price_set_id},{key:'rulesCount',label:copy.recordedRules,children:p.rules_count??'—'}]}/>
    {p.rules_count!==null&&p.rules_count!==p.rules.length&&<Alert type="warning" showIcon message={copy.ruleCountMismatch}/>}
    <Typography.Title level={5}>{copy.priceRules}</Typography.Title>
    {p.rules.length?<Table rowKey="id" dataSource={p.rules} pagination={false} scroll={{x:660}} columns={[{title:copy.ruleAttribute,dataIndex:'attribute'},{title:copy.ruleOperator,dataIndex:'operator'},{title:copy.ruleValue,dataIndex:'value',render:(s:string)=><span className="price-rule-value">{s}</span>},{title:copy.rulePriority,dataIndex:'priority'}]}/>:<Typography.Text type="secondary">{copy.noRules}</Typography.Text>}
    {p.price_list_id!==null&&(p.price_list?<>
      <Typography.Title level={5}>{copy.priceList}</Typography.Title>
      <Descriptions column={1} items={[{key:'list',label:copy.priceList,children:p.price_list.title},{key:'id',label:'ID',children:p.price_list.id},{key:'status',label:copy.status,children:p.price_list.status==='active'?copy.active:copy.draft},{key:'type',label:copy.priceListType,children:p.price_list.type==='sale'?copy.sale:copy.override},{key:'starts',label:copy.startsAt,children:p.price_list.starts_at??'—'},{key:'ends',label:copy.endsAt,children:p.price_list.ends_at??'—'},{key:'rulesCount',label:copy.recordedRules,children:p.price_list.rules_count??'—'}]}/>
      {p.price_list.rules_count!==null&&p.price_list.rules_count!==p.price_list.rules.length&&<Alert type="warning" showIcon message={copy.ruleCountMismatch}/>}
      {p.price_list.rules.length?<Table rowKey="id" pagination={false} dataSource={p.price_list.rules} scroll={{x:500}} columns={[{title:copy.ruleAttribute,dataIndex:'attribute'},{title:copy.ruleValue,dataIndex:'value',render:(s:string|null)=><span className="price-rule-value">{s??'NULL'}</span>}]}/>:<Typography.Text type="secondary">{copy.noRules}</Typography.Text>}
    </>:<Alert type="warning" showIcon message={copy.priceListUnavailable} description={p.price_list_id}/>)}
  </Space>
  return <Drawer open title={copy.configuredPrices} width="min(1000px, 100vw)" onClose={()=>{if(!editing)onClose()}} closable={!editing} maskClosable={!editing} keyboard={!editing} destroyOnHidden extra={<Button disabled={!!editing} aria-label={copy.cancel} onClick={onClose}>{copy.cancel}</Button>}>
    <Space direction="vertical" size={24} className="full-width">
      <Typography.Text strong>{variant.title}</Typography.Text>
      <Alert type="info" showIcon message={copy.configuredPricesInfo}/>
      {state.loading?<Spin aria-label={copy.loadingPrices}/>:state.error!==undefined?<Space direction="vertical"><Alert type="error" showIcon message={state.error instanceof APIError&&state.error.status===404?copy.variantNotFound:copy.unavailable}/><Button onClick={()=>setAttempt(n=>n+1)}>{copy.retry}</Button></Space>:d&&<>
        <Descriptions column={1} items={[{key:'sets',label:copy.priceSets,children:d.price_set_ids.join(', ')||'—'}]}/>
        <Table<ConfiguredPrice> rowKey="id" dataSource={d.prices} scroll={{x:800}} locale={{emptyText:<Empty description={copy.noPrices}/>}} expandable={{expandedRowRender:expanded}} columns={[
          {title:copy.currency,dataIndex:'currency_code'},
          {title:copy.amount,dataIndex:'amount',render:(s:string)=><span className="identifier">{amountText(s)}</span>},
          {title:copy.minQuantity,dataIndex:'min_quantity',render:amountText},
          {title:copy.maxQuantity,dataIndex:'max_quantity',render:amountText},
          {title:copy.priceList,dataIndex:'price_list',render:(_,p)=>p.price_list_id===null?<Tag>{copy.basePrice}</Tag>:p.price_list?<span>{p.price_list.title} · {p.price_list.status==='active'?copy.active:copy.draft}</span>:<Tag color="warning">{copy.priceListUnavailable}</Tag>},
          {title:copy.priceRules,dataIndex:'rules',render:(r:ConfiguredPrice['rules'])=>r.length},
          {title:copy.editPrice,render:(_,p)=>access.can('price.write')?<Button onClick={()=>setEditing(p.id)}>{copy.editPrice}</Button>:null},
        ]} pagination={{current:Math.floor(d.offset/d.limit)+1,pageSize:d.limit,total:d.count,showSizeChanger:true,pageSizeOptions:[20,50,100],onChange:(n,size)=>setPage({limit:size,offset:(n-1)*size})}}/>
      </>}
    </Space>
    {editing&&access.can('price.write')&&<PriceEdit productID={productID} variantID={variant.id} priceID={editing} copy={copy} onExpired={onExpired} onClose={()=>{setEditing(undefined);setAttempt(n=>n+1)}}/>}
  </Drawer>
}
