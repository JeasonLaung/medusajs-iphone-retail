import assert from 'node:assert/strict'
import {randomUUID} from 'node:crypto'
import {resolve} from 'node:path'
import {expect as baseExpect} from '../../admin/node_modules/@playwright/test/index.mjs'
const expect=baseExpect.configure({timeout:20000})

export async function verifyWarehouseBatch({browser,admin,origin,owner,sourceName,errors,logs,sku,productID,variantID}){
 assert.equal(origin,'http://127.0.0.1:19200')
 const inventoryURL=origin+'/api/admin/products/'+productID+'/variants/'+variantID+'/inventory'
 const inventoryBefore=await (await admin.request.get(inventoryURL)).text()
 const directoryResponse=await admin.request.get(origin+'/api/admin/warehouse-users');assert.equal(directoryResponse.status(),200)
 const directory=await directoryResponse.json(),location=directory.locations.find(l=>l.name===sourceName),other=directory.locations.find(l=>l.name!==sourceName)
 assert.ok(location);assert.ok(other)
 const email=`local-batch-${owner}@example.invalid`,password='isolated-editor-'+owner+'-password'
 await admin.goto(origin+'/warehouse-users');await admin.getByRole('button',{name:'New warehouse user',exact:true}).click()
 let editor=admin.getByRole('dialog')
 for(const [name,value] of Object.entries({Name:'Isolated batch operator',Email:email,Password:password}))await editor.getByLabel(name,{exact:true}).fill(value)
 await editor.getByText(`${location.name} (${location.id})`,{exact:true}).click();await editor.locator('.ant-transfer-operation button').first().click()
 for(const name of ['View sale devices','Manage sale devices','View returned devices','Manage returned devices']){await editor.getByRole('checkbox',{name,exact:true}).click();await expect(editor.getByRole('checkbox',{name,exact:true})).toBeChecked()}
 await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(editor).toHaveCount(0)
 const worker=await browser.newContext({viewport:{width:390,height:1000},reducedMotion:'reduce'})
 try{
  await worker.addInitScript(()=>{if(!localStorage.getItem('eurorefurb.admin.language'))localStorage.setItem('eurorefurb.admin.language','en')})
  const page=await worker.newPage();page.on('pageerror',e=>errors.push(e.message))
  const login=async()=>{await page.goto(origin+'/account');await page.getByLabel('Email',{exact:true}).fill(email);await page.getByLabel('Password',{exact:true}).fill(password);await page.getByRole('button',{name:'Sign in',exact:true}).click();await expect(page.getByRole('button',{name:'Sign out',exact:true})).toBeVisible()};await login()
  const get=async path=>{const r=await worker.request.get(origin+'/api/admin/'+path);assert.equal(r.status(),200);return r.json()}
  const csrf=()=>worker.cookies().then(c=>c.find(c=>c.name==='retail_admin_csrf')?.value)
  const post=async(path,body)=>worker.request.post(origin+'/api/admin/'+path,{headers:{Origin:origin,'X-CSRF-Token':await csrf()},data:body})
  const directoryPath=`warehouse/locations/${location.id}/devices/`
  const sale=(await get(directoryPath+'sale?offset=0&q=')).devices[0],returned=(await get(directoryPath+'returned?offset=0&q=')).devices[0];assert.ok(sale);assert.ok(returned)
  const change=async(d,category,fields)=>{const path=directoryPath+category+'/'+d.id+'/edit',current=await get(path);const r=await post(path,{operation_id:randomUUID(),expected_revision:current.revision,fields:{...current.fields,...fields}});assert.equal(r.status(),200)}
  await change(sale,'sale',{state:'pending'})
  await change(returned,'returned',{category:'sale',state:'available'})
  const endpoint=`warehouse/locations/${location.id}/device-batches/make-available`
  const read=()=>get(directoryPath+'sale?offset=0&q=')
  const input=async()=>({operation_id:randomUUID(),devices:(await read()).devices.filter(d=>[sale.id,returned.id].includes(d.id)).sort((a,b)=>a.id===sale.id?-1:1).map(d=>({id:d.id,expected_revision:d.revision}))})
  let before=await read();assert.equal((await post(endpoint,await input())).status(),409);assert.deepEqual(await read(),before)
  await change(returned,'sale',{state:'pending'})
  before=await read();const fault={...await input(),operation_id:'00000000-0000-4000-8000-000000000001'}
  assert.equal((await post(endpoint,fault)).status(),503);assert.deepEqual(await read(),before)
  const waveInput=await input(),wave=await Promise.all(Array.from({length:6},()=>post(endpoint,waveInput))),results=[]
  for(const r of wave){assert.equal(r.status(),200);results.push(await r.json())}assert.equal(results.filter(r=>!r.replayed).length,1)
  assert.equal((await post(endpoint,{...waveInput,devices:[...waveInput.devices].reverse()})).status(),409)
  assert.equal((await post(endpoint,{...waveInput,operation_id:randomUUID()})).status(),409)
  assert.equal((await post(endpoint.replace(location.id,other.id),waveInput)).status(),403)
  assert.equal((await post(endpoint,{...waveInput,operation_id:randomUUID(),devices:[waveInput.devices[0],waveInput.devices[0]]})).status(),400)
  await change(sale,'sale',{state:'pending'});await change(returned,'sale',{state:'pending'})
  await page.evaluate(()=>localStorage.setItem('eurorefurb.admin.language','zh'));await page.goto(origin+'/warehouse-devices')
  for(const sn of ['NATIVE-INTAKE-SALE','NATIVE-INTAKE-RETURN'])await page.getByRole('row').filter({hasText:sn}).getByRole('checkbox').check()
  await page.getByRole('button',{name:'批量设为可售',exact:true}).click();editor=page.getByRole('dialog',{name:'批量设为可售',exact:true});await expect(editor.getByText('已选设备: 2',{exact:true})).toBeVisible();assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true);await page.screenshot({path:resolve(logs,'warehouse-batch-zh-confirm.png'),fullPage:true});await editor.getByRole('button',{name:/^取\s*消$/}).click()
  await page.evaluate(()=>localStorage.setItem('eurorefurb.admin.language','en'));await page.goto(origin+'/warehouse-devices')
  for(const sn of ['NATIVE-INTAKE-SALE','NATIVE-INTAKE-RETURN'])await page.getByRole('row').filter({hasText:sn}).getByRole('checkbox').check()
  await page.getByRole('button',{name:'Make selected devices available',exact:true}).click()
  editor=page.getByRole('dialog',{name:'Make selected devices available',exact:true});await expect(editor.getByText('Selected devices: 2',{exact:true})).toBeVisible()
  const bodies=[];let lost=false
  await page.route('**/device-batches/make-available',async route=>{bodies.push(route.request().postDataJSON());if(!lost){const response=await route.fetch();assert.equal(response.status(),200);lost=true;return route.abort('failed')}return route.continue()})
  await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(editor.getByRole('button',{name:'Retry this save',exact:true})).toBeVisible()
  await page.screenshot({path:resolve(logs,'warehouse-batch-en-retry.png'),fullPage:true})
  await editor.getByRole('button',{name:'Retry this save',exact:true}).click();await expect(editor).toHaveCount(0);assert.deepEqual(bodies[0],bodies[1])
  const final=await read();assert.equal(final.devices.filter(d=>[sale.id,returned.id].includes(d.id)&&d.state==='available').length,2)
  await page.evaluate(()=>localStorage.setItem('eurorefurb.admin.language','zh'));await page.reload();await expect(page.getByRole('button',{name:'批量设为可售',exact:true})).toBeDisabled()
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true);await page.screenshot({path:resolve(logs,'warehouse-batch-zh.png'),fullPage:true})
  await admin.goto(origin+'/warehouse-users');await admin.getByRole('button',{name:'Isolated batch operator',exact:true}).click();editor=admin.getByRole('dialog');await editor.getByRole('checkbox',{name:'Manage sale devices',exact:true}).click();await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(editor).toHaveCount(0)
  assert.equal((await worker.request.get(origin+'/api/admin/auth/me')).status(),401)
  await page.evaluate(()=>localStorage.setItem('eurorefurb.admin.language','en'));await login();assert.equal((await post(endpoint,bodies[0])).status(),403)
  await page.goto(origin+'/warehouse-devices');await expect(page.getByRole('row').filter({hasText:'NATIVE-INTAKE-SALE'})).toBeVisible();await expect(page.getByRole('button',{name:'Make selected devices available',exact:true})).toHaveCount(0)
  assert.equal(await (await admin.request.get(inventoryURL)).text(),inventoryBefore)
  return {warehouse_batch_actual_employee:true,batch_device_id:sale.id,batch_location_id:location.id,batch_invalid_second_device_atomic:true,batch_final_receipt_atomic:true,batch_six_concurrent_one_execution:true,batch_lost_reply_same_command:true,batch_stale_duplicate_tamper_external_scope_rejected:true,batch_revoke_denies_replay:true,batch_quantity_inventory_unchanged:true,batch_available_devices:2,batch_en_zh_mobile:true}
 }finally{await worker.close()}
}
