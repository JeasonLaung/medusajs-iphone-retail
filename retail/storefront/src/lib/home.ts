import {publicJSON} from './gin-public.ts'
import {loadProducts} from './products.ts'
import type {FooterData} from './footer-data.ts'

const families=['17','Air','16','15','14','13','12','11','SE']
const images:Record<string,string>={'17':'iphone-17/series.webp',Air:'iphone-air/series.webp','16':'iphone-16/series.webp','15':'iphone-15/series.webp','14':'iphone-14/series.webp','13':'iphone-13.png','12':'iphone-12.png','11':'iphone-11.png',SE:'iphone-se.png'}
export type HomeReview={id:string;author:string;rating:number;title:string;content:string;created_at:string;verified_purchase:boolean;product:{id:string;title:string;handle:string}}
export type HomeReviewsData={reviews:HomeReview[];unavailable:boolean}
export type HomeData={series:{family:string;category:{id:string;name:string;image:string}}[];reviews:HomeReviewsData}
const unavailable=()=>new Error('Home data unavailable')

// Reuse the public iPhone parent's direct children. Fixed source family order,
// not API order, determines both cards and the first three carousel slides.
export function homeSeries(footer:FooterData):HomeData['series']{
 return families.flatMap(family=>{
  const category=footer.iphoneCategory?.category_children.find(c=>c.handle===`iphone-${family.toLowerCase()}-series`)
  return category?[{family,category:{id:category.id,name:category.name,image:`/iphone-series/${images[family]}`}}]:[]
 })
}

// Original public review contract. Gin review storage/moderation is a pending
// migration; a missing/failed optional endpoint retains the source error state.
export async function loadHomeReviewPage(origin:string|undefined,product:HomeReview['product'],fetcher:typeof fetch=fetch):Promise<HomeReview[]|null>{
 try{
  const body=await publicJSON(origin,`/api/store/product-reviews?${new URLSearchParams({product_id:product.id,offset:'0',limit:'5'})}`,128*1024,fetcher) as Record<string,unknown>
  if(!body||!Array.isArray(body.reviews)||body.offset!==0||body.limit!==5||!Number.isSafeInteger(body.count)||Number(body.count)<0||body.reviews.length!==Math.min(Number(body.count),5))throw unavailable()
  const rows=body.reviews.map(raw=>{
   if(!raw||typeof raw!=='object'||Array.isArray(raw))throw unavailable()
   const r=raw as Record<string,unknown>
   const text=(key:string,max:number)=>{const v=r[key];if(typeof v!=='string'||v.length>max)throw unavailable();return v}
   const id=text('id',191),created_at=text('created_at',64)
   if(!id||!Number.isInteger(r.rating)||Number(r.rating)<1||Number(r.rating)>5||!Number.isFinite(Date.parse(created_at))||typeof r.verified_purchase!=='boolean')throw unavailable()
   return {id,created_at,rating:Number(r.rating),author:text('author',500),title:text('title',2000),content:text('content',20000),verified_purchase:r.verified_purchase,product:{...product}}
  })
  if(new Set(rows.map(r=>r.id)).size!==rows.length)throw unavailable()
  return rows
 }catch{return null}
}

export async function loadHome(origin:string|undefined,footer:FooterData,fetcher:typeof fetch=fetch):Promise<HomeData>{
 const deadline=new AbortController(),timer=setTimeout(()=>deadline.abort(),8000)
 const boundedFetch:typeof fetch=(input,init)=>{
  if(deadline.signal.aborted)return Promise.reject(unavailable())
  return fetcher(input,{...init,signal:init?.signal?AbortSignal.any([init.signal,deadline.signal]):deadline.signal})
 }
 try{
 const series=homeSeries(footer),products=new Map<string,HomeReview['product']>()
 // Bound relation reads and never silently render an incomplete category.
 let budget=1000
 for(const {category} of series){
  let offset=0,count=0
  do{
   const page=await loadProducts(origin,{category_id:category.id,limit:100,offset},boundedFetch)
   if(offset&&page.count!==count)throw unavailable()
   count=page.count
   if(count>1000||!page.products.length&&offset<count)throw unavailable()
   budget-=page.products.length;if(budget<0)throw unavailable()
   for(const p of page.products)products.set(p.id,{id:p.id,title:p.title,handle:p.handle})
   offset+=page.products.length
  }while(offset<count)
 }
 const queue=[...products.values()],pages:(HomeReview[]|null)[]=[]
 // Public optional reads, with bounded concurrency rather than one connection
 // per product. Completion order cannot alter ties in source product order.
 let cursor=0
 await Promise.all(Array.from({length:Math.min(4,queue.length)},async()=>{
  while(cursor<queue.length){const i=cursor++;pages[i]=await loadHomeReviewPage(origin,queue[i],boundedFetch)}
 }))
 const reviews=pages.flatMap(page=>page??[]).sort((a,b)=>Date.parse(b.created_at)-Date.parse(a.created_at)).slice(0,3)
 return {series,reviews:{reviews,unavailable:queue.length>0&&pages.every(page=>page===null)}}
 }finally{clearTimeout(timer)}
}
