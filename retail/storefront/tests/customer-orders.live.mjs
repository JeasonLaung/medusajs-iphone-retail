import assert from 'node:assert/strict'
import {spawn} from 'node:child_process'
import {once} from 'node:events'
import {createRequire} from 'node:module'
import {setTimeout as delay} from 'node:timers/promises'
const require=createRequire(new URL('../../admin/package.json',import.meta.url))
const {chromium,expect}=require('@playwright/test')
const target=new URL(process.env.CUSTOMER_ORDERS_TEST_GIN??'')
assert.equal(target.hostname,'127.0.0.1');assert.equal(target.protocol,'http:');assert.equal(target.pathname,'/');assert.ok(target.port)
const key=process.env.CUSTOMER_ORDERS_FIXTURE_KEY;assert.match(key??'',/^[0-9a-f]{64}$/)
const shop='http://127.0.0.1:19377',errors=[],requests=[]
let app,browser
const control=async path=>{
 const controller=new AbortController(),timer=setTimeout(()=>controller.abort(),1500)
 try{return await fetch(new URL('/__fixture/'+path,target),{method:path==='finish'?'POST':'GET',headers:{'X-Fixture-Key':key},signal:controller.signal})}
 finally{clearTimeout(timer)}
}
try{
 let fixture
 for(let i=0;i<250;i++){
  try{const response=await control('ready');if(response.ok){fixture=await response.json();break}}catch{}
  await delay(200)
 }
 assert.ok(fixture,'Owned SQL browser fixture did not become ready')
 for(const path of ['/api/store/regions','/api/store/locales','/api/store/product-categories?handle=iphone&limit=2','/api/store/collections?limit=6']){
  const response=await fetch(new URL(path,target)),body=await response.json()
  assert.equal(response.status,200,`Public shell dependency ${path}: ${JSON.stringify(body)}`)
 }
 const {loadPublicPage}=await import('../src/lib/public-page.ts')
 assert.ok(await loadPublicPage(target.origin,'dk-en'),'Owned routing region unavailable')
 app=spawn(process.execPath,['scripts/start.mjs'],{env:{...process.env,HOST:'127.0.0.1',PORT:'19377',GIN_API_ORIGIN:target.origin,STOREFRONT_ORIGIN:shop,CUSTOMER_BRIDGE_KEY:key},stdio:'ignore'})
 let ready=false
 for(let i=0;i<100;i++){if(app.exitCode!==null)throw Error('Owned Astro stopped');try{if((await fetch(shop+'/health')).ok){ready=true;break}}catch{}await delay(100)}
 assert.ok(ready,'Owned Astro not ready')
 browser=await chromium.launch({headless:true,executablePath:process.env.ADMIN_TEST_BROWSER||'C:/Program Files/Google/Chrome/Application/chrome.exe'})
 const context=await browser.newContext({viewport:{width:1440,height:1000},reducedMotion:'reduce'}),page=await context.newPage()
 page.on('pageerror',e=>errors.push(e.message))
 page.on('request',request=>requests.push(request.url()))
 const login=async(page,email)=>{
  await page.goto(shop+'/dk-en/account/orders')
  await expect(page.getByTestId('login-page')).toBeVisible()
  await page.getByTestId('email-input').fill(email)
  await page.getByTestId('password-input').fill(fixture.password)
  await page.getByTestId('sign-in-button').click()
  await expect(page.getByTestId('customer-authenticated')).toBeVisible()
 }
 await login(page,fixture.email)
 await expect(page.getByTestId('no-orders-container')).toBeVisible()
 const cookies=await context.cookies();assert.equal(cookies.find(c=>c.name==='retail_customer_session')?.httpOnly,true)
 assert.equal(cookies.find(c=>c.name==='retail_customer_csrf')?.httpOnly,false)
 await context.addCookies([{name:'retail_cart_possession',value:fixture.cart_token,url:shop,httpOnly:true,sameSite:'Lax'}])
 const attached=page.waitForResponse(response=>response.url().endsWith(`/customer/orders/${fixture.order_id}/attach`)&&response.request().method()==='POST')
 await page.goto(shop+`/dk-en/order/${fixture.order_id}/confirmed`)
 const attachment=await attached;assert.equal(attachment.status(),200);assert.equal(attachment.headers()['cache-control'],'private, no-store')
 await expect(page.getByTestId('order-complete-container')).toBeVisible()
 await expect(page.getByTestId('order-account-attachment-error')).toHaveCount(0)
 // Account reads must survive losing the independent checkout capability.
 await context.clearCookies({name:'retail_cart_possession'})
 for(const segment of ['dk-en','dk-zh'])for(const width of [390,1440]){
  await page.setViewportSize({width,height:1000})
  const response=await page.goto(shop+`/${segment}/account/orders`)
  assert.equal(response.status(),200);assert.equal(response.headers()['cache-control'],'private, no-store')
  await expect(page.getByTestId('order-card')).toHaveCount(1)
  await expect(page.getByTestId('item-sku')).not.toBeEmpty()
  await expect(page.getByTestId('order-amount')).toContainText('158.75')
  await page.getByTestId('order-details-link').click()
  await expect(page.getByTestId('order-details-container')).toBeVisible()
  await expect(page.getByTestId('shipping-address-summary')).not.toBeEmpty()
  await expect(page.getByTestId('cart-total')).toHaveAttribute('data-value','158.75')
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
  await page.reload();await expect(page.getByTestId('order-details-container')).toBeVisible()
 }
 await page.goto(shop+'/dk-en/account')
 await expect(page.getByTestId('order-wrapper')).toHaveCount(1)
 const other=await browser.newContext({viewport:{width:1440,height:1000}}),otherPage=await other.newPage()
 otherPage.on('pageerror',e=>errors.push(e.message))
 await login(otherPage,'other-order-owner@example.invalid')
 await expect(otherPage.getByTestId('no-orders-container')).toBeVisible()
 const denied=await otherPage.evaluate(async id=>{const response=await fetch('/api/store/customer/orders/'+id);return {status:response.status,cache:response.headers.get('cache-control')}},fixture.order_id)
 assert.deepEqual(denied,{status:404,cache:'private, no-store'})
 await page.getByTestId('account-nav').getByTestId('logout-button').click()
 await expect(page.getByTestId('login-page')).toBeVisible()
 const expired=await page.evaluate(async()=>{const response=await fetch('/api/store/customer/orders');return response.status})
 assert.equal(expired,401)
 assert.deepEqual(errors,[])
 assert.equal(requests.some(url=>/localhost:(8000|9001)|127\.0\.0\.1:(8000|9001)|medusa/i.test(url)),false)
 assert.equal((await control('finish')).status,204)
 console.log('CUSTOMER_ORDERS_JOINT_BROWSER '+JSON.stringify({actual_astro_routes:true,actual_gin_handlers:true,real_mysql_redis:true,mocked_api_responses:false,fixture_preverified_accounts:true,fixture_order_from_paid_cart_rehearsal:true,real_provider_payment:false,login_origin_csrf_signed_bridge:true,empty_before_attach:true,dual_cookie_attachment:true,account_details_without_cart_cookie:true,private_headers:true,cross_account_denied:true,logout_revokes_access:true,exact_total:'158.75',languages:['en','zh'],widths:[390,1440],browser_errors:0}))
}finally{
 await browser?.close()
 if(app&&app.exitCode===null){app.kill();await once(app,'exit')}
}
