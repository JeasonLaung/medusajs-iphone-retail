import type {APIRoute} from 'astro'
import {GIN_API_ORIGIN} from 'astro:env/server'
import {publicJSON} from '../../../lib/gin-public'
import {catalogForRequest} from '../../../lib/customer-catalog-server'
export const POST:APIRoute=async(context)=>{
 const {request}=context
 const headers={'Content-Type':'application/json','Cache-Control':'private, no-store','Vary':'Cookie'}
 let body:unknown
 try{
  if(!request.body)throw new Error()
  const reader=request.body.getReader(),chunks:Uint8Array[]=[];let size=0
  try{while(true){const {done,value}=await reader.read();if(done)break;size+=value.byteLength;if(size>65536)throw new Error();chunks.push(value)}}finally{await reader.cancel().catch(()=>{});reader.releaseLock()}
  const data=new Uint8Array(size);let offset=0;for(const chunk of chunks){data.set(chunk,offset);offset+=chunk.byteLength}body=JSON.parse(new TextDecoder('utf-8',{fatal:true}).decode(data))
 }catch{return new Response('{"error":"Invalid search request"}',{status:400,headers})}
 try{
  const result=await publicJSON(GIN_API_ORIGIN,'/api/store/search',8*1024*1024,catalogForRequest(request,()=>context.clientAddress),false,body)
  return new Response(JSON.stringify(result),{status:200,headers})
 }catch{return new Response('{"error":"Search unavailable"}',{status:503,headers})}
}
