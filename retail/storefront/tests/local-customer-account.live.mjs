import assert from 'node:assert/strict'
import {execFileSync} from 'node:child_process'
import {readFileSync,writeFileSync} from 'node:fs'
import {resolve} from 'node:path'
import {chromium,expect} from '../../admin/node_modules/@playwright/test/index.mjs'

// Independently callable account subflow: no checkout, provider calls or mail.
const shop='http://127.0.0.1:19300',owner=process.env.ACCOUNT_TEST_OWNER
assert.match(owner??'',/^[0-9a-f]{12}$/)
assert.ok(['en','zh'].includes(process.env.ACCOUNT_TEST_LANGUAGE))
const logs=process.env.ACCOUNT_TEST_LOGS,password='isolated-account-'+owner+'-password'
const browser=await chromium.launch({headless:true,executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH??resolve(process.env.LOCALAPPDATA,'ms-playwright/chromium-1234/chrome-win64/chrome.exe')})
const errors=[],cases=[]
try{
 for(const [index,segment,width] of [[1,'gb-en',1440],[2,'gb-zh',390]].filter(([,segment])=>segment===`gb-${process.env.ACCOUNT_TEST_LANGUAGE}`)){
  const context=await browser.newContext({viewport:{width,height:960}}),page=await context.newPage()
  page.on('pageerror',e=>errors.push(e.message))
  const email=`local-account-${owner}-${index}@example.invalid`
  const login=async(status=200)=>{
   await page.getByTestId('email-input').fill(email)
   await page.getByTestId('password-input').fill(password)
   const reply=page.waitForResponse(r=>new URL(r.url()).pathname==='/api/store/customer/auth/login')
   await page.getByTestId('sign-in-button').click()
   assert.equal((await reply).status(),status)
  }
  await page.goto(`${shop}/${segment}/account`,{waitUntil:'domcontentloaded'})
  await expect(page.getByTestId('login-page')).toBeVisible()
  await page.getByTestId('register-button').click()
  const form=page.getByTestId('register-page')
  for(const [id,value] of Object.entries({'first-name-input':'Isolated','last-name-input':'Customer','email-input':email,'password-input':password}))await form.getByTestId(id).fill(value)
  const registration=page.waitForResponse(r=>new URL(r.url()).pathname==='/api/store/customer/auth/register')
  await form.getByTestId('register-button').click()
  assert.equal((await registration).status(),202)
  await expect(page.getByTestId('register-verification-message')).toBeVisible()
  await page.goto(`${shop}/${segment}/account`,{waitUntil:'domcontentloaded'})
  await login(403)
  assert.equal((await context.cookies(shop)).some(c=>c.name==='retail_customer_session'),false)
  const linkFile=resolve(logs,`verification-${index}.txt`)
  execFileSync(process.env.ACCOUNT_TEST_PYTHON,[resolve('../scripts/test-local-customer-account.py'),'--verification-link',process.env.ACCOUNT_TEST_DB,email,linkFile],{stdio:'pipe'})
  const verified=page.waitForResponse(r=>new URL(r.url()).pathname==='/api/store/customer/auth/verify')
  await page.goto(readFileSync(linkFile,'utf8'),{waitUntil:'domcontentloaded'})
  assert.equal((await verified).status(),200)
  await page.goto(`${shop}/${segment}/account/orders`,{waitUntil:'domcontentloaded'})
  await login()
  await expect(page.getByTestId('customer-authenticated')).toBeVisible()
  await expect(page.getByTestId('no-orders-container')).toBeVisible()
  // The copied real Mollie guest order must not become this account's property.
  assert.equal((await context.request.get(shop+'/api/store/customer/orders/order_8bffc6dc-112e-4ed5-8a32-57421bc2ae4d')).status(),404)
  await page.goto(`${shop}/${segment}/account/profile`,{waitUntil:'domcontentloaded'})
  const name=page.getByTestId('account-name-editor')
  await name.getByTestId('edit-button').click()
  await name.getByTestId('first-name-input').fill('Updated')
  const profile=page.waitForResponse(r=>new URL(r.url()).pathname==='/api/store/customer/me'&&r.request().method()==='POST')
  await name.getByTestId('save-button').click()
  assert.equal((await profile).status(),200)
  await page.reload({waitUntil:'domcontentloaded'})
  await expect(page.getByTestId('account-name-editor').getByTestId('current-info')).toHaveText('Updated Customer')
  await page.goto(`${shop}/${segment}/account/addresses`,{waitUntil:'domcontentloaded'})
  await page.getByTestId('add-address-button').click()
  const address=page.getByTestId('add-address-modal')
  for(const [id,value] of Object.entries({'first-name-input':'Updated','last-name-input':'Customer','address-1-input':'10 Test Street','postal-code-input':'SW1A 1AA','city-input':'London'}))await address.getByTestId(id).fill(value)
  // CountrySelect retains a native form select behind the Ant Design control.
  await address.locator('select[name=country_code]').selectOption('gb',{force:true})
  const saved=page.waitForResponse(r=>new URL(r.url()).pathname==='/api/store/customer/addresses'&&r.request().method()==='POST')
  await address.getByTestId('save-button').click()
  assert.equal((await saved).status(),200)
  await expect(page.getByTestId('address-container')).toHaveCount(1)
  await page.reload({waitUntil:'domcontentloaded'})
  await expect(page.getByTestId('address-address')).toContainText('10 Test Street')
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
  await page.screenshot({path:resolve(logs,`account-${segment}.png`),fullPage:true})
  const deletion=page.waitForResponse(r=>new URL(r.url()).pathname.endsWith('/delete')&&r.request().method()==='POST')
  await page.getByTestId('address-delete-button').click()
  assert.equal((await deletion).status(),200)
  await expect(page.getByTestId('address-container')).toHaveCount(0)
  await page.goto(`${shop}/${segment}/account`,{waitUntil:'domcontentloaded'})
  await page.locator('[data-testid="logout-button"]:visible').click()
  await expect(page.getByTestId('login-page')).toBeVisible()
  assert.equal((await context.request.get(shop+'/api/store/customer/orders')).status(),401)
  await login()
  // Mobile overview intentionally displays its navigation instead of the
  // desktop overview body. Check the actual visible navigation and session.
  await expect(page.getByTestId(width<600?'mobile-account-nav':'account-nav')).toBeVisible()
  assert.equal((await context.request.get(shop+'/api/store/customer/me')).status(),200)
  cases.push({segment,width,registration:true,unverified_login_denied:true,encrypted_intent_link_verification:true,login:true,empty_own_orders:true,guest_order_denied:true,profile_persisted:true,address_create_read_delete:true,logout_revoked:true,relogin:true,no_overflow:true})
  await context.close()
 }
 assert.deepEqual(errors,[])
 assert.equal(cases.length,1)
 writeFileSync(resolve(logs,'browser-result.json'),JSON.stringify({passed:true,cases,browser_errors:0,real_email_delivery:false,verification_via_owned_encrypted_intent:true},null,2))
}finally{await browser.close()}
