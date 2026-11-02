// Read-only check of the real local mailbox page; never sends or retries mail.
import assert from 'node:assert/strict'
import {readFileSync,writeFileSync,mkdirSync} from 'node:fs'
import {resolve} from 'node:path'
import {chromium,expect as baseExpect} from '@playwright/test'
const expect=baseExpect.configure({timeout:20000}),origin='http://127.0.0.1:9200'
const logs=resolve('../../logs/retail-native/customer-event-mail-read');mkdirSync(logs,{recursive:true})
const credentials=readFileSync(resolve('../../.private-files/local/admin-credentials.txt'),'utf8')
const credential=name=>credentials.split(/\r?\n/).find(line=>new RegExp('^'+name+'\\s*[:=]','i').test(line))?.replace(/^[^:=]+[:=]\s*/,'').trim()
const browser=await chromium.launch({headless:true,executablePath:resolve(process.env.LOCALAPPDATA,'ms-playwright/chromium-1234/chrome-win64/chrome.exe')})
const errors=[],writes=[],checks=[]
try{
 const context=await browser.newContext({viewport:{width:1440,height:1000},reducedMotion:'reduce'}),page=await context.newPage()
 page.on('pageerror',e=>errors.push(e.message));page.on('request',r=>{if(r.method()!=='GET'&&r.url().startsWith(origin+'/api/admin/notification-mail'))writes.push(r.url())})
 await page.addInitScript(()=>{if(!localStorage.getItem('eurorefurb.admin.language'))localStorage.setItem('eurorefurb.admin.language','en')})
 await page.goto(origin+'/account');await page.getByLabel('Email',{exact:true}).fill(credential('Email'));await page.getByLabel('Password',{exact:true}).fill(credential('Password'))
 await page.getByRole('button',{name:'Sign in',exact:true}).click();await expect(page.getByRole('button',{name:'Sign out',exact:true})).toBeVisible()
 for(const [language,width] of [['en',1440],['zh',390]]){
  await page.evaluate(language=>localStorage.setItem('eurorefurb.admin.language',language),language)
  await page.setViewportSize({width,height:1000});await page.goto(origin+'/mailboxes')
  const jobs=page.getByTestId('event-mail-jobs');await expect(jobs.getByRole('heading',{name:language==='zh'?'客户业务邮件任务':'Customer event email jobs'})).toBeVisible()
  const response=await context.request.get(origin+'/api/admin/notification-mail/customer-jobs?offset=0')
  assert.equal(response.status(),200);assert.match(response.headers()['cache-control'],/no-store/)
  const data=await response.json();assert.equal(data.count,0);assert.deepEqual(data.jobs,[])
  await expect(jobs.locator('.ant-spin-spinning')).toHaveCount(0)
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
  await page.screenshot({path:resolve(logs,language+'-'+width+'.png'),fullPage:true})
  checks.push({language,width,customer_jobs_http:200,no_horizontal_overflow:true})
 }
 assert.deepEqual(writes,[]);assert.deepEqual(errors,[])
 writeFileSync(resolve(logs,'report.json'),JSON.stringify({passed:true,read_only:true,mail_sent:false,mail_writes:0,browser_errors:0,checks},null,2))
}finally{await browser.close()}
