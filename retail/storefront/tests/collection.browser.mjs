import assert from 'node:assert/strict'
import {createServer} from 'node:http'
import {spawn} from 'node:child_process'
import {once} from 'node:events'
import {createRequire} from 'node:module'
import {fileURLToPath} from 'node:url'
import {setTimeout as delay} from 'node:timers/promises'
const require=createRequire(new URL('../../admin/package.json',import.meta.url)),{chromium,expect}=require('@playwright/test')
const host='127.0.0.1',port=19374,shop=`http://${host}:${port}`
const region={id:'region',name:'Europe',currency_code:'eur',automatic_taxes:false,countries:[{iso_2:'dk',iso_3:'dnk',num_code:'208',name:'Denmark',display_name:'Denmark',region_id:'region'}]}
const collection={id:'col',title:'Phone collection',handle:'phones',product_count:25}
const products=Array.from({length:25},(_,i)=>({id:`p-${i}`,title:`Phone ${i}`,handle:`phone-${i}`,subtitle:null,description:null,thumbnail:i===0?'/iphone-thumbnails/iphone-16e/white.webp':null,collection_id:'col',options:[],images:[],display:{colors:[{name:'Blue',hex:'#1677FF'}],model_specs:null,launch_prices:[],source:null},variants:[{id:`v-${i}`,title:'Default',sku:null,thumbnail:null,variant_rank:0,options:[],images:[],configuration_complete:true,manage_inventory:false,allow_backorder:false,inventory_quantity:null,availability:{state:'untracked',can_fulfill:true,max_fulfillable_quantity:null},display:{slug:null,demo_price_basis:null,apple_launch_price:null},calculated_price:{calculated_amount:String(25-i),original_amount:i===24?'10':null,currency_code:'eur',is_calculated_price_tax_inclusive:false,is_original_price_tax_inclusive:false,is_calculated_price_price_list:i===24,is_original_price_price_list:false,calculated_price:{price_list_type:i===24?'sale':null}}}]}))
let failed=false,broken=false,empty=false,child,browser
const calls=[],errors=[]
const api=createServer((req,res)=>{
 const url=new URL(req.url,'http://fixture.invalid');let status=200,body={}
 calls.push(url.pathname+url.search)
 assert.equal(req.headers.cookie,undefined);assert.equal(req.headers.authorization,undefined)
 switch(url.pathname){
  case '/api/store/regions':body={regions:[region],count:1};break
  case '/api/store/locales':body={locales:[],count:0};break
  case '/api/store/product-categories':body={product_categories:[],count:0,limit:2,offset:0};break
  case '/api/store/collections':{
   const limit=Number(url.searchParams.get('limit')||100),handle=url.searchParams.get('handle'),rows=handle&&handle!=='phones'?[]:[collection]
   body={collections:rows,count:rows.length,limit,offset:0};break
  }
  case '/api/store/products':
   assert.equal(url.searchParams.get('collection_id'),'col');assert.equal(url.searchParams.get('region_id'),'region');assert.ok([null,'-created_at'].includes(url.searchParams.get('order')));assert.equal(url.searchParams.get('limit'),'100')
   if(failed){status=503;break}
   {const rows=empty?[]:url.searchParams.has('option_value_id')?products.slice(0,2):products
    body={products:broken?rows.map(p=>({...p,collection_id:'foreign'})):rows,count:rows.length,limit:100,offset:0,capabilities:{pricing:true,availability:true,checkout:false}};break}
  case '/api/store/cart':status=401;break
  default:status=404
 }
 res.writeHead(status,{'Content-Type':'application/json'});res.end(JSON.stringify(body))
})
try{
 const probe=createServer();probe.listen(port,host);await once(probe,'listening');await new Promise(resolve=>probe.close(resolve))
 api.listen(0,host);await once(api,'listening');const address=api.address();assert.ok(address&&typeof address!=='string')
 child=spawn(process.execPath,['scripts/start.mjs'],{env:{...process.env,HOST:host,PORT:String(port),GIN_API_ORIGIN:`http://${host}:${address.port}`,STOREFRONT_ORIGIN:shop},stdio:'ignore'})
 let ready=false;for(let i=0;i<100;i++){if(child.exitCode!==null)throw new Error('Owned Astro stopped');try{if((await fetch(shop+'/health')).ok){ready=true;break}}catch{}await delay(100)}assert.ok(ready)
 const request=async(path,status)=>{const response=await fetch(shop+path);assert.equal(response.status,status,path);return response.text()}
 let html=await request('/dk-en/collections/phones',200)
 assert.match(html,/<h1>Phone collection<\/h1>/);assert.match(html,/Phone 0/);assert.match(html,/Phone 11/);assert.ok(!html.includes('Phone 12<'))
 assert.match(html,/<meta name="description" content="Phone collection collection"/)
 assert.match(html,/<link rel="canonical" href="http:\/\/127.0.0.1:19374\/dk-en\/collections\/phones"/)
 await request('/dk-en/collections/missing',404);await request('/us-en/collections/phones',404)
 for(const query of ['page=0','sortBy=unknown','page=1&page=2'])await request('/dk-en/collections/phones?'+query,400)
 failed=true;await request('/dk-en/collections/phones',503);failed=false
 broken=true;await request('/dk-en/collections/phones',503);broken=false
 browser=await chromium.launch({headless:true,executablePath:process.env.ADMIN_TEST_BROWSER||'C:/Program Files/Google/Chrome/Application/chrome.exe'})
 const context=await browser.newContext({reducedMotion:'reduce'}),page=await context.newPage()
 page.on('pageerror',error=>errors.push(error.message))
 await page.setViewportSize({width:1440,height:1000});await page.goto(shop+'/dk-en/collections/phones')
 await expect(page.getByTestId('products-list').getByTestId('product-wrapper')).toHaveCount(12)
 await expect(page.getByTestId('product-wrapper').first().locator('img')).toHaveAttribute('src','/iphone-thumbnails/iphone-16e/white.webp')
 assert.equal(await page.getByTestId('product-wrapper').first().locator('img').evaluate(async img=>{await img.decode();return img.naturalWidth>0}),true)
 await page.getByTestId('collection-sort').getByText('Price: Low -> High',{exact:true}).click()
 await expect(page).toHaveURL(/sortBy=price_asc/);await expect(page.getByTestId('product-title').first()).toHaveText('Phone 24')
 await expect(page.getByTestId('product-wrapper').first().getByTestId('original-price')).toHaveText('€10.00')
 await expect(page.getByTestId('product-wrapper').first().getByTestId('price')).toHaveText('€1.00')
 await expect(page.getByTestId('products-list').locator('a').first()).toHaveAttribute('href','/dk-en/products/phone-24')
 await page.getByTestId('product-pagination').getByRole('button',{name:'2',exact:true}).click()
 await expect(page).toHaveURL(/page=2/);await expect(page.getByTestId('product-title').first()).toHaveText('Phone 12')
 await page.getByTestId('collection-sort').getByText('Price: High -> Low',{exact:true}).click()
 await expect(page.getByTestId('product-title').first()).toHaveText('Phone 0');assert.equal(new URL(page.url()).searchParams.has('page'),false)
 await page.goto(shop+'/dk-en/collections/phones?page=2&optionValueIds=storage&optionValueIds=colour&keep=yes')
 await expect(page.getByTestId('product-title')).toHaveCount(0)
 await page.getByTestId('collection-sort').getByText('Price: Low -> High',{exact:true}).click()
 await expect(page.getByTestId('product-title')).toHaveCount(2)
 assert.deepEqual(new URL(page.url()).searchParams.getAll('optionValueIds'),['storage','colour']);assert.equal(new URL(page.url()).searchParams.get('keep'),'yes')
 assert.ok(calls.some(c=>c.includes('option_value_id=storage&option_value_id=colour')))
 for(const locale of ['en','zh'])for(const width of [390,1440]){
  await page.setViewportSize({width,height:1000});await page.goto(shop+`/dk-${locale}/collections/phones`)
  await expect(page.getByTestId('collection-sort')).toBeVisible()
  await expect(page.getByTestId('product-wrapper')).toHaveCount(12)
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
  await page.getByTestId('product-wrapper').first().locator('img').evaluate(img=>img.decode())
  await page.screenshot({path:fileURLToPath(new URL(`../../../logs/gin-migration/collection-${locale}-${width}.png`,import.meta.url)),fullPage:true})
 }
 empty=true;await page.reload();await expect(page.getByTestId('collection-page')).toBeVisible();await expect(page.getByTestId('product-wrapper')).toHaveCount(0);await expect(page.getByTestId('product-pagination')).toHaveCount(0)
 assert.deepEqual(errors,[])
 console.log('ASTRO_COLLECTION_BROWSER '+JSON.stringify({formal_astro:true,ssr_first_twelve:true,synthetic_api:true,latest_price_sort_and_pagination:true,sort_resets_page:true,repeated_option_filters_preserved:true,sale_price:true,original_product_links:true,empty_distinct_from_503:true,missing_404:true,invalid_query_400:true,foreign_collection_503:true,no_private_headers:true,widths:[390,1440],en_zh:true,browser_errors:0,real_gin_mysql:false,source_pixel_comparison:false}))
}finally{
 if(browser)await browser.close()
 if(child&&child.exitCode===null){child.kill();await once(child,'exit')}
 await new Promise(resolve=>api.close(resolve))
}
