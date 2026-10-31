import assert from 'node:assert/strict'
import {expect} from '@playwright/test'
import {fileURLToPath} from 'node:url'
import {lostReply} from './checkout-repair.joint.mjs'

export async function refundCheckoutOrder({page,origin,orderID,request,result,index,amount="158.75",paymentID,expectCompletionConflict=false}){
 await page.goto(origin+'/orders/'+orderID)
 const panel=page.getByTestId('order-after-sales').locator('tr[data-row-key="'+request.id+'"]');await expect(panel.getByText('Pending',{exact:true})).toBeVisible()
 await panel.getByRole('button',{name:'Save',exact:true}).click();await page.getByRole('button',{name:'Confirm update',exact:true}).click();await expect(panel.locator('.ant-tag')).toHaveText('Processing')
 await panel.getByRole('button',{name:'Receive device',exact:true}).click();const receipt=page.getByRole('dialog')
 for(const label of ['Originally shipped device','Receiving source / warehouse']){const input=receipt.getByLabel(label,{exact:true});await input.focus();await input.press('ArrowDown');await input.press('Enter')}
 await receipt.getByLabel('Scan serial number / IMEI / EID',{exact:true}).fill(result.sn)
 await lostReply(page,'**/after-sales/'+request.id+'/device-returns',receipt,()=>receipt.getByRole('button',{name:'Confirm device receipt',exact:true}).click(),'Retry this receipt',()=>expect(receipt.getByText('Device received',{exact:true})).toBeVisible())
 await receipt.getByRole('button',{name:'Service device',exact:true}).click();const service=page.getByRole('dialog').last()
 await service.getByLabel('Actual battery health',{exact:true}).fill('90');await service.getByLabel('Inspection notes',{exact:true}).fill('Private original-device return refund inspection')
 await service.locator('input[type="file"]').first().setInputFiles({name:'return-inspection.png',mimeType:'image/png',buffer:Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jB9kAAAAASUVORK5CYII=','base64')})
 await expect(service.locator('.ant-upload-list-item-done')).toHaveCount(1)
 await lostReply(page,'**/after-sales/'+request.id+'/devices/*/service',service,()=>service.getByRole('button',{name:'Save inspection',exact:true}).click(),'Retry this operation',async()=>{await expect(service.getByText('Device operation saved',{exact:true})).toBeVisible();await expect(service.getByText('available',{exact:true})).toBeVisible()})
 const accepted=await submitCheckoutRefund({page,orderID,request,index,amount,paymentID})
 await page.reload();await panel.locator('.ant-select-selector').click();await page.locator('.ant-select-item-option[title="Completed"]').click();await panel.getByRole('button',{name:'Save',exact:true}).click();const completed=expectCompletionConflict?page.waitForResponse(r=>r.request().method()==='POST'&&r.url().endsWith('/orders/'+orderID+'/after-sales')):null
 await page.getByRole('button',{name:'Confirm update',exact:true}).click()
 if(completed){const denied=await completed;assert.equal(denied.status(),409);assert.equal((await denied.json()).error.code,'after_sales.unavailable');await expect(panel.locator('.ant-tag')).toHaveText('Processing')}
 else await expect(panel.locator('.ant-tag')).toHaveText('Completed')
 return accepted
}
export async function submitCheckoutRefund({page,orderID,request,index,amount,paymentID}){
 const panel=page.getByTestId('order-after-sales').locator('tr[data-row-key="'+request.id+'"]');await page.reload();await panel.getByRole('button',{name:'Refund history and actions',exact:true}).click();const drawer=page.getByRole('dialog')
 const payment=drawer.getByLabel('Original payment',{exact:true});await payment.focus();if(paymentID){await payment.click();await page.locator('.ant-select-item-option').filter({hasText:paymentID}).click()}else{await payment.press('ArrowDown');await payment.press('Enter')}
 await drawer.getByLabel('Refund amount (two decimal places)',{exact:true}).fill(amount)
 const accepted=await lostReply(page,'**/after-sales/'+request.id+'/refunds',drawer,()=>drawer.getByRole('button',{name:'Submit refund instruction',exact:true}).click(),'Retry same refund operation',()=>expect(drawer.getByText('Instruction accepted. Check its processing status.',{exact:true})).toBeVisible(),202)
 assert.equal(accepted.amount,amount)
 // The first real worker process loses the provider POST response. A fresh
 // process waits for its SQL lease and discovers it by GET; browser only reads.
 await expect(drawer.getByText('Posted',{exact:true})).toBeVisible({timeout:90000})
 await expect(drawer.getByRole('button',{name:'Submit refund instruction',exact:true})).toBeDisabled()
 await page.screenshot({path:fileURLToPath(new URL('../../../logs/gin-migration/'+(process.env.CHECKOUT_EXCHANGE_REFUND==='1'?'exchange-refund':'refund')+'-admin-joint-'+index+'.png',import.meta.url))})
 return accepted
}
export async function verifyCheckoutRefund({page,context,shop,orderID,result,request,segment}){
 await page.reload();const section=page.getByTestId('after-sales-section');await section.getByRole('button').first().click();await expect(section).toContainText(request.copy.completedRequest)
 await expect(page.getByTestId('timeline-refund')).toHaveCount(1);await expect(page.getByTestId('shipment')).toContainText(result.tracking)
 const raw=await (await context.request.get(shop+'/api/store/customer/orders/'+orderID+'/after-sales')).json();assert.equal(raw.requests.length,1);assert.equal(raw.requests[0].status,'completed');assert.ok(!JSON.stringify(raw).includes('photos'));assert.ok(!JSON.stringify(raw).includes('actor_id'))
 const notices=await (await context.request.get(shop+'/api/store/customer/notifications?offset=0')).json();assert.equal(notices.count,4);assert.deepEqual(notices.notifications.map(n=>n.kind).sort(),['after_sales.completed','after_sales.processing','refund.created','shipment.created']);assert.ok(notices.notifications.every(n=>n.resource_id===orderID))
 await expect(page.getByTestId('notification-badge')).toHaveText('3');await page.getByTestId('notification-bell').click();const zh=segment.endsWith('-zh'),dialog=page.getByRole('dialog')
 const refundNotice=dialog.getByRole('listitem').filter({hasText:zh?'您的退款已记账':'Your refund has been recorded'});await expect(refundNotice).toBeVisible()
 await refundNotice.getByRole('button',{name:zh?'查看详情':'View details',exact:true}).click();await expect(page).toHaveURL(shop+'/'+segment+'/account/orders/details/'+orderID);await expect(page.getByTestId('timeline-refund')).toBeVisible();await expect(page.getByTestId('notification-badge')).toHaveText('2');await page.getByTestId('notification-bell').click()
 await dialog.getByRole('button',{name:zh?'全部已读':'Mark all as read',exact:true}).click();await expect(page.getByTestId('notification-badge')).toHaveCount(0);await dialog.getByRole('button',{name:zh?'关闭消息通知':'Close notifications',exact:true}).click()
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
 await section.getByRole('button').first().click();await section.scrollIntoViewIfNeeded();await page.screenshot({path:fileURLToPath(new URL('../../../logs/gin-migration/refund-customer-'+(zh?'zh':'en')+'-'+page.viewportSize().width+'.png',import.meta.url))})
}
