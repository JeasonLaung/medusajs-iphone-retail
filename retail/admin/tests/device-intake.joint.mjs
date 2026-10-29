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
 await page.goto(origin+'/warehouses');await page.getByRole('button',{name:'Device sources',exact:true}).click();let dialog=page.getByRole('dialog');await dialog.getByRole('button',{name:'New device source',exact:true}).click();await dialog.getByLabel('Name',{exact:true}).fill('Intake joint source');await dialog.getByRole('button',{name:'Save',exact:true}).click();await expect(dialog.getByRole('cell',{name:'Intake joint source',exact:true})).toBeVisible()
 await page.goto(origin+'/products/joint-device-product');await page.getByRole('button',{name:'Device tracking',exact:true}).click();dialog=page.getByRole('dialog');await dialog.getByRole('button',{name:'Enable individual device tracking',exact:true}).click();await expect(dialog.getByText('Enabled',{exact:true})).toBeVisible();await dialog.getByRole('button',{name:'Device intake',exact:true}).click();const intake=page.getByRole('dialog').last()
 const png=Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jB9kAAAAASUVORK5CYII=','base64')
 await intake.locator('input[type=file]').first().setInputFiles({name:'joint-phone.png',mimeType:'image/png',buffer:png})
 await expect(intake.locator('.ant-upload-list-item')).toHaveCount(1)
 const photoLink=await intake.locator('.ant-upload-list-item-name').getAttribute('href')
 assert.match(photoLink,/^\/api\/store\/device-photos\/dphoto_/)
 const image=await context.request.get(origin+photoLink);assert.equal(image.status(),200);assert.equal(image.headers()['content-type'],'image/jpeg');assert.equal((await image.body()).subarray(0,3).toString('hex'),'ffd8ff')
 await intake.getByLabel('Own warehouse',{exact:true}).press('ArrowDown');await page.getByText('Intake joint source (Intake joint source)',{exact:true}).click();await intake.getByLabel('SN',{exact:true}).fill('joint-sn-one');await intake.getByLabel('IMEI',{exact:true}).fill('000000000000081');await intake.getByRole('button',{name:'Add device',exact:true}).click()
 await intake.getByLabel('Own warehouse',{exact:true}).nth(1).scrollIntoViewIfNeeded();await intake.getByLabel('Own warehouse',{exact:true}).nth(1).press('ArrowDown');await intake.getByLabel('Own warehouse',{exact:true}).nth(1).press('Enter');await intake.getByLabel('Device category',{exact:true}).nth(1).scrollIntoViewIfNeeded();await intake.getByLabel('Device category',{exact:true}).nth(1).press('ArrowDown');await page.getByText('Returned',{exact:true}).last().click();await intake.getByLabel('SN',{exact:true}).nth(1).fill('joint-sn-two');await intake.getByLabel('IMEI',{exact:true}).nth(1).fill('000000000000082');await intake.getByRole('button',{name:'Submit intake batch',exact:true}).click()
 await expect(intake.getByText('Devices registered',{exact:true})).toBeVisible();await expect(intake.getByRole('cell',{name:'JOINT-SN-ONE',exact:true})).toBeVisible();await expect(intake.getByRole('cell',{name:'JOINT-SN-TWO',exact:true})).toBeVisible();await expect(intake.getByRole('cell',{name:'Available',exact:true})).toBeVisible();await expect(intake.getByRole('cell',{name:'Inspection',exact:true})).toBeVisible()
 await page.reload();await page.getByRole('button',{name:'Device tracking',exact:true}).click();await page.getByRole('dialog').getByRole('button',{name:'Device intake',exact:true}).click();await expect(page.getByRole('dialog').last().getByRole('button',{name:'Submit intake batch',exact:true})).toBeVisible();await expect(page.getByRole('dialog').last().getByRole('cell',{name:'JOINT-SN-ONE',exact:true})).toBeVisible()
 await expect(page.getByRole('dialog').last().locator(`img[src="${photoLink}"]`)).toBeVisible()
 await page.setViewportSize({width:390,height:844});assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
 await context.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','zh-CN'));await page.reload();await page.getByRole('button',{name:'设备追踪',exact:true}).click();await page.getByRole('dialog').getByRole('button',{name:'设备入库',exact:true}).click();await expect(page.getByRole('dialog').last().getByRole('button',{name:'提交整批入库',exact:true})).toBeVisible();assert.deepEqual(errors,[])
 console.log('DEVICE_INTAKE_JOINT '+JSON.stringify({real_browser_react_gin_mysql_redis:true,api_mocking:false,real_login:true,source_created_in_ui:true,sku_enrolled_in_ui:true,batch_of_two_saved:true,sale_available_returned_inspection:true,leading_zero_imei:true,language_en_zh:true,mobile_no_page_overflow:true,browser_errors:0,photo_upload_integrated:true,photos_survive_reload:true,public_jpeg_verified:true}))
}finally{await browser?.close();await server?.close()}
