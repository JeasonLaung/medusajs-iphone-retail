import assert from 'node:assert/strict'
import {readFileSync,writeFileSync} from 'node:fs'
import {execFileSync} from 'node:child_process'
import {resolve} from 'node:path'
import {chromium,expect as baseExpect} from '../../admin/node_modules/@playwright/test/index.mjs'
const expect=baseExpect.configure({timeout:20000}),shop='http://127.0.0.1:19300'
const logs=process.env.ACCOUNT_TEST_LOGS,owner=process.env.ACCOUNT_TEST_CUSTOMER_OWNER,errors=[],cases=[]
assert.match(owner??'',/^[0-9a-f]{12}$/)
assert.match(process.env.ACCOUNT_TEST_DB??'',/^retail_account_[0-9a-f]{12}$/)
const email=`local-account-${owner}-1@example.invalid`,password=`isolated-account-${owner}-password`,key='retail:customer-cart-transfer:v1'
const browser=await chromium.launch({headless:true,executablePath:resolve(process.env.LOCALAPPDATA,'ms-playwright/chromium-1234/chrome-win64/chrome.exe')})
try{
 for(const [index,{segment,width}] of [{segment:'gb-en',width:390},{segment:'gb-zh',width:1440}].entries()){
  const context=await browser.newContext({viewport:{width,height:1000},reducedMotion:'reduce'}),page=await context.newPage()
  page.on('pageerror',e=>errors.push(e.message))
  const read=async path=>{const r=await context.request.get(shop+'/api/store/'+path);assert.equal(r.status(),200);return r.json()}
  await page.goto(shop+'/'+segment+'/products/swappie-iphone-17e?v_id=variant_01M51RN7H7HPV9RH69T32QMD2Q');await page.getByTestId('add-product-button').click();await expect(page.getByTestId('checkout-button')).toBeVisible()
  const before=(await read('cart')).cart,amountsBefore=await read('carts/'+before.id+'/amounts'),viewBefore=await read('carts/'+before.id+'/view')
  const attempts=[];let lost=false
  await page.route('**/api/store/customer/cart-transfer',async route=>{
   const body=route.request().postDataJSON(),r=await route.fetch();attempts.push({body,status:r.status()})
   if(r.status()===200&&!lost){lost=true;await route.abort('failed')}else await route.fulfill({response:r})
  })
  await page.goto(shop+'/'+segment+'/account');await page.getByTestId('email-input').fill(email);await page.getByTestId('password-input').fill(password)
  const login=page.waitForResponse(r=>r.url().endsWith('/auth/login'));await page.getByTestId('sign-in-button').click();assert.equal((await login).status(),200)
  await expect(page.getByTestId('login-error-message')).toBeVisible()
  if(index===0){
   assert.equal(attempts.length,1);assert.equal(attempts[0].status,503)
   execFileSync(process.env.ACCOUNT_TEST_PYTHON,[resolve('../scripts/test-local-customer-account.py'),'--enable-owned-cart-transfer='+process.env.ACCOUNT_TEST_DB],{stdio:'pipe',windowsHide:true})
   await page.reload()
  }
  await expect.poll(()=>lost).toBe(true)
  // The committed transfer response was lost. Refresh must consume the durable
  // original journal, even though the customer cookie is already authenticated.
  const pending=await page.evaluate(key=>localStorage.getItem(key),key);assert.ok(pending)
  await page.reload();await expect(page.getByTestId(width<600?'mobile-account-nav':'account-nav')).toBeVisible()
  await expect.poll(()=>page.evaluate(key=>localStorage.getItem(key),key)).toBe(null)
  assert.equal(attempts.length,index===0?3:2);assert.ok(attempts.every(a=>JSON.stringify(a.body)===JSON.stringify(attempts[0].body)))
  assert.deepEqual(attempts.filter(a=>a.status===200).map(a=>a.status),[200,200])
  const after=(await read('cart')).cart,amountsAfter=await read('carts/'+before.id+'/amounts'),viewAfter=await read('carts/'+before.id+'/view')
  assert.equal(after.id,before.id);assert.equal(BigInt(after.version),BigInt(before.version)+1n)
  assert.equal(after.currency_code,before.currency_code);assert.equal(after.locale,before.locale)
  assert.deepEqual(amountsAfter.totals,amountsBefore.totals);assert.deepEqual(amountsAfter.lines,amountsBefore.lines);assert.deepEqual(viewAfter.items,viewBefore.items)
  await page.goto(shop+'/'+segment+'/cart');await expect(page.getByTestId('product-select-button')).toHaveValue('1')
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
  await page.screenshot({path:resolve(logs,'cart-login-'+segment+'.png')})
  await page.goto(shop+'/'+segment+'/account');await page.locator('[data-testid="logout-button"]:visible').click();await expect(page.getByTestId('login-page')).toBeVisible()
  assert.equal((await context.request.get(shop+'/api/store/customer/orders')).status(),401)
  assert.equal((await context.request.get(shop+'/api/store/cart')).status(),401)
  assert.equal((await context.request.get(shop+'/api/store/carts/'+before.id+'/view')).status(),401)
  assert.equal((await context.cookies(shop)).filter(c=>/^retail_(customer|cart)_/.test(c.name)).length,0)
  await page.unrouteAll({behavior:'wait'});await context.close()
  cases.push({segment,width,cart_id:before.id,version_before:before.version,version_after:after.version,disabled_gate_reproduced:index===0,lost_success_response_recovered:true,same_uuid_payload:true,items_and_amounts_preserved:true,logout_cleared_capabilities:true})
 }
 // A new browser with only the same account cannot discover a prior cart by
 // email. The source login transfers only the currently possessed cart.
 const other=await browser.newContext(),page=await other.newPage();await page.goto(shop+'/gb-en/account');await page.getByTestId('email-input').fill(email);await page.getByTestId('password-input').fill(password);await page.getByTestId('sign-in-button').click();await expect(page.getByTestId('customer-authenticated')).toBeVisible()
 assert.equal((await other.request.get(shop+'/api/store/cart')).status(),401)
 for(const c of cases)assert.equal((await other.request.get(shop+'/api/store/carts/'+c.cart_id+'/view')).status(),401)
 await other.close();assert.deepEqual(errors,[])
 writeFileSync(resolve(logs,'browser-result.json'),JSON.stringify({passed:true,scenario:'cart-login',verified_accounts:1,cases,cross_browser_without_cart_capability_denied:true,provider_called:false},null,2))
}finally{await browser.close()}
