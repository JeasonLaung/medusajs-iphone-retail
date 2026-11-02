export async function publicCatalogImage(origin:string|undefined,id:string|undefined,search='',fetcher:typeof fetch=fetch):Promise<Response>{
 const headers={'Cache-Control':'no-store','X-Content-Type-Options':'nosniff','Content-Security-Policy':"default-src 'none'; sandbox"}
 const fail=(status:number)=>new Response(null,{status,headers})
 if(!/^img_[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$/.test(id??'')||search)return fail(404)
 const controller=new AbortController(),timer=setTimeout(()=>controller.abort(),10000)
 try{
  const base=new URL(origin??'');if(!['http:','https:'].includes(base.protocol)||base.username||base.password||base.pathname!=='/'||base.search||base.hash)throw Error()
  const response=await fetcher(new URL('/api/store/catalog-images/'+id,base),{signal:controller.signal,cache:'no-store',credentials:'omit',redirect:'error',headers:{Accept:'image/jpeg,image/png,image/webp'}})
  if(response.status===404){await response.body?.cancel();return fail(404)}
  const mime=response.headers.get('content-type')?.split(';')[0];if(response.status!==200||!mime||!['image/jpeg','image/png','image/webp'].includes(mime)||!response.body)throw Error()
  const reader=response.body.getReader(),chunks:Uint8Array[]=[];let size=0
  try{while(true){const {done,value}=await reader.read();if(done)break;size+=value.length;if(size>3*1024*1024)throw Error();chunks.push(value)}}finally{await reader.cancel().catch(()=>{});reader.releaseLock()}
  if(size===0)throw Error();const bytes=new Uint8Array(size);let offset=0;for(const chunk of chunks){bytes.set(chunk,offset);offset+=chunk.length}
  return new Response(bytes,{headers:{...headers,'Content-Type':mime}})
 }catch{return fail(503)}finally{clearTimeout(timer)}
}
