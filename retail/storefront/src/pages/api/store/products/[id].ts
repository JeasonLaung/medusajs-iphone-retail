import type {APIRoute} from 'astro'
import {GIN_API_ORIGIN} from 'astro:env/server'
import {loadProduct} from '../../../../lib/products'
import {catalogForRequest} from '../../../../lib/customer-catalog-server'
export const GET:APIRoute=async(context)=>{
 const {params,url,request}=context
 const headers={'Content-Type':'application/json','Cache-Control':'private, no-store','Vary':'Cookie'}
 const fail=(status:number)=>new Response('{"error":"Product unavailable"}',{status,headers})
 if(!params.id||!/^[A-Za-z0-9][A-Za-z0-9_-]{0,190}$/.test(params.id))return fail(400)
 const priceContext:Record<string,string>={}
 for(const [key,value]of url.searchParams){if(!['region_id','quantity'].includes(key)||key in priceContext)return fail(400);priceContext[key]=value}
 try{
  const product=await loadProduct(GIN_API_ORIGIN,params.id,priceContext,catalogForRequest(request,()=>context.clientAddress))
  return product?new Response(JSON.stringify({product,capabilities:{pricing:!!priceContext.region_id,availability:true,checkout:false}}),{headers}):fail(404)
 }catch{return fail(503)}
}
