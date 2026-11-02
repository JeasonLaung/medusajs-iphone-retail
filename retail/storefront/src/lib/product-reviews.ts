import {publicJSON} from './gin-public.ts'

export type ReviewStatus='pending'|'approved'|'rejected'
export type ProductReview={id:string;author:string;rating:number;title:string;content:string;created_at:string;verified_purchase:boolean}
export type ProductReviewPage={reviews:ProductReview[];count:number;offset:number;limit:number;average:number;distribution:{rating:number;count:number}[]}
export type ReviewPurchases={orders:{id:string;number:string;created_at:string;eligible:boolean}[];review:{id:string;status:ReviewStatus;verified_purchase:boolean}|null;count:number;offset:number;limit:number}
export type OrderReviewContext={eligible:boolean;reviews:{product_id:string;status:ReviewStatus;verified_purchase:boolean}[]}
export type ReviewSubmission={operation_id:string;product_id:string;order_id?:string;rating:number;title:string;content:string}
export type ReviewReceipt={id:string;status:ReviewStatus;replayed:boolean}
const invalid=()=>new Error('Product reviews unavailable')
const object=(v:unknown):Record<string,any>=>{if(!v||typeof v!=='object'||Array.isArray(v))throw invalid();return v as Record<string,any>}
const text=(v:unknown,max:number,required=false):string=>{if(typeof v!=='string'||v.length>max||required&&!v.length)throw invalid();return v}
const integer=(v:unknown,max=Number.MAX_SAFE_INTEGER):number=>{if(!Number.isSafeInteger(v)||Number(v)<0||Number(v)>max)throw invalid();return Number(v)}
const boolean=(v:unknown):boolean=>{if(typeof v!=='boolean')throw invalid();return v}
const date=(v:unknown):string=>{const s=text(v,64,true);if(!Number.isFinite(Date.parse(s)))throw invalid();return s}
const status=(v:unknown):ReviewStatus=>{if(!['pending','approved','rejected'].includes(String(v))||typeof v!=='string')throw invalid();return v as ReviewStatus}
export const reviewID=(v:unknown):v is string=>typeof v==='string'&&v.length>0&&[...v].length<=255&&!/[\x00-\x1f\x7f/\\?#]/.test(v)
const id=(v:unknown):string=>{if(!reviewID(v))throw invalid();return v}
const orderID=(v:unknown):string=>{if(typeof v!=='string'||!/^[A-Za-z0-9][A-Za-z0-9_-]{0,190}$/.test(v))throw invalid();return v}
const unique=(rows:{id:string}[])=>{if(new Set(rows.map(r=>r.id)).size!==rows.length)throw invalid()}
export function reviewReceipt(raw:unknown):ReviewReceipt{
 const body=object(raw),r=object(body.review)
 if(Object.keys(body).length!==1)throw invalid()
 return {id:id(r.id),status:status(r.status),replayed:boolean(r.replayed)}
}
export function productReviewPage(raw:unknown,offset=0,limit=5):ProductReviewPage{
 const r=object(raw),count=integer(r.count)
 if(r.offset!==offset||r.limit!==limit||!Array.isArray(r.reviews)||r.reviews.length!==Math.min(limit,Math.max(0,count-offset))||!Array.isArray(r.distribution)||r.distribution.length!==5)throw invalid()
 const distribution=r.distribution.map((v:unknown,i:number)=>{const f=object(v);if(f.rating!==i+1)throw invalid();return {rating:i+1,count:integer(f.count)}})
 if(distribution.reduce((n,f)=>n+f.count,0)!==count)throw invalid()
 const average=count?distribution.reduce((n,f)=>n+f.rating*f.count,0)/count:0
 if(typeof r.average!=='number'||!Number.isFinite(r.average)||Math.abs(r.average-average)>1e-12)throw invalid()
 const reviews=r.reviews.map((v:unknown)=>{const f=object(v),rating=integer(f.rating,5);if(rating<1)throw invalid();return {id:id(f.id),author:text(f.author,500),rating,title:text(f.title,2000),content:text(f.content,20000),created_at:date(f.created_at),verified_purchase:boolean(f.verified_purchase)}})
 unique(reviews)
 return {reviews,count,offset,limit,average,distribution}
}
export function reviewPurchases(raw:unknown,offset=0):ReviewPurchases{
 const r=object(raw),count=integer(r.count)
 if(r.offset!==offset||r.limit!==20||!Array.isArray(r.orders)||r.orders.length!==Math.min(20,Math.max(0,count-offset)))throw invalid()
 const orders=r.orders.map((v:unknown)=>{const f=object(v);return {id:orderID(f.id),number:text(f.number,255),created_at:date(f.created_at),eligible:boolean(f.eligible)}});unique(orders)
 const own=r.review===null?null:object(r.review),review=own?{id:id(own.id),status:status(own.status),verified_purchase:boolean(own.verified_purchase)}:null
 return {orders,review,count,offset,limit:20}
}
export function orderReviewContext(raw:unknown):OrderReviewContext{
 const r=object(raw);if(!Array.isArray(r.reviews)||r.reviews.length>1000)throw invalid()
 const reviews=r.reviews.map((v:unknown)=>{const f=object(v);return {product_id:id(f.product_id),status:status(f.status),verified_purchase:boolean(f.verified_purchase)}})
 if(new Set(reviews.map(r=>r.product_id)).size!==reviews.length)throw invalid()
 return {eligible:boolean(r.eligible),reviews}
}
export function reviewQuery(params:URLSearchParams):{productID:string;offset:number;limit:number}{
 for(const [key]of params)if(!['product_id','offset','limit'].includes(key)||params.getAll(key).length!==1)throw invalid()
 const productID=id(params.get('product_id')),parse=(key:string,fallback:number,max:number)=>{const raw=params.get(key);if(raw===null)return fallback;if(!/^(0|[1-9][0-9]*)$/.test(raw))throw invalid();return integer(Number(raw),max)}
 const offset=parse('offset',0,100000),limit=parse('limit',5,20);if(!limit)throw invalid();return {productID,offset,limit}
}
export async function loadProductReviews(origin:string|undefined,productID:string,offset=0,limit=5,fetcher:typeof fetch=fetch):Promise<ProductReviewPage>{
 const params=new URLSearchParams({product_id:productID,offset:String(offset),limit:String(limit)});reviewQuery(params)
 return productReviewPage(await publicJSON(origin,'/api/store/product-reviews?'+params,2*1024*1024,fetcher),offset,limit)
}
