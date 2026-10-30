import {ginPrivateTransport} from './gin-private-transport.ts'
import {customerProof} from './customer-proof.ts'

export type CustomerBridgeConfig={ginOrigin:string|undefined;storefrontOrigin:string|undefined;bridgeKey?:string;clientAddress?:string}
const root='/api/store/customer'
const writes=new Set(['challenge','register','login','verify','resend','logout'].map(name=>`${root}/auth/${name}`))
const defaults={'Cache-Control':'private, no-store','Content-Type':'application/json','X-Content-Type-Options':'nosniff','Referrer-Policy':'no-referrer','Vary':'Cookie, Origin'}
const failure=(status:number)=>new Response('{"error":{"code":"customer.request_unavailable","message":"Customer request unavailable"}}',{status,headers:defaults})
async function bytes(body:ReadableStream<Uint8Array>|null,max:number):Promise<Uint8Array>{
 if(!body)return new Uint8Array()
 const reader=body.getReader(),chunks:Uint8Array[]=[];let size=0
 try{while(true){const {done,value}=await reader.read();if(done)break;size+=value.byteLength;if(size>max)throw new Error();chunks.push(value)}}
 finally{await reader.cancel().catch(()=>{});reader.releaseLock()}
 const out=new Uint8Array(size);let at=0;for(const chunk of chunks){out.set(chunk,at);at+=chunk.length}return out
}
function origin(raw:string|undefined,upstream=false):URL{
 if(!raw)throw new Error()
 const url=new URL(raw)
 if(!['http:','https:'].includes(url.protocol)||url.origin!==raw||url.username||url.password)throw new Error()
 if(!upstream&&url.protocol==='http:'&&!['localhost','127.0.0.1','[::1]'].includes(url.hostname))throw new Error()
 return url
}
// The public route requires a key and adapter-observed client address. Browser
// forwarded headers are never copied into the signed request.
export async function customerBridge(request:Request,config:CustomerBridgeConfig,fetcher:typeof fetch=ginPrivateTransport):Promise<Response>{
 const url=new URL(request.url),post=request.method==='POST',path=url.pathname,action=path.slice((root+'/auth/').length)
 const orderList=request.method==='GET'&&path===root+'/orders',orderRead=request.method==='GET'&&new RegExp('^'+root+'/orders/[A-Za-z0-9][A-Za-z0-9_-]{0,190}(/progress|/devices|/after-sales)?$').test(path),orderAttach=post&&new RegExp('^'+root+'/orders/[A-Za-z0-9][A-Za-z0-9_-]{0,190}/attach$').test(path)
 const afterSalesWrite=post&&new RegExp('^'+root+'/orders/[A-Za-z0-9][A-Za-z0-9_-]{0,190}/after-sales$').test(path)
 const notices=path===root+'/notifications'
 const buybackCreate=post&&path===root+'/buyback/requests'
 const target=path+url.search
 const queryOK=(orderList||notices&&request.method==='GET')&&/^\?offset=(0|[1-9][0-9]{0,5})$/.test(url.search)&&Number(url.searchParams.get('offset'))<=100000
 const address=path===root+'/addresses',addressWrite=new RegExp('^'+root+'/addresses/addr_[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}(/delete)?$').test(path)
 if((url.href.includes('?')&&!queryOK)||!(post&&(writes.has(path)||path===root+'/me'||address||addressWrite||orderAttach||afterSalesWrite||notices||buybackCreate)||request.method==='GET'&&(path===root+'/me'||address||orderList||orderRead||notices)))return failure(404)
 let upstream:URL,shop:URL
 try{upstream=origin(config.ginOrigin,true);shop=origin(config.storefrontOrigin)}catch{return failure(503)}
 if(url.origin!==shop.origin||request.headers.get('sec-fetch-site')==='cross-site'||request.headers.has('origin')&&request.headers.get('origin')!==shop.origin||post&&request.headers.get('origin')!==shop.origin)return failure(403)
 const secure=shop.protocol==='https:',prefix=secure?'__Host-retail_customer_':'retail_customer_',names=['prelogin','session','csrf'].map(name=>prefix+name)
 const cartCookie=secure?'__Host-retail_cart_possession':'retail_cart_possession'
 const forwardedNames=orderAttach?[...names,cartCookie]:names
 const headers=new Headers({Accept:'application/json',Host:shop.host}),cookies:string[]=[],seen=new Set<string>()
 const incoming=request.headers.get('cookie')??''
 if(incoming.length>32768)return failure(400)
 for(const part of incoming.split(';')){
  const at=part.indexOf('='),name=part.slice(0,at).trim(),value=part.slice(at+1).trim()
  if(at<0||!forwardedNames.includes(name))continue
  if(seen.has(name)||!/^[0-9a-f]{64}$/.test(value))return failure(401)
  seen.add(name);cookies.push(`${name}=${value}`)
 }
 if(cookies.length)headers.set('Cookie',cookies.join('; '))
 for(const name of ['Origin','Sec-Fetch-Site'])if(request.headers.has(name))headers.set(name,request.headers.get(name)!)
 let body:Uint8Array|undefined
 if(post){
  if(request.headers.get('content-type')?.split(';',1)[0].trim().toLowerCase()!=='application/json')return failure(415)
  const csrf=request.headers.get('x-csrf-token')
  if(csrf!==null){if(!/^[0-9a-f]{64}$/.test(csrf))return failure(400);headers.set('X-CSRF-Token',csrf)}
  headers.set('Content-Type','application/json')
  try{body=await bytes(request.body,16384)}catch{return failure(413)}
 }
 const controller=new AbortController(),timer=setTimeout(()=>controller.abort(),10000)
 try{
  if(config.bridgeKey!==undefined){
   const proof=customerProof(config.bridgeKey,config.clientAddress??'',request.method,target,headers,body)
   proof.forEach((value,name)=>headers.set(name,value))
  }
  const response=await fetcher(new URL(target,upstream),{method:request.method,headers,body:body as BodyInit|undefined,redirect:'error',credentials:'omit',cache:'no-store',signal:AbortSignal.any([request.signal,controller.signal])})
  const status=response.status,out=new Headers(defaults)
  if((orderList||orderRead||orderAttach||afterSalesWrite||notices||buybackCreate)&&status===202||buybackCreate&&status===200)throw new Error()
  const accepted=((afterSalesWrite||buybackCreate)&&status===201)||[200,202,400,401,403,404,409,413,415,429,503].includes(status)
  if(!accepted){await response.body?.cancel().catch(()=>{});return failure(503)}
  if(response.headers.get('cache-control')!=='private, no-store'||response.headers.get('content-type')?.split(';',1)[0].trim().toLowerCase()!=='application/json')throw new Error()
  const data=await bytes(response.body,orderList||orderRead?4*1024*1024:65536),parsed=JSON.parse(new TextDecoder('utf-8',{fatal:true}).decode(data))
  if(!parsed||typeof parsed!=='object'||Array.isArray(parsed))throw new Error()
  const success=status===200||status===202||(afterSalesWrite||buybackCreate)&&status===201,issued=new Map<string,boolean>()
  for(const cookie of response.headers.getSetCookie()){
   if(cookie.length>1024)throw new Error()
   const parts=cookie.split(';').map(part=>part.trim()),[name,value,...extra]=parts[0].split('='),kind=name.slice(prefix.length),attrs=new Map<string,string>()
   if(!names.includes(name)||issued.has(kind)||extra.length)throw new Error()
   for(const part of parts.slice(1)){
    const at=part.indexOf('='),key=(at<0?part:part.slice(0,at)).toLowerCase(),v=at<0?'':part.slice(at+1)
    if(attrs.has(key)||!['path','expires','max-age','httponly','secure','samesite'].includes(key)||['httponly','secure'].includes(key)&&at>=0)throw new Error()
    attrs.set(key,v)
   }
   if(attrs.get('path')!=='/'||attrs.get('samesite')?.toLowerCase()!=='lax'||attrs.has('secure')!==secure||attrs.has('httponly')!==(kind!=='csrf'))throw new Error()
   const deleting=value===''&&attrs.get('max-age')==='0'
   if(!deleting&&(!/^[0-9a-f]{64}$/.test(value)||!/^([1-9][0-9]{0,5})$/.test(attrs.get('max-age')??'')||Number(attrs.get('max-age'))>43200||!Number.isFinite(Date.parse(attrs.get('expires')??''))))throw new Error()
   if(deleting){if(!post||!(kind==='prelogin'&&['register','login','verify','resend','logout'].includes(action)||success&&action==='logout'))throw new Error()}
   else if(!success||!post||!(action==='challenge'&&kind==='prelogin'||action==='login'&&['session','csrf'].includes(kind)))throw new Error()
   issued.set(kind,deleting);out.append('Set-Cookie',cookie)
  }
  if(success){
   if(action==='challenge'&&(issued.size!==1||issued.get('prelogin')!==false))throw new Error()
   if(action==='login'&&(issued.get('session')!==false||issued.get('csrf')!==false||issued.get('prelogin')!==true||issued.size!==3))throw new Error()
   if(action==='logout'&&(issued.size!==3||[...issued.values()].some(value=>!value)))throw new Error()
  }
  const id=response.headers.get('X-Request-ID'),retry=response.headers.get('Retry-After')
  if(id&&/^req_[0-9a-f-]{36}$/.test(id))out.set('X-Request-ID',id)
  if(status===429&&retry&&/^[1-9][0-9]{0,5}$/.test(retry))out.set('Retry-After',retry)
  if(!success){
   const verification=status===403&&parsed.state==='verification_required'&&parsed.error?.code==='customer.verification_required'
   return new Response(JSON.stringify(verification?{state:'verification_required',error:{code:'customer.verification_required',message:'Verify your email before signing in'}}:{error:{code:'customer.request_unavailable',message:'Customer request unavailable'}}),{status,headers:out})
  }
  // Successful Gin responses already use explicit Account projections. Refuse
  // accidental private fields even if a later handler changes serialization.
  const text=JSON.stringify(parsed)
  if(/"(?:password|password_hash|auth_version|session|token|token_hash|payload)"\s*:/.test(text))throw new Error()
  return new Response(text,{status,headers:out})
 }catch{return failure(503)}finally{clearTimeout(timer)}
}
