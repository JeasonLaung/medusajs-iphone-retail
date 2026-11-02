import assert from 'node:assert/strict'
import {mkdir,writeFile,readFile} from 'node:fs/promises'
import {fileURLToPath} from 'node:url'
import {join} from 'node:path'
import {chromium,expect} from '@playwright/test'
const copy=JSON.parse(await readFile(new URL('../../../medusa-store/shared/dhl-demo-copy.json',import.meta.url),'utf8'))
const output=fileURLToPath(new URL('../../../logs/gin-migration/shipping-demo-'+Date.now()+'/',import.meta.url))
await mkdir(output,{recursive:true,mode:0o700})
const browser=await chromium.launch({channel:'chrome',headless:true})
const rows=[],errors=[],writes=[],diagnostics=[]
try{
 for(const platform of ['source','astro'])for(const lang of ['en','zh'])for(const width of [390,1440]){
  const t=copy[lang],context=await browser.newContext({reducedMotion:'reduce',timezoneId:'Europe/Copenhagen',viewport:{width,height:900}})
  await context.route('https://**/*',route=>route.abort())
  const page=await context.newPage();page.setDefaultTimeout(25000)
  page.on('pageerror',e=>errors.push({platform,lang,width,message:e.message}))
  page.on('request',r=>{if(['GET','HEAD','OPTIONS'].includes(r.method()))return;const u=new URL(r.url()),v={platform,method:r.method(),path:u.pathname};if(platform==='source'&&u.origin==='http://127.0.0.1:8000'&&u.pathname==='/__nextjs_original-stack-frames')diagnostics.push(v);else writes.push(v)})
  const response=await page.goto(`http://127.0.0.1:${platform==='source'?8000:9300}/dk-${lang}/shipping-demo`)
  assert.equal(response.status(),200)
  const section=page.getByTestId('dhl-official-tracking'),button=section.getByRole('button',{name:t.refresh})
  await expect(section.getByRole('heading',{name:t.title})).toBeVisible()
  await expect(section).toContainText(`${t.requested}: 7777777770`)
  await expect(button).toBeEnabled()
  await expect(page.locator('meta[name="robots"]')).toHaveAttribute('content',/noindex.*nofollow/)
  const stable=await section.locator('p').allTextContents()
  const comparison=stable.filter(p=>!p.startsWith(t.checked+':'))
  const checked=stable.find(p=>p.startsWith(t.checked+':'))
  assert.ok(checked)
  const link=section.getByRole('link',{name:t.source})
  await expect(link).toHaveAttribute('href','https://developer.dhl.com/tracking')
  await expect(link).toHaveAttribute('rel',/noopener/)
  const api=platform==='source'?'**/store/shipping-demo/dhl':'**/api/store/shipping-demo/dhl'
  const transportFailure=route=>route.abort('failed')
  await context.route(api,transportFailure)
  await button.click();await expect(section.getByText(t.error,{exact:true})).toBeVisible()
  assert.deepEqual((await section.locator('p').allTextContents()).filter(p=>!p.startsWith(t.checked+':')),comparison)
  await context.unroute(api,transportFailure)
  await button.click();await expect(button).toBeEnabled()
  await expect(section.getByText(t.error,{exact:true})).toHaveCount(0)
  assert.deepEqual((await section.locator('p').allTextContents()).filter(p=>!p.startsWith(t.checked+':')),comparison)
  const overflow=await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth)
  assert.equal(overflow,false)
  await page.screenshot({path:join(output,`${platform}-${lang}-${width}.png`),fullPage:true})
  rows.push({platform,lang,width,status:200,comparison,refresh_retry:true,prior_result_retained_on_failure:true,robots_noindex:true,overflow})
  console.log(JSON.stringify({platform,lang,width,passed:true}))
  await context.close()
 }
 const comparisons=rows.filter(r=>r.platform==='source').map(s=>{const target=rows.find(r=>r.platform==='astro'&&r.lang===s.lang&&r.width===s.width);assert.deepEqual(target.comparison,s.comparison);return {lang:s.lang,width:s.width,displayed_event_text_equal:true}})
 assert.deepEqual(errors,[]);assert.deepEqual(writes,[])
 await writeFile(join(output,'report.json'),JSON.stringify({rows,comparisons,errors,writes,diagnostics,actual_local_services:true,actual_official_sample:true,synthetic_api_responses:false,transport_abort_retry_test:true,business_writes:false,account_authenticated_joint_verified:false,full_storefront_accepted:false,goal_complete:false},null,2)+'\n')
 console.log(JSON.stringify({output,passed:true,cases:rows.length}))
}finally{await browser.close()}
