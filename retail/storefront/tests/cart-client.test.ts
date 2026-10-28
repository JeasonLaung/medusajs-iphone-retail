import test from 'node:test'
import assert from 'node:assert/strict'
import {CartClient,CartError,browserCartClient,scopeCSRFFromCookies,cartCSRFFromCookies,type CartCreation} from '../src/lib/cart-client.ts'

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

function amountFixture() {
 const base = {subtotal:'200',total:'225',tax_total:'45',original_subtotal:'200',original_total:'250',original_tax_total:'50',discount_subtotal:'20',discount_tax_total:'5',discount_total:'25'}
 return {cart_id:cart.id,version:cart.version,currency_code:'eur',pricing_recalculated:false,checkout:false,
  totals:{...base,item_total:'225',item_subtotal:'200',item_tax_total:'45',item_discount_total:'25',original_item_total:'250',original_item_subtotal:'200',original_item_tax_total:'50',shipping_total:'0',shipping_subtotal:'0',shipping_tax_total:'0',shipping_discount_total:'0',original_shipping_total:'0',original_shipping_subtotal:'0',original_shipping_tax_total:'0',credit_line_total:'0',credit_line_subtotal:'0',credit_line_tax_total:'0'},
  lines:[{id:'line_new',variant_id:'variant_retired',quantity:'2',unit_price:'9007199254740993.000000000000000000001',is_tax_inclusive:false,totals:{...base}}],
  shipping_methods:[{id:'shipping_saved',name:'Saved delivery',amount:'12.5',is_tax_inclusive:true,totals:{...base}}]}
}

test('persisted amounts retain exact strings without enabling pricing or checkout', async () => {
 const fixture = amountFixture()
 const seen: {url:string;init:RequestInit}[] = []
 const c = client(async (url,init) => {seen.push({url:String(url),init:init!});return reply(fixture)})
 const result = await c.retrieveAmounts(cart.id)
 assert.deepEqual(result,fixture)
 assert.equal(result!.lines[0].unit_price,'9007199254740993.000000000000000000001')
 assert.equal(result!.version,'9007199254740993123')
 assert.equal(seen[0].url,'/api/store/carts/cart_new/amounts')
 assert.equal(seen[0].init.credentials,'same-origin');assert.equal(seen[0].init.cache,'no-store')
 for (const status of [401,404]) assert.equal(await client(async()=>reply({},status)).retrieveAmounts(cart.id),null)
 await error(client(async()=>reply({},503)).retrieveAmounts(cart.id),'unavailable',503,false)
 await error(c.retrieveAmounts('../other'),'input',null,false)
 // The amounts route has its own larger bounded body budget. Ordinary cart
 // creation and details keep their original 64 KiB limit.
 const larger = amountFixture();larger.shipping_methods[0].name='x'.repeat(9000)
 larger.lines=Array.from({length:200},(_,i)=>({...larger.lines[0],id:`line_${i}`}))
 assert.ok(JSON.stringify(larger).length>65536)
 assert.equal((await client(async()=>reply(larger)).retrieveAmounts(cart.id))!.lines.length,200)
})

test('malformed persisted amounts never become a partial or payable cart', async () => {
 for (const edit of [
  (v:ReturnType<typeof amountFixture>)=>{v.cart_id='other'},
  (v:ReturnType<typeof amountFixture>)=>{v.version='18446744073709551616'},
  (v:ReturnType<typeof amountFixture>)=>{v.version='1\n'},
  (v:ReturnType<typeof amountFixture>)=>{v.checkout=true},
  (v:ReturnType<typeof amountFixture>)=>{v.pricing_recalculated=true},
  (v:ReturnType<typeof amountFixture>)=>{v.totals.total='1\n'},
  (v:ReturnType<typeof amountFixture>)=>{v.lines[0].unit_price='-1'},
  (v:ReturnType<typeof amountFixture>)=>{v.lines[0].quantity='0'},
  (v:ReturnType<typeof amountFixture>)=>{v.lines[0].quantity='1000000001'},
  (v:ReturnType<typeof amountFixture>)=>{v.lines.push(v.lines[0])},
  (v:ReturnType<typeof amountFixture>)=>{v.shipping_methods.push(v.shipping_methods[0])},
  (v:ReturnType<typeof amountFixture>)=>{Object.assign(v.lines[0],{metadata:{private:'secret'}})},
  (v:ReturnType<typeof amountFixture>)=>{Object.assign(v.totals,{private:'secret'})},
  (v:ReturnType<typeof amountFixture>)=>{Object.assign(v.lines[0],{quantity:2})},
  (v:ReturnType<typeof amountFixture>)=>{Object.assign(v.totals,{total:225})},
 ]) {
  const value=amountFixture();edit(value)
  await error(client(async()=>reply(value)).retrieveAmounts(cart.id),'protocol',null,false)
 }
 await error(client(async()=>new Response('x'.repeat(4*1024*1024+1),{headers:{'Content-Type':'application/json','Cache-Control':'private, no-store'}})).retrieveAmounts(cart.id),'protocol',null,false)
})

