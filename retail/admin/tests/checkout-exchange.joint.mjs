import {submitCheckoutRefund} from './checkout-refund.joint.mjs'
import assert from 'node:assert/strict'
import {expect} from '@playwright/test'
import {fileURLToPath} from 'node:url'
import {lostReply} from './checkout-repair.joint.mjs'

export async function exchangeCheckoutOrder({page,origin,orderID,request,result,index,pack,warehouse,parcel,retryLost,settlePayment,dispatchReplacement,expectedDifference}){
 await page.goto(origin+'/orders/'+orderID)
 const panel=page.getByTestId('order-after-sales');await expect(panel.getByText('Pending',{exact:true})).toBeVisible()
 await panel.getByRole('button',{name:'Save',exact:true}).click();await page.getByRole('button',{name:'Confirm update',exact:true}).click();await expect(panel.locator('.ant-tag')).toHaveText('Processing')
 await panel.getByRole('button',{name:'Receive device',exact:true}).click();const receipt=page.getByRole('dialog')
 for(const label of ['Originally shipped device','Receiving source / warehouse']){const input=receipt.getByLabel(label,{exact:true});await input.focus();await input.press('ArrowDown');await input.press('Enter')}
 await receipt.getByLabel('Scan serial number / IMEI / EID',{exact:true}).fill(result.sn)
 await lostReply(page,'**/after-sales/'+request.id+'/device-returns',receipt,()=>receipt.getByRole('button',{name:'Confirm device receipt',exact:true}).click(),'Retry this receipt',()=>expect(receipt.getByText('Device received',{exact:true})).toBeVisible())
 await receipt.getByRole('button',{name:'Service device',exact:true}).click();let service=page.getByRole('dialog').last()
 await service.getByRole('button',{name:'Assign replacement device',exact:true}).click();const replacement=page.getByRole('dialog').last(),sn='CHECKOUT-DEVICE-'+(index+4),tracking='000-CHECKOUT-EXCHANGE-'+index
 const candidate=replacement.getByRole('combobox',{name:'Replacement device',exact:true});await candidate.fill(sn);await expect(page.locator('.ant-select-item-option').filter({hasText:sn})).toHaveCount(1);await candidate.press('ArrowDown');await candidate.press('Enter')
 const native=await lostReply(page,'**/after-sales/'+request.id+'/devices/*/replacement',replacement,()=>replacement.getByRole('button',{name:'Confirm replacement',exact:true}).click(),'Retry this replacement',()=>expect(replacement.getByText('Replacement device assigned',{exact:true})).toBeVisible())
 const difference=expectedDifference??(process.env.CHECKOUT_EXCHANGE_PAYMENT==='1'?'6':process.env.CHECKOUT_EXCHANGE_REFUND==='1'?'-12':'0')
 assert.equal(native.pending_difference,difference);assert.ok(native.exchange_id);assert.ok(native.item_id)
 await expect(replacement.getByText('Pending order difference',{exact:true}).locator('..')).toContainText(difference)
 if(settlePayment){
  await page.reload();await page.getByRole('button',{name:'Create device parcel',exact:true}).click();const unpaid=page.getByRole('dialog')
  await expect(unpaid.getByRole('button',{name:'Pack and deduct stock',exact:true})).toBeDisabled()
  await expect(unpaid.getByRole('row').filter({has:page.getByText(sn,{exact:true})}).getByRole('checkbox')).toBeDisabled()
  const rejected=await page.evaluate(async order=>{
   const api=await import('/src/api.ts'),view=await api.readOrderPacking(order,new AbortController().signal)
   if(view.paid||view.devices.length!==1)throw Error('Expected unpaid replacement packing context')
   try{await api.saveOrderPacking(order,{operation_id:crypto.randomUUID(),expected_revision:view.revision,action:'reserve',ids:[view.devices[0].assignment_id]});return 200}catch(e){return e.status}
  },orderID)
  assert.equal(rejected,409)
  await unpaid.getByRole('button',{name:'Cancel',exact:true}).click()
  await settlePayment(native)
 }
 await page.reload();let shipped
 if(dispatchReplacement){shipped=await dispatchReplacement(sn,tracking)}else{
 const packed=await pack(sn);shipped=packed;const d=await parcel(shipped),shipping=d.getByTestId('warehouse-shipment')
 await shipping.getByLabel('Tracking number',{exact:true}).fill(tracking);await shipping.getByLabel('Tracking URL',{exact:true}).fill('https://tracking.example.invalid/'+tracking)
 await retryLost(warehouse,'**/shipment',()=>shipping.getByRole('button',{name:'Ship entire parcel',exact:true}).click(),shipping,()=>expect(d.getByText(tracking,{exact:true})).toBeVisible())
 await expect(shipping.getByRole('button',{name:'Ship entire parcel',exact:true})).toBeDisabled()
 }
 // Inspect the physically received original independently; no quantity restock
 // or additional payment is manufactured by the rehearsal.
 await page.reload();await panel.getByRole('button',{name:'Receive device',exact:true}).click();await page.getByRole('dialog').getByRole('button',{name:'Service device',exact:true}).click();service=page.getByRole('dialog').last()
 await service.getByLabel('Actual battery health',{exact:true}).fill('90');await service.getByLabel('Inspection notes',{exact:true}).fill('Private original-device exchange inspection')
 await service.locator('input[type="file"]').first().setInputFiles({name:'exchange-inspection.png',mimeType:'image/png',buffer:Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jB9kAAAAASUVORK5CYII=','base64')})
 await expect(service.locator('.ant-upload-list-item-done')).toHaveCount(1)
 await lostReply(page,'**/after-sales/'+request.id+'/devices/*/service',service,()=>service.getByRole('button',{name:'Save inspection',exact:true}).click(),'Retry this operation',async()=>{await expect(service.getByText('Device operation saved',{exact:true})).toBeVisible();await expect(service.getByText('available',{exact:true})).toBeVisible()})
 if(process.env.CHECKOUT_EXCHANGE_REFUND==='1'){
  await page.reload();await panel.locator('.ant-select-selector').click();await page.locator('.ant-select-item-option[title="Completed"]').click()
  const denied=page.waitForResponse(r=>r.request().method()==='POST'&&new URL(r.url()).pathname.endsWith('/after-sales'))
  await panel.getByRole('button',{name:'Save',exact:true}).click();await page.getByRole('button',{name:'Confirm update',exact:true}).click();assert.equal((await denied).status(),409);await expect(panel.locator('.ant-tag')).toHaveText('Processing')
  await submitCheckoutRefund({page,orderID,request,index,amount:'12.00'})
 }
 const refreshed=page.waitForResponse(r=>r.request().method()==='GET'&&new URL(r.url()).pathname.endsWith('/after-sales')).then(r=>r.json())
 await page.reload();await refreshed;await expect(panel.locator('.ant-tag')).toHaveText('Processing')
 await panel.locator('.ant-select-selector').click();await page.locator('.ant-select-item-option[title="Completed"]').click();await expect(panel.locator('.ant-select-selection-item')).toHaveText('Completed')
 const completed=page.waitForResponse(r=>r.request().method()==='POST'&&new URL(r.url()).pathname.endsWith('/after-sales'))
 await panel.getByRole('button',{name:'Save',exact:true}).click();await page.getByRole('button',{name:'Confirm update',exact:true}).click()
 const response=await completed;assert.equal(response.request().postDataJSON().status,'completed');assert.equal(response.status(),200);assert.equal((await response.json()).requests.find(r=>r.id===request.id).status,'completed')
 await expect(panel.locator('.ant-tag')).toHaveText('Completed')
 return {sn,tracking,shipped,...native}
}

export async function verifyCheckoutExchange({page,context,shop,orderID,result,request,replacement,segment,expectedUnread}){
 await page.reload();await expect(page.getByTestId('shipment')).toHaveCount(2)
 for(const tracking of [result.tracking,replacement.tracking]){const shipment=page.getByTestId('shipment').filter({hasText:tracking});await expect(shipment).toBeVisible();await expect(shipment.locator('a')).toHaveAttribute('href','https://tracking.example.invalid/'+tracking)}
 const section=page.getByTestId('after-sales-section');await section.getByRole('button').first().click();await expect(section).toContainText(request.copy.completedRequest);await expect(section.getByTestId('repair-return')).toHaveCount(0)
 const raw=await (await context.request.get(shop+'/api/store/customer/orders/'+orderID+'/devices')).json();assert.equal(raw.devices.length,2)
 const original=raw.devices.find(d=>d.serial_number===result.sn),next=raw.devices.find(d=>d.serial_number===replacement.sn)
 assert.equal(original.status,'returned');assert.equal(original.tracking_number,result.tracking);assert.equal(next.status,'shipped');assert.equal(next.replacement_of,original.assignment_id);assert.equal(next.tracking_number,replacement.tracking);assert.equal(next.item_id,replacement.item_id)
 await section.getByRole('checkbox').first().check();await expect(section.getByText(new RegExp(replacement.sn))).toBeVisible();await expect(section.getByText(new RegExp(result.sn))).toHaveCount(0)
 const notices=await (await context.request.get(shop+'/api/store/customer/notifications?offset=0')).json();const credit=process.env.CHECKOUT_EXCHANGE_REFUND==='1',payment=process.env.CHECKOUT_EXCHANGE_PAYMENT==='1';assert.equal(notices.count,credit||payment?5:4);assert.deepEqual(notices.notifications.map(n=>n.kind).sort(),['after_sales.completed','after_sales.processing',...(credit?['refund.created']:[]),...(payment?['payment.received']:[]),'shipment.created','shipment.created']);if(credit)await expect(page.getByTestId('timeline-refund')).toHaveCount(1)
 await expect(page.getByTestId('notification-badge')).toHaveText(expectedUnread===undefined?(credit||payment?'4':'3'):String(expectedUnread));await page.getByTestId('notification-bell').click();const zh=segment.endsWith('-zh'),dialog=page.getByRole('dialog')
 await dialog.getByRole('button',{name:zh?'全部已读':'Mark all as read',exact:true}).click();await expect(page.getByTestId('notification-badge')).toHaveCount(0);await dialog.getByRole('button',{name:zh?'关闭消息通知':'Close notifications',exact:true}).click()
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
 await page.getByTestId('shipment').last().scrollIntoViewIfNeeded();await page.screenshot({path:fileURLToPath(new URL('../../../logs/gin-migration/'+(payment?'exchange-payment':credit?'exchange-refund':'exchange')+'-customer-'+(zh?'zh':'en')+'-'+page.viewportSize().width+'.png',import.meta.url))})
}
