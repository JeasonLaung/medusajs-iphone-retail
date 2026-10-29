import assert from 'node:assert/strict'
import {spawn} from 'node:child_process'
import {once} from 'node:events'
import {createRequire} from 'node:module'
import {setTimeout as delay} from 'node:timers/promises'
const require=createRequire(new URL('../../admin/package.json',import.meta.url)),{chromium,expect}=require('@playwright/test')
const target=new URL(process.env.CUSTOMER_ORDERS_TEST_GIN??''),key=process.env.PROMOTION_FIXTURE_KEY,shop='http://127.0.0.1:19378'
assert.equal(target.hostname,'127.0.0.1');assert.equal(target.protocol,'http:');assert.equal(target.pathname,'/');assert.ok(target.port);assert.match(key??'',/^[0-9a-f]{64}$/)
let app,browser
const control=async path=>{
 const controller=new AbortController(),timer=setTimeout(()=>controller.abort(),1500)
 try{return await fetch(new URL('/__fixture/'+path,target),{method:path==='finish'?'POST':'GET',headers:{'X-Fixture-Key':key},signal:controller.signal})}finally{clearTimeout(timer)}
}
try{
 let fixture
 for(let i=0;i<250;i++){try{const response=await control('ready');if(response.ok){fixture=await response.json();break}}catch{}await delay(200)}
 assert.ok(fixture,'Owned promotion fixture not ready')
 const {loadPublicPage}=await import('../src/lib/public-page.ts');assert.ok(await loadPublicPage(target.origin,'dk-en'))
 app=spawn(process.execPath,['scripts/start.mjs'],{env:{...process.env,HOST:'127.0.0.1',PORT:'19378',GIN_API_ORIGIN:target.origin,STOREFRONT_ORIGIN:shop,CUSTOMER_BRIDGE_KEY:''},stdio:'ignore'})
 let ready=false
 for(let i=0;i<100;i++){if(app.exitCode!==null)throw Error('Owned Astro stopped');try{if((await fetch(shop+'/health')).ok){ready=true;break}}catch{}await delay(100)}assert.ok(ready)
 browser=await chromium.launch({headless:true,executablePath:process.env.ADMIN_TEST_BROWSER||'C:/Program Files/Google/Chrome/Application/chrome.exe'})
 const errors=[],writes=[]
 for(const segment of ['dk-en','dk-zh'])for(const width of [390,1440]){
  const context=await browser.newContext({viewport:{width,height:1100},reducedMotion:'reduce'}),page=await context.newPage()
  page.on('pageerror',e=>errors.push(e.message))
  page.on('request',request=>{if(request.method()==='POST'&&request.url().endsWith('/promotions'))writes.push(request.postDataJSON())})
  await context.addCookies([{name:'retail_cart_possession',value:fixture.cart_token,url:shop,httpOnly:true,sameSite:'Lax'},{name:'retail_cart_csrf',value:fixture.cart_csrf,url:shop,sameSite:'Lax'}])
  for(const endpoint of ['/api/store/cart',`/api/store/carts/${fixture.cart_id}/view`]){
   const response=await context.request.get(shop+endpoint)
   assert.equal(response.status(),200,`${endpoint}: ${await response.text()}`)
  }
  const path=`/api/store/carts/${fixture.cart_id}/promotions`
  await page.goto(shop+`/${segment}/checkout?step=delivery`)
  await expect(page.getByTestId('discount-input')).toBeEnabled()
  await expect(page.getByTestId('discount-code-tag')).toHaveCount(1)
  await expect(page.getByTestId('discount-code-tag')).toContainText('COMMAND_AUTO')
  await expect(page.getByTestId('discount-code-tag').getByRole('button')).toHaveCount(0)
  await expect(page.getByTestId('checkout-total')).toHaveText(/\d/)
  // Delivery keeps the original order-details section collapsed. textContent
  // reads its amount; innerText is empty for descendants of closed <details>.
  const baseline=await page.getByTestId('checkout-total').textContent()
  assert.ok(baseline?.trim())
  const read=async()=>page.evaluate(async path=>{const response=await fetch(path);return {status:response.status,cache:response.headers.get('cache-control'),body:await response.json()}},path)
  const before=await read();assert.equal(before.status,200);assert.equal(before.cache,'private, no-store')
  await page.getByTestId('discount-input').fill('COMMAND10')
  const saved=page.waitForResponse(r=>r.request().method()==='POST'&&r.url().endsWith(path))
  await page.getByTestId('discount-apply-button').click();assert.equal((await saved).status(),200)
  await expect(page.getByTestId('discount-code-tag')).toHaveCount(2)
  await expect(page.getByTestId('checkout-total')).not.toHaveText(baseline)
  const after=await read();assert.equal(BigInt(after.body.version),BigInt(before.body.version)+1n)
  assert.deepEqual(after.body.promotions.map(p=>p.code).sort(),['COMMAND10','COMMAND_AUTO'])
  await page.reload();await expect(page.getByTestId('discount-code-tag')).toHaveCount(2)
  await page.getByTestId('discount-code-tag').filter({hasText:'COMMAND10'}).getByRole('button').click()
  await expect(page.getByTestId('discount-code-tag')).toHaveCount(1)
  await expect(page.getByTestId('checkout-total')).toHaveText(baseline)
  await expect(page.getByTestId('discount-input')).toBeEnabled()
  await page.getByTestId('discount-input').fill('UNKNOWN_LIVE_CODE')
  const invalid=page.waitForResponse(r=>r.request().method()==='POST'&&r.url().endsWith(path))
  await page.getByTestId('discount-apply-button').click();assert.equal((await invalid).status(),400)
  await expect(page.getByTestId('discount-error-message')).toBeVisible()
  const rejected=await read();assert.equal(BigInt(rejected.body.version),BigInt(before.body.version)+2n)
  await expect(page.getByTestId('checkout-total')).toHaveText(baseline)
  assert.equal(await page.evaluate(()=>localStorage.getItem('retail:checkout-promotion-intent:v1')),null)
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
  await context.close()
 }
 assert.equal(writes.length,12);assert.deepEqual(errors,[])
 assert.equal((await control('finish')).status,204)
 console.log('PROMOTION_JOINT_BROWSER '+JSON.stringify({actual_astro_checkout:true,actual_gin_handlers:true,real_mysql_redis:true,mocked_api_responses:false,manual_code_added_and_removed:true,automatic_retained_and_nonremovable:true,exact_total_restored:true,version_incremented_twice_per_pair:true,refresh_persists_discount:true,unknown_code_400_no_amount_or_version_change:true,private_no_store:true,csrf_cart_cookie_transport:true,languages:['en','zh'],widths:[390,1440],browser_errors:0,real_external_payments_or_mail:false}))
}finally{await browser?.close();if(app&&app.exitCode===null){app.kill();await once(app,'exit')}}
