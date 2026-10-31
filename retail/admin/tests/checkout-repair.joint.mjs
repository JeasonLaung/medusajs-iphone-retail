import assert from 'node:assert/strict'
import {expect} from '@playwright/test'
import {readFileSync} from 'node:fs'
import {fileURLToPath} from 'node:url'

async function lostReply(page,pattern,root,submit,retry,complete,status=200){
 const requests=[];let body
 const retryName=retry==='重试'?/重\s*试$/:new RegExp(retry.replace(/[.*+?^${}()|[\]\\]/g,'\\$&')+'$')
 await page.route(pattern,async route=>{
  if(route.request().method()!=='POST'){await route.continue();return}
  requests.push(route.request().postDataJSON());const r=await route.fetch();assert.equal(r.status(),status);body=await r.json()
  if(requests.length===1)await route.abort('failed');else await route.fulfill({response:r})
 })
 try{
  await submit();const retryButton=root.getByRole('button',{name:retryName});await expect(retryButton).toBeVisible();await expect(retryButton).toBeEnabled();await retryButton.click();await complete()
  assert.equal(requests.length,2);assert.deepEqual(requests[0],requests[1]);return body
 }finally{await page.unrouteAll({behavior:'wait'})}
}
export async function enableCheckoutTracking(page,origin){
 await page.goto(origin+'/products/add-product')
 await page.getByRole('row').filter({has:page.getByRole('cell',{name:'00123456789012345678',exact:true})}).getByRole('button',{name:'Device tracking',exact:true}).click()
 const d=page.getByRole('dialog');await d.getByRole('button',{name:'Enable individual device tracking',exact:true}).click();await expect(d.getByText('Enabled',{exact:true})).toBeVisible()
}
export async function requestCheckoutRepair({page,orderID,result,segment}){
 const copy=JSON.parse(readFileSync(new URL(`../../storefront/src/components/account/translations/${segment.endsWith('-zh')?'zh':'en'}.json`,import.meta.url)))
 const section=page.getByTestId('after-sales-section')
 await section.getByRole('button').first().click()
 await section.locator('.ant-select-selector').click();await page.locator('.ant-select-item-option').filter({hasText:copy.repairType}).click()
 await section.getByRole('checkbox').first().check()
 await section.locator('label').filter({hasText:result.sn}).getByRole('checkbox').check()
 await section.getByRole('textbox').last().fill('Original device needs warranty repair after delivery.')
 const response=await lostReply(page,'**/api/store/customer/orders/'+orderID+'/after-sales',section,()=>section.getByRole('button',{name:copy.submitRequest,exact:true}).click(),copy.retry,()=>expect(section).toContainText(copy.requestSuccess),201)
 assert.equal(response.request.type,'repair');assert.equal(response.request.items.length,1);assert.equal(response.request.items[0].device_ids.length,1)
 await page.reload();await section.getByRole('button').first().click();await expect(section).toContainText(copy.openRequest)
 return {id:response.request.id,copy}
}
export async function repairCheckoutOrder({page,origin,orderID,request,result,index}){
 await page.goto(origin+'/orders/'+orderID)
 const panel=page.getByTestId('order-after-sales');await expect(panel.getByText('Pending',{exact:true})).toBeVisible()
 await panel.getByRole('button',{name:'Save',exact:true}).click();await page.getByRole('button',{name:'Confirm update',exact:true}).click();await expect(panel.locator('.ant-tag')).toHaveText('Processing')
 await panel.getByRole('button',{name:'Receive device',exact:true}).click();const receipt=page.getByRole('dialog')
 for(const label of ['Originally shipped device','Receiving source / warehouse']){const input=receipt.getByLabel(label,{exact:true});await input.focus();await input.press('ArrowDown');await input.press('Enter')}
 await receipt.getByLabel('Scan serial number / IMEI / EID',{exact:true}).fill(result.sn)
 await lostReply(page,'**/after-sales/'+request.id+'/device-returns',receipt,()=>receipt.getByRole('button',{name:'Confirm device receipt',exact:true}).click(),'Retry this receipt',()=>expect(receipt.getByText('Device received',{exact:true})).toBeVisible())
 await receipt.getByRole('button',{name:'Service device',exact:true}).click();const service=page.getByRole('dialog').last()
 await expect(service.getByText('repair',{exact:true})).toBeVisible()
 await service.getByLabel('Actual battery health',{exact:true}).fill('90');await service.getByLabel('Inspection notes',{exact:true}).fill('Original device repaired with private inspection evidence')
 await service.locator('input[type="file"]').first().setInputFiles({name:'inspection.png',mimeType:'image/png',buffer:Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jB9kAAAAASUVORK5CYII=','base64')})
 await expect(service.locator('.ant-upload-list-item-done')).toHaveCount(1)
 const path='**/after-sales/'+request.id+'/devices/*/service'
 await lostReply(page,path,service,()=>service.getByRole('button',{name:'Save inspection',exact:true}).click(),'Retry this operation',async()=>{await expect(service.getByText('Device operation saved',{exact:true})).toBeVisible();await expect(service.getByText('repair_ready',{exact:true})).toBeVisible()})
 const refreshed=page.waitForResponse(r=>r.request().method()==='GET'&&r.url().includes('/after-sales/'+request.id+'/devices/')&&r.url().endsWith('/service')&&r.status()===200)
 await service.getByRole('button',{name:'Reload',exact:true}).click();await (await refreshed).finished();await expect(service.getByRole('button',{name:'Dispatch repaired device',exact:true})).toBeEnabled()
 const tracking='000-CHECKOUT-REPAIR-'+index
 await service.getByRole('textbox',{name:'Return tracking number',exact:true}).fill(tracking)
 await lostReply(page,path,service,()=>service.getByRole('button',{name:'Dispatch repaired device',exact:true}).click(),'Retry this operation',()=>expect(service.getByText(tracking,{exact:true})).toBeVisible())
 await page.reload();await expect(panel.locator('.ant-tag')).toHaveText('Processing');await panel.locator('.ant-select-selector').click();await page.locator('.ant-select-item-option[title="Completed"]').click();await panel.getByRole('button',{name:'Save',exact:true}).click();await page.getByRole('button',{name:'Confirm update',exact:true}).click();await expect(panel.locator('.ant-tag')).toHaveText('Completed')
 return tracking
}
export async function verifyCheckoutRepair({page,context,shop,orderID,result,request,tracking}){
 await page.reload();const section=page.getByTestId('after-sales-section');await section.getByRole('button').first().click()
 await expect(section).toContainText(request.copy.completedRequest);await expect(section.getByTestId('repair-return')).toHaveCount(1);await expect(section.getByTestId('repair-return')).toContainText(tracking);await expect(section.getByTestId('repair-return')).toContainText(result.sn)
 // Original sales shipment remains a separate immutable fulfillment.
 await expect(page.getByTestId('shipment')).toHaveCount(1);await expect(page.getByTestId('shipment')).toContainText(result.tracking)
 const response=await context.request.get(shop+'/api/store/customer/orders/'+orderID+'/after-sales');assert.equal(response.status(),200);const raw=await response.json();assert.equal(raw.requests.length,1);assert.equal(raw.requests[0].repair_shipments.length,1);assert.equal(raw.requests[0].repair_shipments[0].tracking_number,tracking)
 assert.ok(!JSON.stringify(raw).includes('private inspection'));assert.ok(!JSON.stringify(raw).includes('photos'));assert.ok(!JSON.stringify(raw).includes('actor_id'))
 await expect(page.getByTestId('notification-badge')).toHaveText('2');await page.getByTestId('notification-bell').click()
 const notices=await (await context.request.get(shop+'/api/store/customer/notifications?offset=0')).json();assert.equal(notices.count,3);assert.deepEqual(notices.notifications.map(n=>n.kind).sort(),['after_sales.completed','after_sales.processing','shipment.created'])
 const zh=request.copy.retry==='重试',dialog=page.getByRole('dialog');await dialog.getByRole('button',{name:zh?'全部已读':'Mark all as read',exact:true}).click();await expect(page.getByTestId('notification-badge')).toHaveCount(0);await dialog.getByRole('button',{name:zh?'关闭消息通知':'Close notifications',exact:true}).click()
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
 await section.getByTestId('repair-return').scrollIntoViewIfNeeded()
 await page.screenshot({path:fileURLToPath(new URL('../../../logs/gin-migration/repair-customer-'+(zh?'zh':'en')+'-'+page.viewportSize().width+'.png',import.meta.url))})
}
