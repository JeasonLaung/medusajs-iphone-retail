import assert from 'node:assert/strict'
import {randomUUID} from 'node:crypto'
import {resolve} from 'node:path'
import {expect as baseExpect} from '../../admin/node_modules/@playwright/test/index.mjs'
const expect=baseExpect.configure({timeout:20000})

export async function verifyWarehouseDeviceEdit({browser,admin,origin,owner,sourceName,errors,logs,sku,productID,variantID}){
 assert.equal(origin,'http://127.0.0.1:19200')
 const inventoryURL=origin+'/api/admin/products/'+productID+'/variants/'+variantID+'/inventory'
 const inventoryBefore=await (await admin.request.get(inventoryURL)).text()
 const directoryResponse=await admin.request.get(origin+'/api/admin/warehouse-users');assert.equal(directoryResponse.status(),200)
 const directory=await directoryResponse.json(),location=directory.locations.find(l=>l.name===sourceName),other=directory.locations.find(l=>l.name!==sourceName)
 assert.ok(location);assert.ok(other)
 const email=`local-editor-${owner}@example.invalid`,password='isolated-editor-'+owner+'-password'
 await admin.goto(origin+'/warehouse-users');await admin.getByRole('button',{name:'New warehouse user',exact:true}).click()
 let editor=admin.getByRole('dialog')
 for(const [name,value] of Object.entries({Name:'Isolated device editor',Email:email,Password:password}))await editor.getByLabel(name,{exact:true}).fill(value)
 await editor.getByText(`${location.name} (${location.id})`,{exact:true}).click();await editor.locator('.ant-transfer-operation button').first().click()
 for(const name of ['View sale devices','Manage sale devices','View returned devices','Manage returned devices']){await editor.getByRole('checkbox',{name,exact:true}).click();await expect(editor.getByRole('checkbox',{name,exact:true})).toBeChecked()}
 await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(editor).toHaveCount(0)
 const worker=await browser.newContext({viewport:{width:390,height:1000},reducedMotion:'reduce'})
 try{
  await worker.addInitScript(()=>{if(!localStorage.getItem('eurorefurb.admin.language'))localStorage.setItem('eurorefurb.admin.language','en')})
  const page=await worker.newPage();page.on('pageerror',e=>errors.push(e.message))
  const login=async()=>{await page.goto(origin+'/account');await page.getByLabel('Email',{exact:true}).fill(email);await page.getByLabel('Password',{exact:true}).fill(password);await page.getByRole('button',{name:'Sign in',exact:true}).click();await expect(page.getByRole('button',{name:'Sign out',exact:true})).toBeVisible()};await login()
  const get=async path=>{const r=await worker.request.get(origin+'/api/admin/'+path);assert.equal(r.status(),200);return r.json()}
  const list=await get(`warehouse/locations/${location.id}/devices/sale?offset=0&q=`),d=list.devices.find(d=>d.serial_number==='NATIVE-INTAKE-SALE');assert.ok(d)
  const base=`warehouse/locations/${location.id}/devices/sale/${d.id}/edit`
  const read=()=>get(base),initial=await read();assert.equal(initial.serial_number,'NATIVE-INTAKE-SALE');assert.deepEqual(initial.locations.map(l=>l.id),[location.id]);assert.ok(initial.sources.every(s=>s.location_id===location.id))
  const post=async body=>{const csrf=(await worker.cookies()).find(c=>c.name==='retail_admin_csrf')?.value;return worker.request.post(origin+'/api/admin/'+base,{headers:{Origin:origin,'X-CSRF-Token':csrf},data:body})}
  const input=fields=>({operation_id:randomUUID(),expected_revision:initial.revision,fields:{...initial.fields,...fields}})
  assert.equal((await worker.request.get(origin+'/api/admin/products')).status(),403)
  assert.equal((await worker.request.get(origin+'/api/admin/'+base.replace(location.id,other.id))).status(),403)
  assert.equal((await post(input({location_id:other.id}))).status(),403)
  assert.equal((await post(input({state:'shipped'}))).status(),409)
  assert.equal((await post(input({imei2:'000000000000082'}))).status(),409)
  assert.equal((await post(input({imei:'000000000000083',notes:'owned-edit-receipt-rollback'}))).status(),503)
  assert.deepEqual(await read(),initial)
  const concurrent=input({bin:'Concurrent bin'})
  const wave=await Promise.all(Array.from({length:6},()=>post(concurrent)));const outcomes=[]
  for(const r of wave){assert.equal(r.status(),200);outcomes.push(await r.json())}assert.equal(outcomes.filter(r=>!r.replayed).length,1)
  const open=async()=>{await page.goto(origin+'/warehouse-devices');await page.getByRole('row').filter({hasText:'NATIVE-INTAKE-SALE'}).getByRole('button',{name:'Edit device',exact:true}).click();return page.getByRole('dialog',{name:'Edit device',exact:true})}
  editor=await open();await expect(editor.getByText('SN: NATIVE-INTAKE-SALE',{exact:true})).toBeVisible();assert.equal(await editor.getByLabel('Serial number',{exact:true}).count(),0)
  await editor.getByLabel('IMEI',{exact:true}).fill('000000000000083');await editor.getByLabel('IMEI 2',{exact:true}).fill('000000000000084')
  await editor.getByLabel('Cost (minor units)',{exact:true}).fill('12345');await editor.getByLabel('Bin',{exact:true}).fill('Updated shelf');await editor.getByLabel('Notes',{exact:true}).fill('Actual warehouse editor verification')
  const bodies=[];let lost=false
  await page.route('**/edit',async route=>{if(route.request().method()!=='POST')return route.continue();bodies.push(route.request().postDataJSON());if(!lost){const r=await route.fetch();assert.equal(r.status(),200);lost=true;return route.abort('failed')}return route.continue()})
  await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(editor.getByRole('button',{name:'Retry this save',exact:true})).toBeVisible();await editor.getByRole('button',{name:'Retry this save',exact:true}).click();await expect(editor).toHaveCount(0);assert.deepEqual(bodies[0],bodies[1])
  const saved=await read();assert.equal(saved.fields.imei,'000000000000083');assert.equal(saved.fields.imei2,'000000000000084');assert.equal(saved.fields.cost_cents,12345);assert.equal(saved.fields.bin,'Updated shelf');assert.equal(saved.fields.photos.length,1);assert.equal(saved.fields.variant_id,variantID)
  assert.equal((await post({...bodies[0],fields:{...bodies[0].fields,notes:'altered'}})).status(),409)
  assert.equal((await post({...bodies[0],operation_id:randomUUID()})).status(),409)
  await page.evaluate(()=>localStorage.setItem('eurorefurb.admin.language','zh-CN'));await page.goto(origin+'/warehouse-devices');await page.getByRole('row').filter({hasText:'NATIVE-INTAKE-SALE'}).getByRole('button',{name:'编辑设备',exact:true}).click()
  editor=page.getByRole('dialog',{name:'编辑设备',exact:true});await expect(editor.getByLabel('库位',{exact:true})).toHaveValue('Updated shelf');await expect(editor.getByLabel('成本（分）',{exact:true})).toHaveValue('12345');assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true);await page.screenshot({path:resolve(logs,'warehouse-device-edit-zh.png'),fullPage:true});await editor.getByRole('button',{name:/^取\s*消$/}).click()
  await admin.goto(origin+'/warehouse-users');await admin.getByRole('button',{name:'Isolated device editor',exact:true}).click();editor=admin.getByRole('dialog');await editor.getByRole('checkbox',{name:'Manage sale devices',exact:true}).click();await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(editor).toHaveCount(0)
  assert.equal((await worker.request.get(origin+'/api/admin/auth/me')).status(),401)
  await page.evaluate(()=>localStorage.setItem('eurorefurb.admin.language','en'));await login();assert.equal((await post(bodies[0])).status(),403)
  await page.goto(origin+'/warehouse-devices');await expect(page.getByRole('row').filter({hasText:'NATIVE-INTAKE-SALE'})).toBeVisible();await expect(page.getByRole('button',{name:'Edit device',exact:true})).toHaveCount(0)
  assert.equal(await (await admin.request.get(inventoryURL)).text(),inventoryBefore)
  return {warehouse_edit_actual_employee:true,edit_device_id:d.id,edit_location_id:location.id,edit_identity_updated_serial_preserved:true,edit_cost_bin_notes_photos:true,edit_six_first_concurrent_unique:true,edit_lost_response_same_command:true,edit_duplicate_imei_external_scope_state_rejected:true,edit_last_receipt_failure_rolled_back:true,edit_permission_revoke_session_and_receipt:true,edit_en_zh_mobile:true,edit_quantity_stock_reservations_unchanged:true}
 }finally{await worker.close()}
}
