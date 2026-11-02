import assert from 'node:assert/strict'
import {join} from 'node:path'
import {expect} from '@playwright/test'

// Real built Astro SSR and hydration against the owned Gin/MySQL catalog.
export async function verifyNativeSearch(context,astro,output,errors){
 const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message))
 const cases=[['',30],['quantities',30],['verifying',30],['the',0],['the iphone',30],['iphone the',30],['iphone pr',30],['i13pm',30],['iphone unfindable',0],['i13 unfindable',0],['苹果',0],['space-gray',30],['space gray',30],['6.1',30],['6 1',0],['a@example.test',30],['a example test',0],['phone128',0],['phones128',30],['1.2e3',30],['8.3.0',30],['catalog.example.test',30],['https://catalog.example.test/phones',30],['/phones',30],['catalog example test',0],['/usr/local/phones.txt',30],['usr local phones',0],['hiddenkeyword',0],['visibleword',30],['amp',0],['scriptsecret',0],['<b>verifying</b>',30],['&amp;',0],['<script>verifying</script>',0],['rankneedle close',4],['nativematerial',2],['nativefinish',2],['choiceshared',2],['nativematerial choiceshared',1],['nativematerial choiceother',1],['choicedeleted',0],['choiceunselected',0]]
 const results=[]
 for(const lang of ['en','zh'])for(const width of [1440,390]){
  await page.setViewportSize({width,height:900})
  for(const [query,count]of cases){
   const url=astro+'/dk-'+lang+'/store'+(query?'?q='+encodeURIComponent(query):'')+(query==='rankneedle close'?'&sortBy=product':'')
   const response=await page.goto(url);assert.equal(response.status(),200)
   const html=await response.text();assert.equal((html.match(/data-testid="product-wrapper"/g)||[]).length,count,'SSR '+query)
   const rankOrder=['03','01','02','00']
   if(query==='rankneedle close'){
    const ids=[...html.matchAll(/href="\/dk-(?:en|zh)\/products\/search-english-(\d+)"/g)].map(m=>m[1])
    assert.deepEqual([...new Set(ids)],rankOrder,'SSR cover-density order')
   }
   await expect(page.locator('astro-island[client="load"][ssr]')).toHaveCount(0)
   await expect(page.getByTestId('product-wrapper')).toHaveCount(count)
   if(query==='rankneedle close'){
    const hrefs=await page.getByTestId('products-list').locator('a[href*="/products/search-english-"]').evaluateAll(nodes=>nodes.map(n=>n.getAttribute('href').split('-').pop()))
    assert.deepEqual([...new Set(hrefs)],rankOrder,'hydrated cover-density order')
   }
   if(!count)await expect(page.getByTestId('no-products')).toBeVisible()
   await expect(page.getByTestId('products-error')).toHaveCount(0)
   assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
   if(count){
    await expect(page.getByTestId('product-title').first()).toHaveText('iPhone 13 Pro Max')
    assert.ok((await page.getByTestId('products-list').locator('a').first().getAttribute('href')).startsWith('/dk-'+lang+'/products/search-english-'))
   }
   results.push({lang,width,query,count,ssr:true,hydration:true})
  }
  await page.goto(astro+'/dk-'+lang+'/store?q=verifying')
  await expect(page.getByTestId('product-wrapper')).toHaveCount(30)
  await expect(page.locator('astro-island[client="load"][ssr]')).toHaveCount(0)
  if(width===1440){
   const input=page.getByTestId('store-search-input').filter({visible:true})
   await expect(input).toHaveValue('verifying')
   await input.fill('the');await expect(page.getByTestId('no-products')).toBeVisible()
   await expect.poll(()=>new URL(page.url()).searchParams.get('q')).toBe('the')
   await page.goBack();await expect(input).toHaveValue('verifying');await expect(page.getByTestId('product-wrapper')).toHaveCount(30)
   await page.getByTestId('store-search-clear').filter({visible:true}).click()
   await expect(input).toHaveValue('');await expect(page.getByTestId('product-wrapper')).toHaveCount(30)
   await expect.poll(()=>new URL(page.url()).searchParams.has('q')).toBe(false)
  }
  await page.screenshot({path:join(output,'search-native-'+lang+'-'+width+'.png'),fullPage:true})
 }
 // Exercise the real Astro public bridge with the largest editor-supported
 // option identity. No request interception or synthetic API response.
 const qualified='型'.repeat(100)+':'+'值'.repeat(500)
 const qualifiedResponse=await context.request.post(astro+'/api/store/search',{data:{region_id:'option-region',queries:[{key:'long-option',filters:{option_values:[qualified]},sort:'relevance',page:1,limit:50}]}})
 assert.equal(qualifiedResponse.status(),200)
 const qualifiedResult=(await qualifiedResponse.json()).results[0]
 assert.equal(qualifiedResult.count,1)
 assert.equal(qualifiedResult.hits[0].id,'search-english-04')
 assert.deepEqual(qualifiedResult.hits[0].values.option_values,[qualified])
 await page.close()
 return {real_built_astro:true,api_mocking:false,actual_gin_mysql:true,owned_products:30,cases:results,desktop_typing_clear_history:true,localized_links:true,mobile_no_overflow:true,cover_density_order:true,option_title_and_value_search:true,qualified_option_601_chars_via_actual_astro_bridge:true,deleted_and_unselected_options_absent:true,full_search_parity:false}
}
