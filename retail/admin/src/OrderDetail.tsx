import { useEffect, useState } from 'react'
import { Alert, Button, Descriptions, Empty, Space, Spin, Table, Tag, Typography } from 'antd'
import { Link, useLocation, useParams } from 'react-router-dom'
import { APIError, getOrder } from './api'
import type { OrderDetail as Detail, OrderLine } from './api'
import type { Copy } from './i18n'
import { amountText } from './VariantPrices'
import OrderAmounts from './OrderAmounts'
import OrderPayments from './OrderPayments'
import OrderFulfillments from './OrderFulfillments'

export default function OrderDetail({copy,onExpired}:{copy:Copy;onExpired():void}) {
  const {id=''}=useParams()
  const location=useLocation()
  const [attempt,setAttempt]=useState(0)
  const [state,setState]=useState<{loading:boolean;data?:Detail;error?:unknown}>({loading:true})
  useEffect(()=>{
    const controller=new AbortController()
    setState({loading:true})
    getOrder(id,controller.signal).then(data=>{if(!controller.signal.aborted)setState({loading:false,data})}).catch(error=>{
      if(controller.signal.aborted)return
      if(error instanceof APIError && error.status===401)onExpired()
      else setState({loading:false,error})
    })
    return()=>controller.abort()
  },[id,attempt,onExpired])
  const order=state.data?.order
  const statuses:Record<string,string>={pending:copy.pendingOrder,completed:copy.completedOrder,draft:copy.draft,archived:copy.archivedOrder,canceled:copy.canceledOrder,requires_action:copy.requiresActionOrder}
  return <section className="account-panel" aria-label={copy.orderLines}>
    <Space direction="vertical" size={24} className="full-width">
      <Link to={`/orders${location.search}`} className="product-link">{copy.backOrders}</Link>
      <Typography.Paragraph type="secondary">{copy.orderReadInfo}</Typography.Paragraph>
      {state.loading?<Spin />:state.error!==undefined?<Space direction="vertical"><Alert type="error" showIcon message={state.error instanceof APIError && state.error.status===404?copy.orderNotFound:copy.unavailable} /><Button onClick={()=>setAttempt(n=>n+1)}>{copy.retry}</Button></Space>:order&&<>
        <Typography.Title level={3}>{order.custom_display_id??order.display_id??order.id}</Typography.Title>
        <Descriptions column={{xs:1,sm:2,lg:3}} items={[
          {key:'status',label:copy.status,children:<Tag color={order.status==='completed'?'success':order.status==='requires_action'?'warning':undefined}>{statuses[order.status]??order.status}</Tag>},
          {key:'email',label:copy.email,children:order.email??'—'},
          {key:'currency',label:copy.currency,children:order.currency_code},
          {key:'version',label:copy.orderVersion,children:order.version},
          {key:'created',label:copy.createdAt,children:<time dateTime={order.created_at}>{new Date(order.created_at).toLocaleString(document.documentElement.lang)}</time>},
        ]} />
        <Table<OrderLine> rowKey="id" dataSource={state.data?.lines??[]} scroll={{x:1000}} pagination={false} locale={{emptyText:<Empty description={copy.noOrderLines} />}} columns={[
          {title:copy.productTitle,dataIndex:'title',render:(title:string,row)=><Space direction="vertical" size={0}><Typography.Text>{title}</Typography.Text><Typography.Text type="secondary">{row.variant_sku??'—'}</Typography.Text></Space>},
          {title:copy.orderedQuantity,dataIndex:'quantity',render:amountText},
          {title:copy.snapshotUnitPrice,dataIndex:'unit_price',render:(value:string,row)=><Space direction="vertical" size={0}><Typography.Text>{amountText(value)} {order.currency_code}</Typography.Text><Typography.Text type="secondary">{copy.taxInclusive}: {row.is_tax_inclusive?copy.yes:copy.no}</Typography.Text></Space>},
          {title:copy.fulfilledQuantity,dataIndex:'fulfilled_quantity',render:amountText},
          {title:copy.shippedQuantity,dataIndex:'shipped_quantity',render:amountText},
        ]} expandable={{expandedRowRender:row=><Descriptions column={{xs:1,sm:3}} items={[
          {key:'requested',label:copy.returnRequestedQuantity,children:amountText(row.return_requested_quantity)},
          {key:'received',label:copy.returnReceivedQuantity,children:amountText(row.return_received_quantity)},
          {key:'dismissed',label:copy.returnDismissedQuantity,children:amountText(row.return_dismissed_quantity)},
        ]} />}} />
        <OrderAmounts id={order.id} version={order.version} copy={copy} onExpired={onExpired}/>
<OrderPayments id={order.id} version={order.version} copy={copy} onExpired={onExpired}/>
<OrderFulfillments id={order.id} version={order.version} copy={copy} onExpired={onExpired}/>
      </>}
    </Space>
  </section>
}
