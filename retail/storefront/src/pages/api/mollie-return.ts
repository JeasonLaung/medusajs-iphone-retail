import type {APIRoute} from 'astro'
import {GIN_API_ORIGIN,STOREFRONT_ORIGIN} from 'astro:env/server'
import {CartClient} from '../../lib/cart-client'
import {cartBridge} from '../../lib/cart-bridge'
// Stable original callback URL. GET only routes to the status page; it never
// confirms payment, accepts a remote status or performs a provider operation.
export const GET:APIRoute=async({request,url})=>{
 const headers={'Cache-Control':'private, no-store','Referrer-Policy':'no-referrer'}
 if(!STOREFRONT_ORIGIN||url.origin!==STOREFRONT_ORIGIN)return new Response('Payment return unavailable',{status:503,headers})
 try{
  const incoming=new Headers();for(const name of ['Cookie','Origin','Sec-Fetch-Site'])if(request.headers.has(name))incoming.set(name,request.headers.get(name)!)
  // Cross-site navigation from the provider is expected. Do not forward its
  // Origin as a trusted write: these two requests are private reads only.
  incoming.delete('Origin');incoming.delete('Sec-Fetch-Site')
  const client=new CartClient({readScopeCSRF:()=>undefined,fetcher:path=>cartBridge(new Request(new URL(String(path),url),{headers:incoming}),{ginOrigin:GIN_API_ORIGIN,storefrontOrigin:STOREFRONT_ORIGIN})})
  const cart=await client.current();if(!cart)return new Response('Payment session unavailable',{status:404,headers})
  const ids=url.searchParams.getAll('cart_id');if(ids.length>1||ids.length===1&&ids[0]!==cart.id)return new Response('Payment session unavailable',{status:404,headers})
  const contacts=await client.retrieveContacts(cart.id);if(!contacts||!contacts.shipping_address)return new Response('Payment return unavailable',{status:503,headers})
  const language=cart.locale.toLowerCase().startsWith('zh')?'zh':'en',country=contacts.shipping_address.country_code
  return new Response(null,{status:303,headers:{...headers,Location:`/${country}-${language}/payment-result?cart_id=${encodeURIComponent(cart.id)}`}})
 }catch{return new Response('Payment return unavailable',{status:503,headers})}
}
