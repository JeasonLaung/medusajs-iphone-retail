import assert from 'node:assert/strict'
import {fileURLToPath} from 'node:url'
import {requestCheckoutAfterSales} from '../../admin/tests/checkout-repair.joint.mjs'

// Continue the same registered customer's paid exchange, after the replacement
// actually shipped. Only the hosted provider and test mail delivery are fixtures.
export async function refundCheckoutSupplement({page,context,shop,orderID,replacement,segment,index,fulfiller,expect}){
 await page.reload()
 const paymentResponse=await context.request.get(shop+'/api/store/customer/orders/'+orderID+'/payments')
 assert.equal(paymentResponse.status(),200)
 const payment=await paymentResponse.json()
 const paid=payment.payments.find(o=>o.applied&&o.state==='paid')
 assert.ok(paid);assert.equal(paid.amount,'6.00')
 // The exchange verifier leaves the request form open with the replacement
 // selected; reload restores the normal collapsed section before a new request.
 const request=await requestCheckoutAfterSales({page,orderID,result:replacement,segment,type:'return'})
 const accepted=await fulfiller.refund(orderID,request,replacement,index,{amount:'6.00',expectCompletionConflict:true,paymentID:'pay_supp_'+paid.operation_id})
 assert.equal(accepted.payment_id,'pay_supp_'+paid.operation_id)
 await page.reload()
 const section=page.getByTestId('after-sales-section');await section.getByRole('button').first().click()
 const rawResponse=await context.request.get(shop+'/api/store/customer/orders/'+orderID+'/after-sales');assert.equal(rawResponse.status(),200)
 const raw=await rawResponse.json();assert.equal(raw.requests.length,2);assert.equal(raw.requests.find(r=>r.type==='exchange').status,'completed');assert.equal(raw.requests.find(r=>r.id===request.id).type,'return');assert.equal(raw.requests.find(r=>r.id===request.id).status,'processing')
 const deviceResponse=await context.request.get(shop+'/api/store/customer/orders/'+orderID+'/devices');assert.equal(deviceResponse.status(),200)
 const devices=await deviceResponse.json();assert.equal(devices.devices.length,2);assert.ok(devices.devices.every(d=>d.status==='returned'));assert.equal(devices.devices.find(d=>d.serial_number===replacement.sn).tracking_number,replacement.tracking)
 await expect(page.getByTestId('shipment')).toHaveCount(2);await expect(page.getByTestId('timeline-refund')).toHaveCount(1)
 const noticeResponse=await context.request.get(shop+'/api/store/customer/notifications?offset=0');assert.equal(noticeResponse.status(),200)
 const notices=await noticeResponse.json();assert.equal(notices.count,7);assert.deepEqual(notices.notifications.map(n=>n.kind).sort(),['after_sales.completed','after_sales.processing','after_sales.processing','payment.received','refund.created','shipment.created','shipment.created'])
 const zh=segment.endsWith('-zh');await expect(page.getByTestId('notification-badge')).toHaveText('2');await page.getByTestId('notification-bell').click()
 const dialog=page.getByRole('dialog'),notice=dialog.getByRole('listitem').filter({hasText:zh?'您的退款已记账':'Your refund has been recorded'})
 await notice.getByRole('button',{name:zh?'查看详情':'View details',exact:true}).click();await expect(page).toHaveURL(shop+'/'+segment+'/account/orders/details/'+orderID);await expect(page.getByTestId('timeline-refund')).toBeVisible()
 await page.getByTestId('notification-bell').click();await dialog.getByRole('button',{name:zh?'全部已读':'Mark all as read',exact:true}).click();await expect(page.getByTestId('notification-badge')).toHaveCount(0);await dialog.getByRole('button',{name:zh?'关闭消息通知':'Close notifications',exact:true}).click()
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
 await page.getByTestId('timeline-refund').scrollIntoViewIfNeeded();await page.screenshot({path:fileURLToPath(new URL('../../../logs/gin-migration/supplement-refund-customer-'+(zh?'zh':'en')+'-'+page.viewportSize().width+'.png',import.meta.url))})
}
