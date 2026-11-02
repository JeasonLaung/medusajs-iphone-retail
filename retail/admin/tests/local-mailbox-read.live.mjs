// Read-only checks against the user's native preview. Never saves a mailbox,
// retries a mail job or starts a sending worker.
import assert from 'node:assert/strict'
import {readFileSync,writeFileSync,mkdirSync} from 'node:fs'
import {resolve} from 'node:path'
import {chromium,expect as baseExpect} from '@playwright/test'
const expect=baseExpect.configure({timeout:20000}),origin='http://127.0.0.1:9200'
const logs=resolve('../../logs/retail-native/mailbox-read');mkdirSync(logs,{recursive:true})
const credentials=readFileSync(resolve('../../.private-files/local/admin-credentials.txt'),'utf8')
const credential=name=>credentials.split(/\r?\n/).find(line=>new RegExp('^'+name+'\\s*[:=]','i').test(line))?.replace(/^[^:=]+[:=]\s*/,'').trim()
const browser=await chromium.launch({headless:true,executablePath:resolve(process.env.LOCALAPPDATA,'ms-playwright/chromium-1234/chrome-win64/chrome.exe')})
const errors=[],writes=[],checks=[]
try{
 const context=await browser.newContext({viewport:{width:1440,height:1000},reducedMotion:'reduce'}),page=await context.newPage()
 page.on('pageerror',e=>errors.push(e.message))
 page.on('request',request=>{if(request.method()!=='GET'&&request.url().startsWith(origin+'/api/admin/notification-mail/'))writes.push(request.url())})
 await context.addInitScript(()=>{if(!localStorage.getItem('eurorefurb.admin.language'))localStorage.setItem('eurorefurb.admin.language','en')})
 await page.goto(origin+'/account');await page.getByLabel('Email',{exact:true}).fill(credential('Email'));await page.getByLabel('Password',{exact:true}).fill(credential('Password'))
 await page.getByRole('button',{name:'Sign in',exact:true}).click();await expect(page.getByRole('button',{name:'Sign out',exact:true})).toBeVisible()
 for(const [language,width] of [['en',1440],['zh',390]]){
  await page.evaluate(language=>localStorage.setItem('eurorefurb.admin.language',language),language)
  await page.setViewportSize({width,height:1000});await page.goto(origin+'/mailboxes')
  const zh=language==='zh',accounts=await context.request.get(origin+'/api/admin/notification-mail/accounts')
  assert.equal(accounts.status(),200);assert.match(accounts.headers()['cache-control'],/no-store/)
  const directory=await accounts.json();assert.deepEqual(directory.accounts,[]);assert.equal(directory.registration_account_id,null)
  const jobs=await context.request.get(origin+'/api/admin/notification-mail/order-jobs?offset=0');assert.equal(jobs.status(),200)
  const jobData=await jobs.json();assert.equal(jobData.count,1);assert.equal(jobData.jobs[0].status,'pending');assert.equal(jobData.jobs[0].attempts,0)
  await expect(page.getByTestId('order-mail-jobs')).toBeVisible()
  await page.getByRole('button',{name:zh?'添加邮箱':'Add mailbox',exact:true}).click()
  const dialog=page.getByRole('dialog');await expect(dialog.getByLabel('Resend API Key',{exact:true})).toBeVisible()
  assert.equal(await dialog.getByLabel('Resend API Key',{exact:true}).inputValue(),'')
  // Use the close control's cancel/reset callback without depending on the
  // Drawer extra toolbar's placement in the accessibility tree.
  await page.locator('.ant-drawer-close').click();await expect(dialog).not.toBeVisible()
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
  await page.screenshot({path:resolve(logs,language+'-'+width+'.png'),fullPage:true})
  checks.push({language,width,accounts_http:200,jobs_http:200,empty_directory:true,pending_job_unsent:true,blank_secret_editor:true,cancel_without_save:true,no_horizontal_overflow:true})
 }
 assert.deepEqual(writes,[]);assert.deepEqual(errors,[])
 writeFileSync(resolve(logs,'report.json'),JSON.stringify({passed:true,read_only:true,mail_sent:false,notification_mail_writes:0,browser_errors:0,checks},null,2))
}finally{await browser.close()}
