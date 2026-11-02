// Read-only product-entry configuration check. Never saves products or stock.
import assert from 'node:assert/strict'
import {readFileSync,writeFileSync,mkdirSync} from 'node:fs'
import {resolve} from 'node:path'
import {chromium,expect as baseExpect} from '@playwright/test'
const expect=baseExpect.configure({timeout:20000}),origin='http://127.0.0.1:9200'
const logs=resolve('../../logs/retail-native/product-entry-read');mkdirSync(logs,{recursive:true})
const credentials=readFileSync(resolve('../../.private-files/local/admin-credentials.txt'),'utf8')
const credential=name=>credentials.split(/\r?\n/).find(line=>new RegExp('^'+name+'\\s*[:=]','i').test(line))?.replace(/^[^:=]+[:=]\s*/,'').trim()
const browser=await chromium.launch({headless:true,executablePath:resolve(process.env.LOCALAPPDATA,'ms-playwright/chromium-1234/chrome-win64/chrome.exe')})
const errors=[],writes=[],checks=[]
try{
 const context=await browser.newContext({viewport:{width:1440,height:1000},reducedMotion:'reduce'}),page=await context.newPage()
 page.on('pageerror',e=>errors.push(e.message))
 page.on('request',request=>{if(request.method()!=='GET'&&request.url().startsWith(origin+'/api/admin/products'))writes.push(request.url())})
 await context.addInitScript(()=>{if(!localStorage.getItem('eurorefurb.admin.language'))localStorage.setItem('eurorefurb.admin.language','en')})
 await page.goto(origin+'/account');await page.getByLabel('Email',{exact:true}).fill(credential('Email'));await page.getByLabel('Password',{exact:true}).fill(credential('Password'))
 await page.getByRole('button',{name:'Sign in',exact:true}).click();await expect(page.getByRole('button',{name:'Sign out',exact:true})).toBeVisible()
 for(const [language,width] of [['en',1440],['zh',390]]){
  await page.evaluate(language=>localStorage.setItem('eurorefurb.admin.language',language),language)
  await page.setViewportSize({width,height:1000});await page.goto(origin+'/products')
  const zh=language==='zh',creation=await context.request.get(origin+'/api/admin/products/creation')
  assert.equal(creation.status(),200);assert.match(creation.headers()['cache-control'],/no-store/)
  const data=await creation.json();assert.equal(data.enabled,true)
  await page.getByRole('button',{name:zh?'新建商品':'Create product',exact:true}).click()
  const dialog=page.getByRole('dialog');await expect(dialog.getByLabel(zh?'商品链接标识':'Handle',{exact:true})).toBeVisible()
  await page.locator('.ant-drawer-close').click();await expect(dialog).not.toBeVisible()
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
  await page.screenshot({path:resolve(logs,language+'-'+width+'.png'),fullPage:true})
  checks.push({language,width,creation_http:200,creation_enabled:true,cancel_without_save:true,no_horizontal_overflow:true})
 }
 assert.deepEqual(writes,[]);assert.deepEqual(errors,[])
 writeFileSync(resolve(logs,'report.json'),JSON.stringify({passed:true,read_only:true,mail_sent:false,product_writes:0,browser_errors:0,checks},null,2))
}finally{await browser.close()}
