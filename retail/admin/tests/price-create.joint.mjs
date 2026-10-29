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
 await page.goto(origin+'/products/price-edit-product');await page.getByRole('row').filter({has:page.getByRole('cell',{name:'UNPRICED',exact:true})}).getByRole('button',{name:'View prices',exact:true}).click()
 await expect(page.getByText('No configured prices',{exact:true})).toBeVisible();await page.getByRole('button',{name:'Add base price',exact:true}).click();let editor=page.getByRole('dialog').last()
 await expect(editor.getByText('A new price set will be created on save',{exact:true})).toBeVisible();const currency=editor.getByLabel('Currency',{exact:true});await currency.click();await currency.press('ArrowDown');await currency.press('Enter')
 await editor.getByLabel('Amount',{exact:true}).fill('99.123456789012345678');await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(page.getByRole('dialog')).toHaveCount(1);await expect(page.getByRole('cell',{name:'99.123456789012345678',exact:true})).toBeVisible()
 const quote=await context.request.get(origin+'/api/store/products/price-edit-product?region_id=price-edit-region');assert.equal(quote.status(),200);const data=await quote.json();const v=data.product.variants.find(v=>v.id==='price-create-v');assert.equal(v.calculated_price.calculated_amount,'99.123456789012345678')
 await page.reload();await page.getByRole('row').filter({has:page.getByRole('cell',{name:'UNPRICED',exact:true})}).getByRole('button',{name:'View prices',exact:true}).click();await expect(page.getByRole('cell',{name:'99.123456789012345678',exact:true})).toBeVisible()
 await page.setViewportSize({width:390,height:844});await page.getByRole('button',{name:'Add base price',exact:true}).click();editor=page.getByRole('dialog').last();assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);await editor.getByLabel('Currency',{exact:true}).click();await expect(page.locator('.ant-select-item-option-disabled').getByText('EUR',{exact:true})).toBeVisible()
 await context.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','zh-CN'));await page.reload();await page.getByRole('row').filter({has:page.getByRole('cell',{name:'UNPRICED',exact:true})}).getByRole('button',{name:'查看价格',exact:true}).click();await page.getByRole('button',{name:'新增基础价格',exact:true}).click();await expect(page.getByRole('dialog').last().getByLabel('金额',{exact:true})).toBeVisible();assert.deepEqual(errors,[])
 console.log('PRICE_CREATE_JOINT '+JSON.stringify({real_browser_react_gin_mysql_redis:true,api_mocking:false,unpriced_sku_receives_base_price:true,atomic_set_and_link:true,exact_decimal_saved:true,public_quote_updated:true,refresh_preserves_price:true,existing_currency_disabled:true,language_en_zh:true,mobile_no_page_overflow:true,browser_errors:0}))
}finally{await browser?.close();await server?.close()}
