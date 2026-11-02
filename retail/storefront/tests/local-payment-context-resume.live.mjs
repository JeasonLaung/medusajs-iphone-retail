// Read an already completed test order from its owned SQL checkpoint. Never
// create another remote payment to compensate for a browser assertion failure.
import assert from 'node:assert/strict'
import {readFileSync,writeFileSync} from 'node:fs'
import {resolve} from 'node:path'
import {chromium,expect as baseExpect} from '../../admin/node_modules/@playwright/test/index.mjs'
const expect=baseExpect.configure({timeout:15000}),origin='http://127.0.0.1:19300'
const logs=process.env.ACCOUNT_TEST_LOGS,owner=process.env.ACCOUNT_TEST_CUSTOMER_OWNER
assert.match(owner??'',/^[0-9a-f]{12}$/)
const paid=JSON.parse(readFileSync(resolve(logs,'context-resume.json'),'utf8')),errors=[]
const browser=await chromium.launch({headless:true,executablePath:resolve(process.env.LOCALAPPDATA,'ms-playwright/chromium-1234/chrome-win64/chrome.exe')})
try{
 const context=await browser.newContext({viewport:{width:390,height:900},reducedMotion:'reduce'}),page=await context.newPage()
 page.on('pageerror',e=>errors.push(e.message))
 await page.goto(origin+'/gb-en/account/orders')
 await page.getByTestId('email-input').fill(`local-account-${owner}-1@example.invalid`)
 await page.getByTestId('password-input').fill('isolated-account-'+owner+'-password')
 const login=page.waitForResponse(r=>r.url().endsWith('/auth/login'));await page.getByTestId('sign-in-button').click();assert.equal((await login).status(),200)
 const response=await context.request.get(origin+'/api/store/customer/orders/'+paid.order_id);assert.equal(response.status(),200)
 await page.goto(origin+'/gb-en/account/orders/details/'+paid.order_id)
 await expect(page.getByTestId('order-details-container')).toBeVisible();await expect(page.getByTestId('order-details-container')).toContainText('790')
 assert.deepEqual(errors,[]);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
 await page.screenshot({path:resolve(logs,'context-changed-owned-order.png'),fullPage:true})
 writeFileSync(resolve(logs,'browser-result.json'),JSON.stringify({passed:true,scenario:'payment-context-changed',...paid,real_mollie_test_payment:true,completed_checkpoint_customer_read:true,no_new_remote_payment_on_resume:true,browser_errors:0},null,2))
}finally{await browser.close()}
