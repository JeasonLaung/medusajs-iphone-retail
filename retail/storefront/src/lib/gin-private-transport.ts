import {request as httpRequest} from 'node:http'
import {request as httpsRequest} from 'node:https'
import {Readable} from 'node:stream'

// Node's built-in fetch ignores a custom Host header. Gin's Host/Origin guard
// needs the configured public Host while connecting to the fixed private URL.
// Native request preserves that header and never follows a redirect.
export const ginPrivateTransport:typeof fetch=async(input,init)=>{
 const url=new URL(String(input))
 if(!['http:','https:'].includes(url.protocol))throw new Error('Invalid upstream')
 return new Promise<Response>((resolve,reject)=>{
  const send=url.protocol==='https:'?httpsRequest:httpRequest
  const request=send(url,{method:init?.method??'GET',headers:Object.fromEntries(new Headers(init?.headers)),signal:init?.signal??undefined},response=>{
   const headers=new Headers()
   for(let i=0;i<response.rawHeaders.length;i+=2)headers.append(response.rawHeaders[i],response.rawHeaders[i+1])
   try{
    resolve(new Response(Readable.toWeb(response) as ReadableStream<Uint8Array>,{status:response.statusCode??503,headers}))
   }catch(error){response.destroy();reject(error)}
  })
  request.on('error',reject)
  // The bridge has already bounded and buffered this JSON body. No streams,
  // browser headers, URL-selected targets or automatic retries are accepted.
  if(init?.body!==undefined&&!(init.body instanceof Uint8Array)){request.destroy();reject(new Error('Invalid body'));return}
  request.end(init?.body)
 })
}
