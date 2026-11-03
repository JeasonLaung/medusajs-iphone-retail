// Isolated Gin/Astro checkout loading probe. Prepare through production APIs;
// subsequent navigation is read-only and never visits a payment provider.
import assert from 'node:assert/strict'
import {randomUUID} from 'node:crypto'
import {writeFileSync} from 'node:fs'
import {resolve} from 'node:path'
import {chromium,expect as baseExpect} from '../../admin/node_modules/@playwright/test/index.mjs'
import {CartClient} from '../src/lib/cart-client.ts'

const origin='http://127.0.0.1:19300',logs=process.env.ACCOUNT_TEST_LOGS,owner=process.env.ACCOUNT_TEST_OWNER
assert.equal(process.env.ACCOUNT_TEST_SCENARIO,'checkout-readiness')
assert.match(owner??'',/^[0-9a-f]{12}$/)
const expect=baseExpect.configure({timeout:30000})
const browser=await chromium.launch({headless:true,executablePath:resolve(process.env.LOCALAPPDATA,'ms-playwright/chromium-1234/chrome-win64/chrome.exe')})
const cases=[],cartIDs=[],errors=[],diagnostics=[],forbiddenWrites=[]
let activePage,activeCase,pending=new Set()
try{
 const regions=await(await browser.request.newContext()).get('http://127.0.0.1:19100/api/store/regions')
 const region=(await regions.json()).regions.find(r=>r.currency_code==='eur'&&r.countries.some(c=>c.iso_2==='gb'))
 assert.ok(region)
 for(const segment of ['gb-en','gb-zh'])for(const width of [390,1440]){
  const context=await browser.newContext({viewport:{width,height:900},reducedMotion:'reduce'})
  let scopeCSRF
  const api=new CartClient({readScopeCSRF:()=>scopeCSRF,fetcher:async(path,init)=>{
   const response=await context.request.fetch(origin+path,{method:init.method,headers:{...init.headers,Origin:origin},data:init.body,failOnStatusCode:false})
   return new Response(await response.body(),{status:response.status(),headers:response.headersArray().map(({name,value})=>[name,value])})
  }})
  const scope=await api.openScope();scopeCSRF=scope.csrfToken
  const created=await api.create({operationID:randomUUID(),regionID:region.id,locale:segment.endsWith('-zh')?'zh-CN':'en'},scope)
  const cartID=created.cart.id,auth={csrfToken:created.csrfToken}
  cartIDs.push(cartID)
  let version=(await api.addItem(cartID,{operationID:randomUUID(),version:created.cart.version,variantID:'variant_01M51RN7H7HPV9RH69T32QMD2Q',quantity:'1'},auth)).version
  const address={first_name:'Isolated',last_name:'Loading',company:'',address_1:'1 Sandbox Street',address_2:'',city:'London',country_code:'gb',province:'',postal_code:'SW1A 1AA',phone:'+441234567890'}
  version=(await api.saveContacts(cartID,{operationID:randomUUID(),version,email:`local-loading-${owner}@example.invalid`,shipping_address:address,billing_address:address},auth)).version
  const delivery=await api.retrieveDelivery(cartID),option=delivery.options.find(o=>!o.insufficient_inventory)
  assert.ok(option)
  version=(await api.saveShipping(cartID,{operationID:randomUUID(),version,optionID:option.id},auth)).version
  const initial=await api.retrieveView(cartID)
  assert.equal(initial.amounts.version,version)
  // A browser must never mutate this prepared checkpoint during this probe.
  await context.route('**/api/store/**',route=>{
   if(['GET','HEAD'].includes(route.request().method()))return route.continue()
   forbiddenWrites.push({case:activeCase,path:new URL(route.request().url()).pathname})
   return route.abort('blockedbyclient')
  })
  const page=await context.newPage();activePage=page;pending=new Set()
  page.on('pageerror',e=>errors.push({case:activeCase,message:e.message}))
  page.on('console',entry=>{if(entry.type()==='error')diagnostics.push({case:activeCase,kind:'console',message:entry.text().slice(0,1500)})})
  page.on('request',request=>{if(request.url().startsWith(origin)&&request.resourceType()!=='websocket')pending.add(request)})
  page.on('requestfinished',request=>pending.delete(request))
  page.on('requestfailed',request=>{pending.delete(request);diagnostics.push({case:activeCase,kind:'request-failed',path:new URL(request.url()).pathname,error:request.failure()?.errorText})})
  await page.addInitScript(()=>{
   window.__checkoutHydration=[]
   document.addEventListener('astro:hydrate',event=>window.__checkoutHydration.push({type:'hydrate',component:event.target.getAttribute('component-url'),at:performance.now()}),true)
   document.addEventListener('astro:hydration-error',event=>window.__checkoutHydration.push({type:'error',component:event.detail?.componentUrl,message:String(event.detail?.error)}),true)
  })
  const unusedRoute=route=>route.continue()
  for(const mode of ['cold','reload','interception-early','interception-stable']){
   activeCase={segment,width,mode}
   if(mode.startsWith('interception'))await page.route('**/context-payment',unusedRoute)
   const started=Date.now()
   const response=mode==='reload'?await page.reload({waitUntil:'domcontentloaded'}):await page.goto(origin+`/${segment}/checkout?step=payment`,{waitUntil:'domcontentloaded'})
   assert.equal(response.status(),200)
   if(mode==='interception-early')await page.unroute('**/context-payment',unusedRoute)
   await expect(page.getByTestId('submit-payment-button')).toBeEnabled()
   await expect(page.locator('astro-island[component-url*="gin-checkout"]')).not.toHaveAttribute('ssr','')
   assert.equal((await api.retrieveView(cartID)).amounts.totals.total,initial.amounts.totals.total)
   assert.equal((await api.current()).version,version)
   assert.equal(await page.evaluate(()=>localStorage.getItem('retail:checkout-payment-intent:v1')),null)
   const hydration=await page.evaluate(()=>window.__checkoutHydration)
   assert.ok(hydration.some(event=>event.type==='hydrate'&&event.component.includes('gin-checkout')))
   assert.ok(!hydration.some(event=>event.type==='error'))
   cases.push({...activeCase,ready_ms:Date.now()-started,hydration})
   if(mode==='interception-stable')await page.unroute('**/context-payment',unusedRoute)
  }
  await context.close();activePage=null
 }
 assert.deepEqual(errors,[]);assert.deepEqual(forbiddenWrites,[])
 writeFileSync(resolve(logs,'browser-result.json'),JSON.stringify({passed:true,scenario:'checkout-readiness',cart_ids:cartIDs,cases,errors,diagnostics,forbidden_writes:forbiddenWrites,business_api_mocking:false,external_payment_created:false},null,2))
}catch(error){
 const state=activePage?await activePage.evaluate(()=>({readyState:document.readyState,hydration:window.__checkoutHydration,islands:[...document.querySelectorAll('astro-island')].map(island=>({component:island.getAttribute('component-url'),ssr:island.hasAttribute('ssr'),awaitChildren:island.hasAttribute('await-children')}))})).catch(()=>null):null
 writeFileSync(resolve(logs,'readiness-failure.json'),JSON.stringify({activeCase,cases,errors,diagnostics,forbiddenWrites,state,pending:[...pending].map(request=>({path:new URL(request.url()).pathname,type:request.resourceType()}))},null,2))
 if(activePage){await activePage.screenshot({path:resolve(logs,'readiness-failure.png'),fullPage:true}).catch(()=>{});writeFileSync(resolve(logs,'readiness-failure.html'),await activePage.content().catch(()=>''))}
 throw error
}finally{await browser.close()}