const addition = {operationID,version:'1',variantID:'variant_one',quantity:'1'}
const itemReceipt = {cart_id:cart.id,version:'2',line_id:'line_one',quantity:'2',replayed:false}
test('draft item addition sends only server command fields with possession CSRF and exact receipt', async () => {
 const original={...addition,unit_price:'0',store_id:'foreign'}, before=structuredClone({...addition,unit_price:'0',store_id:'foreign'})
 let calls=0
 const c=client(async (url,init)=>{
  calls++; assert.equal(url,`/api/store/carts/${cart.id}/line-items`)
  assert.equal(init!.method,'POST');assert.equal(init!.credentials,'same-origin')
  assert.deepEqual(JSON.parse(String(init!.body)),{version:'1',variant_id:'variant_one',quantity:'1'})
  const headers=new Headers(init!.headers)
  assert.equal(headers.get('X-CSRF-Token'),nextCSRF);assert.equal(headers.get('Idempotency-Key'),operationID)
  assert.equal(headers.get('Authorization'),null);assert.equal(headers.get('Cookie'),null)
  return reply(itemReceipt)
 })
 assert.deepEqual(await c.addItem(cart.id,original,{csrfToken:nextCSRF}),itemReceipt)
 assert.equal(calls,1);assert.deepEqual(original,before)
 assert.equal(cartCSRFFromCookies(`retail_cart_scope_csrf=${csrf}; retail_cart_csrf=${nextCSRF}`,false),nextCSRF)
 assert.equal(cartCSRFFromCookies(`retail_cart_csrf=${csrf}; __Host-retail_cart_csrf=${nextCSRF}`,true),nextCSRF)
 assert.throws(()=>cartCSRFFromCookies(`retail_cart_csrf=${csrf}; retail_cart_csrf=${csrf}`,false))
 assert.equal(cartCSRFFromCookies(`retail_cart_possession=secret`,false),undefined)
})
test('draft item input and receipt bounds reject ambiguity; failures retain original operation', async () => {
 let calls=0
 const invalid=client(async()=>{calls++;return reply(itemReceipt)})
 for(const change of [{version:'0'},{version:'01'},{version:'18446744073709551615'},{quantity:'1000000001'},{quantity:'1.0'},{quantity:'1\n'},{variantID:'../v'},{operationID:operationID.toUpperCase()}]) await error(invalid.addItem(cart.id,{...addition,...change},{csrfToken:nextCSRF}),'input',null,false)
 assert.equal(calls,0)
 for(const patch of [{version:'3'},{version:2},{quantity:'0'},{quantity:'1000000001'},{cart_id:'foreign'},{line_id:'../line'},{replayed:'true'},{metadata:{secret:true}}]) await error(client(async()=>reply({...itemReceipt,...patch})).addItem(cart.id,addition,{csrfToken:nextCSRF}),'protocol',200,true)
 for(const [status,kind] of [[422,'item'],[409,'conflict'],[403,'csrf'],[503,'unavailable']] as const) await error(client(async()=>reply({},status)).addItem(cart.id,addition,{csrfToken:nextCSRF}),kind,status,status===503)
 const seen:string[]=[]
 const uncertain=client(async (_url,init)=>{seen.push(String(init!.body)+new Headers(init!.headers).get('Idempotency-Key'));if(seen.length===1)throw Error('lost response');return reply({...itemReceipt,replayed:true})})
 await error(uncertain.addItem(cart.id,addition,{csrfToken:nextCSRF}),'unavailable',null,true)
 assert.equal(seen.length,1)
 assert.equal((await uncertain.addItem(cart.id,addition,{csrfToken:nextCSRF})).replayed,true)
 assert.equal(seen[0],seen[1])
 const large={...addition,version:'9007199254740993123'}
 const result=await client(async()=>reply({...itemReceipt,version:'9007199254740993124'})).addItem(cart.id,large,{csrfToken:nextCSRF})
 assert.equal(result.version,'9007199254740993124')
})
