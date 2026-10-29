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
 const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message))
 await page.goto(origin+'/login');await page.getByLabel('Email',{exact:true}).fill('browser@example.invalid');await page.getByLabel('Password',{exact:true}).fill('artificial-browser-password');await page.getByRole('button',{name:'Sign in',exact:true}).click();await expect(page.getByRole('button',{name:'Sign out',exact:true})).toBeVisible()
 await page.goto(origin+'/products/joint-device-product');await expect(page.getByRole('cell',{name:'DEVICE-JOINT',exact:true})).toBeVisible();await page.getByRole('button',{name:'Device tracking',exact:true}).click()
 const drawer=page.getByRole('dialog');await expect(drawer.getByText('Disabled',{exact:true})).toBeVisible();await drawer.getByRole('button',{name:'Enable individual device tracking',exact:true}).click();await expect(drawer.getByText('Enabled',{exact:true})).toBeVisible();await expect(drawer.getByRole('button',{name:'Enable individual device tracking',exact:true})).toHaveCount(0)
 await page.reload();await page.getByRole('button',{name:'Device tracking',exact:true}).click();await expect(drawer.getByText('Enabled',{exact:true})).toBeVisible()
 const response=await context.request.get(origin+'/api/admin/products/joint-device-product/variants/joint-device-variant/device-tracking');assert.equal(response.status(),200);const data=await response.json();assert.equal(data.enabled,true);assert.equal(data.inventory_item_id,'joint-warehouse-item')
 await page.setViewportSize({width:390,height:844});await page.reload();await page.getByRole('button',{name:'Device tracking',exact:true}).click();await expect(drawer.getByText('Enabled',{exact:true})).toBeVisible();assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
 await context.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','zh-CN'));await page.reload();await page.getByRole('button',{name:'设备追踪',exact:true}).click();await expect(drawer.getByText('已开启',{exact:true})).toBeVisible();assert.deepEqual(errors,[])
 console.log('DEVICE_TRACKING_JOINT '+JSON.stringify({real_browser_react_gin_mysql_redis:true,api_mocking:false,real_login:true,product_sku_entry:true,enrolled_and_reloaded:true,tracking_projection_verified:true,language_en_zh:true,mobile_no_page_overflow:true,browser_errors:0}))
}finally{await browser?.close();await server?.close()}
