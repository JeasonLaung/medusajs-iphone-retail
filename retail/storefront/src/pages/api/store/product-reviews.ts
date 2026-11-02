import type {APIRoute} from 'astro'
import {GIN_API_ORIGIN} from 'astro:env/server'
import {loadProductReviews,reviewQuery} from '../../../lib/product-reviews'
const headers={'Cache-Control':'no-store','Content-Type':'application/json','X-Content-Type-Options':'nosniff'}
export const GET:APIRoute=async({url})=>{
 let q:ReturnType<typeof reviewQuery>
 try{q=reviewQuery(url.searchParams)}catch{return new Response('{"error":{"code":"review.invalid_request"}}',{status:400,headers})}
 try{return new Response(JSON.stringify(await loadProductReviews(GIN_API_ORIGIN,q.productID,q.offset,q.limit)),{headers})}
 catch{return new Response('{"error":{"code":"review.unavailable"}}',{status:503,headers})}
}
