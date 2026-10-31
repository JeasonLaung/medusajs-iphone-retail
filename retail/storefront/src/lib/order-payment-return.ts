import {customerBridge,type CustomerBridgeConfig} from './customer-bridge.ts'
import {customerClient,CustomerError} from './customer-client.ts'
import {paymentUUID,paymentOrderID} from './order-payments.ts'
// The completed cart is only immutable metadata here. A provider navigation
// never reuses its bearer cookie or performs a payment write/observation.
export async function orderPaymentReturn(request:Request,config:CustomerBridgeConfig,fetcher?:typeof fetch):Promise<Response>{
 const headers={'Cache-Control':'private, no-store','Referrer-Policy':'no-referrer','Content-Type':'text/plain; charset=utf-8'},url=new URL(request.url)
 const fail=(status:number)=>new Response(status===401?'Sign in to view your payment in My orders.':'Payment return unavailable',{status,headers})
 if(!config.storefrontOrigin||url.origin!==config.storefrontOrigin||!config.bridgeKey)return fail(503)
 if(request.headers.has('x-forwarded-for'))return fail(403)
 const keys=['cart_id','order_id','order_payment_id']
 if([...url.searchParams.keys()].length!==3||keys.some(k=>url.searchParams.getAll(k).length!==1)||[...url.searchParams.keys()].some(k=>!keys.includes(k)))return fail(404)
 const order=url.searchParams.get('order_id'),operation=url.searchParams.get('order_payment_id')
 if(!paymentOrderID(order)||!paymentUUID(operation)||!paymentOrderID(url.searchParams.get('cart_id')))return fail(404)
 const incoming=new Headers()
 if(request.headers.has('cookie'))incoming.set('Cookie',request.headers.get('cookie')!)
 const api=customerClient(path=>customerBridge(new Request(new URL(String(path),url),{headers:incoming}),config,fetcher))
 try{
  await api.orderPayment(order,operation)
  const detail=await api.order(order),country=detail.shipping_address.country_code,language=detail.locale.toLowerCase().startsWith('zh')?'zh':'en'
  if(!/^[a-z]{2}$/.test(country))return fail(503)
  return new Response(null,{status:303,headers:{...headers,Location:`/${country}-${language}/account/orders/details/${encodeURIComponent(order)}#order-supplemental-payment`}})
 }catch(e){return fail(e instanceof CustomerError&&[401,404].includes(e.status)?e.status:503)}
}
