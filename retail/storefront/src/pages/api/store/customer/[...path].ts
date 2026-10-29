import type {APIRoute} from 'astro'
import {GIN_API_ORIGIN,STOREFRONT_ORIGIN,CUSTOMER_BRIDGE_KEY} from 'astro:env/server'
import {customerBridge} from '../../../../lib/customer-bridge'
export const GET:APIRoute=({request,clientAddress})=>{
 // Astro's Node adapter may derive clientAddress from X-Forwarded-For when
 // forwarded-host validation succeeds. No proxy is trusted in this runtime.
 if(request.headers.has('x-forwarded-for'))return new Response('{"error":{"code":"customer.invalid_proxy"}}',{status:403,headers:{'Content-Type':'application/json','Cache-Control':'private, no-store','Referrer-Policy':'no-referrer'}})
 if(!CUSTOMER_BRIDGE_KEY)return new Response('{"error":{"code":"customer.unavailable"}}',{status:503,headers:{'Content-Type':'application/json','Cache-Control':'private, no-store','Referrer-Policy':'no-referrer'}})
 return customerBridge(request,{ginOrigin:GIN_API_ORIGIN,storefrontOrigin:STOREFRONT_ORIGIN,bridgeKey:CUSTOMER_BRIDGE_KEY,clientAddress})
}
export const POST:APIRoute=GET
