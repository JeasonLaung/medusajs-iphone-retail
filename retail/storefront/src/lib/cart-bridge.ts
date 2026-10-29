// Private, fixed-path transport. Public catalog requests must continue using
// publicJSON: they must never acquire these credential forwarding rules.
import {ginPrivateTransport} from './gin-private-transport.ts'
export type CartBridgeConfig={ginOrigin:string|undefined;storefrontOrigin:string|undefined}
const id='[A-Za-z0-9][A-Za-z0-9_-]{0,190}'
const read=new RegExp(`^/api/store/(?:cart|carts/${id}(?:/(?:amounts|view|contacts|delivery-options))?)$`)
const write=new RegExp(`^/api/store/(?:cart-sessions|carts|carts/${id}/(?:contacts|shipping-methods|payment-status|complete|line-items(?:/${id})?))$`)
const privateHeaders={'Cache-Control':'private, no-store','Content-Type':'application/json','X-Content-Type-Options':'nosniff','Vary':'Cookie, Origin'}
const failure=(status:number)=>new Response('{"error":"Cart request unavailable"}',{status,headers:privateHeaders})
async function bytes(body:ReadableStream<Uint8Array>|null,max:number):Promise<Uint8Array>{
 if(!body)return new Uint8Array()
 const reader=body.getReader(),chunks:Uint8Array[]=[];let size=0
 try{
  while(true){const {done,value}=await reader.read();if(done)break;size+=value.byteLength;if(size>max)throw new Error('size');chunks.push(value)}
 }finally{await reader.cancel().catch(()=>{});reader.releaseLock()}
 const out=new Uint8Array(size);let at=0;for(const chunk of chunks){out.set(chunk,at);at+=chunk.length}return out
}
function origin(raw:string|undefined,upstream=false):URL{
 if(!raw)throw new Error()
 const url=new URL(raw)
 if(!['https:','http:'].includes(url.protocol)||url.origin!==raw||url.username||url.password)throw new Error()
 if(!upstream&&url.protocol==='http:'&&!['localhost','127.0.0.1','[::1]'].includes(url.hostname))throw new Error()
 return url
}

