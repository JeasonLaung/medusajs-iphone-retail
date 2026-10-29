import assert from 'node:assert/strict'
import {createServer} from 'vite'
import {chromium,expect} from '@playwright/test'
let server,browser
const origin='http://127.0.0.1:19383',errors=[]
if(!process.env.ADMIN_TEST_API_TARGET)throw Error('Owned API target required')
try{
 server=await createServer({server:{port:19383,strictPort:true,host:'127.0.0.1'},logLevel:'error'});await server.listen()
 browser=await chromium.launch({headless:true,executablePath:process.env.ADMIN_TEST_BROWSER||'C:/Program Files/Google/Chrome/Application/chrome.exe'})
 const admin=await browser.newContext({reducedMotion:'reduce'}),worker=await browser.newContext({reducedMotion:'reduce'})
 for(const context of [admin,worker])await context.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','en'))
 const page=await admin.newPage(),warehouse=await worker.newPage();for(const p of [page,warehouse])p.on('pageerror',e=>errors.push(e.message))
 const login=async(p,email,password)=>{
  await p.goto(origin+'/account');await p.getByLabel('Email',{exact:true}).fill(email);await p.getByLabel('Password',{exact:true}).fill(password);await p.getByRole('button',{name:'Sign in',exact:true}).click();await expect(p.getByRole('button',{name:'Sign out',exact:true})).toBeVisible()
 }
 await login(page,'browser@example.invalid','artificial-browser-password')
 await page.goto(origin+'/warehouse-users');await page.getByRole('button',{name:'New warehouse user',exact:true}).click()
 const editor=page.getByRole('dialog')
 await editor.getByLabel('Name',{exact:true}).fill('Joint warehouse operator')
 await editor.getByLabel('Email',{exact:true}).fill('warehouse-joint@example.invalid')
 await editor.getByLabel('Password',{exact:true}).fill('artificial-warehouse-joint-password')
 await editor.getByText('Warehouse A (joint-warehouse-a)',{exact:true}).click();await editor.locator('.ant-transfer-operation button').first().click()
 for(const label of ['View inventory','Edit inventory']){await editor.getByRole('checkbox',{name:label,exact:true}).click();await expect(editor.getByRole('checkbox',{name:label,exact:true})).toBeChecked()}
 await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(page.getByRole('button',{name:'Joint warehouse operator',exact:true})).toBeVisible()
 await login(warehouse,'warehouse-joint@example.invalid','artificial-warehouse-joint-password')
 await warehouse.getByRole('menuitem',{name:'Warehouse inventory',exact:true}).click();await expect(warehouse.getByRole('cell',{name:'WAREHOUSE-JOINT',exact:true})).toBeVisible()
 await expect(warehouse.getByRole('radio',{name:'Warehouse A',exact:true})).toBeVisible();await expect(warehouse.getByRole('radio',{name:'Warehouse B',exact:true})).toHaveCount(0)
 await expect(warehouse.getByRole('menuitem',{name:'Products',exact:true})).toHaveCount(0)
 assert.equal((await worker.request.get(origin+'/api/admin/orders')).status(),403)
 assert.equal((await worker.request.get(origin+'/api/admin/warehouse/locations/joint-warehouse-b/inventory')).status(),403)
 const payload=await worker.request.get(origin+'/api/admin/warehouse/locations/joint-warehouse-a/inventory');assert.equal(payload.status(),200);assert.ok(!(await payload.text()).includes('must-not-escape'))
 assert.ok((await warehouse.textContent('body')).includes('100.000000000000000001'))
 await warehouse.getByRole('button',{name:'Edit stock',exact:true}).click();const stock=warehouse.getByRole('dialog')
 await stock.getByLabel('Stocked quantity',{exact:true}).fill('99.000000000000000001');await stock.getByRole('button',{name:'Save',exact:true}).click();await expect(stock).not.toBeVisible()
 await warehouse.reload();await expect(warehouse.getByRole('cell',{name:'WAREHOUSE-JOINT',exact:true})).toBeVisible();assert.ok((await warehouse.textContent('body')).includes('99.000000000000000001'))
 await warehouse.setViewportSize({width:390,height:844});await warehouse.reload();await expect(warehouse.getByRole('cell',{name:'WAREHOUSE-JOINT',exact:true})).toBeVisible();assert.equal(await warehouse.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
 await worker.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','zh-CN'));await warehouse.reload();await expect(warehouse.getByRole('heading',{name:'仓库库存',exact:true})).toBeVisible()
 await page.getByRole('button',{name:'Joint warehouse operator',exact:true}).click();await editor.getByLabel('Enabled',{exact:true}).click();await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(editor).not.toBeVisible()
 assert.equal((await worker.request.get(origin+'/api/admin/auth/me')).status(),401)
 await warehouse.reload();await expect(warehouse.getByLabel('密码',{exact:true})).toBeVisible()
 assert.deepEqual(errors,[])
 console.log('WAREHOUSE_INVENTORY_JOINT '+JSON.stringify({real_browser_react_gin_mysql_redis:true,api_mocking:false,admin_creates_scoped_user:true,new_user_login:true,warehouse_menu_only:true,cross_warehouse_denied:true,private_projection:true,exact_stock_saved_reloaded:true,admin_disable_revokes_old_session:true,language_en_zh:true,mobile_no_page_overflow:true,browser_errors:0}))
}finally{await browser?.close();await server?.close()}
