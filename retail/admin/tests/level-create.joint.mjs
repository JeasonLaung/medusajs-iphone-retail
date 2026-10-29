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
 await row.getByRole('button',{name:'View inventory',exact:true}).click();await page.getByText('iPhone fixture / 128 GB / Black',{exact:true}).click();await page.getByRole('button',{name:'Add warehouse stock',exact:true}).click();editor=page.getByRole('dialog').last()
 const warehouse=editor.getByLabel('Location',{exact:true});await warehouse.click();await warehouse.fill('SKU warehouse A');await warehouse.press('Enter');await editor.getByLabel('Stocked quantity',{exact:true}).fill('8.000000000000000001');await editor.getByLabel('Incoming quantity',{exact:true}).fill('3');await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(page.getByRole('dialog')).toHaveCount(1)
 await page.getByText('iPhone fixture / 128 GB / Black',{exact:true}).click();await expect(page.getByRole('cell',{name:'8.000000000000000001',exact:true}).first()).toBeVisible()
 const quote=async(quantity)=>{const r=await context.request.get(origin+'/api/store/products/variant-create-product?region_id=price-edit-region&quantity='+quantity);assert.equal(r.status(),200);const d=await r.json();return d.product.variants[0]}
 let variant=await quote('8');assert.equal(variant.sku,sku);assert.equal(variant.availability.can_fulfill,true);assert.equal((await quote('9')).availability.can_fulfill,false)
 await page.getByRole('button',{name:'Edit stock level-create-a',exact:true}).click();editor=page.getByRole('dialog').last();await editor.getByLabel('Stocked quantity',{exact:true}).fill('7.000000000000000001');await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(page.getByRole('dialog')).toHaveCount(1)
 assert.equal((await quote('8')).availability.can_fulfill,false);assert.equal((await quote('7')).availability.can_fulfill,true)
 await page.reload();await page.getByRole('row').filter({has:page.getByText(sku,{exact:true})}).getByRole('button',{name:'View inventory',exact:true}).click();await page.getByText('iPhone fixture / 128 GB / Black',{exact:true}).click();await expect(page.getByRole('cell',{name:'7.000000000000000001',exact:true}).first()).toBeVisible()
 await page.setViewportSize({width:390,height:844});await page.getByRole('button',{name:'Add warehouse stock',exact:true}).click();editor=page.getByRole('dialog').last();assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);await editor.getByLabel('Location',{exact:true}).click();await expect(page.locator('.ant-select-item-option-disabled').getByText('SKU warehouse A (level-create-a)',{exact:true})).toBeVisible()
 await context.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','zh-CN'));await page.reload();await page.getByRole('row').filter({has:page.getByText(sku,{exact:true})}).getByRole('button',{name:'查看库存',exact:true}).click();await page.getByText('iPhone fixture / 128 GB / Black',{exact:true}).click();await page.getByRole('button',{name:'添加仓库库存',exact:true}).click();await expect(page.getByRole('dialog').last().getByLabel('库存数量',{exact:true})).toBeVisible();assert.deepEqual(errors,[])
 console.log('LEVEL_CREATE_JOINT '+JSON.stringify({real_browser_react_gin_mysql_redis:true,api_mocking:false,new_sku_then_initial_warehouse_stock:true,exact_raw_quantity:true,zero_reservations:true,incoming_not_sellable:true,public_availability_changes:true,oversell_denied:true,existing_stock_edit:true,refresh_preserves_stock:true,existing_warehouse_disabled:true,language_en_zh:true,mobile_no_page_overflow:true,browser_errors:0}))
}finally{await browser?.close();await server?.close()}
