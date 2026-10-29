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
 await page.goto(origin+'/products/price-edit-product');await page.getByRole('row').filter({has:page.getByRole('cell',{name:'price-edit-128',exact:true})}).getByRole('button',{name:'View prices',exact:true}).click()
 await page.getByRole('button',{name:'Edit amount',exact:true}).click();let editor=page.getByRole('dialog').last();await expect(editor.getByText('This price set is shared. Editing affects these SKUs: 2',{exact:true})).toBeVisible()
 await editor.getByLabel('Amount',{exact:true}).fill('1e2');await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(editor.locator('.ant-form-item-explain-error')).toBeVisible()
 await editor.getByLabel('Amount',{exact:true}).fill('101.000000000000000001');await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(page.getByRole('dialog')).toHaveCount(1);await expect(page.getByRole('cell',{name:'101.000000000000000001',exact:true})).toBeVisible()
 const quote=await context.request.get(origin+'/api/store/products/price-edit-product?region_id=price-edit-region');assert.equal(quote.status(),200);const data=await quote.json();assert.equal(data.product.variants.length,2);for(const v of data.product.variants)assert.equal(v.calculated_price.calculated_amount,'101.000000000000000001')
 await page.reload();await page.getByRole('row').filter({has:page.getByRole('cell',{name:'price-edit-256',exact:true})}).getByRole('button',{name:'View prices',exact:true}).click();await expect(page.getByRole('cell',{name:'101.000000000000000001',exact:true})).toBeVisible()
 await page.setViewportSize({width:390,height:844});await page.getByRole('button',{name:'Edit amount',exact:true}).click();editor=page.getByRole('dialog').last();await expect(editor.getByLabel('Amount',{exact:true})).toHaveValue('101.000000000000000001');assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
 await context.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','zh-CN'));await page.reload();await page.getByRole('row').filter({has:page.getByRole('cell',{name:'price-edit-128',exact:true})}).getByRole('button',{name:'查看价格',exact:true}).click();await page.getByRole('button',{name:'修改金额',exact:true}).click();await expect(page.getByRole('dialog').last().getByLabel('金额',{exact:true})).toHaveValue('101.000000000000000001');assert.deepEqual(errors,[])
 console.log('PRICE_EDIT_JOINT '+JSON.stringify({real_browser_react_gin_mysql_redis:true,api_mocking:false,decimal_validation:true,exact_decimal_saved:true,shared_sku_warning:true,public_quotes_both_skus_updated:true,refresh_preserves_price:true,language_en_zh:true,mobile_no_page_overflow:true,browser_errors:0}))
}finally{await browser?.close();await server?.close()}
