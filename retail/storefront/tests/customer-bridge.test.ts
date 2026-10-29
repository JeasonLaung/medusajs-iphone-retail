import test from 'node:test'
import assert from 'node:assert/strict'
import {createServer} from 'node:http'
import {once} from 'node:events'
import {customerBridge} from '../src/lib/customer-bridge.ts'
const shop='http://127.0.0.1:9300',config={ginOrigin:'http://127.0.0.1:9191',storefrontOrigin:shop},root='/api/store/customer',token='a'.repeat(64)
const request=(path='/me',init:RequestInit={})=>new Request(shop+root+path,init)
const post=(path:string,headers:Record<string,string>={})=>request('/auth/'+path,{method:'POST',headers:{Origin:shop,'Content-Type':'application/json',...headers},body:'{}'})
const response=(body:unknown={},status=200,cookies:string[]=[])=>{
 const headers=new Headers({'Content-Type':'application/json','Cache-Control':'private, no-store'})
 for(const cookie of cookies)headers.append('Set-Cookie',cookie)
 return Response.json(body,{status,headers})
}
const cookie=(kind:string,value=token,secure=false)=>`${secure?'__Host-':''}retail_customer_${kind}=${value}; Path=/; Max-Age=43200; Expires=${new Date(Date.now()+43200000).toUTCString()}; ${kind==='csrf'?'':'HttpOnly; '}${secure?'Secure; ':''}SameSite=Lax`
const clear=(kind:string)=>`retail_customer_${kind}=; Path=/; Max-Age=0; ${kind==='csrf'?'':'HttpOnly; '}SameSite=Lax`

