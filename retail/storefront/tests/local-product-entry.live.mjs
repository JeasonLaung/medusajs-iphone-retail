import assert from 'node:assert/strict'
import {randomUUID} from 'node:crypto'
import {readFileSync,writeFileSync} from 'node:fs'
import {resolve} from 'node:path'
import {chromium,expect} from '../../admin/node_modules/@playwright/test/index.mjs'
const origin='http://127.0.0.1:19200',logs=process.env.ACCOUNT_TEST_LOGS
if(!logs||process.env.ACCOUNT_TEST_SCENARIO!=='product-entry')throw Error('Owned product-entry runner required')
const credentials=readFileSync(resolve('../../.private-files/local/admin-credentials.txt'),'utf8')
const credential=name=>credentials.split(/\r?\n/).find(line=>new RegExp('^'+name+'\\s*[:=]','i').test(line))?.replace(/^[^:=]+[:=]\s*/,'').trim()
const errors=[],browser=await chromium.launch({headless:true,executablePath:resolve(process.env.LOCALAPPDATA,'ms-playwright/chromium-1234/chrome-win64/chrome.exe')})
let page
try{
 const context=await browser.newContext({reducedMotion:'reduce'});await context.addInitScript(()=>{if(!localStorage.getItem('eurorefurb.admin.language'))localStorage.setItem('eurorefurb.admin.language','en')})
 page=await context.newPage();page.setDefaultTimeout(20000);page.on('pageerror',e=>errors.push(e.message))
 await page.goto(origin+'/account');await page.getByLabel('Email',{exact:true}).fill(credential('Email'));await page.getByLabel('Password',{exact:true}).fill(credential('Password'));await page.getByRole('button',{name:'Sign in',exact:true}).click();await expect(page.getByRole('button',{name:'Sign out',exact:true})).toBeVisible()
 await page.goto(origin+'/products');await page.getByRole('button',{name:'Create product',exact:true}).click()
 let editor=page.getByRole('dialog',{name:'Create product',exact:true});const handle='native-browser-'+randomUUID(),title='Native 商品 '+randomUUID().slice(0,8)
 await editor.getByLabel('Product',{exact:true}).fill(title);await editor.getByLabel('Handle',{exact:true}).fill(handle);const status=editor.getByLabel('Status',{exact:true});await status.focus();await status.press('ArrowDown');await page.locator('.ant-select-dropdown:not(.ant-select-dropdown-hidden)').getByText('Published',{exact:true}).click()
 for(const label of ['Sales channels','Shipping profile']){const input=editor.getByLabel(label,{exact:true});if(await editor.locator('.ant-form-item').filter({has:input}).locator('.ant-select-selection-item').count()===0){await input.focus();await input.press('ArrowDown');await input.press('Enter');await input.press('Tab')}}
 await editor.getByRole('button',{name:'Add option group',exact:true}).click();await editor.getByLabel('Option title',{exact:true}).fill('Storage');const vals=editor.getByLabel('Option values',{exact:true});await vals.fill('128 GB,256 GB,');await vals.press('Tab')
 await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(editor).toHaveCount(0);await expect(page).toHaveURL(/\/products\/prod_/)
 const productID=decodeURIComponent(new URL(page.url()).pathname.split('/').at(-1));await expect(page.getByRole('heading',{name:title,exact:true})).toBeVisible()
 await page.getByRole('button',{name:'Create SKU',exact:true}).click();editor=page.getByRole('dialog',{name:'Create SKU',exact:true});await editor.getByLabel('Variant title',{exact:true}).fill('128 GB');const storage=editor.getByLabel('Storage',{exact:true});await storage.click();await storage.fill('128 GB');await storage.press('Enter');await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(editor).toHaveCount(0)
 const row=page.getByRole('row').filter({has:page.getByRole('cell',{name:'128 GB',exact:true}).first()}).first();await expect(row.getByText(/^ERP[1-9][0-9]{4,}$/)).toBeVisible();const sku=await row.getByText(/^ERP[1-9][0-9]{4,}$/).innerText()
 await row.getByRole('button',{name:'View prices',exact:true}).click();await page.getByRole('button',{name:'Add base price',exact:true}).click();editor=page.getByRole('dialog').last();const currency=editor.getByLabel('Currency',{exact:true});await currency.click();await currency.fill('eur');await currency.press('Enter');await editor.getByLabel('Amount',{exact:true}).fill('129.123456789012345678');await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(page.getByRole('cell',{name:'129.123456789012345678',exact:true})).toBeVisible()
 const regionResponse=await context.request.get(origin+'/api/store/regions');assert.equal(regionResponse.status(),200);const regions=await regionResponse.json();const region=regions.regions.find(r=>r.currency_code==='eur');assert.ok(region);const publicResponse=await context.request.get(origin+'/api/store/products/'+productID+'?region_id='+region.id);assert.equal(publicResponse.status(),200);const publicData=await publicResponse.json();assert.equal(publicData.product.handle,handle);assert.equal(publicData.product.variants[0].sku,sku);assert.equal(publicData.product.variants[0].calculated_price.calculated_amount,'129.123456789012345678');await page.reload();await expect(page.getByText(sku,{exact:true})).toBeVisible()

 await row.getByRole('button',{name:'View inventory',exact:true}).click()
 await page.locator('.ant-collapse-header').first().click()
 await page.getByRole('button',{name:'Add warehouse stock',exact:true}).click()
 editor=page.getByRole('dialog',{name:'Add warehouse stock',exact:true})
 const location=editor.getByLabel('Location',{exact:true});await location.click();await page.locator('.ant-select-dropdown:not(.ant-select-dropdown-hidden)').getByText(/European/).click()
 await editor.getByLabel('Stocked quantity',{exact:true}).fill('3')
 await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(editor).toHaveCount(0)
 const stocked=await context.request.get(origin+'/api/store/products/'+productID+'?region_id='+region.id);const stockedData=(await stocked.json()).product
 assert.equal(String(stockedData.variants[0].inventory_quantity),'3')
 const shop=await context.newPage()
 for(const language of ['en','zh']){
  await shop.setViewportSize({width:390,height:844});await shop.goto('http://127.0.0.1:19300/dk-'+language+'/products/'+handle)
  await expect(shop.getByTestId('product-title')).toHaveText(title);await expect(shop.getByTestId('add-product-button')).toBeEnabled()
  assert.equal(await shop.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
  await shop.screenshot({path:resolve(logs,'product-entry-'+language+'.png'),fullPage:true})
 }
 assert.deepEqual(errors,[])
 writeFileSync(resolve(logs,'browser-result.json'),JSON.stringify({passed:true,scenario:'product-entry',product_id:productID,handle,generated_sku:sku,actual_admin_product_sku_price_stock:true,exact_price:'129.123456789012345678',stocked_quantity:3,public_inventory_quantity:3,astro_en_zh_purchase_enabled:true,mobile_overflow:false,browser_errors:0,real_payment_created:false},null,2))
}catch(error){console.error('VISIBLE_DIALOGS',await page?.getByRole('dialog').allTextContents());throw error}finally{await browser.close()}
