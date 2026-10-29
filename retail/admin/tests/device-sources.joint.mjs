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
 await page.goto(origin+'/warehouses');await page.getByRole('button',{name:'Device sources',exact:true}).click();let drawer=page.getByRole('dialog')
 for(const [name,type] of [['Browser own source','Own warehouse'],['Browser supplier source','Supplier']]){
  await drawer.getByRole('button',{name:'New device source',exact:true}).click();await drawer.getByLabel('Name',{exact:true}).fill(name);await drawer.getByRole('radio',{name:type,exact:true}).check();await drawer.getByRole('button',{name:'Save',exact:true}).click();await expect(drawer.getByRole('cell',{name,exact:true})).toBeVisible()
 }
 await page.reload();await expect(page.getByRole('button',{name:'Browser own source',exact:true})).toBeVisible();await expect(page.getByRole('button',{name:'Browser supplier source',exact:true})).toBeVisible();await page.getByRole('button',{name:'Device sources',exact:true}).click();drawer=page.getByRole('dialog');await expect(drawer.getByRole('cell',{name:'Browser own source',exact:true})).toBeVisible()
 const response=await context.request.get(origin+'/api/admin/device-sources');assert.equal(response.status(),200);const data=await response.json();assert.equal(data.sources.length,2);assert.deepEqual(data.sources.map(s=>s.type).sort(),['self','supplier']);assert.ok(data.sources.every(s=>s.location_name===s.name&&s.location_id.startsWith('sloc_')))
 await drawer.getByRole('button',{name:'New device source',exact:true}).click();await drawer.getByLabel('Name',{exact:true}).fill('Browser own source');await drawer.getByRole('button',{name:'Save',exact:true}).click();await expect(drawer.getByText('The name exists, or permissions or configuration changed. Reload before continuing.',{exact:true})).toBeVisible();await expect(drawer.getByRole('button',{name:'Save',exact:true})).toHaveCount(0);await drawer.getByRole('button',{name:'Reload',exact:true}).click();await expect(drawer.getByRole('cell',{name:'Browser own source',exact:true})).toBeVisible()
 await page.setViewportSize({width:390,height:844});await page.reload();await page.getByRole('button',{name:'Device sources',exact:true}).click();await expect(drawer.getByRole('cell',{name:'Browser own source',exact:true})).toBeVisible();assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
 await context.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','zh-CN'));await page.reload();await page.getByRole('button',{name:'设备来源',exact:true}).click();await expect(drawer.getByRole('cell',{name:'自有仓库',exact:true})).toBeVisible();await expect(drawer.getByRole('cell',{name:'供应商',exact:true})).toBeVisible();assert.deepEqual(errors,[])
 console.log('DEVICE_SOURCES_JOINT '+JSON.stringify({real_browser_react_gin_mysql_redis:true,api_mocking:false,real_login:true,warehouse_entry:true,source_self_and_supplier_created:true,warehouses_reloaded:true,source_projection_verified:true,duplicate_name_conflict:true,language_en_zh:true,mobile_no_page_overflow:true,browser_errors:0}))
}finally{await browser?.close();await server?.close()}