test('customer bridge forwards only customer cookies and explicit browser proofs',async()=>{
 const result=await customerBridge(post('register',{Cookie:`retail_customer_prelogin=${token}; retail_admin_session=PRIVATE; retail_cart_possession=PRIVATE; medusa_token=PRIVATE`,Authorization:'PRIVATE','X-Forwarded-For':'1.2.3.4','X-CSRF-Token':token}),config,async(url,init)=>{
  assert.equal(String(url),config.ginOrigin+root+'/auth/register')
  const headers=new Headers(init?.headers)
  assert.equal(headers.get('Host'),'127.0.0.1:9300');assert.equal(headers.get('Cookie'),`retail_customer_prelogin=${token}`)
  assert.equal(headers.get('Origin'),shop);assert.equal(headers.get('X-CSRF-Token'),token)
  assert.equal(headers.get('Authorization'),null);assert.equal(headers.get('X-Forwarded-For'),null)
  assert.equal(init?.redirect,'error');assert.equal(init?.cache,'no-store');assert.equal(init?.credentials,'omit')
  return response({state:'verification_required',delivery:'pending'},202,[clear('prelogin')])
 })
 assert.equal(result.status,202);assert.equal(result.headers.get('Cache-Control'),'private, no-store');assert.equal(result.headers.get('Referrer-Policy'),'no-referrer')
 assert.deepEqual(await result.json(),{state:'verification_required',delivery:'pending'})
 assert.equal(result.headers.getSetCookie().length,1)
})
test('customer bridge rejects paths, queries, origins, duplicate cookies and large bodies before transport',async()=>{
 let calls=0;const transport:typeof fetch=async()=>{calls++;return response()}
 const cases:[Request,number][]=[
  [request('/me?token=PRIVATE'),404],[request('/me?'),404],[request('/auth/unknown',{method:'POST'}),404],
  [request('/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},body:'{}'}),403],
  [post('login',{Origin:'https://evil.example'}),403],[request('/me',{headers:{'Sec-Fetch-Site':'cross-site'}}),403],
  [request('/me',{headers:{Cookie:`retail_customer_session=${token}; retail_customer_session=${token}`}}),401],
  [post('login',{'X-CSRF-Token':'malformed'}),400],[post('login',{'Content-Type':'text/plain'}),415],
  [request('/auth/login',{method:'POST',headers:{Origin:shop,'Content-Type':'application/json'},body:'x'.repeat(16385)}),413]
 ]
 for(const [req,status] of cases)assert.equal((await customerBridge(req,config,transport)).status,status)
 assert.equal((await customerBridge(request(),{...config,ginOrigin:undefined},transport)).status,503)
 assert.equal(calls,0)
})
test('customer challenge/login/logout accept only complete correctly scoped cookie changes',async()=>{
 assert.equal((await customerBridge(post('challenge'),config,async()=>response({csrf_token:token},200,[cookie('prelogin')]))).status,200)
 const login=()=>response({customer:{id:'cust_1'},csrf_token:token},200,[clear('prelogin'),cookie('session'),cookie('csrf')])
 assert.equal((await customerBridge(post('login'),config,async()=>login())).headers.getSetCookie().length,3)
 assert.equal((await customerBridge(post('logout'),config,async()=>response({success:true},200,['session','csrf','prelogin'].map(clear)))).status,200)
 for(const cookies of [[cookie('session')],[cookie('session'),cookie('csrf')],[cookie('prelogin'),cookie('prelogin')],[cookie('prelogin').replace('Path=/','Path=/; Domain=evil.test')],[cookie('prelogin').replace('SameSite=Lax','SameSite=None')],[cookie('prelogin').replace('HttpOnly; ','')],[cookie('prelogin')+'; Secure=false'],['retail_admin_session='+token+'; Path=/; SameSite=Lax']]){
  assert.equal((await customerBridge(post('challenge'),config,async()=>response({},200,cookies))).status,503)
 }
 const https='https://shop.example'
 const secure=await customerBridge(new Request(https+root+'/auth/challenge',{method:'POST',headers:{Origin:https,'Content-Type':'application/json'},body:'{}'}),{...config,storefrontOrigin:https},async()=>response({csrf_token:token},200,[cookie('prelogin',token,true)]))
 assert.equal(secure.status,200)
})
test('customer errors preserve consumed challenge clearing and verification state without leaking error bodies',async()=>{
 const verification=await customerBridge(post('login'),config,async()=>response({state:'verification_required',error:{code:'customer.verification_required',message:'PRIVATE'}},403,[clear('prelogin')]))
 assert.equal(verification.status,403);assert.equal((await verification.json()).state,'verification_required');assert.equal(verification.headers.getSetCookie().length,1)
 for(const status of [400,401,409,429,503]){
  const out=await customerBridge(post('login'),config,async()=>{
   const result=response({error:{message:'PRIVATE_DATABASE_ERROR'}},status,[clear('prelogin')]);result.headers.set('Retry-After','60');return result
  })
  assert.equal(out.status,status);assert.ok(!(await out.text()).includes('PRIVATE'));assert.equal(out.headers.getSetCookie().length,1)
  assert.equal(out.headers.get('Retry-After'),status===429?'60':null)
 }
 assert.equal((await customerBridge(post('login'),config,async()=>response({},401,[cookie('session')]))).status,503)
 for(const body of [{password_hash:'PRIVATE'},{session:token},{customer:{auth_version:1}}])assert.equal((await customerBridge(request(),config,async()=>response(body))).status,503)
 assert.equal((await customerBridge(request(),config,async()=>response({},302))).status,503)
})
test('native customer transport preserves public Host and never follows redirect',async()=>{
 let calls=0
 const server=createServer((req,res)=>{calls++;assert.equal(req.headers.host,'127.0.0.1:9300');assert.equal(req.headers.authorization,undefined);res.writeHead(302,{Location:'/redirect'});res.end()})
 try{
  server.listen(0,'127.0.0.1');await once(server,'listening');const address=server.address();assert.ok(address&&typeof address!=='string')
  assert.equal((await customerBridge(request(),{...config,ginOrigin:`http://127.0.0.1:${address.port}`})).status,503)
  assert.equal(calls,1)
 }finally{await new Promise<void>((resolve,reject)=>server.close(err=>err?reject(err):resolve()))}
})
