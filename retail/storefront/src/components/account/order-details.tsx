// Layout provenance: Medusa order/templates/order-details-template.tsx.
import {useEffect,useState} from 'react'
import {Alert,Spin,Tag} from 'antd'
import {useAccountContext,useAccountCopy} from './context'
import {Button} from './ui'
import Link from './localized-link'
import type {OrderConfirmation} from '../../lib/cart-client'
import {SavedOrderSections} from '../order/confirmation'
import {useOrderProgress,OrderProgress,ShipmentTracking} from '../order/progress'
export default function AccountOrderDetails({orderID}:{orderID:string}){
 const {api,segment}=useAccountContext(),{t,locale}=useAccountCopy(),progress=useOrderProgress(orderID)
 const [order,setOrder]=useState<OrderConfirmation|null>(null),[failed,setFailed]=useState(false),[retry,setRetry]=useState(0)
 useEffect(()=>{let live=true;setOrder(null);setFailed(false);api.order(orderID).then(value=>{if(live)setOrder(value)}).catch(()=>{if(live)setFailed(true)});return()=>{live=false}},[orderID,retry,api])
 return <div className="flex flex-col justify-center gap-y-4"><div className="flex gap-2 justify-between items-center"><h1 className="text-2xl-semi">{t('orderDetails')}</h1><Link href="/account/orders" className="flex gap-2 items-center text-ui-fg-subtle hover:text-ui-fg-base" data-testid="back-to-overview-button"><span aria-hidden>×</span>{t('backOverview')}</Link></div>{failed?<Alert showIcon type="error" message={t('error')} action={<Button onClick={()=>setRetry(n=>n+1)}>{t('timelineRetry')}</Button>}/>:!order?<Spin/>:<div className="flex flex-col gap-4 h-full bg-white w-full" data-testid="order-details-container"><div><p>{t('orderDate')} <span data-testid="order-date">{new Date(order.created_at).toLocaleDateString(locale)}</span></p><p className="mt-2 text-ui-fg-interactive">{t('orderNumberLabel')}<span data-testid="order-id">{order.custom_display_id||order.display_id}</span></p><p className="mt-4">{t('orderStatus')} <Tag>{order.status==='canceled'?t('fulfillmentCanceled'):order.status==='completed'?t('completed'):order.status==='archived'?t('orderArchived'):t('orderPending')}</Tag></p></div><div className="grid grid-cols-1 gap-8 small:grid-cols-[minmax(0,1fr)_260px]"><div className="min-w-0 flex flex-col gap-4"><SavedOrderSections order={order} segment={segment} account shipments={<ShipmentTracking resource={progress}/>}/>{order.status!=='canceled'&&<div><h2 className="text-xl-semi">{t('afterSales')}</h2><p>{t('afterSalesDescription')}</p><Button disabled>{t('requestAfterSales')}</Button><p>{t('afterSalesUnavailable')}</p></div>}</div><aside className="order-first min-w-0 small:order-last"><OrderProgress resource={progress}/></aside></div></div>}</div>
}
