import test from 'node:test'
import assert from 'node:assert/strict'
import {CartClient,CartError,browserCartClient,scopeCSRFFromCookies,type CartCreation} from '../src/lib/cart-client.ts'

const csrf = 'a'.repeat(64), nextCSRF = 'b'.repeat(64)
const operationID = 'f8230181-a32c-4127-838b-92b7c033ef01'
const input: CartCreation = {operationID,regionID:'region_eu',locale:'en'}
const cart = {id:'cart_new',region_id:'region_eu',sales_channel_id:'channel_one',currency_code:'eur',locale:'en',version:'9007199254740993123'}
const capabilities = {items:false,checkout:false}
const expires = '2026-10-29T00:00:00Z'
function reply(value: unknown, status = 200, headers: Record<string,string> = {}): Response {
 return new Response(JSON.stringify(value),{status,headers:{'Content-Type':'application/json','Cache-Control':'private, no-store',...headers}})
}
function created(replayed = false) { return {cart:{...cart},csrf_token:nextCSRF,expires_at:expires,replayed,capabilities:{...capabilities}} }
function client(fetcher: typeof fetch, cookie: string | undefined = csrf) { return new CartClient({fetcher,readScopeCSRF:()=>cookie}) }
async function error(action: Promise<unknown>, kind: CartError['kind'], status: number | null, uncertain: boolean) {
 await assert.rejects(action,(err: unknown) => err instanceof CartError && err.kind === kind && err.status === status && err.uncertain === uncertain)
}

test('scope creation and reuse send only same-origin private requests and CSRF', async () => {
 const calls: {path: string; init: RequestInit}[] = []
 const fetcher: typeof fetch = async (path,init) => {
  calls.push({path:String(path),init:init!})
  return reply(calls.length === 1 ? {csrf_token:csrf,expires_at:expires,reused:false} : {csrf_token:csrf,reused:true})
 }
 assert.deepEqual(await new CartClient({fetcher,readScopeCSRF:()=>undefined}).openScope(),{csrfToken:csrf})
 assert.deepEqual(await client(fetcher).openScope(),{csrfToken:csrf})
 for (const call of calls) {
  assert.equal(call.path,'/api/store/cart-sessions')
  assert.equal(call.init.method,'POST'); assert.equal(call.init.body,'{}')
  assert.equal(call.init.credentials,'same-origin'); assert.equal(call.init.cache,'no-store'); assert.equal(call.init.redirect,'error')
  assert.ok(call.init.signal instanceof AbortSignal)
  const headers = new Headers(call.init.headers)
  assert.equal(headers.get('Content-Type'),'application/json'); assert.equal(headers.get('Cookie'),null); assert.equal(headers.get('Authorization'),null)
 }
 assert.equal(new Headers(calls[0]!.init.headers).get('X-CSRF-Token'),null)
 assert.equal(new Headers(calls[1]!.init.headers).get('X-CSRF-Token'),csrf)
})

test('creation explicitly projects region/locale, preserves IDs and exact uint64 version', async () => {
 const original = {...input,unit_price:'0',store_id:'other',customer_id:'old'}
 const before = structuredClone(original)
 let calls = 0
 const c = client(async (path,init) => {
  calls++; assert.equal(path,'/api/store/carts')
  assert.deepEqual(JSON.parse(String(init!.body)),{region_id:'region_eu',locale:'en'})
  assert.equal(new Headers(init!.headers).get('Idempotency-Key'),operationID)
  assert.equal(new Headers(init!.headers).get('X-CSRF-Token'),csrf)
  return reply(created(),201)
 })
 const result = await c.create(original,{csrfToken:csrf})
 assert.deepEqual(result,{cart,csrfToken:nextCSRF,expiresAt:expires,replayed:false})
 assert.equal(calls,1); assert.deepEqual(original,before)
})

test('uncertain POST never automatically retries; explicit retry keeps the same operation and scope', async () => {
 const operations: string[] = [], bodies: string[] = []
 const c = client(async (path,init) => {
  assert.equal(path,'/api/store/carts')
  operations.push(new Headers(init!.headers).get('Idempotency-Key')!)
  bodies.push(String(init!.body))
  if (operations.length === 1) throw new Error('private network diagnostic must not escape')
  return reply(created(true))
 })
 await error(c.create(input,{csrfToken:csrf}),'unavailable',null,true)
 assert.equal(operations.length,1)
 assert.equal((await c.create(input,{csrfToken:csrf})).replayed,true)
 assert.deepEqual(operations,[operationID,operationID]); assert.equal(bodies[0],bodies[1])
})

test('read uses possession cookies implicitly, and only explicit 401/404 produces an inaccessible cart', async () => {
 const c = client(async (path,init) => {
  assert.equal(path,'/api/store/carts/cart_new'); assert.equal(init!.method,'GET')
  assert.equal(init!.body,undefined); assert.equal(init!.credentials,'same-origin')
  assert.equal(new Headers(init!.headers).get('X-CSRF-Token'),null)
  return reply({cart,capabilities})
 })
 assert.deepEqual(await c.retrieve('cart_new'),cart)
 for (const status of [401,404]) assert.equal(await client(async()=>reply({error:'private'},status)).retrieve('cart_new'),null)
 await error(client(async()=>reply({error:'dependency'},503)).retrieve('cart_new'),'unavailable',503,false)
 await error(client(async()=>{throw new Error('network')}).retrieve('cart_new'),'unavailable',null,false)
})

