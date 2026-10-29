import test from 'node:test'
import assert from 'node:assert/strict'
import {createServer} from 'node:http'
import {once} from 'node:events'
import {cartBridge} from '../src/lib/cart-bridge.ts'
const shop='http://127.0.0.1:9300',config={ginOrigin:'http://127.0.0.1:9191',storefrontOrigin:shop}
const req=(path='/api/store/cart',init:RequestInit={})=>new Request(shop+path,init)
const good=(body:unknown={},headers:Record<string,string>={})=>Response.json(body,{headers:{'Cache-Control':'private, no-store',...headers}})
test('private bridge forwards only scoped cart cookies and preserves public Host and write proof',async()=>{
 const token='a'.repeat(64),key='01234567-89ab-4def-8123-456789abcdef'
 const response=await cartBridge(req('/api/store/carts/p/line-items',{method:'POST',headers:{Origin:shop,'Content-Type':'application/json',Cookie:`retail_cart_possession=${token}; retail_admin_session=PRIVATE; medusa_token=PRIVATE`,Authorization:'PRIVATE','X-CSRF-Token':token,'Idempotency-Key':key,'X-Forwarded-Host':'evil.test'},body:'{"version":"1","variant_id":"v","quantity":"1"}'}),config,async(url,init)=>{
  assert.equal(String(url),config.ginOrigin+'/api/store/carts/p/line-items');const h=new Headers(init?.headers)
  assert.equal(h.get('Host'),'127.0.0.1:9300');assert.equal(h.get('Origin'),shop);assert.equal(h.get('Cookie'),`retail_cart_possession=${token}`)
  assert.equal(h.get('Authorization'),null);assert.equal(h.get('X-Forwarded-Host'),null);assert.equal(h.get('Idempotency-Key'),key);assert.equal(h.get('X-CSRF-Token'),token)
  assert.equal(init?.redirect,'error');assert.equal(init?.credentials,'omit');assert.equal(new TextDecoder().decode(init?.body as Uint8Array),'{"version":"1","variant_id":"v","quantity":"1"}')
  return good({accepted:true})
 })
 assert.equal(response.status,200);assert.equal(response.headers.get('cache-control'),'private, no-store');assert.deepEqual(await response.json(),{accepted:true})
})
test('private bridge refuses wrong origin, path, method, duplicate cookies and oversized JSON before transport',async()=>{
 let calls=0;const transport:typeof fetch=async()=>{calls++;return good()}
 for(const [request,status] of [
  [req('/api/admin/me'),404],[req('/api/store/cart?store=other'),404],[req('/api/store/cart',{method:'DELETE'}),404],
  [req('/api/store/carts',{method:'POST',headers:{'Content-Type':'application/json'},body:'{}'}),403],
  [req('/api/store/cart',{headers:{Origin:'https://evil.test'}}),403],[req('/api/store/cart',{headers:{'Sec-Fetch-Site':'cross-site'}}),403],
  [req('/api/store/cart',{headers:{Cookie:`retail_cart_csrf=${'a'.repeat(64)}; retail_cart_csrf=${'a'.repeat(64)}`}}),401],
  [req('/api/store/carts',{method:'POST',headers:{Origin:shop,'Content-Type':'text/plain'},body:'{}'}),415],
  [req('/api/store/carts',{method:'POST',headers:{Origin:shop,'Content-Type':'application/json'},body:'x'.repeat(16385)}),413]
 ] as const)assert.equal((await cartBridge(request,config,transport)).status,status)
 assert.equal(calls,0)
 assert.equal((await cartBridge(req(),{...config,storefrontOrigin:undefined},transport)).status,503)
})
test('only expected paired Gin cart cookies survive successful scope/creation, never arbitrary upstream headers',async()=>{
 const t='b'.repeat(64),headers=new Headers({'Content-Type':'application/json','Cache-Control':'private, no-store','Location':'https://evil.test','Access-Control-Allow-Origin':'*'})
 headers.append('Set-Cookie',`retail_cart_scope=${t}; Path=/; HttpOnly; SameSite=Lax`);headers.append('Set-Cookie',`retail_cart_scope_csrf=${t}; Path=/; SameSite=Lax`)
 const call=async(h=headers)=>cartBridge(req('/api/store/cart-sessions',{method:'POST',headers:{Origin:shop,'Content-Type':'application/json'},body:'{}'}),config,async()=>new Response('{}',{headers:h}))
 const result=await call();assert.equal(result.status,200);assert.equal(result.headers.getSetCookie().length,2);assert.equal(result.headers.get('Location'),null);assert.equal(result.headers.get('Access-Control-Allow-Origin'),null)
 const secureOrigin='https://shop.example'
 const secureHeaders=new Headers({'Content-Type':'application/json','Cache-Control':'private, no-store'})
 secureHeaders.append('Set-Cookie',`__Host-retail_cart_scope=${t}; Path=/; Secure; HttpOnly; SameSite=Lax`)
 secureHeaders.append('Set-Cookie',`__Host-retail_cart_scope_csrf=${t}; Path=/; Secure; SameSite=Lax`)
 const secure=await cartBridge(new Request(secureOrigin+'/api/store/cart-sessions',{method:'POST',headers:{Origin:secureOrigin,'Content-Type':'application/json',Cookie:`retail_cart_scope=${'a'.repeat(64)}; __Host-retail_cart_scope=${t}`},body:'{}'}),{...config,storefrontOrigin:secureOrigin},async(_input,init)=>{assert.equal(new Headers(init?.headers).get('Cookie'),`__Host-retail_cart_scope=${t}`);return new Response('{}',{headers:secureHeaders})})
 assert.equal(secure.status,200);assert.equal(secure.headers.getSetCookie().length,2)

 for(const value of [`retail_admin_session=${t}; Path=/; HttpOnly; SameSite=Lax`,`retail_cart_scope=${t}; Path=/; Domain=evil.test; HttpOnly; SameSite=Lax`,`retail_cart_scope=${t}; Path=/; HttpOnly; SameSite=Lax; Path=/evil`,`retail_cart_scope=${t}; Path=/; HttpOnly; SameSite=Lax; Secure=false`]){
  const bad=new Headers(headers);bad.delete('Set-Cookie');bad.append('Set-Cookie',value);assert.equal((await call(bad)).status,503)
 }
})
test('error status and safe retry metadata survive; private upstream errors and redirects do not',async()=>{
 for(const status of [401,403,409,422,429,500,302]){
  const response=await cartBridge(req(),config,async()=>new Response('PRIVATE_DATABASE_ERROR',{status,headers:{'Retry-After':'10','X-Request-ID':'req_12345678-1234-1234-1234-123456789abc','Set-Cookie':'admin=PRIVATE'}}))
  assert.equal(response.status,[500,302].includes(status)?503:status);assert.ok(!(await response.text()).includes('PRIVATE'));assert.equal(response.headers.getSetCookie().length,0)
  assert.equal(response.headers.get('Retry-After'),status===429?'10':null)
 }
 assert.equal((await cartBridge(req(),config,async()=>new Response('not json',{headers:{'content-type':'application/json','cache-control':'private, no-store'}}))).status,503)
})

