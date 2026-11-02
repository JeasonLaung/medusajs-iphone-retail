import assert from 'node:assert/strict'
import {verifyWarehouseDeviceEdit} from './local-warehouse-edit.live.mjs'
import {verifyWarehouseInspection} from './local-warehouse-inspection.live.mjs'
import {verifyCatalogOperations} from './local-catalog-operations.live.mjs'
import {randomUUID} from 'node:crypto'
import {readFileSync,writeFileSync} from 'node:fs'
import {resolve} from 'node:path'
import {chromium,expect} from '../../admin/node_modules/@playwright/test/index.mjs'
const origin='http://127.0.0.1:19200',logs=process.env.ACCOUNT_TEST_LOGS,deviceEntry=['device-entry','warehouse-inspection','warehouse-edit'].includes(process.env.ACCOUNT_TEST_SCENARIO),stockQuantity=deviceEntry?1:3
const catalogOperations=process.env.ACCOUNT_TEST_SCENARIO==='catalog-operations'
let deviceFacts={},catalogFacts={}
if(!logs||!['product-entry','device-entry','catalog-operations','warehouse-inspection','warehouse-edit'].includes(process.env.ACCOUNT_TEST_SCENARIO))throw Error('Owned product-entry runner required')
const credentials=readFileSync(resolve('../../.private-files/local/admin-credentials.txt'),'utf8')
const credential=name=>credentials.split(/\r?\n/).find(line=>new RegExp('^'+name+'\\s*[:=]','i').test(line))?.replace(/^[^:=]+[:=]\s*/,'').trim()
const errors=[],browser=await chromium.launch({headless:true,executablePath:resolve(process.env.LOCALAPPDATA,'ms-playwright/chromium-1234/chrome-win64/chrome.exe')})
let page
try{
 const context=await browser.newContext({reducedMotion:'reduce'});await context.addInitScript(()=>{if(!localStorage.getItem('eurorefurb.admin.language'))localStorage.setItem('eurorefurb.admin.language','en')})
 page=await context.newPage();page.setDefaultTimeout(20000);page.on('pageerror',e=>errors.push(e.message))
 await page.goto(origin+'/account');await page.getByLabel('Email',{exact:true}).fill(credential('Email'));await page.getByLabel('Password',{exact:true}).fill(credential('Password'));await page.getByRole('button',{name:'Sign in',exact:true}).click();await expect(page.getByRole('button',{name:'Sign out',exact:true})).toBeVisible()
 await page.goto(origin+'/products');await page.getByRole('button',{name:'Create product',exact:true}).click()
 let editor=page.getByRole('dialog',{name:'Create product',exact:true});const handle='native-browser-'+randomUUID(),title=(deviceEntry?'iPhone native 入库 ':'Native 商品 ')+randomUUID().slice(0,8)
 await editor.getByLabel('Product',{exact:true}).fill(title);await editor.getByLabel('Handle',{exact:true}).fill(handle);const status=editor.getByLabel('Status',{exact:true});await status.focus();await status.press('ArrowDown');await page.locator('.ant-select-dropdown:not(.ant-select-dropdown-hidden)').getByText('Published',{exact:true}).click()
 for(const label of ['Sales channels','Shipping profile']){const input=editor.getByLabel(label,{exact:true});if(await editor.locator('.ant-form-item').filter({has:input}).locator('.ant-select-selection-item').count()===0){await input.focus();await input.press('ArrowDown');await input.press('Enter');await input.press('Tab')}}
 await editor.getByRole('button',{name:'Add option group',exact:true}).click();await editor.getByLabel('Option title',{exact:true}).fill('Storage');const vals=editor.getByLabel('Option values',{exact:true});await vals.fill('128 GB,256 GB,');await vals.press('Tab')
 await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(editor).toHaveCount(0);await expect(page).toHaveURL(/\/products\/prod_/)
 const productID=decodeURIComponent(new URL(page.url()).pathname.split('/').at(-1));await expect(page.getByRole('heading',{name:title,exact:true})).toBeVisible()
 await page.getByRole('button',{name:'Create SKU',exact:true}).click();editor=page.getByRole('dialog',{name:'Create SKU',exact:true});await editor.getByLabel('Variant title',{exact:true}).fill('128 GB');const storage=editor.getByLabel('Storage',{exact:true});await storage.click();await storage.fill('128 GB');await storage.press('Enter');await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(editor).toHaveCount(0)
 const row=page.getByRole('row').filter({has:page.getByRole('cell',{name:'128 GB',exact:true}).first()}).first();await expect(row.getByText(/^ERP[1-9][0-9]{4,}$/)).toBeVisible();const sku=await row.getByText(/^ERP[1-9][0-9]{4,}$/).innerText()
 await row.getByRole('button',{name:'View prices',exact:true}).click();await page.getByRole('button',{name:'Add base price',exact:true}).click();editor=page.getByRole('dialog').last();const currency=editor.getByLabel('Currency',{exact:true});await currency.click();await currency.fill('eur');await currency.press('Enter');await editor.getByLabel('Amount',{exact:true}).fill('129.123456789012345678');await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(page.getByRole('cell',{name:'129.123456789012345678',exact:true})).toBeVisible()
 const regionResponse=await context.request.get(origin+'/api/store/regions');assert.equal(regionResponse.status(),200);const regions=await regionResponse.json();const region=regions.regions.find(r=>r.currency_code==='eur');assert.ok(region);const publicResponse=await context.request.get(origin+'/api/store/products/'+productID+'?region_id='+region.id);assert.equal(publicResponse.status(),200);const publicData=await publicResponse.json();assert.equal(publicData.product.handle,handle);assert.equal(publicData.product.variants[0].sku,sku);assert.equal(publicData.product.variants[0].calculated_price.calculated_amount,'129.123456789012345678');await page.reload();await expect(page.getByText(sku,{exact:true})).toBeVisible()

 if(deviceEntry){
  const variantID=publicData.product.variants[0].id,sourceName='Native intake '+process.env.ACCOUNT_TEST_OWNER
  await page.goto(origin+'/warehouses');await page.getByRole('button',{name:'Device sources',exact:true}).click()
  let sourceDialog=page.getByRole('dialog');await sourceDialog.getByRole('button',{name:'New device source',exact:true}).click()
  await sourceDialog.getByLabel('Name',{exact:true}).fill(sourceName);await sourceDialog.getByRole('button',{name:'Save',exact:true}).click()
  await expect(sourceDialog.getByRole('cell',{name:sourceName,exact:true})).toBeVisible()
  await page.goto(origin+'/products/'+productID);await page.getByRole('button',{name:'Device tracking',exact:true}).click()
  const tracking=page.getByRole('dialog');await tracking.getByRole('button',{name:'Enable individual device tracking',exact:true}).click()
  await expect(tracking.getByText('Enabled',{exact:true})).toBeVisible();await tracking.getByRole('button',{name:'Device intake',exact:true}).click()
  const intake=page.getByRole('dialog',{name:'Device intake',exact:true})
  const png=Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jB9kAAAAASUVORK5CYII=','base64')
  await intake.locator('input[type=file]').first().setInputFiles({name:'native-phone.png',mimeType:'image/png',buffer:png})
  await expect(intake.locator('.ant-upload-list-item')).toHaveCount(1)
  const photoLink=await intake.locator('.ant-upload-list-item-name').getAttribute('href');assert.match(photoLink,/^\/api\/store\/device-photos\/dphoto_/)
  const image=await context.request.get(origin+photoLink);assert.equal(image.status(),200);assert.equal(image.headers()['content-type'],'image/jpeg');assert.equal((await image.body()).subarray(0,3).toString('hex'),'ffd8ff')
  await intake.getByLabel('Own warehouse',{exact:true}).press('ArrowDown');await page.getByText(sourceName+' ('+sourceName+')',{exact:true}).click()
  await intake.getByLabel('SN',{exact:true}).fill('native-intake-sale');await intake.getByLabel('IMEI',{exact:true}).fill('000000000000081')
  await intake.getByRole('button',{name:'Add device',exact:true}).click()
  const secondSource=intake.getByLabel('Own warehouse',{exact:true}).nth(1);await secondSource.scrollIntoViewIfNeeded();await secondSource.press('ArrowDown');await page.getByText(sourceName+' ('+sourceName+')',{exact:true}).last().click()
  const category=intake.getByLabel('Device category',{exact:true}).nth(1);await category.scrollIntoViewIfNeeded();await category.press('ArrowDown');await page.getByText('Returned',{exact:true}).last().click()
  await intake.getByLabel('SN',{exact:true}).nth(1).fill('native-intake-return');await intake.getByLabel('IMEI',{exact:true}).nth(1).fill('000000000000082')
  let lost=false;const commands=[]
  await page.route('**/api/admin/products/*/variants/*/device-intake',async route=>{
   if(route.request().method()!=='POST')return route.continue()
   commands.push(route.request().postDataJSON());const response=await route.fetch()
   assert.equal(response.status(),200)
   if(!lost){lost=true;await route.abort('failed')}else await route.fulfill({response})
  })
  await intake.getByRole('button',{name:'Submit intake batch',exact:true}).click()
  await intake.getByRole('button',{name:'Retry this save',exact:true}).click()
  await expect(intake.getByText('Devices registered',{exact:true})).toBeVisible();assert.equal(commands.length,2);assert.deepEqual(commands[0],commands[1])
  await expect(intake.getByRole('cell',{name:'NATIVE-INTAKE-SALE',exact:true})).toBeVisible();await expect(intake.getByRole('cell',{name:'NATIVE-INTAKE-RETURN',exact:true})).toBeVisible()
  await expect(intake.getByRole('cell',{name:'Available',exact:true})).toBeVisible();await expect(intake.getByRole('cell',{name:'Inspection',exact:true})).toBeVisible()
  await page.reload();await page.getByRole('button',{name:'Device tracking',exact:true}).click();await page.getByRole('dialog').getByRole('button',{name:'Device intake',exact:true}).click()
  await expect(page.getByRole('dialog',{name:'Device intake',exact:true}).locator('img[src="'+photoLink+'"]')).toBeVisible()
  await page.setViewportSize({width:390,height:844});assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
  await page.screenshot({path:resolve(logs,'device-intake-en.png'),fullPage:true})
  await page.evaluate(()=>localStorage.setItem('eurorefurb.admin.language','zh'));await page.reload();await page.getByRole('button',{name:'设备追踪',exact:true}).click();await page.getByRole('dialog').getByRole('button',{name:'设备入库',exact:true}).click()
  await expect(page.getByRole('dialog').last().getByRole('cell',{name:'NATIVE-INTAKE-SALE',exact:true})).toBeVisible();assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
  await page.screenshot({path:resolve(logs,'device-intake-zh.png'),fullPage:true})
  await page.evaluate(()=>localStorage.setItem('eurorefurb.admin.language','en'));await page.setViewportSize({width:1440,height:1000});await page.goto(origin+'/products/'+productID)
  deviceFacts={variant_id:variantID,source_name:sourceName,source_auto_warehouse:true,tracking_enrolled:true,photo_uploaded_and_jpeg_read:true,photos_survive_reload:true,sale_available_return_inspection:true,leading_zero_imei:true,intake_lost_reply_same_command:true}
 }

 await row.getByRole('button',{name:'View inventory',exact:true}).click()
 await page.locator('.ant-collapse-header').first().click()
 await page.getByRole('button',{name:'Add warehouse stock',exact:true}).click()
 editor=page.getByRole('dialog',{name:'Add warehouse stock',exact:true})
 const location=editor.getByLabel('Location',{exact:true});await location.click();await page.locator('.ant-select-dropdown:not(.ant-select-dropdown-hidden)').getByText(deviceEntry?new RegExp(deviceFacts.source_name):/European/).click()
 await editor.getByLabel('Stocked quantity',{exact:true}).fill(String(stockQuantity))
 await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(editor).toHaveCount(0)
 const stocked=await context.request.get(origin+'/api/store/products/'+productID+'?region_id='+region.id);const stockedData=(await stocked.json()).product
 assert.equal(String(stockedData.variants[0].inventory_quantity),String(stockQuantity))
 if(process.env.ACCOUNT_TEST_SCENARIO==='warehouse-edit')catalogFacts=await verifyWarehouseDeviceEdit({browser,admin:page,origin,owner:process.env.ACCOUNT_TEST_OWNER,sourceName:deviceFacts.source_name,errors,logs,sku,productID,variantID:stockedData.variants[0].id})
 if(process.env.ACCOUNT_TEST_SCENARIO==='warehouse-inspection')catalogFacts=await verifyWarehouseInspection({browser,admin:page,origin,owner:process.env.ACCOUNT_TEST_OWNER,sourceName:deviceFacts.source_name,errors,logs,sku,productID,variantID:stockedData.variants[0].id})
 if(catalogOperations)catalogFacts=await verifyCatalogOperations({page,context,origin,productID,variantID:stockedData.variants[0].id,region,sku})
 const shop=await context.newPage();shop.on('pageerror',e=>errors.push(e.message))
 for(const language of ['en','zh']){
  await shop.setViewportSize({width:390,height:844});await shop.goto('http://127.0.0.1:19300/dk-'+language+'/products/'+handle,{waitUntil:'domcontentloaded'})
  await expect(shop.getByTestId('product-title')).toHaveText(title);await expect(shop.getByTestId('add-product-button')).toBeEnabled()
  if(catalogOperations){await expect(shop.locator('img[src="'+catalogFacts.image_url+'"]').first()).toBeVisible();await shop.waitForFunction(url=>[...document.images].some(i=>i.getAttribute('src')===url&&i.complete&&i.naturalWidth===2),catalogFacts.image_url)}
  assert.equal(await shop.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
  await shop.screenshot({path:resolve(logs,'product-entry-'+language+'.png'),fullPage:true})
 }
 assert.deepEqual(errors,[])
 writeFileSync(resolve(logs,'browser-result.json'),JSON.stringify({passed:true,scenario:process.env.ACCOUNT_TEST_SCENARIO,...deviceFacts,product_id:productID,handle,generated_sku:sku,actual_admin_product_sku_price_stock:true,exact_price:'129.123456789012345678',stocked_quantity:stockQuantity,public_inventory_quantity:stockQuantity,...catalogFacts,astro_en_zh_purchase_enabled:true,mobile_overflow:false,browser_errors:0,real_payment_created:false},null,2))
}catch(error){console.error('VISIBLE_DIALOGS',await page?.getByRole('dialog').allTextContents());throw error}finally{await browser.close()}