test('typed failures discard server body/message and expose only bounded tracing/retry metadata', async () => {
 const kinds: [number,CartError['kind']][] = [[400,'input'],[403,'csrf'],[409,'conflict'],[413,'input'],[415,'input'],[422,'region'],[429,'rate_limit'],[500,'unavailable'],[503,'unavailable']]
 for (const [status,kind] of kinds) {
  const c = client(async()=>reply({error:{message:'server secret',code:'arbitrary'}},status,{'X-Request-ID':`req_${operationID}`,'Retry-After':'900'}))
  try { await c.create(input,{csrfToken:csrf}); assert.fail('failure returned cart') }
  catch (err) {
   assert.ok(err instanceof CartError); assert.equal(err.kind,kind); assert.equal(err.status,status)
   assert.equal(err.uncertain,kind === 'unavailable'); assert.equal(err.requestID,`req_${operationID}`); assert.equal(err.retryAfter,900)
   assert.equal(err.message.includes('secret'),false)
  }
 }
})

test('CSRF cookie parsing ignores bearer/legacy cookies and rejects duplicate/noncanonical values', () => {
 assert.equal(scopeCSRFFromCookies(`retail_cart_possession=secret; _medusa_cart_id=old`,false),undefined)
 assert.equal(scopeCSRFFromCookies(`retail_cart_scope_csrf=${csrf}; ignored=x`,false),csrf)
 assert.equal(scopeCSRFFromCookies(`retail_cart_scope_csrf=${csrf}`,true),undefined)
 assert.equal(scopeCSRFFromCookies(`__Host-retail_cart_scope_csrf=${csrf}`,true),csrf)
 for (const value of [`retail_cart_scope_csrf=${csrf}; retail_cart_scope_csrf=${csrf}`,`retail_cart_scope_csrf=${csrf.toUpperCase()}`,`retail_cart_scope_csrf=%61`, `retail_cart_scope_csrf=`]) assert.throws(()=>scopeCSRFFromCookies(value,false),CartError)
 assert.throws(()=>browserCartClient(),CartError)
})

test('unsupported capabilities, private/extra cart fields, wrong IDs and invalid versions fail closed', async () => {
 for (const edit of [
  (v: ReturnType<typeof created>) => { v.capabilities.items = true },
  (v: ReturnType<typeof created>) => { v.capabilities.checkout = true },
  (v: ReturnType<typeof created>) => { v.cart.region_id = 'other' },
  (v: ReturnType<typeof created>) => { v.cart.locale = 'zh-CN' },
  (v: ReturnType<typeof created>) => { Object.assign(v.cart,{email:'private'}) },
  (v: ReturnType<typeof created>) => { v.cart.version = '18446744073709551616' },
  (v: ReturnType<typeof created>) => { v.cart.version = '01' },
  (v: ReturnType<typeof created>) => { v.cart.version = '1\n' },
  (v: ReturnType<typeof created>) => { Object.assign(v.cart,{version:1}) },
  (v: ReturnType<typeof created>) => { v.cart.currency_code = 'EUR' },
  (v: ReturnType<typeof created>) => { v.csrf_token = '' },
  (v: ReturnType<typeof created>) => { v.expires_at = 'not-a-date' },
  (v: ReturnType<typeof created>) => { v.replayed = true },
 ]) {
  const value = created(); edit(value)
  await error(client(async()=>reply(value,201)).create(input,{csrfToken:csrf}),'protocol',201,true)
 }
 await error(client(async()=>reply({cart:{...cart,id:'other'},capabilities})).retrieve('cart_new'),'protocol',null,false)
})

test('response MIME/privacy/size/encoding boundaries never return a partial cart', async () => {
 const responses = [
  ()=>new Response('<html>error</html>',{headers:{'Content-Type':'text/html'}}),
  ()=>reply(created(),201,{'Cache-Control':'public'}),
  ()=>new Response('x'.repeat(65537),{status:201,headers:{'Content-Type':'application/json','Cache-Control':'private, no-store'}}),
  ()=>new Response(new Uint8Array([0xff,0xfe]),{status:201,headers:{'Content-Type':'application/json','Cache-Control':'private, no-store'}}),
  ()=>new Response('{} {}',{status:201,headers:{'Content-Type':'application/json','Cache-Control':'private, no-store'}}),
 ]
 for (const response of responses) {
  await assert.rejects(client(async()=>response()).create(input,{csrfToken:csrf}),(err: unknown)=>err instanceof CartError && err.uncertain)
 }
})

test('invalid caller input fails before fetch and does not issue a scope automatically', async () => {
 let calls = 0
 const c = client(async()=>{calls++;throw new Error('must not reach network')})
 for (const change of [{operationID:operationID.toUpperCase()},{operationID:operationID+'\n'},{regionID:'../other'},{regionID:'region\n'},{locale:'en_US'},{locale:'en\n'},{locale:'a'.repeat(36)}]) await error(c.create({...input,...change},{csrfToken:csrf}),'input',null,false)
 await error(c.create(input,{csrfToken:''}),'input',null,false)
 await error(c.retrieve('/other'),'input',null,false)
 assert.equal(calls,0)
})
