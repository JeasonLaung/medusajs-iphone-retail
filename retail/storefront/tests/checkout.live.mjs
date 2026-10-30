import assert from 'node:assert/strict'
import {spawn} from 'node:child_process'
import {once} from 'node:events'
import {createRequire} from 'node:module'
import {setTimeout as delay} from 'node:timers/promises'
const require=createRequire(new URL('../../admin/package.json',import.meta.url)),{chromium,expect}=require('@playwright/test')
const target=new URL(process.env.CUSTOMER_ORDERS_TEST_GIN??''),key=process.env.CHECKOUT_FIXTURE_KEY,shop='http://127.0.0.1:19379'
assert.equal(target.hostname,'127.0.0.1');assert.equal(target.protocol,'http:');assert.equal(target.pathname,'/');assert.ok(target.port);assert.match(key??'',/^[0-9a-f]{64}$/)
const browse=process.env.CHECKOUT_PRODUCT_BROWSE==='1',fresh=process.env.CHECKOUT_FRESH_CART==='1'
assert.ok(!fresh||browse)
let app,browser
const control=async path=>{
 const controller=new AbortController(),timer=setTimeout(()=>controller.abort(),1500)
 try{return await fetch(new URL('/__fixture/'+path,target),{method:path==='ready'?'GET':'POST',headers:{'X-Fixture-Key':key},signal:controller.signal})}finally{clearTimeout(timer)}
}
try{
 let fixture
 for(let i=0;i<250;i++){try{const response=await control('ready');if(response.ok){fixture=await response.json();break}}catch{}await delay(200)}
 assert.equal(fixture?.fresh_cart,fresh);assert.equal(fixture?.carts?.length,fresh?0:4,'Owned checkout fixtures not ready')
 app=spawn(process.execPath,['scripts/start.mjs'],{env:{...process.env,HOST:'127.0.0.1',PORT:'19379',GIN_API_ORIGIN:target.origin,STOREFRONT_ORIGIN:shop,CUSTOMER_BRIDGE_KEY:''},stdio:'ignore'})
 let ready=false
 for(let i=0;i<100;i++){if(app.exitCode!==null)throw Error('Owned Astro stopped');try{if((await fetch(shop+'/health')).ok){ready=true;break}}catch{}await delay(100)}assert.ok(ready)
 browser=await chromium.launch({headless:true,executablePath:process.env.ADMIN_TEST_BROWSER||'C:/Program Files/Google/Chrome/Application/chrome.exe'})
 const errors=[],orders=new Set(),starts=[],completions=[]
 let index=0
 for(const segment of ['dk-en','dk-zh'])for(const width of [390,1440]){
  const cart=fresh?{}:fixture.carts[index],context=await browser.newContext({viewport:{width,height:1100},reducedMotion:'reduce'}),page=await context.newPage()
  index++;context.setDefaultTimeout(12000)
  page.on('pageerror',e=>errors.push(e.message))
  page.on('request',request=>{
   if(request.method()==='POST'&&request.url().endsWith('/payment-attempts'))starts.push({input:request.postDataJSON(),key:request.headers()['idempotency-key']})
   if(request.method()==='POST'&&request.url().endsWith('/complete'))completions.push(request.postDataJSON())
   assert.ok(!/^https?:\/\/(localhost|127\.0\.0\.1):(8000|9001)\//.test(request.url()),'Medusa requested')
  })
  if(!fresh)await context.addCookies([{name:'retail_cart_possession',value:cart.token,url:shop,httpOnly:true,sameSite:'Lax'},{name:'retail_cart_csrf',value:cart.csrf,url:shop,sameSite:'Lax'}])
  // Only the external hosted payment screen is a test double. No Gin/Astro
  // business response is intercepted, and no real payment service is called.
  await page.route('https://www.mollie.com/**',async route=>{
   assert.match(route.request().url(),/^https:\/\/www\.mollie\.com\/checkout\/joint\/payses_/)
   await route.fulfill({status:200,contentType:'text/html',body:`<h1>Isolated provider checkout</h1><a href="${shop}/${segment}/payment-result?cart_id=${cart.id}">Return to shop</a>`})
  })
  if(browse){
   await page.goto(shop+`/${segment}/products/add-product?v_id=add-v`,{waitUntil:"domcontentloaded"})
   await expect(page.getByTestId('product-title')).toHaveText('Saved product')
   await expect(page.getByTestId('add-product-button')).toBeEnabled()
   const created=fresh?page.waitForResponse(r=>r.request().method()==='POST'&&r.url()===shop+'/api/store/carts'):null
   const added=page.waitForResponse(r=>r.request().method()==='POST'&&r.url().endsWith('/line-items'))
   await page.getByTestId('add-product-button').click()
   assert.equal((await added).status(),200)
   if(created){const response=await created;assert.equal(response.status(),201);cart.id=(await response.json()).cart.id;const cookies=await context.cookies(shop);cart.csrf=cookies.find(c=>c.name==='retail_cart_csrf')?.value;assert.match(cart.csrf??'',/^[0-9a-f]{64}$/)}
   await expect(page).toHaveURL(new RegExp(`/${segment}/cart$`))
   await expect(page.getByTestId('checkout-button')).toBeVisible()
   await page.reload()
   await expect(page.getByTestId('checkout-button')).toBeVisible()
   await page.getByTestId('checkout-button').click()
  }else await page.goto(shop+`/${segment}/checkout?step=address`)
  await expect(page.getByTestId('shipping-first-name-input')).toBeVisible()
  const values={'shipping-first-name-input':'Joint','shipping-last-name-input':'Customer','shipping-email-input':`checkout-${index}@example.invalid`,'confirm-email-input':`checkout-${index}@example.invalid`,'shipping-phone-input':'+45 11111111','shipping-address-input':'Joint fixture street','shipping-postal-code-input':'0012','shipping-city-input':'Copenhagen'}
  for(const [id,value] of Object.entries(values))await page.getByTestId(id).fill(value)
  await expect(page.getByTestId('billing-address-checkbox')).toBeChecked()
  await expect(page.getByTestId('shipping-country-select')).toHaveValue('dk')
  await page.getByTestId('submit-address-button').click()
  await expect(page).toHaveURL(/step=delivery/)
  await page.locator('input[value="delivery-standard"]').click()
  await expect(page.getByTestId('submit-delivery-option-button')).toBeEnabled()
  await expect(page.getByTestId('checkout-total')).toHaveText(/158\.75/)
  await page.getByTestId('submit-delivery-option-button').click()
  await expect(page).toHaveURL(/step=payment/)
  await expect(page.getByTestId('payment-method-option')).toHaveAttribute('aria-checked','true')
  await page.getByTestId('submit-payment-button').click()
  await expect(page).toHaveURL(/step=review/)
  await expect(page.getByTestId('submit-order-button')).toBeEnabled()
  // Reload must recover the same frozen payment rather than charge again.
  await page.reload()
  await expect(page.getByTestId('submit-order-button')).toBeEnabled()
  assert.deepEqual(starts.at(-1),starts.at(-2),'Reload changed the original payment operation')
  await page.getByTestId('submit-order-button').click()
  await expect(page).toHaveURL(/^https:\/\/www\.mollie\.com\/checkout\/joint\//)
  const pendingResponse=page.waitForResponse(r=>r.url().endsWith('/payment-status'))
  await page.getByRole('link',{name:'Return to shop'}).click()
  const pending=await pendingResponse;assert.equal(pending.status(),200);assert.equal((await pending.json()).state,'pending')
  await expect(page.getByTestId('payment-result')).toHaveAttribute('data-status','pending')
  assert.equal((await control('paid/'+cart.id)).status,204)
  const paidResponse=page.waitForResponse(r=>r.url().endsWith('/payment-status'))
  const completionResponse=page.waitForResponse(r=>r.url().endsWith('/complete'))
  await page.reload()
  const paid=await paidResponse;assert.equal(paid.status(),200);assert.equal((await paid.json()).state,'paid_pending_order')
  assert.equal((await completionResponse).status(),200)
  await expect(page).toHaveURL(new RegExp(`/${segment}/order/order_[^/]+/confirmed`))
  await expect(page.getByTestId('order-complete-container')).toBeVisible()
  await expect(page.getByTestId('cart-total')).toHaveAttribute('data-value','158.75')
  await expect(page.getByTestId('shipping-address-summary')).toContainText('Joint fixture street')
  await expect(page.getByTestId('shipping-address-summary')).toContainText('0012')
  await expect(page.getByTestId('payment-method')).toContainText('Mollie')
  const url=page.url(),orderID=new URL(url).pathname.split('/').at(-2)
  assert.ok(!orders.has(orderID));orders.add(orderID)
  assert.equal(await page.evaluate(()=>localStorage.getItem('retail:checkout-payment-intent:v1')),null)
  await page.reload();await expect(page.getByTestId('order-complete-container')).toBeVisible();assert.equal(page.url(),url)
  const replay=await context.request.post(shop+`/api/store/carts/${cart.id}/complete`,{data:{},headers:{Origin:shop,'X-CSRF-Token':cart.csrf}})
  assert.equal(replay.status(),200);assert.equal((await replay.json()).order_id,orderID)
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
  await context.clearCookies()
  const denied=await context.request.get(shop+`/api/store/orders/${orderID}/confirmation`)
  assert.equal(denied.status(),401)
  await context.close()
 }
 assert.equal(orders.size,4);assert.equal(starts.length,8);assert.equal(completions.length,4);assert.deepEqual(errors,[])
 assert.equal((await control('finish')).status,204)
 console.log('CHECKOUT_JOINT_BROWSER '+JSON.stringify({actual_astro_checkout:true,actual_gin_handlers:true,real_mysql_redis:true,mocked_business_api_responses:false,isolated_provider_adapter_and_hosted_page:true,real_provider_payments:false,preseeded_cart_items:!browse,precreated_empty_cart:browse&&!fresh,anonymous_cart_creation:fresh,product_page_add_cart_checkout:browse,address_delivery_payment_review_result_confirmation:true,pending_before_paid:true,payment_reload_replays_original:true,confirmation_reload:true,completion_replay_same_order:true,unauthenticated_confirmation_denied:true,payment_journal_cleared:true,languages:['en','zh'],widths:[390,1440],unique_orders:4,browser_errors:0,real_email_delivery:false}))
}finally{await browser?.close();if(app&&app.exitCode===null){app.kill();await once(app,'exit')}}
