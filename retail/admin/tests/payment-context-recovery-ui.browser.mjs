import assert from 'node:assert/strict'
import {mkdir,writeFile} from 'node:fs/promises'
import {join} from 'node:path'
import {chromium,expect} from '@playwright/test'

// Rendered Astro UI regression only. Every commerce write is intercepted;
// this deliberately does not claim a Gin/MySQL/provider joint acceptance.
const origin=process.env.CONTEXT_ASTRO_ORIGIN,output=process.env.CONTEXT_BROWSER_OUTPUT
if(!/^http:\/\/127\.0\.0\.1:\d+$/.test(origin??'')||!output)throw Error('Explicit local Astro origin/output required')
const contextKey='retail:cart-context-intent:v1',paymentKey='retail:checkout-payment-intent:v1'
const operationID='22345678-1234-4234-8234-123456789012',initialID='12345678-1234-4234-8234-123456789012'
const journal={regionID:'ui-test-region',currency:'eur',locale:'zh-CN',cartID:'ui-test-cart',version:'5',operationID,kind:'payment-save'}
const payment={cartID:journal.cartID,version:'4',operationID:initialID}
const errors=[],cases=[]
await mkdir(output,{recursive:true})
const browser=await chromium.launch({channel:'chrome',headless:true})
try{
 for(const lang of ['en','zh'])for(const width of [390,1440]){
  const context=await browser.newContext({viewport:{width,height:900},reducedMotion:'reduce'})
  const page=await context.newPage();page.setDefaultTimeout(15000)
  page.on('pageerror',error=>errors.push(error.message))
  const writes=[];let release
  let gate=new Promise(resolve=>{release=resolve})
  await page.route('**/api/store/**',async route=>{
   const request=route.request()
   if(request.method()==='GET'){await route.continue();return}
   assert.equal(new URL(request.url()).pathname,'/api/store/carts/ui-test-cart/context-payment/restore')
   writes.push({path:new URL(request.url()).pathname,key:request.headers()['idempotency-key'],body:request.postDataJSON()})
   await gate;await route.abort('failed')
  })
  try{
   const path='/dk-'+lang+'/about?recovery=ui#contact'
   await page.goto(origin+path)
   await context.addCookies([{name:'retail_cart_csrf',value:'a'.repeat(64),url:origin,sameSite:'Strict'}])
   await page.evaluate(({contextKey,paymentKey,journal,payment})=>{localStorage.setItem(contextKey,JSON.stringify(journal));localStorage.setItem(paymentKey,JSON.stringify(payment))},{contextKey,paymentKey,journal,payment})
   const beforeCookies=await context.cookies(),restoreName=lang==='zh'?'恢复之前的设置':'Restore previous preferences'
   const open=async()=>{await page.locator('astro-island[component-url*="gin-header"]:not([ssr])').waitFor();await page.getByRole('button',{name:lang==='zh'?'国家与语言':'Country and language',exact:true}).click();await expect(page.getByRole('dialog')).toBeVisible()}
   await open();const restore=page.getByRole('button',{name:restoreName,exact:true})
   await expect(restore).toBeVisible();await restore.click();await expect.poll(()=>writes.length).toBe(1)
   await expect(restore).toBeDisabled()
   assert.equal((await page.evaluate(key=>JSON.parse(localStorage.getItem(key)),contextKey)).kind,'payment-restore')
   release();await expect(page.getByRole('alert')).toBeVisible();await expect(restore).toBeEnabled()
   assert.equal(page.url(),origin+path)
   assert.deepEqual(await context.cookies(),beforeCookies)
   const stored=await page.evaluate(key=>localStorage.getItem(key),paymentKey)
   assert.deepEqual(JSON.parse(stored),payment)
   await page.reload();await open();await expect(restore).toBeVisible()
   gate=Promise.resolve()
   await restore.click();await expect.poll(()=>writes.length).toBe(2);await expect(page.getByRole('alert')).toBeVisible()
   assert.equal(writes.length,2);assert.deepEqual(writes[0],writes[1]);assert.equal(writes[0].key,operationID)
   assert.deepEqual(writes[0].body,{version:'5',region_id:journal.regionID,locale:journal.locale})
   assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true)
   await page.screenshot({path:join(output,`recovery-${lang}-${width}.png`)})
   cases.push({lang,width,pending_disabled:true,reload_preserves_restore:true,same_command_retry:true,navigation_cookie_preserved:true})
  }finally{release();await context.close()}
 }
 assert.deepEqual(errors,[])
 await writeFile(join(output,'result.json'),JSON.stringify({rendered_astro:true,api_writes_intercepted:true,actual_database_joint:false,cases,errors},null,2)+'\n')
}finally{await browser.close()}
