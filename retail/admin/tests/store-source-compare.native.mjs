import assert from 'node:assert/strict'
import {mkdir,writeFile} from 'node:fs/promises'
import {fileURLToPath} from 'node:url'
import {join} from 'node:path'
import {chromium,expect} from '@playwright/test'

// Read-only comparison against the existing local source and migrated services.
// Never signs in, clicks commerce actions or changes configuration/data.
const output=fileURLToPath(new URL('../../../logs/gin-migration/store-source-compare-'+Date.now()+'/',import.meta.url))
await mkdir(output,{recursive:true})
const browser=await chromium.launch({channel:'chrome',headless:true})
const cases=[['all',''],['iphone13','q=iPhone%2013'],['shorthand','q=i13pm'],['prefix','q=iphone%20pr'],['stopwords','q=the%20iphone'],['series','series=iPhone%2013'],['storage','storage=256%20GB'],['colour','color=Black'],['price','sortBy=product%2Fsort%2Fmin_price_eur%3Aasc'],['quantity_plural','q=quantities'],['stem_verify','q=verifying'],['stopword_only','q=the'],['last_term_relax','q=iphone%20unfindable'],['expanded_all','q=i13%20unfindable'],['cn','q=%E8%8B%B9%E6%9E%9C'],['relevance','q=Pro&sortBy=product']]
const rows=[]
try{
 for(const platform of ['source','astro']){
  const context=await browser.newContext({reducedMotion:'reduce',timezoneId:'Europe/Copenhagen'})
  await context.route('https://**/*',route=>route.abort())
  const page=await context.newPage();page.setDefaultTimeout(30000)
  for(const lang of ['en','zh'])for(const width of [390,768,1440]){
   for(const [key,query]of cases){
    if(key!=='all'&&(lang!=='en'||width!==1440))continue
    const errors=[],onError=e=>errors.push(e.message);page.on('pageerror',onError)
    await page.setViewportSize({width,height:1000})
    const url=`http://127.0.0.1:${platform==='source'?8000:9300}/dk-${lang}/store${query?'?'+query:''}`
    const response=await page.goto(url);assert.equal(response.status(),200,url)
    await expect(page.getByTestId('products-list').or(page.getByTestId('no-products')).or(page.getByTestId('products-error'))).toBeVisible()
    await expect(page.getByTestId('products-error')).toHaveCount(0)
    // Wait until any paging/search skeleton and React loading state settles.
    await expect(page.getByTestId('products-list').getByTestId('product-wrapper').or(page.getByTestId('no-products')).first()).toBeVisible()
    await expect(page.locator('astro-island[client="load"][ssr]')).toHaveCount(0)
    // Source and Astro may paint the SSR cards before search routing and the
    // responsive hook hydrate. Observe the intended URL query and a stable
    // loaded card list after all local search requests have settled.
    if(width===1440)await expect(page.getByTestId('store-search-input').filter({visible:true})).toHaveValue(new URL(url).searchParams.get('q')??'')
    await page.waitForLoadState('networkidle')
    await page.evaluate(()=>document.fonts.ready)
    const cards=await page.getByTestId('products-list').locator('li').evaluateAll(nodes=>nodes.map(li=>{
     const a=li.querySelector('a'),card=li.querySelector('[data-testid="product-wrapper"]'),img=li.querySelector('img'),css=card&&getComputedStyle(card),box=card?.getBoundingClientRect()
     let image=img?.getAttribute('src')??null
     if(image){const u=new URL(image,location.origin);image=u.pathname==='/_next/image'?u.searchParams.get('url'):u.pathname}
     return {handle:a?.getAttribute('href')?.split('/products/')[1],text:li.textContent.replace(/\s+/g,' ').trim(),image,geometry:box?{x:box.x,y:box.y,width:box.width,height:box.height}:null,style:css?{display:css.display,columns:css.gridTemplateColumns,font:css.fontFamily,fontSize:css.fontSize,background:css.backgroundColor}:null}
    }))
    const row={platform,lang,width,case:key,url,cards,body_overflow:await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),errors}
    if(key==='all'){
     await page.screenshot({path:join(output,`${platform}-${lang}-${width}.png`)})
     if(cards.length){await page.getByTestId('product-wrapper').first().screenshot({path:join(output,`${platform}-card-${lang}-${width}.png`)})}
    }
    rows.push(row);page.off('pageerror',onError)
    console.log(JSON.stringify({platform,lang,width,case:key,count:cards.length,errors}))
   }
  }
  await context.close()
 }
 const comparisons=rows.filter(r=>r.platform==='source').map(source=>{
  const target=rows.find(r=>r.platform==='astro'&&r.lang===source.lang&&r.width===source.width&&r.case===source.case)
  const ids=r=>r.cards.map(c=>c.handle),sameSet=JSON.stringify([...ids(source)].sort())===JSON.stringify([...ids(target)].sort())
  return {lang:source.lang,width:source.width,case:source.case,source_count:source.cards.length,astro_count:target.cards.length,same_set:sameSet,same_order:JSON.stringify(ids(source))===JSON.stringify(ids(target)),different_cards:source.cards.flatMap(s=>{const t=target.cards.find(t=>t.handle===s.handle);return !t||s.text!==t.text||s.image!==t.image?[{handle:s.handle,source:s,target:t??null}]:[]}),first_geometry:{source:source.cards[0]?.geometry,astro:target.cards[0]?.geometry}}
 })
 await writeFile(join(output,'result.json'),JSON.stringify({rows,comparisons,read_only:true,real_local_services:true,source_pixel_accepted:false,goal_complete:false},null,2)+'\n')
 console.log('STORE_SOURCE_COMPARE '+JSON.stringify({output,comparisons:comparisons.map(({different_cards,...r})=>({...r,different_card_count:different_cards.length}))}))
}finally{await browser.close()}