export async function cartBridge(request:Request,config:CartBridgeConfig,fetcher:typeof fetch=ginPrivateTransport):Promise<Response>{
 const requestURL=new URL(request.url),isWrite=request.method==='POST',path=requestURL.pathname
 if(requestURL.search||!(request.method==='GET'&&read.test(path)||isWrite&&write.test(path)))return failure(404)
 let upstream:URL,shop:URL
 try{upstream=origin(config.ginOrigin,true);shop=origin(config.storefrontOrigin)}catch{return failure(503)}
 if(requestURL.origin!==shop.origin||request.headers.get('sec-fetch-site')==='cross-site'||request.headers.has('origin')&&request.headers.get('origin')!==shop.origin||isWrite&&request.headers.get('origin')!==shop.origin)return failure(403)
 const secure=shop.protocol==='https:',prefix=secure?'__Host-retail_cart_':'retail_cart_'
 const names=['scope','scope_csrf','possession','csrf'].map(k=>prefix+k)
 const incoming=request.headers.get('cookie')??''
 if(incoming.length>32768)return failure(400)
 const cookies:string[]=[],seen=new Set<string>()
 for(const part of incoming.split(';')){
  const at=part.indexOf('='),name=part.slice(0,at).trim(),value=part.slice(at+1).trim()
  if(at<0||!names.includes(name))continue
  if(seen.has(name)||!/^[0-9a-f]{64}$/.test(value))return failure(401)
  seen.add(name);cookies.push(`${name}=${value}`)
 }
 const headers=new Headers({Accept:'application/json',Host:shop.host})
 if(cookies.length)headers.set('Cookie',cookies.join('; '))
 // Preserve the configured public origin for both Gin's general Host guard
 // and its independently configured cart origin check. Never trust forwarded
 // headers or synthesize a trusted Origin for a browser's untrusted request.
 if(request.headers.has('origin'))headers.set('Origin',request.headers.get('origin')!)
 if(request.headers.has('sec-fetch-site'))headers.set('Sec-Fetch-Site',request.headers.get('sec-fetch-site')!)
 let body:Uint8Array|undefined
 if(isWrite){
  if(request.headers.get('content-type')?.split(';',1)[0].trim().toLowerCase()!=='application/json')return failure(415)
  for(const name of ['X-CSRF-Token','Idempotency-Key']){
   const value=request.headers.get(name)
   if(value){if(value.length>128||/[\x00-\x1f\x7f]/.test(value))return failure(400);headers.set(name,value)}
  }
  headers.set('Content-Type','application/json')
  try{body=await bytes(request.body,16384)}catch{return failure(413)}
 }
 const controller=new AbortController(),timer=setTimeout(()=>controller.abort(),11000)
 try{
  const response=await fetcher(new URL(path,upstream),{method:request.method,headers,body:body as BodyInit|undefined,credentials:'omit',redirect:'error',cache:'no-store',signal:AbortSignal.any([request.signal,controller.signal])})
  const status=response.status,resultHeaders=new Headers(privateHeaders)
  const requestID=response.headers.get('X-Request-ID'),retry=response.headers.get('Retry-After')
  if(requestID&&/^req_[0-9a-f-]{36}$/.test(requestID))resultHeaders.set('X-Request-ID',requestID)
  if(status===429&&retry&&/^(0|[1-9][0-9]{0,5})$/.test(retry))resultHeaders.set('Retry-After',retry)
  if(status!==200&&status!==201){
   await response.body?.cancel().catch(()=>{})
   return new Response('{"error":"Cart request unavailable"}',{status:[400,401,403,404,409,413,415,422,429].includes(status)?status:503,headers:resultHeaders})
  }
  if(response.headers.get('cache-control')!=='private, no-store'||response.headers.get('content-type')?.split(';',1)[0].trim().toLowerCase()!=='application/json')throw new Error()
  const data=await bytes(response.body,4*1024*1024)
  JSON.parse(new TextDecoder('utf-8',{fatal:true}).decode(data))
  const setCookies=response.headers.getSetCookie(),issued=new Set<string>()
  if(setCookies.length>2)throw new Error()
  for(const cookie of setCookies){
   if(cookie.length>1024)throw new Error()
   const parts=cookie.split(';').map(p=>p.trim()),[name,value,...extra]=parts[0].split('='),attrs=parts.slice(1).map(p=>p.toLowerCase())
   const expected=path==='/api/store/cart-sessions'?['scope','scope_csrf']:path==='/api/store/carts'?['possession','csrf']:[]
   if(!isWrite||!expected.includes(name.slice(prefix.length))||!name.startsWith(prefix)||!/^[0-9a-f]{64}$/.test(value)||extra.length||issued.has(name)||!attrs.includes('path=/')||!attrs.includes('samesite=lax')||attrs.some(a=>a.startsWith('domain='))||attrs.includes('secure')!==secure||attrs.includes('httponly')!==['scope','possession'].includes(name.slice(prefix.length)))throw new Error()
   const attributeNames=new Set<string>()
   for(const attribute of parts.slice(1)){
    const at=attribute.indexOf('='),key=(at<0?attribute:attribute.slice(0,at)).toLowerCase(),v=at<0?'':attribute.slice(at+1)
    if(attributeNames.has(key)||!['path','max-age','expires','httponly','secure','samesite'].includes(key))throw new Error()
    attributeNames.add(key)
    if(key==='path'&&v!=='/'||key==='max-age'&&(!/^[1-9][0-9]{0,9}$/.test(v)||Number(v)>2147483647)||key==='expires'&&!Number.isFinite(Date.parse(v))||['httponly','secure'].includes(key)&&at>=0||key==='samesite'&&v.toLowerCase()!=='lax')throw new Error()
   }
   issued.add(name);resultHeaders.append('Set-Cookie',cookie)
  }
  if(setCookies.length===1)throw new Error()
  return new Response(data as BodyInit,{status,headers:resultHeaders})
 }catch{return failure(503)}finally{clearTimeout(timer)}
}
