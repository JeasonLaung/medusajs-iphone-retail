import {useEffect,useState} from 'react'
import {Alert,Button,Descriptions,Space,Spin,Table,Typography} from 'antd'
import {APIError,getOrderAmounts} from './api'
import type {OrderAmounts as Amounts} from './api'
import type {Copy} from './i18n'
import {amountText} from './VariantPrices'

export default function OrderAmounts({id,version,copy,onExpired}:{id:string;version:number;copy:Copy;onExpired():void}) {
 const [attempt,setAttempt]=useState(0)
 const [state,setState]=useState<{loading:boolean;data?:Amounts;error?:unknown}>({loading:true})
 useEffect(()=>{
  const controller=new AbortController();setState({loading:true})
  getOrderAmounts(id,controller.signal).then(data=>{
   if(controller.signal.aborted)return
   if(data.order_id!==id||data.version!==version)throw new APIError(0,'orders.version_changed')
   setState({loading:false,data})
  }).catch(error=>{
   if(controller.signal.aborted)return
   if(error instanceof APIError&&error.status===401)onExpired()
   else setState({loading:false,error})
  })
  return()=>controller.abort()
 },[id,version,attempt,onExpired])
 const data=state.data
 const amount=(value:string)=><span className="price-rule-value">{amountText(value)} {data?.currency_code}</span>
 return <section aria-label={copy.orderAmounts}>
  <Space direction="vertical" size={16} className="full-width">
   <Typography.Title level={4}>{copy.orderAmounts}</Typography.Title>
   {state.loading?<Spin/>:state.error!==undefined?<Space direction="vertical"><Alert type="error" showIcon message={state.error instanceof APIError&&state.error.code==='orders.version_changed'?copy.orderVersionChanged:copy.unavailable}/><Button onClick={()=>setAttempt(n=>n+1)}>{copy.retry}</Button></Space>:data&&<>
    <Typography.Paragraph type="secondary">{copy.orderAmountsInfo}</Typography.Paragraph>
    <Descriptions column={{xs:1,sm:2,lg:3}} items={[
     {key:'items',label:copy.itemSubtotal,children:amount(data.totals.item_subtotal)},
     {key:'shipping',label:copy.shippingSubtotal,children:amount(data.totals.shipping_subtotal)},
     {key:'discount',label:copy.discountBeforeTax,children:amount(data.totals.discount_subtotal)},
     {key:'tax',label:copy.taxTotal,children:amount(data.totals.tax_total)},
     {key:'credit',label:copy.creditTotal,children:amount(data.totals.credit_line_total)},
     {key:'total',label:copy.orderTotal,children:<Typography.Text strong>{amount(data.totals.total)}</Typography.Text>},
    ]}/>
    {data.summary&&<Descriptions column={{xs:1,sm:2}} items={[
     {key:'transaction',label:copy.recordedTransaction,children:amount(data.summary.transaction_total)},
     {key:'pending',label:copy.pendingDifference,children:amount(data.summary.pending_difference)},
    ]}/>}
    {data.shipping_methods.length>0&&<Table<Amounts['shipping_methods'][number]> rowKey="id" size="small" dataSource={data.shipping_methods} pagination={false} scroll={{x:600}} columns={[
     {title:copy.shippingMethodName,dataIndex:'name'},
     {title:copy.shippingSubtotal,render:(_,row)=>amount(row.totals.subtotal)},
     {title:copy.taxTotal,render:(_,row)=>amount(row.totals.tax_total)},
     {title:copy.orderTotal,render:(_,row)=>amount(row.totals.total)},
    ]}/>}
   </>}
  </Space>
 </section>
}
