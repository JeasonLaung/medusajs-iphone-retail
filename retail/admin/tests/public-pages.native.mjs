import assert from 'node:assert/strict'
import {mkdir,writeFile} from 'node:fs/promises'
import {fileURLToPath} from 'node:url'
import {join} from 'node:path'
import {chromium,expect} from '@playwright/test'

// Public source/target comparison against actual local Gin/MySQL and Next.
// No synthetic responses, sign-in, checkout actions or service restarts.
const output=fileURLToPath(new URL('../../../logs/gin-migration/public-pages-'+Date.now()+'/',import.meta.url))
await mkdir(output,{recursive:true,mode:0o700})
const browser=await chromium.launch({channel:'chrome',headless:true})
const rows=[],errors=[]
const paths=['about','help','reviews','blog','blog/choose-your-storage','blog/compare-iphone-configurations','blog/before-you-order']
try{
 for(const platform of ['source','astro']){
  const context=await browser.newContext({reducedMotion:'reduce',timezoneId:'Europe/Copenhagen'})
  await context.route('https://**/*',route=>route.abort())
  const page=await context.newPage()
  page.on('pageerror',e=>errors.push({platform,error:e.message}))
  for(const lang of ['en','zh'])for(const width of [390,1440])for(const path of paths){
   await page.setViewportSize({width,height:900})
   const url=`http://127.0.0.1:${platform==='source'?8000:9300}/dk-${lang}/${path}`
   const response=await page.goto(url)
   assert.equal(response.status(),200,url)
   await expect(page.locator('main').first().locator('h1')).toHaveCount(1)
   await expect(page.locator('astro-island[client="load"][ssr]')).toHaveCount(0)
   await page.waitForLoadState('networkidle')
   await page.evaluate(()=>document.fonts.ready)
   // Next's outer main wraps navigation/footer and its page has another main.
   // Compare the closest page main, matching Astro's single semantic main.
   const content=await page.locator('h1').evaluate(heading=>{const main=heading.closest('main');return ({
    text:main.textContent.replace(/\s+/g,' ').trim(),
    heading:main.querySelector('h1').textContent,
    links:[...main.querySelectorAll('a')].map(a=>({href:a.getAttribute('href'),text:a.textContent.replace(/\s+/g,' ').trim()})),
    headingBox:(()=>{const b=main.querySelector('h1').getBoundingClientRect();return {x:b.x,y:b.y,width:b.width,height:b.height}})(),
   })})
   const localeGeometry=await page.locator('h1').evaluate((heading,locale)=>{
    // Diagnostic only: source hardcodes html lang=en even for Chinese. Chrome
    // chooses a different macOS CJK fallback. Preserve raw measurements above
    // and restore the DOM after measuring the same document locale.
    const root=document.documentElement,original=root.lang
    try{root.lang=locale;const b=heading.getBoundingClientRect();return {original_lang:original,matched_lang:locale,box:{x:b.x,y:b.y,width:b.width,height:b.height}}}
    finally{root.lang=original}
   },lang==='zh'?'zh-CN':'en')
   const overflow=await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth)
   assert.equal(overflow,false,url)
   const canonical=await page.locator('link[rel="canonical"]').getAttribute('href')
   if(platform==='astro')assert.equal(new URL(canonical).pathname,`/dk-${lang}/${path}`)
   if(path==='reviews'){
    await page.screenshot({path:join(output,`${platform}-${lang}-${width}-reviews.png`)})
    const back=page.locator('main').getByRole('link',{name:lang==='en'?'Back to the store':'返回商店'})
    await expect(back).toHaveAttribute('href',`/dk-${lang}/store`)
   }
   rows.push({platform,lang,width,path,status:response.status(),title:await page.title(),description:await page.locator('meta[name="description"]').getAttribute('content'),canonical,content,localeGeometry,overflow})
  }
  const navigation=[]
  for(const lang of ['en','zh'])for(const width of [390,1440]){
   await page.setViewportSize({width,height:900})
   await page.goto(`http://127.0.0.1:${platform==='source'?8000:9300}/dk-${lang}/about`)
   await expect(page.locator('astro-island[client="load"][ssr]')).toHaveCount(0)
   if(width===390){
    await page.getByTestId('nav-menu-button').click()
    const link=page.locator('header').getByRole('link',{name:lang==='en'?'Reviews':'用户评价',exact:true}).filter({visible:true})
    await link.focus();await link.press('Enter')
   }else{
    await page.locator('header').getByRole('link',{name:lang==='en'?'Reviews':'用户评价',exact:true}).click()
   }
   await expect(page).toHaveURL(new RegExp(`/dk-${lang}/reviews$`))
   await expect(page.getByRole('heading',{level:1})).toHaveText(lang==='en'?'Customer reviews.':'用户评价。')
   const back=page.locator('h1').locator('xpath=ancestor::main[1]').getByRole('link',{name:lang==='en'?'Back to the store':'返回商店'})
   await back.click();await expect(page.getByTestId('product-wrapper')).toHaveCount(30)
   navigation.push({lang,width,reviews_from_header:true,mobile_keyboard:width===390,return_to_store:true})
  }
  await writeFile(join(output,`${platform}-navigation.json`),JSON.stringify(navigation,null,2)+'\n')
  await context.close()
 }
 const comparisons=rows.filter(r=>r.platform==='source').map(source=>{
  const target=rows.find(t=>t.platform==='astro'&&t.lang===source.lang&&t.width===source.width&&t.path===source.path)
  return {lang:source.lang,width:source.width,path:source.path,text_equal:source.content.text===target.content.text,links_equal:JSON.stringify(source.content.links)===JSON.stringify(target.content.links),title_equal:source.title===target.title,heading_geometry_equal:JSON.stringify(source.content.headingBox)===JSON.stringify(target.content.headingBox),heading_geometry_same_document_locale:JSON.stringify(source.localeGeometry.box)===JSON.stringify(target.localeGeometry.box),description_equal:source.description===target.description}
 })
 const report={output,rows,comparisons,errors,read_only:true,api_mocking:false,actual_local_services:true,full_storefront_accepted:false}
 await writeFile(join(output,'report.json'),JSON.stringify(report,null,2)+'\n')
 console.log(JSON.stringify({output,pages:rows.length,comparisons,errors}))
 assert.deepEqual(errors,[])
 assert.ok(comparisons.every(r=>r.text_equal&&r.links_equal&&r.title_equal&&r.heading_geometry_same_document_locale&&r.description_equal),'Public source/target differences; inspect ignored report')
}finally{await browser.close()}
