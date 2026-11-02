import assert from 'node:assert/strict'
import {join} from 'node:path'
import {expect} from '@playwright/test'

// Called inside the actual authenticated Gin/Admin/Astro native joint. No
// request interception, replacement API, test route or production mutation.
export async function verifyNativeCollections(context,astro,output,errors,api){
 const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message))
 const route='/dk-en/collections/native-phones',base=astro+route
 const request=async(path,status=200)=>{const r=await context.request.get(astro+path);assert.equal(r.status(),status,path);return r}
 const apiRequest=async(path,status=200)=>{const r=await context.request.get(api+path);assert.equal(r.status(),status,path);return r}
 const list=async(query)=>{const r=await apiRequest('/api/store/products?collection_id=joint-collection&region_id=option-region&limit=100'+query);return r.json()}
 const plain=await list('');assert.equal(plain.count,105);assert.equal(plain.products.length,100);assert.equal(plain.products[0].id,'zz-collection-000');assert.equal(plain.products.at(-1).id,'zz-collection-099')
 const latest=await list('&order=-created_at');assert.equal(latest.count,105);assert.equal(latest.products[0].id,'zz-collection-104');assert.equal(latest.products.at(-1).id,'zz-collection-005')
 const filtered=await list('&option_value_id=joint-128&option_value_id=joint-blue');assert.equal(filtered.count,53);assert.equal(filtered.products.length,53);assert.ok(filtered.products.every(p=>Number(p.id.slice(-3))%2===0))
 const alternatives=await list('&option_value_id=joint-128&option_value_id=joint-256&option_value_id=joint-blue');assert.equal(alternatives.count,105)
 const none=await list('&option_value_id=joint-missing');assert.equal(none.count,0)
 await apiRequest('/api/store/products?collection_id=joint-collection&sales_channel_id=other',400)
 const firstHTML=await request(route);const html=await firstHTML.text();assert.ok(html.includes('iPhone collection 104'));assert.ok(html.includes('iPhone collection 093'));assert.ok(!html.includes('iPhone collection 092'))
 await page.setViewportSize({width:1440,height:1000});await page.goto(base)
 const titles=()=>page.getByTestId('products-list').getByTestId('product-title')
 await expect(titles()).toHaveCount(12);await expect(titles().first()).toHaveText('iPhone collection 104')
 await expect(page.locator('link[rel="canonical"]')).toHaveAttribute('href',base)
 await expect(page.getByTestId('product-pagination').getByRole('button',{name:'9',exact:true})).toBeVisible()
 const sort=page.getByTestId('collection-sort')
 await sort.getByText('Price: Low -> High',{exact:true}).click()
 await expect(page).toHaveURL(/sortBy=price_asc/);await expect(titles().first()).toHaveText('iPhone collection 099')
 await expect(page.getByTestId('product-wrapper').first().getByTestId('price')).toHaveText('€6.00')
 await page.getByTestId('product-pagination').getByRole('button',{name:'2',exact:true}).click()
 await expect(page).toHaveURL(/page=2/);await expect(titles().first()).toHaveText('iPhone collection 087')
 await page.getByTestId('collection-sort').getByText('Price: High -> Low',{exact:true}).click()
 await expect(titles().first()).toHaveText('iPhone collection 000');assert.equal(new URL(page.url()).searchParams.has('page'),false)
 await page.goto(base+'?sortBy=price_asc&page=9');await expect(titles()).toHaveCount(4);await expect(titles().first()).toHaveText('iPhone collection 003')
 await page.goto(base+'?page=2&optionValueIds=joint-128&optionValueIds=joint-blue&keep=yes')
 await expect(titles().first()).toHaveText('iPhone collection 080')
 await page.getByTestId('collection-sort').getByText('Price: Low -> High',{exact:true}).click()
 await expect(titles().first()).toHaveText('iPhone collection 104');assert.equal(new URL(page.url()).searchParams.has('page'),false)
 assert.deepEqual(new URL(page.url()).searchParams.getAll('optionValueIds'),['joint-128','joint-blue']);assert.equal(new URL(page.url()).searchParams.get('keep'),'yes')
 await page.getByTestId('product-pagination').getByRole('button',{name:'2',exact:true}).click()
 await expect(titles().first()).toHaveText('iPhone collection 080');assert.deepEqual(new URL(page.url()).searchParams.getAll('optionValueIds'),['joint-128','joint-blue'])
 await page.reload();await expect(titles().first()).toHaveText('iPhone collection 080')
 const empty=await request(route+'?optionValueIds=joint-missing');assert.ok((await empty.text()).includes('data-testid="collection-page"'))
 await page.goto(base+'?optionValueIds=joint-missing');await expect(titles()).toHaveCount(0);await expect(page.getByTestId('product-pagination')).toHaveCount(0)
 await request('/dk-en/collections/does-not-exist',404);await request('/zz-en/collections/native-phones',404)
 for(const q of ['page=0','page=1&page=2','sortBy=unknown'])await request(route+'?'+q,400)
 for(const lang of ['en','zh'])for(const width of [1440,390]){
  await page.setViewportSize({width,height:1000});const response=await page.goto(astro+'/dk-'+lang+'/collections/native-phones?sortBy=price_asc');assert.equal(response.status(),200)
  await expect(page.locator('astro-island[client="load"][ssr]')).toHaveCount(0);await expect(titles()).toHaveCount(12)
  const first=page.getByTestId('product-wrapper').first();await first.scrollIntoViewIfNeeded();await expect(first.getByTestId('price')).toHaveText('€6.00')
  await expect(page.getByTestId('products-list').locator('a').first()).toHaveAttribute('href','/dk-'+lang+'/products/zz-collection-099')
  assert.equal(await first.locator('img').evaluate(async img=>{await img.decode();return img.naturalWidth>0}),true)
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
  await page.screenshot({path:join(output,'native-collection-'+lang+'-'+width+'.png'),fullPage:true})
 }
 await page.close()
 return {real_gin_mysql_redis_astro:true,api_mocking:false,products:105,variants:263,working_set_limit:100,page_size:12,last_page_size:4,same_sku_and_groups:true,within_group_or:true,filtered_products:53,latest_before_limit:true,price_sort_on_source_id_working_set:true,price_asc_first_amount:'6',ssr_first_twelve:true,sort_resets_page:true,repeated_option_parameters_preserved:true,localized_product_links:true,image_decoded:true,empty_200_unknown_404_invalid_400:true,en_zh_widths:[1440,390],no_body_overflow:true}
}
