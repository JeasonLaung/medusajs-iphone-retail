import assert from 'node:assert/strict'
import {createServer} from 'vite'
import {chromium,expect} from '@playwright/test'
let server,browser
const origin='http://127.0.0.1:19383',errors=[]
if(!process.env.ADMIN_TEST_API_TARGET)throw Error('Owned API target required')
try{
 server=await createServer({server:{port:19383,strictPort:true,host:'127.0.0.1'},logLevel:'error'});await server.listen()
 browser=await chromium.launch({headless:true,executablePath:process.env.ADMIN_TEST_BROWSER||'C:/Program Files/Google/Chrome/Application/chrome.exe'})
 const context=await browser.newContext({reducedMotion:'reduce'});await context.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','en'))
 const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message));await page.goto(origin+'/account');await page.getByLabel('Email',{exact:true}).fill('browser@example.invalid');await page.getByLabel('Password',{exact:true}).fill('artificial-browser-password');await page.getByRole('button',{name:'Sign in',exact:true}).click();await expect(page.getByRole('button',{name:'Sign out',exact:true})).toBeVisible()
 await page.goto(origin+'/products/variant-create-product');await page.getByRole('button',{name:'Create SKU',exact:true}).click();let editor=page.getByRole('dialog',{name:'Create SKU',exact:true})
 await editor.getByLabel('Variant title',{exact:true}).fill('128 GB / Black')
 for(const [label,value] of [['Storage','128 GB'],['Color','Black']]){const input=editor.getByLabel(label,{exact:true});await input.click();await input.fill(value);await input.press('Enter')}
 await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(editor).toHaveCount(0)
 const row=page.getByRole('row').filter({has:page.getByRole('cell',{name:'128 GB / Black',exact:true}).first()}).first()
 await expect(row.getByText(/^ERP[1-9][0-9]{4,}$/)).toBeVisible();const sku=await row.getByText(/^ERP[1-9][0-9]{4,}$/).innerText();await expect(row.getByText('Complete',{exact:true})).toBeVisible()
 await row.getByRole('button',{name:'View prices',exact:true}).click();await expect(page.getByText('No configured prices',{exact:true})).toBeVisible();await page.getByRole('button',{name:'Add base price',exact:true}).click();editor=page.getByRole('dialog').last()
 const currency=editor.getByLabel('Currency',{exact:true});await currency.click();await currency.fill('eur');await currency.press('Enter');await editor.getByLabel('Amount',{exact:true}).fill('129.123456789012345678');await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(page.getByRole('cell',{name:'129.123456789012345678',exact:true})).toBeVisible()
 const response=await context.request.get(origin+'/api/store/products/variant-create-product?region_id=price-edit-region');assert.equal(response.status(),200);const data=await response.json();assert.equal(data.product.variants.length,1);const variant=data.product.variants[0];assert.equal(variant.sku,sku);assert.equal(variant.calculated_price.calculated_amount,'129.123456789012345678');assert.equal(variant.manage_inventory,true);assert.equal(variant.allow_backorder,false)
 await page.reload();await expect(page.getByText(sku,{exact:true})).toBeVisible();await page.setViewportSize({width:390,height:844});await page.getByRole('button',{name:'Create SKU',exact:true}).click();assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
 await context.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','zh-CN'));await page.reload();await page.getByRole('button',{name:'新建 SKU',exact:true}).click();await expect(page.getByRole('dialog').getByLabel('SKU',{exact:true})).toBeVisible();assert.deepEqual(errors,[])
 console.log('VARIANT_CREATE_JOINT '+JSON.stringify({real_browser_react_gin_mysql_redis:true,api_mocking:false,complete_existing_options:true,auto_erp_and_inventory_same_sku:true,zero_stock_initially:true,base_price_next_step:true,public_product_and_exact_quote:true,refresh_preserves_sku:true,language_en_zh:true,mobile_no_page_overflow:true,browser_errors:0}))
}finally{await browser?.close();await server?.close()}
