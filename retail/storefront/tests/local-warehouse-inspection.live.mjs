import assert from 'node:assert/strict'
import {randomUUID} from 'node:crypto'
import {resolve} from 'node:path'
import {expect as baseExpect} from '../../admin/node_modules/@playwright/test/index.mjs'
const expect=baseExpect.configure({timeout:20000})

export async function verifyWarehouseInspection({browser,admin,origin,owner,sourceName,errors,logs,sku,productID,variantID}){
 assert.equal(origin,'http://127.0.0.1:19200')
 const inventoryURL=origin+'/api/admin/products/'+productID+'/variants/'+variantID+'/inventory'
 const inventoryResponse=await admin.request.get(inventoryURL);assert.equal(inventoryResponse.status(),200);const inventoryBefore=await inventoryResponse.text()
 const directoryResponse=await admin.request.get(origin+'/api/admin/warehouse-users');assert.equal(directoryResponse.status(),200)
 const directory=await directoryResponse.json(),location=directory.locations.find(l=>l.name===sourceName),other=directory.locations.find(l=>l.name!==sourceName)
 assert.ok(location);assert.ok(other)
 const email=`local-inspector-${owner}@example.invalid`,password='isolated-inspector-'+owner+'-password'
 await admin.goto(origin+'/warehouse-users');await admin.getByRole('button',{name:'New warehouse user',exact:true}).click()
 let editor=admin.getByRole('dialog')
 for(const [name,value] of Object.entries({Name:'Isolated return inspector',Email:email,Password:password}))await editor.getByLabel(name,{exact:true}).fill(value)
 await editor.getByText(`${location.name} (${location.id})`,{exact:true}).click();await editor.locator('.ant-transfer-operation button').first().click()
 for(const name of ['View sale devices','View returned devices','Manage returned devices']){await editor.getByRole('checkbox',{name,exact:true}).click();await expect(editor.getByRole('checkbox',{name,exact:true})).toBeChecked()}
 await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(editor).toHaveCount(0)
 const worker=await browser.newContext({viewport:{width:390,height:1000},reducedMotion:'reduce'})
 try{
  await worker.addInitScript(()=>{if(!localStorage.getItem('eurorefurb.admin.language'))localStorage.setItem('eurorefurb.admin.language','en')})
  const page=await worker.newPage();page.on('pageerror',e=>errors.push(e.message))
  await page.goto(origin+'/account');await page.getByLabel('Email',{exact:true}).fill(email);await page.getByLabel('Password',{exact:true}).fill(password);await page.getByRole('button',{name:'Sign in',exact:true}).click();await expect(page.getByRole('button',{name:'Sign out',exact:true})).toBeVisible()
  const get=async path=>{const r=await worker.request.get(origin+'/api/admin/'+path);assert.equal(r.status(),200);return r.json()}
  const list=()=>get(`warehouse/locations/${location.id}/devices/returned?offset=0&q=`)
  const d=(await list()).devices.find(d=>d.serial_number==='NATIVE-INTAKE-RETURN');assert.ok(d)
  const base=`warehouse/locations/${location.id}/devices/returned/${d.id}/inspection`
  // Search the original ERP code through the Admin product definition instead.
  const all=(await get(`warehouse/locations/${location.id}/device-intake/returned/variants?q=&offset=0`)).variants
  const alternate=all.find(v=>v.id!==d.variant_id&&v.sku);assert.ok(alternate)
  assert.equal((await worker.request.get(origin+'/api/admin/products')).status(),403)
  assert.equal((await worker.request.get(origin+'/api/admin/'+base.replace(location.id,other.id))).status(),403)
  const initial=await get(base);assert.equal(initial.repair,false);assert.equal(initial.device.imei,'000000000000082')
  assert.equal(JSON.stringify(initial).includes('cost'),false);assert.equal(JSON.stringify(initial).includes('order_id'),false);assert.equal(JSON.stringify(initial).includes('notes'),false)
  const csrf=(await worker.cookies()).find(c=>c.name==='retail_admin_csrf')?.value;assert.ok(csrf)
  const post=body=>worker.request.post(origin+'/api/admin/'+base,{headers:{Origin:origin,'X-CSRF-Token':csrf},data:body})
  const invalid={operation_id:randomUUID(),expected_revision:initial.device.revision,action:'inspect',result:'supplier_return',variant_id:d.variant_id,grade:'B',battery_health:85,notes:'Supplier return forbidden for own source',photos:[],tracking_number:''}
  assert.equal((await post(invalid)).status(),409);assert.equal((await get(base)).device.revision,initial.device.revision)
  assert.equal((await post({...invalid,result:'repair',notes:'owned-inspection-receipt-rollback'})).status(),503)
  assert.equal((await get(base)).device.revision,initial.device.revision)
  const concurrent={...invalid,operation_id:randomUUID(),result:'repair',notes:'Concurrent first command'}
  const firstWave=await Promise.all(Array.from({length:6},()=>post(concurrent)))
  const receipts=[];for(const r of firstWave){assert.equal(r.status(),200);receipts.push(await r.json())}
  assert.equal(receipts.filter(r=>!r.replayed).length,1)

  const open=async()=>{await page.goto(origin+'/warehouse-devices');await page.getByRole('radio',{name:'Returned',exact:true}).check();await page.getByRole('row').filter({hasText:'NATIVE-INTAKE-RETURN'}).getByRole('button',{name:'Inspect device',exact:true}).click();return page.getByRole('dialog',{name:'Inspect returned device',exact:true})}
  editor=await open()
  await editor.getByLabel('Inspection result',{exact:true}).focus();await editor.getByLabel('Inspection result',{exact:true}).press('ArrowDown');await page.locator('.ant-select-dropdown:not(.ant-select-dropdown-hidden)').getByText('Repair',{exact:true}).click()
  const select=editor.getByRole('combobox',{name:'Inspected SKU',exact:true});await select.fill(alternate.sku);await expect(page.locator('.ant-select-dropdown:not(.ant-select-dropdown-hidden)').getByText(new RegExp(alternate.sku))).toBeVisible();await page.locator('.ant-select-dropdown:not(.ant-select-dropdown-hidden)').getByText(new RegExp(alternate.sku)).click()
  await editor.getByLabel('Actual battery health',{exact:true}).fill('85');await editor.getByLabel('Inspection notes',{exact:true}).fill('Inspection requires repair; tested different SKU')
  const png=Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jB9kAAAAASUVORK5CYII=','base64')
  await editor.locator('input[type=file]').first().setInputFiles({name:'inspection.png',mimeType:'image/png',buffer:png});await expect(editor.locator('.ant-upload-list-item')).toHaveCount(1)
  const commands=[];let dropped=false
  await page.route('**/inspection',async route=>{if(route.request().method()!=='POST')return route.continue();commands.push(route.request().postDataJSON());if(!dropped){const r=await route.fetch();assert.equal(r.status(),200);dropped=true;return route.abort('failed')}return route.continue()})
  await editor.getByRole('button',{name:'Save inspection',exact:true}).click();await expect(editor.getByRole('button',{name:'Retry this save',exact:true})).toBeVisible();await editor.getByRole('button',{name:'Retry this save',exact:true}).click();await expect(editor).toHaveCount(0)
  assert.deepEqual(commands[0],commands[1]);const repaired=await get(base);assert.equal(repaired.device.state,'repair');assert.equal(repaired.device.variant_id,alternate.id)
  const replay=await Promise.all(Array.from({length:6},()=>post(commands[0])));for(const r of replay){assert.equal(r.status(),200);assert.equal((await r.json()).replayed,true)}
  assert.equal((await post({...commands[0],notes:'Altered replay'})).status(),409)
  assert.equal((await post({...commands[0],operation_id:randomUUID()})).status(),409)
  await page.evaluate(()=>localStorage.setItem('eurorefurb.admin.language','zh-CN'));await page.goto(origin+'/warehouse-devices');await page.getByRole('radio',{name:'退回',exact:true}).check();await page.getByRole('row').filter({hasText:'NATIVE-INTAKE-RETURN'}).getByRole('button',{name:'检测处理',exact:true}).click()
  const chinese=page.getByRole('dialog',{name:'检测退回设备',exact:true});await expect(chinese.getByRole('button',{name:'保存检测结果',exact:true})).toBeEnabled();await expect(chinese.getByLabel('实际电池健康度',{exact:true})).toBeVisible();assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true);await page.screenshot({path:resolve(logs,'warehouse-inspection-zh-drawer.png'),fullPage:true});await chinese.getByRole('button',{name:/^取\s*消$/}).click();await expect(chinese).toHaveCount(0)
  await page.evaluate(()=>localStorage.setItem('eurorefurb.admin.language','en'))
  editor=await open()
  // Returning to the current product's original SKU uses the same searchable picker.
  const ownSKU=sku
  const back=editor.getByRole('combobox',{name:'Inspected SKU',exact:true});await back.fill(ownSKU);await page.locator('.ant-select-dropdown:not(.ant-select-dropdown-hidden)').getByText(new RegExp(ownSKU)).click()
  await editor.getByLabel('Inspection notes',{exact:true}).fill('Repaired and passed; retain original identity')
  await editor.getByRole('button',{name:'Save inspection',exact:true}).click();await expect(editor).toHaveCount(0)
  await page.getByRole('radio',{name:'Sale',exact:true}).check();const row=page.getByRole('row').filter({hasText:'NATIVE-INTAKE-RETURN'});await expect(row.getByRole('cell',{name:'available',exact:true})).toBeVisible()
  await page.reload();await expect(page.getByRole('row').filter({hasText:'NATIVE-INTAKE-RETURN'})).toBeVisible()
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true);await page.screenshot({path:resolve(logs,'warehouse-inspection-en.png'),fullPage:true})
  await page.evaluate(()=>localStorage.setItem('eurorefurb.admin.language','zh-CN'));await page.reload();await expect(page.getByRole('row').filter({hasText:'NATIVE-INTAKE-RETURN'})).toBeVisible();assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true);await page.screenshot({path:resolve(logs,'warehouse-inspection-zh.png'),fullPage:true})
  await admin.goto(origin+'/warehouse-users');await admin.getByRole('button',{name:'Isolated return inspector',exact:true}).click()
  const permission=admin.getByRole('dialog');await permission.getByRole('checkbox',{name:'Manage returned devices',exact:true}).click();await expect(permission.getByRole('checkbox',{name:'Manage returned devices',exact:true})).not.toBeChecked();await permission.getByRole('button',{name:'Save',exact:true}).click();await expect(permission).toHaveCount(0)
  assert.equal((await worker.request.get(origin+'/api/admin/auth/me')).status(),401)
  assert.equal((await post(commands[0])).status(),401)
  await page.evaluate(()=>localStorage.setItem('eurorefurb.admin.language','en'));await page.goto(origin+'/account');await page.getByLabel('Email',{exact:true}).fill(email);await page.getByLabel('Password',{exact:true}).fill(password);await page.getByRole('button',{name:'Sign in',exact:true}).click();await expect(page.getByRole('button',{name:'Sign out',exact:true})).toBeVisible()
  const newCSRF=(await worker.cookies()).find(c=>c.name==='retail_admin_csrf')?.value
  assert.equal((await worker.request.post(origin+'/api/admin/'+base,{headers:{Origin:origin,'X-CSRF-Token':newCSRF},data:commands[0]})).status(),403)
  await page.goto(origin+'/warehouse-devices');await page.getByRole('radio',{name:'Returned',exact:true}).check();await expect(page.getByRole('button',{name:'Inspect device',exact:true})).toHaveCount(0)
  const inventoryAfter=await admin.request.get(inventoryURL);assert.equal(inventoryAfter.status(),200);assert.equal(await inventoryAfter.text(),inventoryBefore)
  return {warehouse_inspection_actual_employee:true,inspection_quantity_stock_and_reservations_unchanged:true,inspection_permission_change_revokes_old_session:true,inspection_readonly_replay_denied:true,inspection_device_id:d.id,inspection_alternate_variant_id:alternate.id,inspection_location_id:location.id,inspection_cross_sku_repair:true,inspection_pass_original_sku:true,inspection_photos_uploaded:true,inspection_lost_reply_same_command:true,inspection_six_concurrent_first_and_replays:true,inspection_receipt_failure_rolled_back:true,inspection_altered_stale_external_scope_rejected:true,inspection_en_zh_mobile:true}
 }finally{await worker.close()}
}