test('real private transport preserves public Host and paired cookies without following redirects',async()=>{
 let seen=false
 const upstream=createServer((request,response)=>{
  assert.equal(request.headers.host,'127.0.0.1:9300');assert.equal(request.headers.origin,shop)
  assert.equal(request.headers.authorization,undefined);seen=true
  if(request.url==='/api/store/cart'){response.writeHead(302,{Location:'/redirect-must-not-run'});response.end();return}
  assert.equal(request.url,'/api/store/cart-sessions')
  response.writeHead(200,{'Content-Type':'application/json','Cache-Control':'private, no-store','Set-Cookie':[`retail_cart_scope=${'a'.repeat(64)}; Path=/; HttpOnly; SameSite=Lax`,`retail_cart_scope_csrf=${'b'.repeat(64)}; Path=/; SameSite=Lax`]})
  response.end('{}')
 })
 try{
  upstream.listen(0,'127.0.0.1');await once(upstream,'listening')
  const address=upstream.address();assert.ok(address&&typeof address!=='string')
  const result=await cartBridge(req('/api/store/cart-sessions',{method:'POST',headers:{Origin:shop,'Content-Type':'application/json'},body:'{}'}),{...config,ginOrigin:`http://127.0.0.1:${address.port}`})
  assert.equal(result.status,200);assert.equal(result.headers.getSetCookie().length,2);assert.ok(seen)
  assert.equal((await cartBridge(req('/api/store/cart',{headers:{Origin:shop}}),{...config,ginOrigin:`http://127.0.0.1:${address.port}`})).status,503)
 }finally{await new Promise<void>((resolve,reject)=>upstream.close(error=>error?reject(error):resolve()))}
})
