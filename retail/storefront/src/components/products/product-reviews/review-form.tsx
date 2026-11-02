// Original product-review form layout; current customer and durable commands use Gin.
import {Input as AntInput} from 'antd'
import {useEffect,useId,useRef,useState} from 'react'
import {Button as AntButton} from '../../account/ui'
import {RatingInput} from '../../home/rating'
import {customerClient,CustomerError} from '../../../lib/customer-client'
import type {ReviewPurchases,ReviewSubmission} from '../../../lib/product-reviews'
import {getReviewCopy} from './i18n'
import styles from './styles.module.css'

type Client=ReturnType<typeof customerClient>
export default function ReviewForm({antDesign=false,api:providedAPI,productId,locale,orderId,onSuccess,onCancel,onPendingChange}:{antDesign?:boolean;api?:Client;productId:string;locale:string;orderId?:string;onSuccess():void;onCancel():void;onPendingChange?(pending:boolean):void}){
 const [localAPI]=useState(()=>customerClient()),api=providedAPI??localAPI
 const TitleInput=antDesign?AntInput:'input',ContentInput=antDesign?AntInput.TextArea:'textarea',ActionButton=antDesign?AntButton:'button'
 const copy=getReviewCopy(locale),id=useId(),intlLocale=locale.startsWith('zh')||locale.endsWith('-zh')?'zh-CN':'en-GB'
 const [rating,setRating]=useState(5),[selectedOrder,setSelectedOrder]=useState(orderId??''),[title,setTitle]=useState(''),[content,setContent]=useState('')
 const [purchases,setPurchases]=useState<ReviewPurchases|null>(null),[ordersLoading,setOrdersLoading]=useState(!orderId),[ordersError,setOrdersError]=useState(false),[retry,setRetry]=useState(0)
 const [error,setError]=useState<''|'login'|'duplicate'|'order'|'submit'|'invalid'>(''),[submitting,setSubmitting]=useState(false),[attempt,setAttempt]=useState<ReviewSubmission>()
 const live=useRef(false),failedOffset=useRef(0),requestGeneration=useRef(0),sending=useRef(false)
 useEffect(()=>{live.current=true;return()=>{live.current=false}},[])
 useEffect(()=>{onPendingChange?.(!!attempt||submitting);return()=>onPendingChange?.(false)},[attempt,submitting,onPendingChange])
 useEffect(()=>{
  if(orderId)return
  const current=++requestGeneration.current;setOrdersLoading(true)
  api.reviewPurchases(productId).then(result=>{if(live.current&&current===requestGeneration.current){setPurchases(result);setOrdersError(false)}}).catch(()=>{if(live.current&&current===requestGeneration.current){failedOffset.current=0;setOrdersError(true)}}).finally(()=>{if(live.current&&current===requestGeneration.current)setOrdersLoading(false)})
 },[api,productId,orderId,retry])
 async function moreOrders(offset:number){
  if(ordersLoading||sending.current)return
  const current=++requestGeneration.current;setOrdersLoading(true)
  try{
   const result=await api.reviewPurchases(productId,offset)
   if(live.current&&current===requestGeneration.current){setPurchases(previous=>({...result,orders:[...new Map([...(previous?.orders??[]),...result.orders].map(o=>[o.id,o])).values()]}));setOrdersError(false)}
  }catch{if(live.current&&current===requestGeneration.current){failedOffset.current=offset;setOrdersError(true)}}
  finally{if(live.current&&current===requestGeneration.current)setOrdersLoading(false)}
 }
 async function submit(){
  if(sending.current||ordersLoading||error==='duplicate')return
  let input=attempt
  if(!input){
   const heading=title.trim(),body=content.trim()
   if(heading.length<2||heading.length>120||body.length<10||body.length>3000){setError('invalid');return}
   input={operation_id:crypto.randomUUID(),product_id:productId,...(selectedOrder?{order_id:selectedOrder}:{}),rating,title:heading,content:body};setAttempt(input)
  }
  sending.current=true;setSubmitting(true);setError('')
  try{await api.submitReview(input);if(live.current){setAttempt(undefined);onSuccess()}}
  catch(e){
   if(!live.current)return
   if(e instanceof CustomerError&&e.status===409&&e.code==='review.already_submitted'){setAttempt(undefined);setError('duplicate')}
   else if(e instanceof CustomerError&&([401,403].includes(e.status)&&e.code!=='review.order_ineligible'))setError('login')
   else if(e instanceof CustomerError&&['review.order_unavailable','review.order_ineligible','review.product_unavailable'].includes(e.code??'')){setAttempt(undefined);setError('order')}
   else if(e instanceof CustomerError&&e.status===400){setAttempt(undefined);setError('invalid')}
   else setError('submit')
  }finally{sending.current=false;if(live.current)setSubmitting(false)}
 }
 const frozen=submitting||!!attempt
 if(purchases?.review)return <p className={styles.notice} role="status">{copy.duplicate} {copy[purchases.review.status]}</p>
 return <form className={antDesign?'my-4 p-4 bg-ui-bg-subtle rounded-md':styles.form} data-testid="review-form" onSubmit={event=>{event.preventDefault();void submit()}}>
  <fieldset disabled={frozen} className={antDesign?'min-w-0 flex flex-col gap-3':styles.fields}>
   <legend>{copy.rating}</legend><div className={styles.ratingOptions}><RatingInput value={rating} onChange={setRating} disabled={frozen} label={copy.rating} starLabels={[1,2,3,4,5].map(value=>copy.stars.replace('{rating}',String(value)))}/></div>
   {orderId?<p className={styles.muted}>{copy.verified}</p>:<>
    <label className={styles.label} htmlFor={`${id}-order`}>{copy.linkOrder}</label>
    <select id={`${id}-order`} value={selectedOrder} onChange={e=>setSelectedOrder(e.target.value)} disabled={ordersLoading||frozen}><option value="">{copy.noOrder}</option>{purchases?.orders.filter(o=>o.eligible).map(o=><option key={o.id} value={o.id}>{copy.orderOption.replace('{number}',o.number).replace('{date}',new Date(o.created_at).toLocaleDateString(intlLocale,{timeZone:'UTC'}))}</option>)}</select>
    <p className={styles.muted}>{copy.orderNote}</p>{ordersLoading&&<p role="status" className={styles.muted}>{copy.loadingOrders}</p>}
    {ordersError&&<p role="alert" className={styles.error}>{copy.purchasesError} <ActionButton type="button" className={antDesign?undefined:styles.link} onClick={()=>failedOffset.current?void moreOrders(failedOffset.current):setRetry(n=>n+1)}>{copy.retry}</ActionButton></p>}
    {purchases&&purchases.offset+purchases.limit<purchases.count&&<ActionButton type="button" disabled={ordersLoading||frozen} className={antDesign?undefined:styles.link} onClick={()=>void moreOrders(purchases.offset+purchases.limit)}>{copy.moreOrders}</ActionButton>}
   </>}
   <label className={styles.label} htmlFor={`${id}-title`}>{copy.title}</label><TitleInput id={`${id}-title`} name="title" required minLength={2} maxLength={120} placeholder={copy.titlePlaceholder} value={title} onChange={e=>setTitle(e.target.value)}/>
   <label className={styles.label} htmlFor={`${id}-content`}>{copy.content}</label><ContentInput id={`${id}-content`} name="content" required minLength={10} maxLength={3000} rows={5} placeholder={copy.contentPlaceholder} value={content} onChange={e=>setContent(e.target.value)}/>
   <p className={styles.muted}>{copy.moderation}</p>
  </fieldset>
  {error&&<p className={styles.error} role="alert">{error==='duplicate'?copy.duplicate:error==='login'?copy.loginError:error==='order'?copy.orderError:error==='invalid'?copy.invalid:copy.submitError}</p>}
  {attempt&&!submitting&&<p className={styles.muted} role="status">{copy.uncertain}</p>}
  <div className={styles.formActions}><ActionButton className={antDesign?undefined:styles.button} type="submit" disabled={ordersLoading||submitting||error==='duplicate'}>{submitting?copy.submitting:attempt?copy.retrySame:copy.submit}</ActionButton><ActionButton className={antDesign?undefined:styles.link} type="button" disabled={frozen} onClick={onCancel}>{copy.cancel}</ActionButton></div>
 </form>
}
