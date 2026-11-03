import assert from 'node:assert/strict'
import {execFileSync,spawn} from 'node:child_process'
import {readFileSync,writeFileSync,existsSync} from 'node:fs'
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
 admin.on('pageerror',e=>errors.push(e.message));await admin.addInitScript(()=>{if(window===window.top)localStorage.setItem('eurorefurb.admin.language','en')})
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
 const inboxWorker=spawn(process.env.WINDOWS_MAIL_FIXTURE,['-test.run=^TestWindowsInboxBrowser$','-test.v'],{env:process.env,windowsHide:true,stdio:['ignore','pipe','pipe']})
 let inboxOutput='',inboxExit
 inboxWorker.stdout.on('data',value=>{inboxOutput+=value});inboxWorker.stderr.on('data',value=>{inboxOutput+=value})
 const inboxDone=new Promise(resolve=>inboxWorker.on('close',code=>{inboxExit=code;resolve(code)}))
 try{
  await expect.poll(()=>existsSync(resolve(logs,'inbox-ready.json'))||inboxExit!==undefined,{timeout:30000}).toBe(true)
  assert.equal(inboxExit,undefined,inboxOutput)
  await admin.reload();await admin.getByRole('button',{name:'Open inbox',exact:true}).click()
  const inbox=admin.getByTestId('mail-inbox')
  await inbox.getByRole('button',{name:'Sync IMAP inbox',exact:true}).click()
  await expect(inbox.getByRole('button',{name:'Sync IMAP inbox',exact:true})).toBeEnabled()
  await expect(inbox.locator('tbody tr[data-row-key]')).toHaveCount(2)
  const syncResponse=admin.waitForResponse(r=>r.request().method()==='POST'&&r.url().endsWith('/sync'))
  await inbox.getByRole('button',{name:'Sync IMAP inbox',exact:true}).click()
  assert.equal((await syncResponse).status(),200);assert.equal((await(await syncResponse).json()).imported,0)
  const inbound=inbox.locator('tbody tr[data-row-key]').filter({hasText:/IMAP/})
  await inbound.getByRole('button',{name:/IMAP/}).click()
  drawer=admin.getByRole('dialog');await expect(drawer.locator('pre')).toContainText('Isolated inbound message')
  await drawer.getByRole('tab',{name:'HTML',exact:true}).click()
  const frame=drawer.locator('iframe');await expect(frame).toHaveAttribute('sandbox','')
  assert.equal(await admin.evaluate(()=>window.__mailScriptExecuted),undefined)
  const original=await context.request.get(await drawer.getByRole('link',{name:'Download original email'}).getAttribute('href').then(path=>adminOrigin+path))
  assert.equal(original.status(),200);assert.ok((await original.text()).includes('Isolated inbound message'))
  const attachment=await context.request.get(adminOrigin+await drawer.getByRole('link',{name:'fixture.txt',exact:true}).getAttribute('href'))
  assert.equal(attachment.status(),200);assert.equal(await attachment.text(),'Attachment fixture')
  await drawer.locator('.ant-drawer-close').click()
  await inbound.getByRole('button',{name:'Archive',exact:true}).click();await expect(inbox.locator('tbody tr[data-row-key]').filter({hasText:/IMAP/})).toHaveCount(0)
  const selectFolder=async label=>{await inbox.locator('.ant-select').click();await admin.locator(`.ant-select-item-option[title="${label}"]`).click()}
  await selectFolder('Archive');await expect(inbox.locator('tbody tr[data-row-key]')).toHaveCount(1)
  await inbox.getByRole('button',{name:'Move to trash',exact:true}).click();await expect(inbox.locator('tbody tr[data-row-key]')).toHaveCount(0)
  await selectFolder('Trash');await expect(inbox.locator('tbody tr[data-row-key]')).toHaveCount(1)
  await inbox.getByRole('button',{name:'Restore',exact:true}).click();await expect(inbox.locator('tbody tr[data-row-key]')).toHaveCount(0)
  await selectFolder('Archive');await inbox.getByRole('button',{name:'Move to inbox',exact:true}).click()
  await selectFolder('Inbox');await expect(inbox.locator('tbody tr[data-row-key]')).toHaveCount(2)
  await admin.setViewportSize({width:390,height:900});await admin.screenshot({path:resolve(logs,'imap-inbox-mobile.png'),fullPage:true})
  await admin.setViewportSize({width:1440,height:1000})
  const compose=admin.getByTestId('mail-compose')
  await compose.getByRole('button',{name:'Compose email',exact:true}).click();drawer=admin.getByRole('dialog')
  for(const [name,value]of Object.entries({To:'compose@example.invalid',Cc:'cc@example.invalid',Bcc:'bcc@example.invalid',Subject:'Compose fixture',Message:'Isolated manual body'}))await drawer.getByLabel(name,{exact:true}).fill(value)
  await drawer.getByLabel('Add attachments',{exact:true}).setInputFiles({name:'fixture.txt',mimeType:'text/plain',buffer:Buffer.from('Attachment fixture')})
  await expect(drawer.getByText('fixture.txt',{exact:false})).toBeVisible()
  await drawer.getByRole('button',{name:'Save draft',exact:true}).click()
  await expect(drawer.getByRole('button',{name:'Send saved email',exact:true})).toBeEnabled()
  await drawer.locator('.ant-drawer-close').click();await compose.getByRole('button',{name:'Compose fixture',exact:true}).click();drawer=admin.getByRole('dialog')
  await expect(drawer.getByLabel('Message',{exact:true})).toHaveValue('Isolated manual body')
  let submittedURL
  await admin.route('**/compositions/*/send',async route=>{submittedURL=route.request().url();const response=await route.fetch();assert.equal(response.status(),200);assert.equal((await response.json()).status,'sent');await route.abort('failed')})
  await drawer.getByRole('button',{name:'Send saved email',exact:true}).click();await expect(drawer.locator('.ant-alert-error')).toBeVisible()
  await admin.unrouteAll({behavior:'wait'})
  await drawer.getByRole('button',{name:'Load saved version',exact:true}).click();await expect(drawer.getByText('The sending service accepted this email.',{exact:true})).toBeVisible()
  const replay=await context.request.post(submittedURL,{headers:{Origin:adminOrigin,'X-CSRF-Token':(await context.cookies()).find(c=>c.name==='retail_admin_csrf')?.value??''},data:{revision:1}})
  assert.equal(replay.status(),200);assert.equal((await replay.json()).status,'sent')
  await drawer.locator('.ant-drawer-close').click()
 }finally{
  writeFileSync(resolve(logs,'inbox-browser-done'),'done')
  await inboxDone;writeFileSync(resolve(logs,'inbox-sql.log'),inboxOutput)
 }
 assert.equal(inboxExit,0,inboxOutput)
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
 // Extend the existing mail subflow only: no payment, warehouse or after-sales
 // command is repeated. Configure each inherited protocol via the actual UI.
 for(const provider of ['smtp','quguo']){
  await admin.getByRole('button',{name:'Edit mailbox',exact:true}).click();drawer=admin.getByRole('dialog')
  await drawer.locator('.ant-select').first().click()
  await admin.locator(`.ant-select-item-option[title="${provider==='smtp'?'SMTP':'Relayme API'}"]`).click()
  if(provider==='smtp'){
   for(const [label,value]of Object.entries({'SMTP host':'smtp.example.invalid','SMTP username':'fixture','SMTP password':'smtp-isolated-secret'}))await drawer.getByLabel(label,{exact:true}).fill(value)
  }else{
   for(const [label,value]of Object.entries({'Relayme API root URL':'https://relayme.example.invalid','Key ID':'fixture','Secret':'relayme-isolated-secret'}))await drawer.getByLabel(label,{exact:true}).fill(value)
  }
  await drawer.getByRole('checkbox',{name:'Configure IMAP receiving',exact:true}).check()
  for(const [label,value]of Object.entries({'IMAP host':'imap.example.invalid','IMAP username':'fixture','IMAP password':'imap-isolated-secret'}))await drawer.getByLabel(label,{exact:true}).fill(value)
  await saveMailboxWithLostReply(admin,drawer)
  const configured=await context.request.get(adminOrigin+'/api/admin/notification-mail/accounts');assert.equal(configured.status(),200)
  const publicRaw=await configured.text();for(const secret of ['smtp-isolated-secret','imap-isolated-secret','relayme-isolated-secret'])assert.ok(!publicRaw.includes(secret))
  const saved=JSON.parse(publicRaw).accounts[0].transport
  assert.equal(saved.provider,provider);assert.equal(saved.has_imap_password,true)
  assert.equal(saved[provider==='smtp'?'has_smtp_password':'has_relayme_secret'],true)
  await admin.getByRole('button',{name:'Edit mailbox',exact:true}).click();drawer=admin.getByRole('dialog')
  await expect(drawer.getByLabel(provider==='smtp'?'SMTP password':'Secret',{exact:true})).toHaveValue('')
  await expect(drawer.getByLabel('IMAP password',{exact:true})).toHaveValue('')
  await admin.setViewportSize({width:390,height:900})
  assert.equal(await drawer.evaluate(element=>element.scrollWidth<=innerWidth),true)
  await admin.screenshot({path:resolve(logs,`mail-${provider}-mobile.png`),fullPage:true})
  await drawer.getByRole('button',{name:'Save',exact:true}).click();await expect(drawer).not.toBeVisible()
  dispatch('protocol-'+provider)
  await admin.setViewportSize({width:1440,height:1000})
 }
 // Restore the original sender in this disposable database. This does not send
 // another message; earlier Resend verification/order queue evidence remains.
 await admin.getByRole('button',{name:'Edit mailbox',exact:true}).click();drawer=admin.getByRole('dialog')
 await drawer.locator('.ant-select').first().click();await admin.locator('.ant-select-item-option[title="Resend"]').click()
 await drawer.getByLabel('Resend API Key',{exact:true}).fill('re_windows_isolated_mail_key')
 await drawer.getByRole('checkbox',{name:'Configure IMAP receiving',exact:true}).uncheck()
 await drawer.getByRole('button',{name:'Save',exact:true}).click();await expect(drawer).not.toBeVisible()
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true);assert.deepEqual(errors,[])
 await admin.screenshot({path:resolve(logs,'mail-admin-sent.png'),fullPage:true})
 writeFileSync(resolve(logs,'browser-result.json'),JSON.stringify({passed:true,scenario:'mail',actual_admin_mailbox_save:true,smtp_relayme_imap_configuration:true,imap_inbox_read_archive_trash:true,protocol_credentials_encrypted:true,protocol_blank_secrets_preserved:true,protocol_mobile_forms:true,blank_secret_preserved:true,customer_registration_verification_login:true,link_from_dispatched_email:true,paid_order_existing_outbox:true,permanent_rejection_admin_retry_then_sent:true,same_frozen_mail_on_retry:true,lost_reply_exact_command_recovery:true,production_dispatchers_separate_processes:true,isolated_resend_transport:true,real_email_delivery:false,browser_errors:0},null,2))
}catch(e){for(const [i,p]of browser.contexts().flatMap(c=>c.pages()).entries()){await p.screenshot({path:resolve(logs,`failure-${i}.png`),fullPage:true}).catch(()=>{});console.error('FAILURE',p.url(),await p.locator('body').innerText().catch(()=>''))}throw e}finally{await browser.close()}
