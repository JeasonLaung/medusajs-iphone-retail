// Source: Medusa product-reviews widget; Ant components and current Gin authority.
import {useEffect,useRef,useState} from 'react'
import {Alert,Button,Empty,List,Pagination,Space,Spin,Tag,Typography} from 'antd'
import {Link} from 'react-router-dom'
import {APIError} from './api'
import {useAccess} from './Permissions'
import {moderateProductReview,productReviews,type ProductReviewPage,type ReviewCommand,type ReviewStatus} from './product-reviews-api'
import type {Copy} from './i18n'

export default function ProductReviews({productID,copy,onExpired}:{productID:string;copy:Copy;onExpired():void}){
 const access=useAccess(),zh=copy.language==='语言',canWrite=access.can('product.read','product.write'),canOrder=access.can('order.read')
 const text=zh?{heading:'商品评价',description:'审核客户提交的评价，通过后公开展示。',customer:'客户',empty:'暂无评价',verified:'已验证购买',open:'开放评价',order:'查看订单',approve:'批准发布',reject:'拒绝发布',pending:'设为待审核',approved:'已发布',rejected:'未发布',awaiting:'待审核',closed:'评价写入尚未启用',conflict:'评价已变更，请重新加载后操作。',reload:'重新加载评价',uncertain:'尚未确认审核结果，请重试同一请求。'}:{heading:'Product reviews',description:'Moderate customer reviews before publishing them.',customer:'Customer',empty:'No reviews yet',verified:'Verified purchase',open:'Open review',order:'View order',approve:'Approve',reject:'Reject',pending:'Set to pending',approved:'Published',rejected:'Not published',awaiting:'Pending',closed:'Review writes are not enabled',conflict:'The review changed. Reload before continuing.',reload:'Reload reviews',uncertain:'The moderation result is not confirmed. Retry the same request.'}
 const labels:Record<ReviewStatus,string>={pending:text.awaiting,approved:text.approved,rejected:text.rejected}
 const [offset,setOffset]=useState(0),[reload,setReload]=useState(0),[page,setPage]=useState<ProductReviewPage>(),[loading,setLoading]=useState(true),[missing,setMissing]=useState(false),[loadError,setLoadError]=useState(false)
 const [attempt,setAttempt]=useState<{id:string;input:ReviewCommand}>(),[saving,setSaving]=useState(false),[error,setError]=useState<''|'uncertain'|'conflict'|'denied'|'invalid'>('')
 const busy=useRef(false),live=useRef(false)
 useEffect(()=>{live.current=true;return()=>{live.current=false}},[])
 useEffect(()=>{
  const abort=new AbortController();setLoading(true);setLoadError(false)
  productReviews(productID,offset,abort.signal).then(result=>{if(!abort.signal.aborted){setPage(result);setMissing(false)}}).catch(e=>{if(abort.signal.aborted)return;if(e instanceof APIError&&e.status===401)onExpired();else if(e instanceof APIError&&e.status===404)setMissing(true);else setLoadError(true)}).finally(()=>{if(!abort.signal.aborted)setLoading(false)})
  return()=>abort.abort()
 },[productID,offset,reload,onExpired])
 async function save(command:NonNullable<typeof attempt>){
  if(busy.current)return
  busy.current=true;setAttempt(command);setSaving(true);setError('')
  try{await moderateProductReview(command.id,command.input);if(live.current){setAttempt(undefined);setReload(n=>n+1)}}
  catch(e){if(!live.current)return;if(e instanceof APIError&&e.status===401){onExpired();setError('denied')}
   else if(e instanceof APIError&&[400,403,404,409].includes(e.status)){setAttempt(undefined);setError(e.status===409?'conflict':e.status===403?'denied':'invalid')}
   else setError('uncertain')
  }finally{busy.current=false;if(live.current)setSaving(false)}
 }
 const disabled=loading||loadError||saving||!!attempt||!!error||!canWrite||!page?.enabled
 if(missing)return null
 return <section data-testid="admin-product-reviews"><Typography.Title level={4}>{text.heading}</Typography.Title><Typography.Paragraph type="secondary">{text.description}</Typography.Paragraph>
  <Space direction="vertical" size={16} className="full-width">
   {loadError&&<Alert type="error" showIcon message={copy.unavailable} action={<Button disabled={saving||!!attempt} onClick={()=>setReload(n=>n+1)}>{copy.retry}</Button>}/>}
   {page&&!page.enabled&&<Alert type="info" showIcon message={text.closed}/>}
   {error&&<Alert type={error==='uncertain'?'warning':'error'} showIcon message={error==='uncertain'?text.uncertain:error==='conflict'?text.conflict:error==='denied'?copy.accessDenied:copy.unavailable} action={error==='uncertain'&&attempt?<Button loading={saving} onClick={()=>void save(attempt)}>{copy.retrySave}</Button>:<Button disabled={saving} onClick={()=>{setError('');setReload(n=>n+1)}}>{text.reload}</Button>}/>}
   {loading?<Spin aria-label={copy.loadingProduct}/>:page&&<>
    <List dataSource={page.reviews} locale={{emptyText:<Empty description={text.empty}/>}} renderItem={review=><List.Item key={review.id} data-testid="admin-product-review" data-review-id={review.id}>
     <Space direction="vertical" className="full-width" size={8}>
      <Space wrap><Typography.Text strong>{review.author||text.customer}</Typography.Text><Tag color={review.status==='approved'?'success':review.status==='rejected'?'error':'warning'}>{labels[review.status]}</Tag><Typography.Text type="secondary">{review.rating} / 5 · {new Date(review.created_at).toLocaleDateString(zh?'zh-CN':'en-GB',{timeZone:'UTC'})}</Typography.Text></Space>
      <Space wrap><Tag color={review.verified_purchase?'success':undefined}>{review.verified_purchase?text.verified:text.open}</Tag>{review.order_id&&canOrder&&<Link to={'/orders/'+encodeURIComponent(review.order_id)}>{text.order}</Link>}</Space>
      <Typography.Text strong style={{overflowWrap:'anywhere'}}>{review.title}</Typography.Text><Typography.Paragraph style={{whiteSpace:'pre-wrap',overflowWrap:'anywhere',marginBottom:0}}>{review.content}</Typography.Paragraph>
      {canWrite&&<Space wrap>{(['approved','rejected','pending'] as ReviewStatus[]).filter(status=>status!==review.status).map(status=><Button key={status} disabled={disabled} onClick={()=>void save({id:review.id,input:{operation_id:crypto.randomUUID(),expected_revision:review.revision,status}})}>{status==='approved'?text.approve:status==='rejected'?text.reject:text.pending}</Button>)}</Space>}
     </Space>
    </List.Item>}/>
    {page.count>5&&<Pagination size="small" simple current={offset/5+1} total={page.count} pageSize={5} disabled={saving||!!attempt||!!error} onChange={p=>setOffset((p-1)*5)}/>}
   </>}
  </Space>
 </section>
}
