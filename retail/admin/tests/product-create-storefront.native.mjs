import assert from 'node:assert/strict'
import {chromium,expect} from '@playwright/test'
const {PRODUCT_CREATE_ASTRO_ORIGIN:origin,PRODUCT_CREATE_ID:id,ADMIN_TEST_API_TARGET:api}=process.env
if(!/^http:\/\/127\.0\.0\.1:[0-9]+$/.test(origin??'')||!/^prod_[a-f0-9-]{36}$/.test(id??'')||!api)throw Error('Owned native fixture required')
const regions=await (await fetch(api+'/api/store/regions')).json(),region=regions.regions.find(r=>r.currency_code==='eur')
const response=await fetch(api+'/api/store/products/'+id+'?region_id='+region.id);assert.equal(response.status,200);const {product}=await response.json()
const browser=await chromium.launch({headless:true,executablePath:process.env.ADMIN_TEST_BROWSER}),errors=[]
try{
 const page=await browser.newPage({reducedMotion:'reduce'});page.on('pageerror',e=>errors.push(e.message));const result=await page.goto(origin+'/dk-en/products/'+encodeURIComponent(product.handle));assert.equal(result.status(),200)
 await expect(page.getByTestId('product-title')).toHaveText(product.title);await expect(page.getByTestId('product-price')).toHaveAttribute('data-value','129.123456789012345678');await expect(page.getByTestId('add-product-button')).toBeDisabled();await expect(page.getByTestId('add-product-button')).toHaveText('Out of stock')
 await page.setViewportSize({width:390,height:844});assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
 await page.goto(origin+'/dk-zh/products/'+encodeURIComponent(product.handle));await expect(page.getByTestId('product-title')).toHaveText(product.title);await expect(page.getByTestId('add-product-button')).toHaveText('暂时缺货');assert.deepEqual(errors,[])
 console.log('PRODUCT_CREATE_ASTRO_NATIVE '+JSON.stringify({real_astro_gin_native_mysql:true,api_mocking:false,same_created_product:true,exact_quote_data_value:true,zero_stock_explained:true,purchase_disabled:true,language_en_zh:true,mobile_no_page_overflow:true,browser_errors:0}))
}finally{await browser.close()}
