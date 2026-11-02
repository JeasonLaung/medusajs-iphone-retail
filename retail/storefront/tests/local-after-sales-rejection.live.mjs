import assert from 'node:assert/strict'
import {readFileSync,writeFileSync} from 'node:fs'
import {execFileSync} from 'node:child_process'
import {resolve} from 'node:path'
import {chromium,expect as baseExpect} from '../../admin/node_modules/@playwright/test/index.mjs'
import {lostReply} from '../../admin/tests/checkout-repair.joint.mjs'
import {verifyCustomerEventMail} from './local-buyback-mail.live.mjs'

const expect=baseExpect.configure({timeout:15000}),shop='http://127.0.0.1:19300',origin='http://127.0.0.1:19200'
const logs=process.env.ACCOUNT_TEST_LOGS,owner=process.env.ACCOUNT_TEST_CUSTOMER_OWNER
assert.match(owner??'',/^[0-9a-f]{12}$/);assert.equal(process.env.WINDOWS_AFTER_SALES_REJECTION,'1')
const {order_id:orderID}=JSON.parse(readFileSync(resolve(logs,'context-resume.json'),'utf8'))
const copy=JSON.parse(readFileSync(resolve('src/components/account/translations/en.json'),'utf8'))
const credentials=readFileSync(resolve('../../.private-files/local/admin-credentials.txt'),'utf8')
const credential=name=>credentials.split(/\r?\n/).find(x=>new RegExp('^'+name+'\\s*[:=]','i').test(x))?.replace(/^[^:=]+[:=]\s*/,'').trim()
const browser=await chromium.launch({headless:true,executablePath:resolve(process.env.LOCALAPPDATA,'ms-playwright/chromium-1234/chrome-win64/chrome.exe')}),errors=[]
try{
 const context=await browser.newContext({viewport:{width:390,height:900},reducedMotion:'reduce'}),page=await context.newPage(),admin=await context.newPage()
 for(const p of [page,admin])p.on('pageerror',e=>errors.push(e.message))
 await page.goto(shop+'/gb-en/account/orders/details/'+orderID)
 await page.getByTestId('email-input').fill(`local-account-${owner}-1@example.invalid`);await page.getByTestId('password-input').fill('isolated-account-'+owner+'-password');await page.getByTestId('sign-in-button').click();await expect(page.getByTestId('order-details-container')).toBeVisible()
 const section=page.getByTestId('after-sales-section');await section.getByRole('button').first().click()
 await section.locator('.ant-select-selector').click();await page.locator('.ant-select-item-option').filter({hasText:copy.returnType}).click()
 await section.getByRole('checkbox').first().check();await section.getByRole('textbox').last().fill('Please review this unshipped return request; no device or cash has moved.')
 const submitted=await lostReply(page,'**/api/store/customer/orders/'+orderID+'/after-sales',section,()=>section.getByRole('button',{name:copy.submitRequest,exact:true}).click(),copy.retry,()=>expect(section).toContainText(copy.requestSuccess),201)
 const requestID=submitted.request.id
 assert.equal(submitted.request.items.length,1);assert.equal(submitted.request.items[0].device_ids?.length??0,0)
 writeFileSync(resolve(logs,'rejection-guard-input.json'),JSON.stringify({OrderID:orderID,RequestID:requestID}))
 try{
  const output=execFileSync(process.env.WINDOWS_MAIL_FIXTURE,['-test.run=^TestWindowsAfterSalesRejectionGuards$','-test.v'],{env:process.env,timeout:90000,windowsHide:true})
  writeFileSync(resolve(logs,'rejection-guards.log'),output)
 }catch(e){writeFileSync(resolve(logs,'rejection-guards.log'),Buffer.concat([e.stdout??Buffer.alloc(0),e.stderr??Buffer.alloc(0)]));throw Error('Owned rejection guards failed; inspect rejection-guards.log')}
 await admin.setViewportSize({width:1440,height:1000});await admin.addInitScript(()=>{if(!localStorage.getItem('eurorefurb.admin.language'))localStorage.setItem('eurorefurb.admin.language','en')})
 await admin.goto(origin+'/orders/'+orderID);await admin.getByLabel('Email',{exact:true}).fill(credential('Email'));await admin.getByLabel('Password',{exact:true}).fill(credential('Password'));await admin.getByRole('button',{name:'Sign in',exact:true}).click();await expect(admin.getByRole('button',{name:'Sign out',exact:true})).toBeVisible()
 await admin.goto(origin+'/orders/'+orderID)
 const panel=admin.getByTestId('order-after-sales'),row=panel.locator(`tr[data-row-key="${requestID}"]`)
 await expect(row.locator('.ant-tag')).toHaveText('Pending');await row.locator('.ant-select-selector').click();await admin.locator('.ant-select-item-option[title="Rejected"]').click()
 const commands=[],replies=[]
 await admin.route('**/api/admin/orders/'+orderID+'/after-sales',async route=>{
  if(route.request().method()!=='POST')return route.continue()
  commands.push(route.request().postDataJSON())
  if(commands.length===1){const responses=await Promise.all(Array.from({length:6},()=>route.fetch()));for(const response of responses){assert.equal(response.status(),200);replies.push(await response.json())}await route.abort('failed')}
  else{const response=await route.fetch();assert.equal(response.status(),200);assert.deepEqual(await response.json(),replies[0]);await route.fulfill({response})}
 })
 await row.getByRole('button',{name:'Save',exact:true}).click();await admin.getByRole('button',{name:'Confirm update',exact:true}).click()
 await expect(panel.locator('.ant-alert-error')).toBeVisible();await row.getByRole('button',{name:'Retry same operation',exact:true}).click();await expect(row.locator('.ant-tag')).toHaveText('Rejected')
 await admin.unrouteAll({behavior:'wait'});assert.equal(commands.length,2);assert.deepEqual(commands[0],commands[1]);for(const reply of replies)assert.deepEqual(reply,replies[0])
 await expect(row.getByRole('button',{name:'Save',exact:true})).toHaveCount(0)
 const csrf=(await context.cookies(origin)).find(c=>c.name==='retail_admin_csrf')?.value
 assert.ok(csrf)
 const headers={'Content-Type':'application/json','Origin':origin,'X-CSRF-Token':decodeURIComponent(csrf)}
 for(const body of [{...commands[0],status:'completed'},{...commands[0],operation_id:crypto.randomUUID()}]){
  const response=await admin.request.post(origin+'/api/admin/orders/'+orderID+'/after-sales',{headers,data:body});assert.equal(response.status(),409)
 }
 await admin.screenshot({path:resolve(logs,'rejection-admin-en.png'),fullPage:true})
 await admin.evaluate(()=>localStorage.setItem('eurorefurb.admin.language','zh'));await admin.reload();await expect(row.locator('.ant-tag')).toHaveText('已拒绝');await admin.screenshot({path:resolve(logs,'rejection-admin-zh.png'),fullPage:true})
 for(const segment of ['gb-en','gb-zh']){
  const translations=JSON.parse(readFileSync(resolve('src/components/account/translations/'+(segment==='gb-en'?'en':'zh')+'.json'),'utf8'))
  await page.goto(shop+'/'+segment+'/account/orders/details/'+orderID);await section.getByRole('button').first().click()
  const raw=await (await context.request.get(shop+'/api/store/customer/orders/'+orderID+'/after-sales')).json()
  assert.equal(raw.requests.length,1);assert.equal(raw.requests[0].id,requestID);assert.equal(raw.requests[0].status,'rejected');assert.ok(!JSON.stringify(raw).includes('actor_id'))
  await expect(section).toContainText(translations.declined);await expect(section).not.toContainText(translations.openRequest)
  const notices=await (await context.request.get(shop+'/api/store/customer/notifications?offset=0')).json();assert.deepEqual(notices.notifications.map(n=>n.kind),['after_sales.rejected'])
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
  await section.scrollIntoViewIfNeeded();await page.screenshot({path:resolve(logs,'rejection-customer-'+segment+'.png'),fullPage:true})
 }
 await admin.evaluate(()=>localStorage.setItem('eurorefurb.admin.language','en'));await admin.reload()
 const mail=await verifyCustomerEventMail({admin,origin,logs,expect,id:orderID,customerOwner:owner,expectedKinds:['after_sales.rejected'],safety:false})
 assert.deepEqual(errors,[])
 writeFileSync(resolve(logs,'browser-result.json'),JSON.stringify({passed:true,scenario:'rejection-mail',order_id:orderID,request_id:requestID,operation_id:commands[0].operation_id,customer_submission_lost_reply_recovered:true,six_concurrent_review_replies_identical:true,review_lost_reply_same_command:true,changed_payload_and_new_terminal_command_denied:true,customer_en_zh_mobile:true,admin_en_zh:true,...mail,guards:JSON.parse(readFileSync(resolve(logs,'rejection-guards.json'),'utf8')),no_new_payment_refund_or_shipment:true,browser_errors:0},null,2))
}catch(e){for(const [i,p]of browser.contexts().flatMap(c=>c.pages()).entries())await p.screenshot({path:resolve(logs,`rejection-failure-${i}.png`),fullPage:true}).catch(()=>{});throw e}finally{await browser.close()}
