// Layout and copy follow the existing Medusa order timeline / shipment tracking.
import {useEffect,useState} from 'react'
import {Alert,Spin,Steps,Tag} from 'antd'
import {useAccountContext,useAccountCopy} from '../account/context'
import {Button} from '../account/ui'
import type {AccountKey} from '../account/i18n'
import type {CustomerProgress,ProgressKind} from '../../lib/customer-progress'
import {formatCartMoney} from '../cart/types'
const labels:Record<ProgressKind,AccountKey>={placed:'timelinePlaced',paid:'timelinePaid',refund:'timelineRefund',allocated:'timelineAllocated',released:'timelineReleased',reallocated:'timelineReallocated',packed:'timelinePacked',shipped:'timelineShipped',delivered:'timelineDelivered',fulfillment_canceled:'timelineFulfillmentCanceled',canceled:'timelineCanceled'}
export function useOrderProgress(orderID:string){
 const {api}=useAccountContext(),[data,setData]=useState<CustomerProgress|null>(null),[loading,setLoading]=useState(true),[retry,setRetry]=useState(0)
 useEffect(()=>{let live=true;setData(null);setLoading(true);api.orderProgress(orderID).then(value=>{if(live)setData(value)}).catch(()=>{}).finally(()=>{if(live)setLoading(false)});return()=>{live=false}},[api,orderID,retry])
 return {data,loading,retry:()=>setRetry(n=>n+1)}
}
type Resource=ReturnType<typeof useOrderProgress>
export function OrderProgress({resource}:{resource:Resource}){
 const {t,locale}=useAccountCopy(),{segment}=useAccountContext(),{data,loading,retry}=resource
 return <section className="min-w-0" data-testid="order-timeline" aria-label={t('timelineTitle')}><h2 className="text-lg font-semibold">{t('timelineTitle')}</h2>{loading?<Spin/>:!data?<Alert type="error" message={t('timelineError')} action={<Button onClick={retry}>{t('timelineRetry')}</Button>}/>:<Steps direction="vertical" size="small" current={Math.max(0,data.events.length-1)} className="mt-5" items={data.events.map((event,index)=>{
 const latest=index===data.events.length-1,danger=latest&&['canceled','fulfillment_canceled'].includes(event.kind)
 return {key:event.id,status:danger?'error':latest&&event.kind!=='delivered'?'process':'finish',title:<div className="flex flex-wrap items-baseline gap-x-3 gap-y-1"><span className="font-medium">{t(labels[event.kind])}</span>{latest&&<span className="text-xs text-ui-fg-subtle">{t('timelineLatest')}</span>}</div>,description:<div data-testid={'timeline-'+event.kind}><time className="mt-1 block text-sm text-ui-fg-subtle" dateTime={event.occurred_at}>{new Date(event.occurred_at).toLocaleString(locale,{year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',second:'2-digit',hour12:false,timeZone:'Asia/Shanghai',timeZoneName:'short'})}</time>{event.amount!==undefined&&<p className="mt-1 text-sm">{formatCartMoney(event.amount,data.currency_code,segment)}</p>}</div>}
 })}/>}</section>
}
export function ShipmentTracking({resource}:{resource:Resource}){
 const {locale,t}=useAccountCopy(),zh=locale==='zh-CN',{data,loading,retry}=resource
 if(loading)return null
 if(!data)return <div className="mt-6"><Alert type="error" message={zh?'无法加载包裹物流。':'Unable to load shipment tracking.'} action={<Button onClick={retry}>{t('timelineRetry')}</Button>}/></div>
 if(!data.shipments.length)return null
 return <section className="mt-6 min-w-0 space-y-4" data-testid="shipment-tracking" aria-labelledby={'shipments-'+data.order_id}><h3 id={'shipments-'+data.order_id} className="text-base font-semibold">{zh?'包裹物流':'Shipment tracking'}</h3>{data.shipments.map((shipment,index)=><div key={shipment.id} className="rounded-md bg-ui-bg-subtle p-4" data-testid="shipment"><div className="flex flex-wrap items-center justify-between gap-2"><span className="font-medium">{zh?'包裹':'Package'} {index+1}</span><Tag color={shipment.delivered_at?'success':'processing'}>{shipment.delivered_at?t('timelineDelivered'):t('timelineShipped')}</Tag></div>{shipment.labels.length?shipment.labels.map((label,i)=><div key={i} className="mt-3 min-w-0 break-words"><p>{zh?'物流单号：':'Tracking number: '}{label.tracking_number||'—'}</p>{label.tracking_url&&<a className="underline underline-offset-2 focus-visible:outline" href={label.tracking_url} target="_blank" rel="noopener noreferrer">{zh?'前往承运商查询':'Track with carrier'}</a>}</div>):<p className="mt-3 text-sm text-ui-fg-subtle">{zh?'暂未提供物流单号。':'Tracking details are not yet available.'}</p>}<p className="mt-3 text-sm text-ui-fg-subtle">{zh?'暂未接入站内运输轨迹；如提供查询链接，可前往承运商页面查看。':'In-site transit tracking is not available yet. Use the carrier link when provided.'}</p></div>)}</section>
}
