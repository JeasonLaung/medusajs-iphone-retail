// Real Mollie sandbox retry subflow. Preparation uses production APIs, while
// whole-payment cancellation, retry recovery and replacement use the UI.
import assert from 'node:assert/strict'
import {randomUUID} from 'node:crypto'
import {writeFileSync} from 'node:fs'
import {resolve} from 'node:path'
import {chromium,expect as baseExpect} from '../../admin/node_modules/@playwright/test/index.mjs'
import {CartClient} from '../src/lib/cart-client.ts'
const expect=baseExpect.configure({timeout:30000}),origin='http://127.0.0.1:19300'
const owner=process.env.ACCOUNT_TEST_OWNER,logs=process.env.ACCOUNT_TEST_LOGS
assert.match(owner??'',/^[0-9a-f]{12}$/)
assert.equal(process.env.ACCOUNT_TEST_SCENARIO,'payment-retry')
const browser=await chromium.launch({headless:true,executablePath:resolve(process.env.LOCALAPPDATA,'ms-playwright/chromium-1234/chrome-win64/chrome.exe')})
const steps=[],errors=[],diagnostics=[]
let activePage
try{
 const context=await browser.newContext({viewport:{width:390,height:900},reducedMotion:'reduce'}),page=await context.newPage()
 activePage=page
 page.on('pageerror',error=>errors.push(error.message))
 page.on('console',entry=>{if(entry.type()==='error')diagnostics.push({kind:'console',message:entry.text().slice(0,2000)})})
 page.on('requestfailed',request=>{if(request.url().startsWith(origin))diagnostics.push({kind:'network',path:new URL(request.url()).pathname,error:request.failure()?.errorText})})
 page.on('response',response=>{if(response.status()>=400&&response.url().startsWith(origin))diagnostics.push({kind:'http',path:new URL(response.url()).pathname,status:response.status()})})
 let scopeCSRF
 const api=new CartClient({readScopeCSRF:()=>scopeCSRF,fetcher:async(path,init)=>{
  const response=await context.request.fetch(origin+path,{method:init.method,headers:{...init.headers,Origin:origin},data:init.body,failOnStatusCode:false})
  // Playwright flattens repeated Set-Cookie headers with newlines in headers().
  // Preserve separate header entries; its cookie jar already applies them.
  return new Response(await response.body(),{status:response.status(),headers:response.headersArray().map(({name,value})=>[name,value])})
 }})
 const regions=await (await context.request.get('http://127.0.0.1:19100/api/store/regions')).json()
 const region=regions.regions.find(r=>r.currency_code==='eur'&&r.countries.some(c=>c.iso_2==='gb'))
 assert.ok(region)
 const scope=await api.openScope();scopeCSRF=scope.csrfToken
 const created=await api.create({operationID:randomUUID(),regionID:region.id,locale:'en'},scope)
 const cartID=created.cart.id,auth={csrfToken:created.csrfToken}
 let version=(await api.addItem(cartID,{operationID:randomUUID(),version:created.cart.version,variantID:'variant_01M51RN7H7HPV9RH69T32QMD2Q',quantity:'1'},auth)).version
 const address={first_name:'Isolated',last_name:'Retry',company:'',address_1:'1 Sandbox Street',address_2:'',city:'London',country_code:'gb',province:'',postal_code:'SW1A 1AA',phone:'+441234567890'}
 version=(await api.saveContacts(cartID,{operationID:randomUUID(),version,email:`local-retry-${owner}@example.invalid`,shipping_address:address,billing_address:address},auth)).version
 const delivery=await api.retrieveDelivery(cartID)
 assert.equal(delivery.version,version)
 const option=delivery.options.find(o=>!o.insufficient_inventory)
 assert.ok(option)
 version=(await api.saveShipping(cartID,{operationID:randomUUID(),version,optionID:option.id},auth)).version
 steps.push('production API cart/contact/shipping preparation')
 const paymentKey='retail:checkout-payment-intent:v1',contextKey='retail:cart-context-intent:v1'
 const journal=key=>page.evaluate(key=>JSON.parse(localStorage.getItem(key)),key)
 await page.goto(origin+'/gb-en/checkout?step=payment')
 await page.getByTestId('submit-payment-button').click({timeout:60000})
 await expect(page.getByTestId('submit-order-button')).toBeEnabled({timeout:60000})
 const original=await journal(paymentKey)
 assert.equal(original.cartID,cartID);assert.equal(original.version,version)
 await page.getByTestId('submit-order-button').click()
 await page.waitForURL(url=>url.hostname.endsWith('mollie.com'),{timeout:60000})
 await page.locator('#cancel-button button').click({timeout:60000})
 await page.waitForURL(url=>url.origin===origin&&url.pathname.endsWith('/payment-result'),{timeout:60000})
 await expect(page.getByTestId('payment-result')).toHaveAttribute('data-status','failed')
 steps.push('real Mollie whole-payment cancellation observed as failed')
 // Lose only a successful response. The durable command must survive reload;
 // never manufacture a channel status or mock a business API response.
 let lost=false,firstRetry
 const retries=[]
 await page.route('**/context-payment',async route=>{
  const request=route.request()
  const command={key:request.headers()['idempotency-key'],body:request.postDataJSON()}
  retries.push(command)
  const response=await route.fetch()
  if(!lost&&response.status()===200&&(await response.json()).state==='saved'){
   lost=true;firstRetry=await journal(contextKey);await route.abort('failed')
  }else await route.fulfill({response})
 })
 await page.getByRole('button',{name:'Return to checkout',exact:true}).click()
 await expect.poll(()=>lost).toBe(true)
 await expect(page.getByRole('alert')).toBeVisible()
 assert.equal(firstRetry.kind,'payment-retry')
 assert.deepEqual(await journal(paymentKey),original)
 await page.reload()
 await expect(page).toHaveURL(/checkout\?step=payment/)
 assert.equal(await journal(contextKey),null);assert.equal(await journal(paymentKey),null)
 assert.ok(retries.length>=2);for(const retry of retries)assert.deepEqual(retry,retries[0])
 // Keep the test's interception setting stable until the destination island
 // has hydrated; assert readiness before changing the browser network setup.
 await expect(page.getByTestId('submit-payment-button')).toBeEnabled({timeout:60000})
 await page.unroute('**/context-payment')
 steps.push('lost successful retry response recovered with same UUID and body after reload')
 await page.getByTestId('submit-payment-button').click({timeout:60000})
 await expect(page.getByTestId('submit-order-button')).toBeEnabled({timeout:60000})
 const replacement=await journal(paymentKey)
 assert.equal(replacement.cartID,cartID);assert.notEqual(replacement.operationID,original.operationID)
 assert.equal(BigInt(replacement.version),BigInt(original.version)+2n)
 await context.storageState({path:resolve(logs,'retry-payment-state.json')})
 await page.getByTestId('submit-order-button').click()
 await page.waitForURL(url=>url.hostname.endsWith('mollie.com'),{timeout:60000})
 await page.getByRole('button',{name:/PayPal/}).click({timeout:60000})
 await page.locator('input[name="final_state"][value="paid"]').check({timeout:60000})
 await page.getByRole('button',{name:/Continue/}).click()
 await page.waitForURL(url=>url.origin===origin&&/^\/gb-en\/order\/order_[0-9a-f-]+\/confirmed$/.test(url.pathname),{timeout:90000})
 await expect(page.getByTestId('order-complete-container')).toBeVisible()
 const orderID=new URL(page.url()).pathname.split('/').at(-2)
 assert.match(orderID,/^order_[0-9a-f-]{36}$/)
 await page.reload();await expect(page.getByTestId('order-complete-container')).toBeVisible()
 assert.equal(new URL(page.url()).pathname.split('/').at(-2),orderID)
 assert.equal(await journal(paymentKey),null)
 steps.push('explicit replacement payment paid on real Mollie sandbox; guest confirmation survives reload')
 assert.deepEqual(errors,[])
 writeFileSync(resolve(logs,'browser-result.json'),JSON.stringify({passed:true,scenario:'payment-retry',cart_id:cartID,order_id:orderID,real_mollie_test_payment:true,real_mollie_whole_payment_cancel:true,guest_checkout:true,retry_request_replayed:retries.length,steps,errors},null,2))
}catch(error){
 writeFileSync(resolve(logs,'retry-failure.json'),JSON.stringify({steps,errors,diagnostics,url:activePage?.url()},null,2))
 if(activePage){
  await activePage.screenshot({path:resolve(logs,'retry-failure.png'),fullPage:true}).catch(()=>{})
  writeFileSync(resolve(logs,'retry-failure.html'),await activePage.content().catch(()=>''))
 }
 throw error
}finally{await browser.close()}
