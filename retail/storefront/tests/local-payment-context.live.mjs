import assert from 'node:assert/strict'
import {execFileSync} from 'node:child_process'
import {readFileSync,writeFileSync} from 'node:fs'
import {resolve} from 'node:path'
import {chromium,expect as baseExpect} from '../../admin/node_modules/@playwright/test/index.mjs'
import {prepareLocalRefundPayment} from './local-refund-payment.live.mjs'
const expect=baseExpect.configure({timeout:30000}),origin='http://127.0.0.1:19300'
const owner=process.env.ACCOUNT_TEST_OWNER,logs=process.env.ACCOUNT_TEST_LOGS
assert.match(owner??'',/^[0-9a-f]{12}$/)
const email=`local-account-${owner}-1@example.invalid`,password='isolated-account-'+owner+'-password'
const browser=await chromium.launch({headless:true,executablePath:resolve(process.env.LOCALAPPDATA,'ms-playwright/chromium-1234/chrome-win64/chrome.exe')})
const errors=[],steps=[]
try{
 const context=await browser.newContext({viewport:{width:390,height:900},reducedMotion:'reduce'}),page=await context.newPage()
 page.on('pageerror',e=>errors.push(e.message))
 await page.goto(origin+'/gb-en/account');await page.getByTestId('register-button').click()
 const form=page.getByTestId('register-page')
 for(const [id,value] of Object.entries({'first-name-input':'Context','last-name-input':'Customer','email-input':email,'password-input':password}))await form.getByTestId(id).fill(value)
 const registration=page.waitForResponse(r=>r.url().endsWith('/auth/register')&&r.request().method()==='POST')
 await form.getByTestId('register-button').click();assert.equal((await registration).status(),202)
 await expect(page.getByTestId('register-verification-message')).toBeVisible()
 const link=resolve(logs,'context-verification.txt')
 execFileSync(process.env.ACCOUNT_TEST_PYTHON,[resolve('../scripts/test-local-customer-account.py'),'--verification-link',process.env.ACCOUNT_TEST_DB,email,link],{stdio:'pipe'})
 const verified=page.waitForResponse(r=>r.url().endsWith('/auth/verify'))
 await page.goto(readFileSync(link,'utf8'));assert.equal((await verified).status(),200)
 await page.goto(origin+'/gb-en/account/orders');await page.getByTestId('email-input').fill(email);await page.getByTestId('password-input').fill(password)
 const login=page.waitForResponse(r=>r.url().endsWith('/auth/login'));await page.getByTestId('sign-in-button').click();assert.equal((await login).status(),200)
 await expect(page.getByTestId('customer-authenticated')).toBeVisible()
 const paymentKey='retail:checkout-payment-intent:v1',contextKey='retail:cart-context-intent:v1'
 const journal=key=>page.evaluate(key=>JSON.parse(localStorage.getItem(key)),key)
 let cartID,original
 const orderID=await prepareLocalRefundPayment({page,context,origin,email,logs,beforePlaceOrder:async()=>{
  original=await journal(paymentKey);assert.ok(original?.operationID);cartID=original.cartID
  const open=async()=>{
   await page.locator('astro-island[component-url*="gin-header"]:not([ssr])').waitFor({state:'attached'})
   await page.getByRole('button',{name:/^(Country and language|国家与语言)$/,exact:true}).click()
   await expect(page.getByRole('dialog')).toBeVisible()
  }
  const current=async()=>{const r=await context.request.get(origin+'/api/store/cart');assert.equal(r.status(),200);return (await r.json()).cart}
  for(const target of ['zh','en']){
   const requests=[]
   await page.route('**/context-payment',async route=>{
    requests.push({body:route.request().postDataJSON(),key:route.request().headers()['idempotency-key']})
    const response=await route.fetch();assert.equal(response.status(),200)
    if(requests.length===1)await route.abort('failed');else await route.fulfill({response})
   })
   try{
    const before=await current();await page.goto(origin+'/gb-'+(target==='zh'?'en':'zh')+'/about?context=live#contact')
    const choose=async()=>{
     const dialog=page.getByRole('dialog')
     await dialog.getByRole('button',{name:/^United Kingdom — /}).filter({hasText:target==='zh'?'中文':'EN'}).click()
    }
    await open();await choose();await expect(page.getByRole('dialog').getByRole('alert')).toBeVisible()
    const pending=await journal(contextKey);assert.equal(pending.kind,'payment-save')
    await page.reload();assert.deepEqual(await journal(contextKey),pending);await open();await choose()
    await expect(page).toHaveURL(origin+'/gb-'+target+'/about?context=live#contact')
    await expect(page.getByRole('dialog')).toHaveCount(0)
    assert.equal(requests.length,2);assert.deepEqual(requests[0],requests[1])
    assert.deepEqual(await journal(paymentKey),original);assert.equal(await journal(contextKey),null)
    const after=await current();assert.equal(after.id,cartID);assert.equal(BigInt(after.version),BigInt(before.version)+1n)
    const amounts=await context.request.get(origin+'/api/store/carts/'+cartID+'/amounts');assert.equal(amounts.status(),200)
    const priced=await amounts.json();assert.equal(priced.totals.total,'789');assert.equal(priced.currency_code,'eur')
    steps.push({locale:after.locale,version:after.version,same_payment_intent:true,total:priced.totals.total,exact_retry:true})
   }finally{await page.unrouteAll({behavior:'wait'})}
  }
  await page.goto(origin+'/gb-en/checkout?step=payment');await expect(page.getByTestId('submit-order-button')).toBeEnabled()
  assert.deepEqual(await journal(paymentKey),original)
 }})
 assert.equal(steps.length,2);assert.deepEqual(errors,[])
 await page.screenshot({path:resolve(logs,'context-owned-order.png'),fullPage:true})
 writeFileSync(resolve(logs,'browser-result.json'),JSON.stringify({passed:true,scenario:'payment-context',cart_id:cartID,order_id:orderID,steps,real_mollie_test_payment:true,same_payment_after_two_context_saves:true,lost_success_response_and_reload_recovery:true,automatic_order_ownership:true,browser_errors:0,real_email_delivery:false},null,2))
}catch(e){for(const p of browser.contexts().flatMap(c=>c.pages())){console.error('FAILURE',p.url(),await p.locator('body').innerText().catch(()=>''));await p.screenshot({path:resolve(logs,'context-failure.png'),fullPage:true}).catch(()=>{})}throw e}finally{await browser.close()}
