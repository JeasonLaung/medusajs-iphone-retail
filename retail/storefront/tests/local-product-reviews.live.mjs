// Continue an already paid owned checkpoint; reviews never create payments.
import assert from 'node:assert/strict'
import {randomUUID} from 'node:crypto'
import {readFileSync,writeFileSync} from 'node:fs'
import {resolve} from 'node:path'
import {chromium,expect as baseExpect} from '../../admin/node_modules/@playwright/test/index.mjs'
const expect=baseExpect.configure({timeout:15000}),shop='http://127.0.0.1:19300',adminOrigin='http://127.0.0.1:19200'
const logs=process.env.ACCOUNT_TEST_LOGS,owner=process.env.ACCOUNT_TEST_CUSTOMER_OWNER
assert.match(owner??'',/^[0-9a-f]{12}$/)
const paid=JSON.parse(readFileSync(resolve(logs,'context-resume.json'),'utf8')),errors=[]
const credentials=readFileSync(resolve('../../.private-files/local/admin-credentials.txt'),'utf8'),credential=name=>credentials.split(/\r?\n/).find(x=>new RegExp('^'+name+'\\s*[:=]','i').test(x))?.replace(/^[^:=]+[:=]\s*/,'').trim()
const browser=await chromium.launch({headless:true,executablePath:resolve(process.env.LOCALAPPDATA,'ms-playwright/chromium-1234/chrome-win64/chrome.exe')})
try{
 const context=await browser.newContext({viewport:{width:390,height:900},reducedMotion:'reduce'}),page=await context.newPage()
 page.on('pageerror',e=>errors.push(e.message))
 await page.goto(shop+'/gb-en/account/orders');await page.getByTestId('email-input').fill(`local-account-${owner}-1@example.invalid`);await page.getByTestId('password-input').fill('isolated-account-'+owner+'-password')
 const login=page.waitForResponse(r=>r.url().endsWith('/auth/login'));await page.getByTestId('sign-in-button').click();assert.equal((await login).status(),200)
 await page.goto(shop+'/gb-en/account/orders/details/'+paid.order_id)
 const section=page.getByTestId('order-reviews');await expect(section.getByRole('button',{name:'Write a review',exact:true})).toBeEnabled()
 await section.getByRole('button',{name:'Write a review',exact:true}).click()
 const form=page.getByTestId('review-form'),title='Windows verified review '+owner,content='The purchased phone works well. <script>window.reviewInjected=true</script>'
 await form.getByRole('radio',{name:'5 out of 5 stars',exact:true}).click();await form.getByLabel('Review title',{exact:true}).fill(title);await form.getByLabel('Your experience',{exact:true}).fill(content)
 const submissions=[]
 await page.route('**/customer/product-reviews',async route=>{
  submissions.push(route.request().postDataJSON());const response=await route.fetch();assert.equal(response.status(),201)
  if(submissions.length===1)await route.abort('failed');else await route.fulfill({response})
 })
 try{
  await form.getByRole('button',{name:'Submit review',exact:true}).click();await expect(form.getByRole('button',{name:'Retry the same request',exact:true})).toBeVisible()
  await expect(form.getByLabel('Review title',{exact:true})).toBeDisabled();await form.getByRole('button',{name:'Retry the same request',exact:true}).click();await expect(form).toHaveCount(0)
  assert.equal(submissions.length,2);assert.deepEqual(submissions[0],submissions[1])
 }finally{await page.unrouteAll({behavior:'wait'})}
 const productID=submissions[0].product_id
 const publicRead=async()=>{const r=await context.request.get(shop+'/api/store/product-reviews?product_id='+productID+'&offset=0&limit=5');assert.equal(r.status(),200);return r.json()}
 assert.equal((await publicRead()).reviews.some(r=>r.title===title),false)
 const csrf=(await context.cookies(shop)).find(c=>c.name==='retail_customer_csrf').value
 const duplicate=await context.request.post(shop+'/api/store/customer/product-reviews',{headers:{Origin:shop,'X-CSRF-Token':csrf},data:{...submissions[0],operation_id:randomUUID()}});assert.equal(duplicate.status(),409)
 const adminContext=await browser.newContext({viewport:{width:1440,height:1000},reducedMotion:'reduce'}),admin=await adminContext.newPage()
 admin.on('pageerror',e=>errors.push(e.message));await admin.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','en'))
 await admin.goto(adminOrigin+'/account');await admin.getByLabel('Email',{exact:true}).fill(credential('Email'));await admin.getByLabel('Password',{exact:true}).fill(credential('Password'));await admin.getByRole('button',{name:'Sign in',exact:true}).click();await expect(admin.getByRole('button',{name:'Sign out',exact:true})).toBeVisible()
 await admin.goto(adminOrigin+'/products/'+productID)
 const moderation=admin.getByTestId('admin-product-reviews'),row=moderation.getByTestId('admin-product-review').filter({hasText:title})
 await expect(row).toContainText('Pending');await expect(row).toContainText('Verified purchase')
 const commands=[]
 await admin.route('**/api/admin/product-reviews/*',async route=>{
  if(route.request().method()!=='POST'){await route.continue();return}
  commands.push(route.request().postDataJSON());const response=await route.fetch();assert.equal(response.status(),200)
  if(commands.length===1)await route.abort('failed');else await route.fulfill({response})
 })
 try{
  await row.getByRole('button',{name:'Approve',exact:true}).click();await moderation.getByRole('button',{name:'Retry this save',exact:true}).click();await expect(row).toContainText('Published')
  assert.equal(commands.length,2);assert.deepEqual(commands[0],commands[1])
 }finally{await admin.unrouteAll({behavior:'wait'})}
 for(const [button,state,visible] of [['Reject','Not published',false],['Set to pending','Pending',false],['Approve','Published',true]]){
  await row.getByRole('button',{name:button,exact:true}).click();await expect(row).toContainText(state)
  assert.equal((await publicRead()).reviews.some(r=>r.title===title),visible)
 }
 const published=await publicRead(),review=published.reviews.find(r=>r.title===title)
 assert.equal(review.verified_purchase,true);assert.equal(review.rating,5)
 const raw=JSON.stringify(published);for(const secret of [paid.order_id,`local-account-${owner}-1@example.invalid`,'account_id'])assert.ok(!raw.includes(secret))
 for(const segment of ['gb-en','gb-zh']){
  const url=shop+'/'+segment+'/products/swappie-iphone-17e',html=await (await context.request.get(url)).text();assert.ok(html.includes(title));assert.ok(!html.includes('<script>window.reviewInjected=true</script>'))
  await page.goto(url);await expect(page.getByTestId('product-reviews').getByText(title,{exact:true})).toBeVisible()
  assert.equal(await page.evaluate(()=>window.reviewInjected),undefined);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
 }
 assert.deepEqual(errors,[]);await page.screenshot({path:resolve(logs,'published-review-zh.png'),fullPage:true})
 writeFileSync(resolve(logs,'browser-result.json'),JSON.stringify({passed:true,scenario:'reviews',...paid,review_id:review.id,actual_customer_submit:true,actual_admin_moderation:true,linked_paid_unshipped_purchase:true,duplicate_rejected:true,lost_submission_and_moderation_exact_retry:true,public_approved_only:true,escaped_content:true,private_order_identity_not_public:true,en_zh_mobile_public_checked:true,no_new_remote_payment:true,browser_errors:0},null,2))
}catch(e){for(const p of browser.contexts().flatMap(c=>c.pages())){console.error('FAILURE',p.url(),await p.locator('body').innerText().catch(()=>''));await p.screenshot({path:resolve(logs,'review-failure-'+browser.contexts().flatMap(c=>c.pages()).indexOf(p)+'.png'),fullPage:true}).catch(()=>{})}throw e}finally{await browser.close()}
