import assert from 'node:assert/strict'
import {mkdir,writeFile} from 'node:fs/promises'
import {fileURLToPath} from 'node:url'
import {join} from 'node:path'
import {chromium,expect} from '@playwright/test'

// Main preview: fresh anonymous browser contexts, no commerce writes or fixtures.
const origin='http://127.0.0.1:9300'
const output=fileURLToPath(new URL('../../../logs/gin-migration/preferences-preview-'+Date.now()+'/',import.meta.url))
await mkdir(output,{recursive:true,mode:0o700})
const browser=await chromium.launch({channel:'chrome',headless:true}),cases=[],errors=[]
let page
try{
 for(const initial of ['en','zh'])for(const width of [390,1440]){
  const context=await browser.newContext({viewport:{width,height:900},reducedMotion:'reduce'})
  page=await context.newPage();page.setDefaultTimeout(20000)
  page.on('pageerror',e=>errors.push(e.message))
  const writes=[]
  page.on('request',r=>{if(!['GET','HEAD','OPTIONS'].includes(r.method())&&new URL(r.url()).pathname.startsWith('/api/store/'))writes.push({method:r.method(),path:new URL(r.url()).pathname})})
  const other=initial==='en'?'zh':'en',locale=other==='zh'?'zh-CN':'en'
  const suffix='/about?selection=a%2Bb&sort=price#contact'
  const target='Deutschland — '+(other==='zh'?'简体中文':'English')
  const open=async()=>{
   await page.getByRole('button',{name:/^(Country and language|国家与语言)$/}).click()
   await expect(page.getByRole('dialog')).toBeVisible()
  }
  const cookie=async()=>(await context.cookies()).find(c=>c.name==='_retail_locale')
  const fit=async()=>assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),'horizontal overflow')
  const response=await page.goto(origin+'/dk-'+initial+suffix)
  assert.equal(response.status(),200)
  await open();await fit()
  await page.getByRole('dialog').getByRole('button',{name:target,exact:true}).click()
  await expect(page).toHaveURL(origin+'/de-'+other+suffix)
  assert.equal((await cookie()).value,locale)
  assert.equal((await cookie()).sameSite,'Strict')
  const cartStatus=(await page.request.get(origin+'/api/store/cart')).status()
  assert.ok([401,404].includes(cartStatus),'anonymous cart lookup')
  await page.reload()
  await expect(page.locator('html')).toHaveAttribute('lang',other==='zh'?'zh-CN':'en')
  await open()
  await expect(page.getByRole('dialog').getByRole('button',{name:target,exact:true})).toHaveAttribute('aria-pressed','true')
  await fit();await page.screenshot({path:join(output,`preferences-${initial}-${width}.png`)})
  await page.keyboard.press('Escape');await expect(page.getByRole('dialog')).toHaveCount(0)
  await page.goBack();await expect(page).toHaveURL(origin+'/dk-'+initial+suffix)
  await expect.poll(async()=>(await cookie()).value).toBe(initial==='zh'?'zh-CN':'en')
  await fit();assert.deepEqual(writes,[])
  cases.push({initial,width,status:response.status(),cart_status:cartStatus,route_query_hash_preserved:true,cookie_reload_and_back:true,selected_state:true,keyboard_escape:true,no_horizontal_overflow:true,store_mutations:writes})
  await context.close();page=undefined
 }
 assert.deepEqual(errors,[])
 await writeFile(join(output,'report.json'),JSON.stringify({origin,cases,errors,api_mocking:false,business_writes:false,real_provider_called:false,goal_complete:false},null,2)+'\n')
 console.log(JSON.stringify({output,cases:cases.length,passed:true}))
}catch(e){
 if(page)await page.screenshot({path:join(output,'failure.png')}).catch(()=>{})
 await writeFile(join(output,'failure.json'),JSON.stringify({error:String(e),cases,errors},null,2)+'\n');throw e
}finally{await browser.close()}
