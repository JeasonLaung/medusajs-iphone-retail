import assert from 'node:assert/strict'
import {chromium} from '@playwright/test'
import {mkdir,writeFile} from 'node:fs/promises'
import {fileURLToPath} from 'node:url'
const output=fileURLToPath(new URL('../../../logs/gin-migration/seo-defaults-'+Date.now()+'/',import.meta.url))
await mkdir(output,{recursive:true,mode:0o700})
const browser=await chromium.launch({channel:'chrome',headless:true}),rows=[],errors=[]
const publicPaths=['','/store','/about','/help','/reviews','/blog','/blog/choose-your-storage']
try{
 const context=await browser.newContext();await context.route('https://**/*',r=>r.abort());const page=await context.newPage()
 const catalogHTML=await (await context.request.get('http://127.0.0.1:9300/dk-en/store')).text()
 const product=catalogHTML.match(/href="(\/dk-en\/products\/[^"?#]+)"/)?.[1]
 assert.ok(product);publicPaths.push(product.replace('/dk-en',''))
 page.on('pageerror',e=>errors.push(e.message))
 for(const platform of ['source','astro'])for(const lang of ['en','zh'])for(const path of [...publicPaths,'/cart','/account','/ui','/shipping-demo','/checkout','/sell/iphone/model',...(platform==='astro'?['/payment-result']:[])]){
  const response=await page.goto(`http://127.0.0.1:${platform==='source'?8000:9300}/dk-${lang}${path}`,{waitUntil:'domcontentloaded'})
  await page.waitForLoadState('domcontentloaded')
  const head=await page.evaluate(()=>({title:document.title,description:document.querySelector('meta[name="description"]')?.content,robots:document.querySelector('meta[name="robots"]')?.content,canonical:document.querySelector('link[rel="canonical"]')?.href,alternates:[...document.querySelectorAll('link[rel="alternate"][hreflang]')].map(n=>({lang:n.hreflang,href:n.href})),og:[...document.querySelectorAll('meta[property^="og:"]')].map(n=>({key:n.getAttribute('property'),value:n.content})),jsonld:[...document.querySelectorAll('script[type="application/ld+json"]')].map(n=>JSON.parse(n.textContent)),icons:[...document.querySelectorAll('link[rel="icon"],link[rel="apple-touch-icon"]')].map(n=>({rel:n.rel,href:new URL(n.href).pathname}))}))
  const row={platform,lang,path,status:response.status(),head};rows.push(row);console.log(JSON.stringify({platform,lang,path,status:row.status}))
  if(platform==='astro'&&response.status()===200){
   assert.equal(head.jsonld.length,1,path);assert.equal(head.alternates.length,3,path);assert.equal(head.canonical.includes('?'),false)
   if(['/cart','/account','/checkout','/payment-result','/sell/iphone/model','/ui','/shipping-demo'].includes(path))assert.match(head.robots,/noindex/,path)
  }
 }
 await writeFile(output+'observations.json',JSON.stringify({rows,errors},null,2)+'\n')
 const normalize=v=>JSON.parse(JSON.stringify(v).replace(/http:\/\/(?:localhost|127\.0\.0\.1):(?:8000|9300)(?=[/"\s]|$)/g,'ORIGIN'))
 const comparisons=[]
 for(const source of rows.filter(r=>r.platform==='source'&&publicPaths.includes(r.path))){
  const target=rows.find(r=>r.platform==='astro'&&r.lang===source.lang&&r.path===source.path)
  assert.equal(target.status,source.status);assert.equal(target.head.title,source.head.title,source.path);assert.equal(target.head.description,source.head.description)
  for(const key of ['canonical','alternates','og','jsonld','icons'])assert.deepEqual(normalize(target.head[key]),normalize(source.head[key]),`${source.lang}${source.path} ${key}`)
  comparisons.push({lang:source.lang,path:source.path,title_description_canonical_languages_og_jsonld_icons_equal:true})
 }
 assert.deepEqual(errors,[])
 await writeFile(output+'report.json',JSON.stringify({rows,comparisons,errors,synthetic_responses:false,business_writes:false,configured_seo_verified:false,full_storefront_accepted:false,goal_complete:false},null,2)+'\n')
 console.log(JSON.stringify({output,cases:rows.length,comparisons:comparisons.length,passed:true}))
 await context.close()
}finally{await browser.close()}
