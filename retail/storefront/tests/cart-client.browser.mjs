import assert from 'node:assert/strict'
import {createServer} from 'node:http'
import {readFile} from 'node:fs/promises'
import {createRequire,stripTypeScriptTypes} from 'node:module'
const require = createRequire(new URL('../../admin/package.json',import.meta.url))
const {chromium} = require('@playwright/test')
const source = stripTypeScriptTypes(await readFile(new URL('../src/lib/cart-client.ts',import.meta.url),'utf8'))
const scopeToken = '1'.repeat(64), scopeCSRF = '2'.repeat(64), bearer = '3'.repeat(64), cartCSRF = '4'.repeat(64)
const cart = {id:'cart_browser_fixture',region_id:'region_fixture',sales_channel_id:'channel_fixture',currency_code:'eur',locale:'en',version:'9007199254740993123'}
const receipts = new Set(), requests = []
let dropNext = false, unavailableRead = false, browser, server
const cookie = (name,value,httpOnly) => `${name}=${value}; Path=/; SameSite=Lax${httpOnly ? '; HttpOnly' : ''}`
const json = (res,status,value,cookies=[]) => { res.writeHead(status,{'Content-Type':'application/json','Cache-Control':'private, no-store',...(cookies.length ? {'Set-Cookie':cookies} : {})}); res.end(JSON.stringify(value)) }
try {
 server = createServer(async (req,res) => {
  if (req.url === '/') { res.writeHead(200,{'Content-Type':'text/html','Cache-Control':'no-store'});res.end('<!doctype html><html><head><link rel="icon" href="data:," /></head><body>Cart transport fixture</body></html>');return }
  if (req.url === '/cart-client.js') { res.writeHead(200,{'Content-Type':'text/javascript','Cache-Control':'no-store'});res.end(source);return }
  const chunks = []
  for await (const chunk of req) chunks.push(chunk)
  const body = Buffer.concat(chunks).toString(), cookieHeader = req.headers.cookie ?? ''
  requests.push({path:req.url,method:req.method,origin:req.headers.origin,csrf:req.headers['x-csrf-token'],key:req.headers['idempotency-key'],cookie:cookieHeader,body})
  if (req.method === 'POST' && req.headers.origin !== `http://127.0.0.1:${server.address().port}`) { json(res,403,{});return }
  if (req.url === '/api/store/cart-sessions') {
   const reused = cookieHeader.includes(`retail_cart_scope=${scopeToken}`)
   if (reused && req.headers['x-csrf-token'] !== scopeCSRF) {json(res,403,{});return}
   json(res,200,{csrf_token:scopeCSRF,reused,...(!reused ? {expires_at:'2026-10-29T00:00:00Z'} : {})},reused ? [] : [cookie('retail_cart_scope',scopeToken,true),cookie('retail_cart_scope_csrf',scopeCSRF,false)])
   return
  }
  if (req.url === '/api/store/carts' && req.method === 'POST') {
   if (!cookieHeader.includes(`retail_cart_scope=${scopeToken}`) || req.headers['x-csrf-token'] !== scopeCSRF) {json(res,401,{});return}
   const key = req.headers['idempotency-key'], replayed = receipts.has(key)
   receipts.add(key)
   if (dropNext) {
    dropNext = false
    // Send response headers first: browsers may transparently retry a reset
    // connection before ANY response, independently of application fetch code.
    res.writeHead(201,{'Content-Type':'application/json','Cache-Control':'private, no-store','Content-Length':'4096'})
    res.flushHeaders(); res.write('{"cart":')
    setTimeout(()=>res.destroy(),20);return
   }
   json(res,replayed ? 200 : 201,{cart,csrf_token:cartCSRF,expires_at:'2026-10-29T00:00:00Z',replayed,capabilities:{items:false,checkout:false}},[cookie('retail_cart_possession',bearer,true),cookie('retail_cart_csrf',cartCSRF,false)])
   return
  }
  if (req.url === '/api/store/carts/'+cart.id && req.method === 'GET') {
   if (unavailableRead) {json(res,503,{error:{message:'artificial dependency failure'}});return}
   if (!cookieHeader.includes(`retail_cart_possession=${bearer}`)) {json(res,401,{});return}
   json(res,200,{cart,capabilities:{items:false,checkout:false}});return
  }
  json(res,404,{})
 })
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve))
 browser = await chromium.launch({headless:true,...(process.env.ADMIN_TEST_BROWSER ? {executablePath:process.env.ADMIN_TEST_BROWSER} : {})})
 const context = await browser.newContext(), page = await context.newPage()
 const pageErrors = []; page.on('pageerror',error=>pageErrors.push(error.message))
 await page.goto(`http://127.0.0.1:${server.address().port}/`)
 await page.evaluate(async()=>{ const {browserCartClient} = await import('/cart-client.js');window.cartClient=browserCartClient();window.cartScope=await window.cartClient.openScope() })
 let visible = await page.evaluate(()=>document.cookie)
 assert.ok(visible.includes(`retail_cart_scope_csrf=${scopeCSRF}`));assert.equal(visible.includes(scopeToken),false)
 const issuedCookies = await context.cookies()
 assert.equal(issuedCookies.find(c=>c.name==='retail_cart_scope').httpOnly,true)
 assert.equal(issuedCookies.find(c=>c.name==='retail_cart_scope_csrf').httpOnly,false)
 const operation = 'f8230181-a32c-4127-838b-92b7c033ef01'
 const first = await page.evaluate(async operationID=>window.cartClient.create({operationID,regionID:'region_fixture',locale:'en'},window.cartScope),operation)
 assert.equal(first.cart.version,cart.version);assert.equal(first.replayed,false)
 visible = await page.evaluate(()=>document.cookie)
 assert.equal(visible.includes(bearer),false);assert.ok(visible.includes(cartCSRF))
 assert.equal((await context.cookies()).find(c=>c.name==='retail_cart_possession').httpOnly,true)
 const read = await page.evaluate(id=>window.cartClient.retrieve(id),cart.id)
 assert.deepEqual(read,cart)
 const beforeReuse = requests.length
 await page.evaluate(async()=>{window.cartScope=await window.cartClient.openScope()})
 assert.equal(requests.length,beforeReuse+1);assert.equal(requests.at(-1).csrf,scopeCSRF)
 const replay = await page.evaluate(async operationID=>window.cartClient.create({operationID,regionID:'region_fixture',locale:'en'},window.cartScope),operation)
 assert.equal(replay.replayed,true);assert.equal(receipts.size,1)

 dropNext = true
 const retryOperation = '35a1954f-bccf-4f7c-88a1-cc49e4418101'
 const beforeDrop = requests.length
 const failed = await page.evaluate(async operationID=>{try {await window.cartClient.create({operationID,regionID:'region_fixture',locale:'en'},window.cartScope);return null} catch(error) {return {kind:error.kind,uncertain:error.uncertain}}},retryOperation)
 assert.deepEqual(failed,{kind:'unavailable',uncertain:true})
 assert.equal(requests.length,beforeDrop+1) // Browser client does not retry.
 const retry = await page.evaluate(async operationID=>window.cartClient.create({operationID,regionID:'region_fixture',locale:'en'},window.cartScope),retryOperation)
 assert.equal(retry.replayed,true);assert.equal(receipts.size,2)
 assert.equal(requests.at(-1).key,requests.at(-2).key)
 unavailableRead = true
 const readError = await page.evaluate(async id=>{try {await window.cartClient.retrieve(id);return null} catch(error) {return {kind:error.kind,status:error.status}}},cart.id)
 assert.deepEqual(readError,{kind:'unavailable',status:503})
 unavailableRead = false
 await context.clearCookies({name:'retail_cart_possession'})
 assert.equal(await page.evaluate(id=>window.cartClient.retrieve(id),cart.id),null)
 assert.deepEqual(pageErrors,[])
 assert.ok(requests.every(r=>!r.path.includes('medusa')))
 console.log('CART_CLIENT_BROWSER '+JSON.stringify({actual_browser_http:true,fixture_api:true,real_gin:false,real_mysql_redis:false,http_only_scope_and_possession:true,readable_csrf_only:true,browser_origin:true,explicit_operation_replay:true,no_automatic_retry:true,dependency_not_empty:true,exact_version:true,production_page_mounted:false}))
 await context.close()
} finally {
 await browser?.close()
 if (server) {server.closeAllConnections();await new Promise(resolve=>server.close(resolve))}
}
