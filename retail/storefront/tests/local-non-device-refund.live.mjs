import assert from 'node:assert/strict'
import {readFileSync,writeFileSync} from 'node:fs'
import {execFileSync} from 'node:child_process'
import {resolve} from 'node:path'
import {chromium,expect as baseExpect} from '../../admin/node_modules/@playwright/test/index.mjs'
import {lostReply} from '../../admin/tests/checkout-repair.joint.mjs'
const expect=baseExpect.configure({timeout:15000}),shop='http://127.0.0.1:19300',adminOrigin='http://127.0.0.1:19200'
const logs=process.env.ACCOUNT_TEST_LOGS,owner=process.env.ACCOUNT_TEST_CUSTOMER_OWNER
assert.match(owner??'',/^[0-9a-f]{12}$/)
assert.equal(process.env.WINDOWS_NON_DEVICE_REFUND,'1')
const paid=JSON.parse(readFileSync(resolve(logs,'context-resume.json'),'utf8')),errors=[]
const credentials=readFileSync(resolve('../../.private-files/local/admin-credentials.txt'),'utf8'),credential=name=>credentials.split(/\r?\n/).find(x=>new RegExp('^'+name+'\\s*[:=]','i').test(x))?.replace(/^[^:=]+[:=]\s*/,'').trim()
const copy=JSON.parse(readFileSync(resolve('src/components/account/translations/en.json'),'utf8'))
const browser=await chromium.launch({headless:true,executablePath:resolve(process.env.LOCALAPPDATA,'ms-playwright/chromium-1234/chrome-win64/chrome.exe')})
try{
 const context=await browser.newContext({viewport:{width:390,height:900},reducedMotion:'reduce'}),page=await context.newPage()
 page.on('pageerror',e=>errors.push(e.message))
 await page.goto(shop+'/gb-en/account/orders');await page.getByTestId('email-input').fill(`local-account-${owner}-1@example.invalid`);await page.getByTestId('password-input').fill('isolated-account-'+owner+'-password')
 const login=page.waitForResponse(r=>r.url().endsWith('/auth/login'));await page.getByTestId('sign-in-button').click();assert.equal((await login).status(),200)
 await page.goto(shop+'/gb-en/account/orders/details/'+paid.order_id)
 const section=page.getByTestId('after-sales-section');await section.getByRole('button').first().click()
 await section.locator('.ant-select-selector').click();await page.locator('.ant-select-item-option').filter({hasText:copy.returnType}).click()
 await section.getByRole('checkbox').first().check();assert.equal(await section.getByRole('checkbox').count(),1)
 await section.getByRole('textbox').last().fill('Please refund this unshipped item using the original payment.')
 const request=await lostReply(page,'**/api/store/customer/orders/'+paid.order_id+'/after-sales',section,()=>section.getByRole('button',{name:copy.submitRequest,exact:true}).click(),copy.retry,()=>expect(section).toContainText(copy.requestSuccess),201)
 assert.equal(request.request.type,'return');assert.equal(request.request.items.length,1);assert.equal(request.request.items[0].device_ids?.length??0,0)
 const requestID=request.request.id
 const adminContext=await browser.newContext({viewport:{width:1440,height:1000},reducedMotion:'reduce'}),admin=await adminContext.newPage()
 admin.on('pageerror',e=>errors.push(e.message));await admin.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','en'))
 await admin.goto(adminOrigin+'/account');await admin.getByLabel('Email',{exact:true}).fill(credential('Email'));await admin.getByLabel('Password',{exact:true}).fill(credential('Password'));await admin.getByRole('button',{name:'Sign in',exact:true}).click();await expect(admin.getByRole('button',{name:'Sign out',exact:true})).toBeVisible()
 await admin.goto(adminOrigin+'/orders/'+paid.order_id)
 const panel=admin.getByTestId('order-after-sales').locator('tr[data-row-key="'+requestID+'"]')
 await expect(panel.getByText('Pending',{exact:true})).toBeVisible();await panel.getByRole('button',{name:'Save',exact:true}).click();await admin.getByRole('button',{name:'Confirm update',exact:true}).click();await expect(panel.locator('.ant-tag')).toHaveText('Processing')
 await panel.getByRole('button',{name:'Refund history and actions',exact:true}).click();const drawer=admin.getByRole('dialog')
 await expect(drawer).toContainText('A refund does not receive goods')
 const payment=drawer.getByLabel('Original payment',{exact:true});await payment.focus();await payment.press('ArrowDown');await payment.press('Enter')
 await drawer.getByLabel('Refund amount (two decimal places)',{exact:true}).fill('789.00')
 const command=await lostReply(admin,'**/after-sales/'+requestID+'/refunds',drawer,()=>drawer.getByRole('button',{name:'Submit refund instruction',exact:true}).click(),'Retry same refund operation',()=>expect(drawer.getByText('Instruction accepted. Check its processing status.',{exact:true})).toBeVisible(),202)
 assert.equal(command.amount,'789.00');assert.equal(command.applied,false)
 writeFileSync(resolve(logs,'non-device-command.json'),JSON.stringify(command))
 try{
  const output=execFileSync(process.env.WINDOWS_NON_DEVICE_FIXTURE,['-test.run=^TestWindowsNonDeviceRefund$','-test.v'],{cwd:resolve('../backend'),env:process.env,timeout:120000,windowsHide:true})
  writeFileSync(resolve(logs,'non-device-fixture.log'),output)
 }catch(e){writeFileSync(resolve(logs,'non-device-fixture.log'),Buffer.concat([e.stdout??Buffer.alloc(0),e.stderr??Buffer.alloc(0)]));throw Error('Owned refund fixture failed; inspect non-device-fixture.log')}
 await admin.reload();await panel.getByRole('button',{name:'Refund history and actions',exact:true}).click();await expect(drawer.getByText('Posted',{exact:true})).toBeVisible()
 await expect(drawer.getByRole('button',{name:'Submit refund instruction',exact:true})).toBeDisabled();await admin.screenshot({path:resolve(logs,'non-device-refund-admin.png')})
 await drawer.getByRole('button',{name:'Close',exact:true}).click();await panel.locator('.ant-select-selector').click();await admin.locator('.ant-select-item-option[title="Completed"]').click();await panel.getByRole('button',{name:'Save',exact:true}).click();await admin.getByRole('button',{name:'Confirm update',exact:true}).click();await expect(panel.locator('.ant-tag')).toHaveText('Completed')
 const reads=[]
 for(const segment of ['gb-en','gb-zh']){
  await page.goto(shop+'/'+segment+'/account/orders/details/'+paid.order_id);await expect(page.getByTestId('timeline-refund')).toHaveCount(1)
  const raw=await (await context.request.get(shop+'/api/store/customer/orders/'+paid.order_id+'/after-sales')).json();assert.equal(raw.requests.length,1);assert.equal(raw.requests[0].status,'completed');assert.ok(!JSON.stringify(raw).includes('actor_id'))
  const notices=await (await context.request.get(shop+'/api/store/customer/notifications?offset=0')).json();assert.equal(notices.count,3);assert.deepEqual(notices.notifications.map(n=>n.kind).sort(),['after_sales.completed','after_sales.processing','refund.created'])
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
  await page.getByTestId('timeline-refund').scrollIntoViewIfNeeded();await page.screenshot({path:resolve(logs,'non-device-refund-'+segment+'.png')});reads.push(segment)
 }
 assert.deepEqual(errors,[])
 writeFileSync(resolve(logs,'browser-result.json'),JSON.stringify({passed:true,scenario:'non-device-refund',order_id:paid.order_id,request_id:requestID,operation_id:command.operation_id,verified_accounts:1,customer_and_refund_lost_response_replay:true,customer_languages:reads,channel_refund_simulated:true,...JSON.parse(readFileSync(resolve(logs,'non-device-fixture-result.json'),'utf8'))},null,2))
}finally{await browser.close()}
