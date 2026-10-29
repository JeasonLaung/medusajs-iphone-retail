import assert from 'node:assert/strict'
import {searchResponse} from './search-fixture.mjs'
import {createServer} from 'node:http'
import {createServer as portProbe} from 'node:net'
import {once} from 'node:events'
import {spawn} from 'node:child_process'
import {createRequire} from 'node:module'
import {fileURLToPath} from 'node:url'
import {product as sourceProduct} from './selection-fixture/src/product.ts'

// Exercise the built, formal route (not a separate preview/test Astro root).
// This server owns synthetic public API data and never accesses live accounts.
const require=createRequire(new URL('../../admin/package.json',import.meta.url))
const {chromium,expect}=require('@playwright/test')
const product=structuredClone(sourceProduct)
product.description='Plain description <script>window.injected=true</script>'
product.images.forEach(image=>image.url='/iphone-thumbnails/iphone-14/blue.webp')
for(const variant of product.variants)variant.images.forEach(image=>image.url='/iphone-thumbnails/iphone-14/blue.webp')
product.images.push(...product.variants.flatMap(variant=>variant.images))
// The shared pure-selection fixture also exercises beyond-Number amounts;
// use normal handset prices for this real page's mobile layout checks.
product.variants.forEach((variant,index)=>variant.calculated_price.calculated_amount=String(499+index*100))
const region={id:'region',name:'Europe',currency_code:'eur',automatic_taxes:false,countries:[{iso_2:'dk',iso_3:'dnk',num_code:'208',name:'Denmark',display_name:'Denmark',region_id:'region'}]}
const upstreamRequests=[]
let dependencyFailed=false,contactsFailed=false,policiesFailed=false,child,browser
let policies={warranty_months:12,return_days:30,warranty_note:{en:"Twelve months covered",zh:"十二个月保障"},return_note:{en:"Thirty day returns",zh:"三十天退货"}}
const api=createServer(async(request,response)=>{
 upstreamRequests.push({url:request.url,headers:request.headers})
 if(dependencyFailed){response.writeHead(503,{'content-type':'application/json'});response.end('{"error":"PRIVATE_UPSTREAM_FAILURE"}');return}
 const url=new URL(request.url,'http://fixture.invalid')
 let body
 switch(url.pathname){
  case '/api/store/search': {let raw='';for await(const chunk of request)raw+=chunk;body=searchResponse(JSON.parse(raw));break}
  case '/api/store/buy-policies':
   if(policiesFailed){response.writeHead(503,{'content-type':'application/json'});response.end('{}');return}
   body={policies};break
  case '/api/store/customer-service':
   if(contactsFailed){response.writeHead(503,{'content-type':'application/json'});response.end('{}');return}
   body={contacts:[{id:'support',name:'Support Team',department:'after_sales',whatsapp:'+4512345678',email:'help@example.test',timezone:'Europe/Copenhagen',days:[1,2,3,4,5],start:'09:00',end:'17:00',availability:'online'}]};break
  case '/api/store/regions':body={regions:[region],count:1};break
  case '/api/store/locales':body={locales:[],count:0};break
  case '/api/store/product-categories':body={product_categories:[],count:0,limit:2,offset:0};break
  case '/api/store/collections':body={collections:[],count:0,limit:6,offset:0};break
  case '/api/store/products':{
   const found=url.searchParams.get('handle')===product.handle
   body={products:found?[product]:[],count:found?1:0,limit:2,offset:0,capabilities:{pricing:true,availability:true,checkout:false}};break
  }
  default:response.writeHead(404);response.end();return
 }
 response.writeHead(200,{'content-type':'application/json'});response.end(JSON.stringify(body))
})
try{
 api.listen(0,'127.0.0.1');await once(api,'listening')
 const probe=portProbe();probe.listen(0,'127.0.0.1');await once(probe,'listening')
 const port=probe.address().port;await new Promise((resolve,reject)=>probe.close(error=>error?reject(error):resolve()))
 const origin=`http://127.0.0.1:${port}`
 child=spawn(process.execPath,['dist/server/entry.mjs'],{env:{...process.env,HOST:'127.0.0.1',PORT:String(port),GIN_API_ORIGIN:`http://127.0.0.1:${api.address().port}`,ASTRO_TELEMETRY_DISABLED:'1'},stdio:['ignore','ignore','pipe'],windowsHide:true})
 let output='';child.stderr.on('data',chunk=>{output=(output+chunk).slice(-4000)})
 child.on('error',()=>{})
 let started=false
 for(let i=0;i<100;i++){
  assert.equal(child.exitCode,null,'Owned Astro exited before readiness')
  try{if((await fetch(origin+'/health')).status===200){started=true;break}}catch{}
  await new Promise(resolve=>setTimeout(resolve,100))
 }
 assert.ok(started,'Owned Astro did not become ready')
 const path='/dk-en/products/'+product.handle
 const reply=await fetch(origin+path,{headers:{Cookie:'retail_admin_session=PRIVATE_COOKIE',Authorization:'Bearer PRIVATE_AUTH'}})
 assert.equal(reply.status,200);assert.equal(reply.headers.get('cache-control'),'no-store');assert.equal(reply.headers.get('set-cookie'),null)
 const html=await reply.text()
 assert.ok(html.includes('data-testid="product-container"'));assert.ok(html.includes('All iPhones'))
 assert.ok(!html.includes('<script>window.injected=true</script>'))
 assert.ok(!html.includes('PRIVATE_COOKIE'));assert.ok(!html.includes('PRIVATE_AUTH'))
 assert.ok(upstreamRequests.every(request=>!request.headers.cookie&&!request.headers.authorization))
 assert.equal((await fetch(origin+'/dk-en/products/missing')).status,404)
 assert.equal((await fetch(origin+'/zz-en/products/'+product.handle)).status,404)
 assert.equal((await fetch(origin+'/dk-fr/products/'+product.handle)).status,404)
 browser=await chromium.launch({headless:true,...(process.env.ADMIN_TEST_BROWSER?{executablePath:process.env.ADMIN_TEST_BROWSER}:{})})
 const page=await browser.newPage(),errors=[]
 page.on('pageerror',error=>{errors.push(error.message);console.error('BROWSER_ERROR',error.message)})

 // No third-party font requests are needed to verify navigation/hydration.
 await page.route('https://**/*',route=>route.abort())
 await page.goto(origin+path+'?campaign=a&campaign=b&v_id=v128#product-info')
 await expect(page.getByRole('heading',{name:product.title,exact:true})).toBeVisible()
 await page.getByRole('radio',{name:/256/}).check()
 await expect(page).toHaveURL(/iphone-13-256gb-blue-S85\?campaign=a&campaign=b#product-info$/)
 await expect(page.locator('link[rel="canonical"]')).toHaveAttribute('href',origin+'/dk-en/products/iphone-13-256gb-blue-S85')
 await expect(page.getByRole('button',{name:'Add to cart',exact:true}).first()).toBeDisabled()
 await page.reload()
 await expect(page.getByRole('radio',{name:/256/})).toBeChecked()
 assert.deepEqual(errors,[])
 await page.screenshot({path:fileURLToPath(new URL('../../../logs/gin-migration/formal-product-desktop.png',import.meta.url)),fullPage:true})
 await page.setViewportSize({width:390,height:844})
 await page.goto(origin+'/dk-zh/products/'+product.handle)
 await expect(page.getByRole('navigation',{name:'面包屑导航'})).toBeVisible()
 await expect(page.locator('html')).toHaveAttribute('lang','zh-CN')
 assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),'Mobile page overflows')
 await page.screenshot({path:fileURLToPath(new URL('../../../logs/gin-migration/formal-product-mobile.png',import.meta.url)),fullPage:true})
 for(const language of ['en','zh']){
  const segment='dk-'+language
  await page.goto(origin+`/${segment}/about#our-focus`)
  await expect(page.locator('#our-focus')).toBeVisible()
  await expect(page.locator('main h1')).toHaveCount(1)
  await page.goto(origin+`/${segment}/help#service-title`)
  await expect(page.locator('#service-title')).toBeVisible()
  await expect(page.locator('a[href="mailto:help%40example.test"]')).toBeVisible()
  await expect(page.getByRole('link',{name:/WhatsApp/}).first()).toHaveAttribute('href','https://wa.me/4512345678')
  await expect(page.getByRole('link',{name:/WhatsApp/}).first()).toHaveAttribute('rel','noopener noreferrer')
  assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth))
  await page.goto(origin+`/${segment}/blog`)
  await expect(page.locator('[data-testid="home-blog"] article')).toHaveCount(3)
  await page.locator('[data-testid="home-blog"] article a').first().click()
  await expect(page).toHaveURL(origin+`/${segment}/blog/choose-your-storage`)
  await expect(page.locator('main h1')).toHaveCount(1)
  await expect(page.locator('main section')).toHaveCount(3)
  await page.locator('main a').first().click()
  await expect(page).toHaveURL(origin+`/${segment}/blog`)
 }
 assert.equal((await fetch(origin+'/dk-en/blog/unknown')).status,404)
 assert.equal((await fetch(origin+'/zz-en/about')).status,404)
 assert.equal((await fetch(origin+'/dk-en/unknown')).status,404)
 contactsFailed=true
 await page.goto(origin+'/dk-zh/help')
 await expect(page.getByText('联系方式暂时无法加载，请稍后重试。',{exact:true})).toBeVisible()
 await expect(page.locator('#shopping')).toBeVisible()
 assert.deepEqual(errors,[])
 // Formal search SSR and hydrated state use the same query/currency contract.
 const store=await fetch(origin+'/dk-en/store?storage=128%20GB',{headers:{Cookie:'PRIVATE_SEARCH_COOKIE',Authorization:'Bearer PRIVATE_SEARCH_AUTH'}})
 assert.equal(store.status,200)
 const storeHTML=await store.text();assert.ok(storeHTML.includes('data-testid="product-wrapper"'));assert.ok(storeHTML.includes('iPhone 13 00'));assert.ok(!storeHTML.includes('iPhone 14 01'))
 await page.setViewportSize({width:1440,height:1000})
 await page.goto(origin+'/dk-en/store')
 await page.waitForFunction(()=>Array.from(document.querySelectorAll('astro-island')).every(island=>!island.hasAttribute('ssr')))
 await expect(page.getByRole('heading',{name:'12 months',exact:true})).toBeVisible()
 await expect(page.getByRole('heading',{name:'30 days',exact:true})).toBeVisible()
 await expect(page.getByText('Twelve months covered',{exact:true})).toBeVisible()
 await expect(page.getByTestId('product-wrapper')).toHaveCount(50)
 await expect(page.getByTestId('sort-by-container')).toHaveValue('product/sort/created_at:desc')
 await page.getByTestId('product-pagination').getByRole('button',{name:'2',exact:true}).click()
 await expect(page.getByTestId('product-wrapper')).toHaveCount(2)
 await expect(page).toHaveURL(/page=2/)
 await page.goBack()
 await expect(page.getByTestId('product-wrapper')).toHaveCount(50)

 await page.getByTestId('sort-by-container').selectOption('product/sort/min_price_eur:desc')
 await expect(page.getByTestId('product-title').first()).toHaveText('iPhone 14 51')
 await expect(page).toHaveURL(/sortBy=product/)
 await page.getByTestId('clear-refinements').click()
 await expect(page.getByTestId('product-title').first()).toHaveText('iPhone 13 00')
 await page.getByTestId('filter-group-storage_options').getByRole('button',{name:/128 GB/}).click()
 await expect(page.getByTestId('product-wrapper')).toHaveCount(26)
 await expect(page).toHaveURL(/storage=128/)
 await expect(page.getByTestId('product-storage').filter({hasText:'256 GB'}).first()).toHaveAttribute('data-disabled','true')
 await page.reload()
 await expect(page.getByTestId('product-wrapper')).toHaveCount(26)
 await page.getByTestId('store-search-input').fill('missing')
 await expect(page.getByTestId('no-products')).toBeVisible()
 await page.getByTestId('clear-refinements').click()
 await expect(page.getByTestId('product-wrapper')).toHaveCount(50)
 await expect(page.getByRole('slider').first()).toHaveAttribute('aria-valuemin','499')
 await expect(page.getByRole('slider').first()).toHaveAttribute('aria-valuemax','550')
 assert.equal(await page.getByTestId('price-histogram').locator('[data-count]').evaluateAll(rows=>rows.reduce((sum,row)=>sum+Number(row.getAttribute('data-count')),0)),52)
 await page.screenshot({path:fileURLToPath(new URL('../../../logs/gin-migration/formal-store-desktop.png',import.meta.url)),fullPage:true})
 await page.setViewportSize({width:390,height:844})
 await page.goto(origin+'/dk-zh/store')
 await page.waitForFunction(()=>Array.from(document.querySelectorAll('astro-island')).every(island=>!island.hasAttribute('ssr')))
 await expect(page.getByRole('heading',{name:'12 个月',exact:true})).toBeVisible()
 await expect(page.getByText('十二个月保障',{exact:true})).toBeVisible()
 await expect(page.getByTestId('mobile-filter-toolbar')).toBeVisible()
 await page.getByTestId('mobile-filter-model').click()
 await expect(page.getByTestId('mobile-filters-panel')).toBeVisible()
 await page.getByTestId('mobile-option-category').first().check()
 await page.getByTestId('show-filter-results').click()
 await expect(page.getByTestId('mobile-filters-panel')).toBeHidden()
 await expect(page.getByTestId('product-wrapper')).toHaveCount(26)
 assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),'Mobile search overflows')
 await page.screenshot({path:fileURLToPath(new URL('../../../logs/gin-migration/formal-store-mobile.png',import.meta.url)),fullPage:true})
 assert.deepEqual(errors,[])
 assert.ok(upstreamRequests.every(request=>!request.headers.cookie&&!request.headers.authorization))
 policies={...policies,warranty_months:0,return_days:0}
 await page.goto(origin+'/dk-en/store')
 await expect(page.getByRole('heading',{name:'No warranty',exact:true})).toBeVisible()
 await expect(page.getByRole('heading',{name:'No returns',exact:true})).toBeVisible()
 policiesFailed=true
 const policyFailure=await fetch(origin+'/dk-en/store')
 assert.equal(policyFailure.status,200)
 assert.ok((await policyFailure.text()).includes('Contact us for details'))
 dependencyFailed=true
 const failedSearch=await fetch(origin+'/api/store/search',{method:'POST',headers:{'Content-Type':'application/json',Cookie:'PRIVATE_SEARCH_COOKIE'},body:JSON.stringify({region_id:'region',queries:[{key:'failure'}]})})
 assert.equal(failedSearch.status,503);assert.ok(!(await failedSearch.text()).includes('PRIVATE_UPSTREAM_FAILURE'))
 assert.equal((await fetch(origin+'/dk-en/store')).status,503)
 const invalidSearch=await fetch(origin+'/api/store/search',{method:'POST',body:'x'.repeat(65537)})
 assert.equal(invalidSearch.status,400)
 const failure=await fetch(origin+path)
 assert.equal(failure.status,503);assert.equal(failure.headers.get('cache-control'),'no-store')
 assert.ok(!(await failure.text()).includes('PRIVATE_UPSTREAM_FAILURE'))
 console.log('ASTRO_FORMAL_PRODUCT_PAGE '+JSON.stringify({formal_built_route:true,public_gin_contract:true,ssr_header_footer:true,private_headers_omitted:true,english_chinese:true,selection_hydration:true,slug_reload:true,query_hash_preserved:true,disabled_unconnected_commerce:true,mobile_no_overflow:true,missing_404_dependency_503:true,stored_buy_policies_and_optional_failure:true,formal_search_ssr_and_hydration:true,search_filters_sort_clear_mobile:true,search_pagination_history:true,search_histogram_bounds:true,search_bridge_failure_and_size:true,formal_about_help_journal:true,service_contacts_and_failure:true,article_navigation:true}))
}finally{
 if(browser)await browser.close()
 if(child&&child.exitCode===null){const ended=once(child,'exit');child.kill();await ended}
 await new Promise(resolve=>api.close(resolve))
}
