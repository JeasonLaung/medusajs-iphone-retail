import type {APIRoute} from 'astro'
import {GIN_API_ORIGIN} from 'astro:env/server'
import {loadProduct} from '../../../../lib/products'
export const GET:APIRoute=async({params,url})=>{
 const headers={'Content-Type':'application/json','Cache-Control':'no-store'}
 const fail=(status:number)=>new Response('{"error":"Product unavailable"}',{status,headers})
 if(!params.id||!/^[A-Za-z0-9][A-Za-z0-9_-]{0,190}$/.test(params.id))return fail(400)
 const context:Record<string,string>={}
 for(const [key,value]of url.searchParams){if(!['region_id','quantity'].includes(key)||key in context)return fail(400);context[key]=value}
 try{
  const product=await loadProduct(GIN_API_ORIGIN,params.id,context)
  return product?new Response(JSON.stringify({product,capabilities:{pricing:!!context.region_id,availability:true,checkout:false}}),{headers}):fail(404)
 }catch{return fail(503)}
}
