import {estimateProjection} from './buyback-client.ts'
const headers={'Content-Type':'application/json','Cache-Control':'no-store','X-Content-Type-Options':'nosniff'}
const failure=(status:number)=>new Response('{"error":"Buyback pricing unavailable"}',{status,headers})
async function bytes(stream:ReadableStream<Uint8Array>|null,max:number){
 if(!stream)throw new Error()
 const reader=stream.getReader(),chunks:Uint8Array[]=[];let size=0
 try{while(true){const {done,value}=await reader.read();if(done)break;size+=value.byteLength;if(size>max)throw new Error();chunks.push(value)}}finally{await reader.cancel().catch(()=>{});reader.releaseLock()}
 const data=new Uint8Array(size);let at=0;for(const part of chunks){data.set(part,at);at+=part.byteLength}return data
}
// Anonymous read-only pricing. Fixed server origin/path, no browser credentials
// or upstream headers. Preserve original bytes so Gin can reject duplicate keys.
export async function buybackBridge(request:Request,origin:string|undefined,fetcher:typeof fetch=fetch){
 const url=new URL(request.url),estimate=url.pathname==='/api/store/buyback/estimate'&&request.method==='POST',prices=url.pathname==='/api/store/buyback/prices'&&request.method==='GET'
 if(!estimate&&!prices)return failure(404)
 if(estimate&&(url.search||url.href.includes('?')||request.headers.get('content-type')?.split(';')[0]!=='application/json'))return failure(400)
 if(prices){const entries=[...url.searchParams];if(entries.length>1||entries.some(([k,v])=>k!=='product_id'||[...v].length>100||/[\x00-\x1f\x7f]/.test(v)))return failure(400)}
 let target:URL
 try{if(!origin)throw new Error();target=new URL(origin);if(!['http:','https:'].includes(target.protocol)||target.username||target.password||target.pathname!=='/'||target.search||target.hash)throw new Error();target=new URL(url.pathname+url.search,target)}catch{return failure(503)}
 let body:Uint8Array|undefined
 try{if(estimate)body=await bytes(request.body,16384)}catch{return failure(400)}
 const controller=new AbortController(),timer=setTimeout(()=>controller.abort(),5000)
 try{
  const response=await fetcher(target,{method:estimate?'POST':'GET',cache:'no-store',credentials:'omit',redirect:'error',signal:controller.signal,headers:{Accept:'application/json',...(estimate?{'Content-Type':'application/json'}:{})},...(body?{body:body as BodyInit}:{})})
  if(response.status!==200){await response.body?.cancel().catch(()=>{});return failure(response.status===400?400:503)}
  if(response.headers.get('content-type')?.split(';')[0]!=='application/json')throw new Error()
  const raw=JSON.parse(new TextDecoder('utf-8',{fatal:true}).decode(await bytes(response.body,131072)))
  if(estimate)return new Response(JSON.stringify(estimateProjection(raw)),{status:200,headers})
  if(!raw||!Array.isArray(raw.prices)||raw.prices.length>100||Object.keys(raw).length!==1)throw new Error()
  const rows=raw.prices.map((r:Record<string,unknown>)=>{
   if(!r||typeof r!=='object'||Object.keys(r).length!==4||typeof r.product_id!=='string'||r.product_id!==url.searchParams.get('product_id')||typeof r.capacity!=='string'||r.capacity.length>256||typeof r.amount!=='number'||!Number.isFinite(r.amount)||r.amount<0||typeof r.currency_code!=='string'||!r.currency_code||r.currency_code.length>16||/[\x00-\x1f\x7f]/.test(r.currency_code))throw new Error()
   return {product_id:r.product_id,capacity:r.capacity,amount:r.amount,currency_code:r.currency_code}
  })
  return new Response(JSON.stringify({prices:rows}),{status:200,headers})
 }catch{return failure(503)}finally{clearTimeout(timer)}
}
