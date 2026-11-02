import type {APIRoute} from 'astro'
import {GIN_API_ORIGIN} from 'astro:env/server'
import {loadShippingDemo} from '../../../../lib/shipping-demo'
import {PUBLIC_NOT_FOUND} from '../../../../lib/gin-public'
const headers={'Cache-Control':'no-store','Content-Type':'application/json','X-Content-Type-Options':'nosniff'}
export const GET:APIRoute=async()=>{
 try{
  const result=await loadShippingDemo(GIN_API_ORIGIN)
  if(result===PUBLIC_NOT_FOUND)return new Response('{"code":"not_found"}',{status:404,headers})
  return new Response(JSON.stringify(result),{headers})
 }catch{return new Response('{"code":"DHL_DEMO_UNAVAILABLE"}',{status:502,headers})}
}
