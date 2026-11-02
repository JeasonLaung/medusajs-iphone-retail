// Original order/components/order-reviews layout and interaction, with Gin transport.
import {useEffect,useState} from 'react'
import {Button} from '../account/ui'
import type {customerClient} from '../../lib/customer-client'
import type {OrderConfirmation} from '../../lib/cart-client'
import type {OrderReviewContext} from '../../lib/product-reviews'
import ReviewForm from '../products/product-reviews/review-form'
import {getReviewCopy} from '../products/product-reviews/i18n'
import styles from '../products/product-reviews/styles.module.css'

export default function OrderReviews({order,segment,api}:{order:OrderConfirmation;segment:string;api:ReturnType<typeof customerClient>}){
 const copy=getReviewCopy(segment)
 const [context,setContext]=useState<OrderReviewContext|null>(null),[loading,setLoading]=useState(true),[retry,setRetry]=useState(0)
 const [activeProduct,setActiveProduct]=useState<string|null>(null),[submitted,setSubmitted]=useState<string[]>([]),[pending,setPending]=useState(false)
 const products=Array.from(new Map(order.items.flatMap(item=>item.product_id?[[item.product_id,item.product_title||item.title] as const]:[])).entries())
 useEffect(()=>{
  let active=true;setLoading(true)
  api.orderReviews(order.id).then(result=>{if(active)setContext(result)}).catch(()=>{if(active)setContext(null)}).finally(()=>{if(active)setLoading(false)})
  return()=>{active=false}
 },[api,order.id,retry])
 if(!products.length)return null
 return <section id="order-reviews" className={styles.orderSection} data-testid="order-reviews">
  <h2>{copy.orderHeading}</h2><p className={styles.muted}>{copy.orderIntro}</p>
  {loading?<p role="status" className="mt-4 text-sm">{copy.loadingOrders}</p>:!context?<div role="alert" className="mt-4 text-sm">{copy.purchasesError} <Button variant="secondary" onClick={()=>setRetry(n=>n+1)}>{copy.retry}</Button></div>:<>
   {!context.eligible&&<p className={styles.muted}>{copy.orderIneligible}</p>}
   <div className="mt-4 space-y-4">{products.map(([productId,title])=>{
    const review=context.reviews.find(r=>r.product_id===productId),status=submitted.includes(productId)?'pending':review?.status
    return <div key={productId} className={styles.orderRow}>
     <div className={styles.orderMeta}><h3 className="font-medium">{title}</h3>{status?<p role="status" className={styles.muted}>{copy[status]}{review&&!review.verified_purchase?' · '+copy.openReview:''}</p>:<Button variant="secondary" disabled={!context.eligible||pending} aria-expanded={activeProduct===productId} onClick={()=>setActiveProduct(activeProduct===productId?null:productId)}>{copy.write}</Button>}</div>
     {activeProduct===productId&&!status&&<div className="mt-4"><ReviewForm key={productId} antDesign api={api} productId={productId} locale={segment} orderId={order.id} onPendingChange={setPending} onCancel={()=>setActiveProduct(null)} onSuccess={()=>{setSubmitted(current=>[...current,productId]);setActiveProduct(null)}}/></div>}
    </div>
   })}</div>
  </>}
 </section>
}
