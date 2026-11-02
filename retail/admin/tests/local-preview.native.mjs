import assert from 'node:assert/strict'
import {mkdir,writeFile} from 'node:fs/promises'
import {fileURLToPath} from 'node:url'
import {join} from 'node:path'
import {chromium,expect} from '@playwright/test'

// Actual local preview only: no fixtures, login, API mocking or commerce writes.
const output=fileURLToPath(new URL('../../../logs/gin-migration/local-preview-'+Date.now()+'/',import.meta.url))
await mkdir(output,{recursive:true})
const browser=await chromium.launch({channel:'chrome',headless:true})
const errors=[],rows=[]
try{
 const context=await browser.newContext({reducedMotion:'reduce'})
 await context.route('https://**/*',route=>route.abort())
 const page=await context.newPage()
 page.on('pageerror',e=>errors.push(e.message))
 for(const lang of ['en','zh']){
  const store=await page.goto(`http://127.0.0.1:9300/dk-${lang}/store`)
  assert.equal(store.status(),200)
  const html=await store.text()
  const paths=[...new Set([...html.matchAll(new RegExp(`href="(/dk-${lang}/products/[^"?]+)"`,'g'))].map(m=>m[1]))]
  assert.equal((html.match(/data-testid="product-wrapper"/g)||[]).length,30)
  assert.equal(paths.length,30)
  for(const path of paths){
   const response=await context.request.get('http://127.0.0.1:9300'+path)
   assert.equal(response.status(),200,path)
   assert.ok((await response.text()).includes('data-testid="product-container"'),path)
  }
  for(const width of [390,1440]){
   await page.setViewportSize({width,height:900})
   for(const path of [`/dk-${lang}/about`,paths[0]]){
    const response=await page.goto('http://127.0.0.1:9300'+path)
    assert.equal(response.status(),200)
    await expect(page.locator('astro-island[client="load"][ssr]')).toHaveCount(0)
    if(path===paths[0])await expect(page.getByTestId('product-container')).toBeVisible()
    assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,path)
    rows.push({path,width,status:response.status(),hydrated:true,overflow:false})
   }
   await page.screenshot({path:join(output,`product-${lang}-${width}.png`)})
  }
  rows.push({lang,store_ssr_cards:30,distinct_product_links:30,product_details_http_verified:paths.length})
 }
 await page.goto('http://127.0.0.1:9200/products')
 await expect(page).toHaveURL(/\/login$/)
 await expect(page.locator('input[type="password"]')).toBeVisible()
 await expect(page.getByRole('button',{name:/sign in|log in|登录/i})).toBeVisible()
 await page.screenshot({path:join(output,'admin-login.png')})
 assert.deepEqual(errors,[])
 const report={output,read_only:true,real_local_services:true,api_mocking:false,rows,errors,admin_anonymous_login_rendered:true,admin_authenticated_business_acceptance:false,external_requests_blocked:true,goal_complete:false}
 await writeFile(join(output,'report.json'),JSON.stringify(report,null,2)+'\n')
 console.log(JSON.stringify(report))
 await context.close()
}finally{await browser.close()}
