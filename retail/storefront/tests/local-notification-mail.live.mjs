import assert from 'node:assert/strict'
import {execFileSync} from 'node:child_process'
import {readFileSync,writeFileSync} from 'node:fs'
import {resolve} from 'node:path'
import {chromium,expect as baseExpect} from '../../admin/node_modules/@playwright/test/index.mjs'
const expect=baseExpect.configure({timeout:15000}),origin='http://127.0.0.1:19300',adminOrigin='http://127.0.0.1:19200'
async function saveMailboxWithLostReply(page,drawer){
 const requests=[]
 await page.route('**/notification-mail/accounts',async route=>{
  if(route.request().method()!=='POST'){await route.continue();return}
  requests.push(route.request().postDataJSON());const response=await route.fetch();assert.equal(response.status(),200)
  if(requests.length===1)await route.abort('failed');else await route.fulfill({response})
 })
 try{
  await drawer.getByRole('button',{name:'Save',exact:true}).click()
  await expect(drawer.locator('.ant-alert-warning')).toBeVisible()
  await drawer.getByRole('button',{name:'Retry this save',exact:true}).click()
  await expect(drawer).not.toBeVisible();assert.equal(requests.length,2);assert.deepEqual(requests[0],requests[1])
 }finally{await page.unrouteAll({behavior:'wait'})}
}
const owner=process.env.ACCOUNT_TEST_OWNER,logs=process.env.ACCOUNT_TEST_LOGS,email=`local-account-${owner}-1@example.invalid`,password='isolated-account-'+owner+'-password'
assert.match(owner??'',/^[0-9a-f]{12}$/)
const credentials=readFileSync(resolve('../../.private-files/local/admin-credentials.txt'),'utf8'),credential=name=>credentials.split(/\r?\n/).find(x=>new RegExp('^'+name+'\\s*[:=]','i').test(x))?.replace(/^[^:=]+[:=]\s*/,'').trim()
const browser=await chromium.launch({headless:true,executablePath:resolve(process.env.LOCALAPPDATA,'ms-playwright/chromium-1234/chrome-win64/chrome.exe')}),errors=[]
try{
 const context=await browser.newContext({viewport:{width:1440,height:1000},reducedMotion:'reduce'}),admin=await context.newPage()
 admin.on('pageerror',e=>errors.push(e.message));await admin.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','en'))
 await admin.goto(adminOrigin+'/account');await admin.getByLabel('Email',{exact:true}).fill(credential('Email'));await admin.getByLabel('Password',{exact:true}).fill(credential('Password'))
 await admin.getByRole('button',{name:'Sign in',exact:true}).click();await expect(admin.getByRole('button',{name:'Sign out',exact:true})).toBeVisible()
 await admin.goto(adminOrigin+'/mailboxes');await admin.getByRole('button',{name:'Add mailbox',exact:true}).click()
 let drawer=admin.getByRole('dialog')
 for(const [name,value] of Object.entries({'Sender name':'Windows mail fixture','Sender email':'windows-notify@example.invalid','Reply-to email':'windows-reply@example.invalid','Resend API Key':'re_windows_isolated_mail_key'}))await drawer.getByLabel(name,{exact:true}).fill(value)
 await drawer.getByRole('checkbox',{name:'Use for registration verification',exact:true}).check()
 await saveMailboxWithLostReply(admin,drawer)
 const directory=await context.request.get(adminOrigin+'/api/admin/notification-mail/accounts');assert.equal(directory.status(),200)
 const raw=await directory.text();assert.ok(!raw.includes('re_windows_isolated_mail_key'))
 const accounts=JSON.parse(raw).accounts;assert.equal(accounts.length,1);assert.equal(accounts[0].has_api_key,true)
 await admin.getByRole('button',{name:'Edit mailbox',exact:true}).click();drawer=admin.getByRole('dialog')
 await expect(drawer.getByLabel('Resend API Key',{exact:true})).toHaveValue('')
 // Save with blank secrets; the subsequent worker must retain the original key.
 await drawer.getByRole('button',{name:'Save',exact:true}).click();await expect(drawer).not.toBeVisible()
 const customer=await browser.newContext({viewport:{width:390,height:900},reducedMotion:'reduce'}),page=await customer.newPage()
 page.on('pageerror',e=>errors.push(e.message))
 await page.goto(origin+'/gb-en/account');await page.getByTestId('register-button').click()
 const form=page.getByTestId('register-page')
 for(const [id,value] of Object.entries({'first-name-input':'Windows','last-name-input':'Mail','email-input':email,'password-input':password}))await form.getByTestId(id).fill(value)
 const registration=page.waitForResponse(r=>r.request().method()==='POST'&&r.url().endsWith('/auth/register'))
 await form.getByTestId('register-button').click();assert.equal((await registration).status(),202)
 await expect(page.getByTestId('register-verification-message')).toBeVisible()
 const dispatch=phase=>{
  const result=execFileSync(process.env.WINDOWS_MAIL_FIXTURE,['-test.run=^TestWindowsMailSubflow$','-test.v'],{env:{...process.env,WINDOWS_MAIL_PHASE:phase},encoding:'utf8',timeout:60000})
  writeFileSync(resolve(logs,'dispatch-'+phase+'.log'),result)
 }
 dispatch('verification')
 const verified=page.waitForResponse(r=>r.url().endsWith('/auth/verify'))
 await page.goto(readFileSync(resolve(logs,'mail-verification-link.txt'),'utf8'));assert.equal((await verified).status(),200)
 await page.goto(origin+'/gb-en/account');await page.getByTestId('email-input').fill(email);await page.getByTestId('password-input').fill(password)
 const login=page.waitForResponse(r=>r.url().endsWith('/auth/login'));await page.getByTestId('sign-in-button').click();assert.equal((await login).status(),200)
 await page.goto(origin+'/gb-en/account/orders');await expect(page.getByTestId('customer-authenticated')).toBeVisible()
 // This is the copied real Mollie paid order's existing outbox, not an inserted
 // fake order or fabricated payment. Only the external email transport is fake.
 dispatch('order-reject')
 await admin.reload();const jobs=admin.getByTestId('order-mail-jobs')
 const row=jobs.getByRole('row').filter({hasText:'order_8bffc6dc-112e-4ed5-8a32-57421bc2ae4d'})
 await expect(row).toContainText('Failed');await expect(row).toContainText('provider_rejected')
 const requests=[]
 await admin.route('**/order-jobs/*/retry',async route=>{
  requests.push(route.request().postDataJSON());const reply=await route.fetch();assert.equal(reply.status(),200)
  if(requests.length===1)await route.abort('failed');else await route.fulfill({response:reply})
 })
 await row.getByRole('button',{name:'Queue again',exact:true}).click();await admin.getByRole('button',{name:'OK',exact:true}).click()
 await expect(jobs.getByText('The retry result is not confirmed. Retry this same operation.',{exact:true})).toBeVisible()
 await row.getByRole('button',{name:'Queue again',exact:true}).click();await expect(row.getByRole('button',{name:'Queue again',exact:true})).toBeDisabled()
 await admin.unrouteAll({behavior:'wait'});assert.equal(requests.length,2);assert.deepEqual(requests[0],requests[1])
 dispatch('order-sent');await admin.reload();await expect(row).toContainText('Provider accepted')
 const projection=await context.request.get(adminOrigin+'/api/admin/notification-mail/order-jobs?offset=0');assert.equal(projection.status(),200)
 const body=await projection.text();for(const secret of ['request_envelope','payload_envelope','sender_fingerprint','provider_id','windows-notify@'])assert.ok(!body.includes(secret))
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true);assert.deepEqual(errors,[])
 await admin.screenshot({path:resolve(logs,'mail-admin-sent.png'),fullPage:true})
 writeFileSync(resolve(logs,'browser-result.json'),JSON.stringify({passed:true,scenario:'mail',actual_admin_mailbox_save:true,blank_secret_preserved:true,customer_registration_verification_login:true,link_from_dispatched_email:true,paid_order_existing_outbox:true,permanent_rejection_admin_retry_then_sent:true,same_frozen_mail_on_retry:true,lost_reply_exact_command_recovery:true,production_dispatchers_separate_processes:true,isolated_resend_transport:true,real_email_delivery:false,browser_errors:0},null,2))
}catch(e){for(const [i,p]of browser.contexts().flatMap(c=>c.pages()).entries()){await p.screenshot({path:resolve(logs,`failure-${i}.png`),fullPage:true}).catch(()=>{});console.error('FAILURE',p.url(),await p.locator('body').innerText().catch(()=>''))}throw e}finally{await browser.close()}
