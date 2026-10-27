export type Country={iso_2:string;iso_3:string;num_code:string;name:string;display_name:string;region_id:string}
export type Region={id:string;name:string;currency_code:string;automatic_taxes:boolean;countries:Country[]}
const unavailable=()=>new Error('Store regions unavailable')

// Only the server caller supplies its configured origin. No browser URL,
// incoming credentials or framework-specific SDK participates in this request.
export async function loadRegions(origin:string|undefined,fetcher:typeof fetch=fetch):Promise<Region[]>{
 let base:URL
 try{
  if(!origin)throw unavailable()
  base=new URL(origin)
  if(!['http:','https:'].includes(base.protocol)||base.username||base.password||base.pathname!=='/'||base.search||base.hash)throw unavailable()
 }catch{throw unavailable()}
 const controller=new AbortController(),timer=setTimeout(()=>controller.abort(),5000)
 try{
  const response=await fetcher(new URL('/api/store/regions',base),{cache:'no-store',credentials:'omit',redirect:'error',signal:controller.signal,headers:{Accept:'application/json'}})
  if(response.status!==200||response.headers.get('content-type')?.split(';',1)[0].trim().toLowerCase()!=='application/json'||!response.body)throw unavailable()
  const reader=response.body.getReader(),chunks:Uint8Array[]=[];let size=0
  try{
   while(true){const {done,value}=await reader.read();if(done)break;size+=value.byteLength;if(size>2*1024*1024)throw unavailable();chunks.push(value)}
  }finally{await reader.cancel().catch(()=>{});reader.releaseLock()}
  const bytes=new Uint8Array(size);let offset=0
  for(const chunk of chunks){bytes.set(chunk,offset);offset+=chunk.length}
  const data:unknown=JSON.parse(new TextDecoder('utf-8',{fatal:true}).decode(bytes))
  return projectRegions(data)
 }catch{throw unavailable()}finally{clearTimeout(timer)}
}

function object(value:unknown):Record<string,unknown>{if(!value||typeof value!=='object'||Array.isArray(value))throw unavailable();return value as Record<string,unknown>}
function text(value:unknown):string{if(typeof value!=='string')throw unavailable();return value}
function array(value:unknown):unknown[]{if(!Array.isArray(value))throw unavailable();return value}
export function projectRegions(input:unknown):Region[]{
 const body=object(input),regions=array(body.regions)
 if(regions.length>100||body.count!==regions.length)throw unavailable()
 const ids=new Set<string>(),countries=new Set<string>()
 return regions.map(value=>{
  const r=object(value),id=text(r.id),currency=text(r.currency_code)
  if(!id||id.length>191||ids.has(id)||!/^[a-z]{3}$/.test(currency)||typeof r.automatic_taxes!=='boolean')throw unavailable()
  ids.add(id)
  return {id,name:text(r.name),currency_code:currency,automatic_taxes:r.automatic_taxes,countries:array(r.countries).map(value=>{
   const c=object(value),iso=text(c.iso_2)
   if(!/^[a-z]{2}$/.test(iso)||countries.has(iso)||c.region_id!==id)throw unavailable()
   countries.add(iso);if(countries.size>1000)throw unavailable()
   return {iso_2:iso,iso_3:text(c.iso_3),num_code:text(c.num_code),name:text(c.name),display_name:text(c.display_name),region_id:id}
  })}
 })
}
