import assert from 'node:assert/strict'

// Rehearse the real Admin intake, not an SQL-created inspection device.
export async function intakeBuybackDevice({context,origin,owner,expect,png,errors}){
 assert.equal(origin,'http://127.0.0.1:19200')
 const admin=await context.newPage()
 admin.on('pageerror',e=>errors.push(e.message))
 try{
  await admin.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','en'))
  const response=await admin.request.get(origin+'/api/store/products?handle=swappie-iphone-13')
  assert.equal(response.status(),200);const product=(await response.json()).products[0];assert.ok(product)
  const variantsReply=await admin.request.get(origin+'/api/admin/products/'+product.id+'/variants?limit=100&offset=0')
  assert.equal(variantsReply.status(),200)
  const variant=(await variantsReply.json()).variants.filter(v=>v.options.some(o=>o.value.replaceAll(' ','')==='256GB')).sort((a,b)=>a.id.localeCompare(b.id))[0]
  assert.ok(variant?.sku)
  const sourceName='Buyback intake '+owner
  await admin.goto(origin+'/warehouses');await admin.getByRole('button',{name:'Device sources',exact:true}).click()
  let dialog=admin.getByRole('dialog');await dialog.getByRole('button',{name:'New device source',exact:true}).click()
  await dialog.getByLabel('Name',{exact:true}).fill(sourceName)
  const sourceReply=admin.waitForResponse(r=>r.request().method()==='POST'&&r.url().endsWith('/device-sources'))
  await dialog.getByRole('button',{name:'Save',exact:true}).click();const sourceResponse=await sourceReply;assert.equal(sourceResponse.status(),200)
  const source=(await sourceResponse.json()).source
  await expect(dialog.getByRole('cell',{name:sourceName,exact:true})).toBeVisible()
  await admin.goto(origin+'/products/'+product.id+'?limit=100&offset=0')
  await admin.getByRole('row').filter({hasText:variant.sku}).getByRole('button',{name:'Device tracking',exact:true}).click()
  dialog=admin.getByRole('dialog');const enroll=dialog.getByRole('button',{name:'Enable individual device tracking',exact:true})
  if(await enroll.count()){await enroll.click();await expect(dialog.getByText('Enabled',{exact:true})).toBeVisible()}
  await dialog.getByRole('button',{name:'Device intake',exact:true}).click()
  const intake=admin.getByRole('dialog',{name:'Device intake',exact:true})
  await intake.locator('input[type=file]').first().setInputFiles({name:'buyback-intake.png',mimeType:'image/png',buffer:Buffer.from(png,'base64')})
  await expect(intake.locator('.ant-upload-list-item')).toHaveCount(1)
  const photoURL=await intake.locator('.ant-upload-list-item-name').getAttribute('href'),photo=await admin.request.get(origin+photoURL)
  assert.equal(photo.status(),200);assert.equal(photo.headers()['content-type'],'image/jpeg')
  await intake.getByLabel('Own warehouse',{exact:true}).press('ArrowDown')
  await admin.getByText(sourceName+' ('+sourceName+')',{exact:true}).last().click()
  await intake.getByLabel('SN',{exact:true}).fill('SN-JOINT');await intake.getByLabel('IMEI',{exact:true}).fill('000000000000001')
  await intake.getByLabel('Device category',{exact:true}).press('ArrowDown');await admin.getByText('Recycled',{exact:true}).last().click()
  const reply=admin.waitForResponse(r=>r.request().method()==='POST'&&r.url().endsWith('/device-intake'))
  await intake.getByRole('button',{name:'Submit intake batch',exact:true}).click()
  const saved=await reply;assert.equal(saved.status(),200);const devices=(await saved.json()).devices
  assert.equal(devices.length,1);assert.equal(devices[0].state,'inspection');assert.equal(devices[0].location_id,source.location_id)
  await expect(intake.getByText('Devices registered',{exact:true})).toBeVisible()
  return {actual_ui_recycled_intake:true,sql_seeded_devices:false,intake_device_id:devices[0].id,intake_source_id:source.id,intake_location_id:source.location_id,intake_photo_url:photoURL}
 }finally{await admin.close()}
}
