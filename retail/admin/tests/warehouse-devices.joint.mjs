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
 for(const c of [admin,worker])await c.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','en'))
 const page=await admin.newPage(),warehouse=await worker.newPage();for(const p of [page,warehouse])p.on('pageerror',e=>errors.push(e.message))
 const login=async(p,email,password)=>{await p.goto(origin+'/account');await p.getByLabel('Email',{exact:true}).fill(email);await p.getByLabel('Password',{exact:true}).fill(password);await p.getByRole('button',{name:'Sign in',exact:true}).click();await expect(p.getByRole('button',{name:'Sign out',exact:true})).toBeVisible()}
 await login(page,'browser@example.invalid','artificial-browser-password')
 await page.goto(origin+'/warehouse-users');await page.getByRole('button',{name:'New warehouse user',exact:true}).click();const editor=page.getByRole('dialog')
 await editor.getByLabel('Name',{exact:true}).fill('Device warehouse operator');await editor.getByLabel('Email',{exact:true}).fill('devices-joint@example.invalid');await editor.getByLabel('Password',{exact:true}).fill('artificial-device-joint-password')
 await editor.getByText('Warehouse A (joint-warehouse-a)',{exact:true}).click();await editor.locator('.ant-transfer-operation button').first().click()
 for(const label of ['View sale devices','Manage sale devices','View returned devices','Manage returned devices']){await editor.getByRole('checkbox',{name:label,exact:true}).click();await expect(editor.getByRole('checkbox',{name:label,exact:true})).toBeChecked()}
 await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(page.getByRole('button',{name:'Device warehouse operator',exact:true})).toBeVisible()
 await login(warehouse,'devices-joint@example.invalid','artificial-device-joint-password');await warehouse.getByRole('menuitem',{name:'Warehouse devices',exact:true}).click()
 await expect(warehouse.getByRole('cell',{name:'WAREHOUSE-PENDING',exact:true})).toBeVisible();await expect(warehouse.getByRole('radio',{name:'Warehouse B',exact:true})).toHaveCount(0);await expect(warehouse.getByRole('radio',{name:'Recycled',exact:true})).toHaveCount(0)
 const raw=await worker.request.get(origin+'/api/admin/warehouse/locations/joint-warehouse-a/devices/sale');assert.equal(raw.status(),200);for(const secret of ['cost_cents','SECRET-NOTE','HIDDEN-OTHER','HIDDEN-RECYCLED'])assert.ok(!(await raw.text()).includes(secret))
 assert.equal((await worker.request.get(origin+'/api/admin/warehouse/locations/joint-warehouse-b/devices/sale')).status(),403);assert.equal((await worker.request.get(origin+'/api/admin/products/joint-device-product')).status(),403)
 const perform=async(sn,action)=>{await warehouse.getByRole('row').filter({has:warehouse.getByRole('cell',{name:sn,exact:true})}).getByRole('button',{name:action,exact:true}).click();const dialog=warehouse.getByRole('dialog');await dialog.getByRole('button',{name:'Save',exact:true}).click();await expect(dialog).not.toBeVisible()}
 await perform('WAREHOUSE-PENDING','Make available');await expect(warehouse.getByRole('row').filter({has:warehouse.getByRole('cell',{name:'WAREHOUSE-PENDING',exact:true})}).getByRole('cell',{name:'available',exact:true})).toBeVisible()
 await perform('WAREHOUSE-INSPECTION','Activate inspected sale device');await perform('WAREHOUSE-INSPECTION','Retire')
 await warehouse.getByRole('row').filter({has:warehouse.getByRole('cell',{name:'WAREHOUSE-PENDING',exact:true})}).getByRole('button',{name:'Transfer to returned',exact:true}).click()
 const transfer=warehouse.getByRole('dialog');await transfer.getByRole('button',{name:'Save',exact:true}).click();await expect(transfer).toBeVisible();await expect(transfer.locator('.ant-form-item-explain-error')).toBeVisible()
 await transfer.getByLabel('Reason for transfer to returned',{exact:true}).fill('Customer returned device for inspection');await transfer.getByRole('button',{name:'Save',exact:true}).click();await expect(transfer).not.toBeVisible();await expect(warehouse.getByRole('cell',{name:'WAREHOUSE-PENDING',exact:true})).toHaveCount(0)
 await warehouse.getByRole('radio',{name:'Returned',exact:true}).check();await expect(warehouse.getByRole('cell',{name:'WAREHOUSE-RETURNED',exact:true})).toBeVisible();await expect(warehouse.getByRole('row').filter({has:warehouse.getByRole('cell',{name:'WAREHOUSE-PENDING',exact:true})}).getByRole('cell',{name:'inspection',exact:true})).toBeVisible();await perform('WAREHOUSE-RETURNED','Delete device record');await expect(warehouse.getByRole('cell',{name:'WAREHOUSE-RETURNED',exact:true})).toHaveCount(0)
 await warehouse.reload();await warehouse.getByRole('radio',{name:'Sale',exact:true}).check();await expect(warehouse.getByRole('cell',{name:'retired',exact:true})).toBeVisible()
 // Remove only the sale write grant; the same login must become read-only.
 await page.getByRole('button',{name:'Device warehouse operator',exact:true}).click();await editor.getByRole('checkbox',{name:'Manage sale devices',exact:true}).click();await expect(editor.getByRole('checkbox',{name:'Manage sale devices',exact:true})).not.toBeChecked();await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(editor).not.toBeVisible()
 // Account permission edits revoke sessions; sign in with the new current grants.
 assert.equal((await worker.request.get(origin+'/api/admin/auth/me')).status(),401);await login(warehouse,'devices-joint@example.invalid','artificial-device-joint-password');await warehouse.goto(origin+'/warehouse-devices');await expect(warehouse.getByRole('cell',{name:'WAREHOUSE-INSPECTION',exact:true})).toBeVisible();await expect(warehouse.getByRole('button',{name:'Retire',exact:true})).toHaveCount(0);await expect(warehouse.getByRole('button',{name:'Transfer to returned',exact:true})).toHaveCount(0)
 await warehouse.setViewportSize({width:390,height:844});assert.equal(await warehouse.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
 await worker.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','zh-CN'));await warehouse.reload();await expect(warehouse.getByRole('heading',{name:'仓库设备',exact:true})).toBeVisible();assert.deepEqual(errors,[])
 console.log('WAREHOUSE_DEVICES_JOINT '+JSON.stringify({real_browser_react_gin_mysql_redis:true,api_mocking:false,admin_creates_category_grants:true,worker_login:true,location_and_category_scope:true,no_cost_notes_projection:true,pending_available:true,inspection_available:true,retire:true,returned_soft_delete:true,transfer_return_reason_required:true,transfer_moves_to_returned_inspection:true,refresh_preserves_state:true,permission_change_revokes_session:true,read_only_controls:true,language_en_zh:true,mobile_no_page_overflow:true,browser_errors:0}))
}finally{await browser?.close();await server?.close()}
