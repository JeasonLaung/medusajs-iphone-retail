import assert from 'node:assert/strict'
import {chromium} from '@playwright/test'
import {mkdir,writeFile} from 'node:fs/promises'
import {fileURLToPath} from 'node:url'
const output=fileURLToPath(new URL('../../../logs/gin-migration/seo-artifacts-'+Date.now()+'/',import.meta.url))
await mkdir(output,{recursive:true,mode:0o700})
const browser=await chromium.launch({channel:'chrome',headless:true}),rows=[],errors=[]
const normalize=v=>v.replace(/http:\/\/(?:localhost|127\.0\.0\.1):(?:8000|9300)(?=[/"\s]|$)/g,'ORIGIN')
try{
 const context=await browser.newContext()
 const publication=await context.request.get('http://127.0.0.1:9100/api/store/seo?path=%2F')
 assert.equal(publication.status(),200,'default metadata fallback is not evidence of SEO configuration availability')
 const config=await publication.json();assert.equal(config.page,null);assert.ok(config.settings['seo.site_url'])
 for(const path of ['/robots.txt','/llms.txt','/BingSiteAuth.xml','/api/seo-artifacts/test-key-1234']){
  const pair=[]
  for(const port of [8000,9300]){
   const r=await context.request.get(`http://127.0.0.1:${port}${path}`),body=await r.text()
   pair.push({port,status:r.status(),content_type:r.headers()['content-type'],cache_control:r.headers()['cache-control'],body})
  }
  assert.equal(pair[1].status,pair[0].status,path)
  assert.equal(pair[1].cache_control,'no-store')
  if(pair[0].status===200){assert.equal(pair[1].content_type,pair[0].content_type);assert.equal(normalize(pair[1].body),normalize(pair[0].body),path)}
  rows.push({path,pair})
 }
 const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message))
 for(const path of ['/robots.txt','/llms.txt']){
  const r=await page.goto('http://127.0.0.1:9300'+path,{waitUntil:'domcontentloaded'});assert.equal(r.status(),200)
  assert.equal(normalize(await page.locator('body').innerText()),normalize(rows.find(row=>row.path===path).pair[1].body))
 }
 await page.goto('http://127.0.0.1:9300/dk-en/about',{waitUntil:'domcontentloaded'})
 assert.equal(await page.locator('link[rel="canonical"]').getAttribute('href'),config.settings['seo.site_url']+'/dk-en/about')
 assert.equal(await page.locator('meta[name="google-site-verification"],#seo-ga4').count(),0)
 assert.deepEqual(errors,[])
 await writeFile(output+'report.json',JSON.stringify({rows,errors,public_config_http_status:publication.status(),default_origin:config.settings['seo.site_url'],browser_text_routes:2,configured_values_verified:false,api_mocking:false,business_writes:false,real_provider_called:false,full_seo_accepted:false,goal_complete:false},null,2)+'\n')
 console.log(JSON.stringify({output,cases:rows.length,browser_text_routes:2,passed:true}))
 await context.close()
}finally{await browser.close()}
