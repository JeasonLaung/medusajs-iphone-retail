import {APIError,csrfCookie,request} from './api'
export type ReviewStatus='pending'|'approved'|'rejected'
export type ProductReview={id:string;author:string;rating:number;title:string;content:string;created_at:string;verified_purchase:boolean;status:ReviewStatus;order_id:string|null;revision:string}
export type ProductReviewPage={reviews:ProductReview[];count:number;offset:number;limit:5;enabled:boolean}
export type ReviewCommand={operation_id:string;expected_revision:string;status:ReviewStatus}
const invalid=()=>new APIError(0,'request.invalid_response')
const id=(v:unknown):v is string=>typeof v==='string'&&v.length>0&&[...v].length<=255&&!/[\x00-\x1f\x7f/\\?#]/.test(v)
const status=(v:unknown):v is ReviewStatus=>typeof v==='string'&&['pending','approved','rejected'].includes(v)
const revision=(v:unknown):v is string=>typeof v==='string'&&/^[0-9a-f]{64}$/.test(v)
export async function productReviews(productID:string,offset=0,signal?:AbortSignal):Promise<ProductReviewPage>{
 if(!id(productID)||!Number.isSafeInteger(offset)||offset<0||offset>100000)throw new APIError(400,'review.invalid_request')
 const raw=await request<ProductReviewPage>('product-reviews?'+new URLSearchParams({product_id:productID,offset:String(offset),limit:'5'}),{signal},'/api/admin/')
 if(!raw||!Number.isSafeInteger(raw.count)||raw.count<0||raw.offset!==offset||raw.limit!==5||typeof raw.enabled!=='boolean'||!Array.isArray(raw.reviews)||raw.reviews.length!==Math.min(5,Math.max(0,raw.count-offset)))throw invalid()
 const reviews=raw.reviews.map(r=>{
  if(!r||!id(r.id)||!status(r.status)||!revision(r.revision)||!Number.isInteger(r.rating)||r.rating<1||r.rating>5||typeof r.author!=='string'||r.author.length>500||typeof r.title!=='string'||r.title.length>2000||typeof r.content!=='string'||r.content.length>20000||typeof r.created_at!=='string'||!Number.isFinite(Date.parse(r.created_at))||typeof r.verified_purchase!=='boolean'||r.order_id!==null&&!id(r.order_id))throw invalid()
  return {id:r.id,author:r.author,rating:r.rating,title:r.title,content:r.content,created_at:r.created_at,verified_purchase:r.verified_purchase,status:r.status,order_id:r.order_id,revision:r.revision}
 })
 if(new Set(reviews.map(r=>r.id)).size!==reviews.length)throw invalid()
 return {reviews,count:raw.count,offset,limit:5,enabled:raw.enabled}
}
export async function moderateProductReview(idValue:string,input:ReviewCommand):Promise<void>{
 if(!id(idValue)||!status(input.status)||!revision(input.expected_revision)||!/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(input.operation_id))throw new APIError(400,'review.invalid_request')
 const csrf=csrfCookie();if(!csrf)throw new APIError(403,'auth.invalid_csrf')
 const raw=await request<{review:{id:string;status:ReviewStatus;replayed:boolean}}>('product-reviews/'+encodeURIComponent(idValue),{method:'POST',body:input,csrf},'/api/admin/')
 if(!raw||Object.keys(raw).length!==1||raw.review?.id!==idValue||raw.review.status!==input.status||typeof raw.review.replayed!=='boolean')throw invalid()
}
