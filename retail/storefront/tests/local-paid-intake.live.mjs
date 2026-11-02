import assert from 'node:assert/strict'

// Actual intake commands for the existing paid SKU. No SQL-created device/source.
export async function intakePaidWarehouseDevices({admin,origin,owner,orderID,expect}){
 assert.equal(origin,'http://127.0.0.1:19200')
 const response=await admin.request.get(origin+'/api/store/products?handle=swappie-iphone-17e')
 assert.equal(response.status(),200);const product=(await response.json()).products[0];assert.ok(product)
 const productID=product.id,variantID='variant_01M51RN7H7HPV9RH69T32QMD2Q',sourceName='Paid intake '+owner
 await admin.goto(origin+'/warehouses');await admin.getByRole('button',{name:'Device sources',exact:true}).click()
 let dialog=admin.getByRole('dialog');await dialog.getByRole('button',{name:'New device source',exact:true}).click()
 await dialog.getByLabel('Name',{exact:true}).fill(sourceName)
 const sourceReply=admin.waitForResponse(r=>r.request().method()==='POST'&&r.url().endsWith('/device-sources'))
 await dialog.getByRole('button',{name:'Save',exact:true}).click();const sourceResponse=await sourceReply;assert.equal(sourceResponse.status(),200)
 const source=(await sourceResponse.json()).source;await expect(dialog.getByRole('cell',{name:sourceName,exact:true})).toBeVisible()
 await admin.goto(origin+'/warehouses')
 await admin.getByRole('row').filter({hasText:source.location_id}).getByRole('button',{name:'Configure delivery',exact:true}).click()
 const delivery=admin.getByRole('dialog',{name:'Warehouse delivery sets',exact:true});await delivery.getByLabel('Delivery set',{exact:true}).click();await admin.getByText('European Warehouse delivery',{exact:true}).last().click()
 const commands=[];await admin.route('**/delivery-sets',async route=>{if(route.request().method()!=='POST')return route.continue();commands.push(route.request().postDataJSON());const response=await route.fetch();assert.equal(response.status(),200);if(commands.length===1)await route.abort('failed');else await route.fulfill({response})})
 await delivery.getByRole('button',{name:'Link delivery set',exact:true}).click();await delivery.getByRole('button',{name:'Retry this save',exact:true}).click();await expect(delivery.locator('.ant-tag')).toHaveText('European Warehouse delivery');assert.equal(commands.length,2);assert.deepEqual(commands[0],commands[1]);await admin.unrouteAll({behavior:'wait'})
 await admin.goto(origin+'/products/'+productID)
 const variantsResponse=await admin.request.get(origin+'/api/admin/products/'+productID+'/variants?limit=100&offset=0')
 assert.equal(variantsResponse.status(),200);const variant=(await variantsResponse.json()).variants.find(v=>v.id===variantID);assert.ok(variant)
 const row=admin.getByRole('row').filter({hasText:variant.sku});await row.getByRole('button',{name:'Device tracking',exact:true}).click()
 dialog=admin.getByRole('dialog');const enroll=dialog.getByRole('button',{name:'Enable individual device tracking',exact:true})
 if(await enroll.count()){await enroll.click();await expect(dialog.getByText('Enabled',{exact:true})).toBeVisible()}
 await dialog.getByRole('button',{name:'Device intake',exact:true}).click();const intake=admin.getByRole('dialog',{name:'Device intake',exact:true})
 const png=Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jB9kAAAAASUVORK5CYII=','base64')
 await intake.locator('input[type=file]').first().setInputFiles({name:'paid-intake.png',mimeType:'image/png',buffer:png})
 await expect(intake.locator('.ant-upload-list-item')).toHaveCount(1)
 const photoURL=await intake.locator('.ant-upload-list-item-name').getAttribute('href'),photo=await admin.request.get(origin+photoURL)
 assert.equal(photo.status(),200);assert.equal(photo.headers()['content-type'],'image/jpeg')
 for(let index=0;index<3;index++){
  if(index)await intake.getByRole('button',{name:'Add device',exact:true}).click()
  const warehouse=intake.getByLabel('Own warehouse',{exact:true}).nth(index);await warehouse.scrollIntoViewIfNeeded();await warehouse.press('ArrowDown');await admin.getByText(sourceName+' ('+sourceName+')',{exact:true}).last().click()
  await intake.getByLabel('SN',{exact:true}).nth(index).fill('NATIVE-PAID-INTAKE-00'+(index+1))
  await intake.getByLabel('IMEI',{exact:true}).nth(index).fill('00000000000008'+(index+1))
  if(index===2){const category=intake.getByLabel('Device category',{exact:true}).nth(index);await category.press('ArrowDown');await admin.getByText('Returned',{exact:true}).last().click()}
 }
 const intakeReply=admin.waitForResponse(r=>r.request().method()==='POST'&&r.url().endsWith('/device-intake'))
 await intake.getByRole('button',{name:'Submit intake batch',exact:true}).click();const intakeResponse=await intakeReply;assert.equal(intakeResponse.status(),200)
 const devices=(await intakeResponse.json()).devices;assert.equal(devices.length,3)
 assert.equal(devices[0].state,'available');assert.equal(devices[2].state,'inspection')
 await expect(intake.getByText('Devices registered',{exact:true})).toBeVisible()
 await admin.goto(origin+'/products/'+productID);await admin.getByRole('row').filter({hasText:variant.sku}).getByRole('button',{name:'View inventory',exact:true}).click()
 await admin.locator('.ant-collapse-header').first().click();await admin.getByRole('button',{name:'Add warehouse stock',exact:true}).click()
 const stock=admin.getByRole('dialog',{name:'Add warehouse stock',exact:true});await stock.getByLabel('Location',{exact:true}).click();await admin.locator('.ant-select-dropdown:not(.ant-select-dropdown-hidden)').getByText(new RegExp(sourceName)).click()
 await stock.getByLabel('Stocked quantity',{exact:true}).fill('2');await stock.getByRole('button',{name:'Save',exact:true}).click();await expect(stock).toHaveCount(0)
 await admin.goto(origin+'/orders/'+orderID)
 return {intake_device_id:devices[0].id,intake_spare_id:devices[1].id,intake_returned_id:devices[2].id,intake_location_id:source.location_id,intake_source_id:source.id,actual_ui_intake_three_devices:true,actual_ui_initial_stock:2,intake_photo_url:photoURL,sql_seeded_devices:false,delivery_link_lost_reply_same_command:true}
}
