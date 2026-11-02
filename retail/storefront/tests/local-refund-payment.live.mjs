// Create a distinct sandbox payment for the refund subflow. Never refund the
// remote payment copied from the user's preview database.
import assert from 'node:assert/strict'
import {writeFileSync} from 'node:fs'
import {resolve} from 'node:path'
import {expect as baseExpect} from '../../admin/node_modules/@playwright/test/index.mjs'
const expect=baseExpect.configure({timeout:30000})

export async function prepareLocalRefundPayment({page,context,origin,email,logs}){
 assert.equal(origin,'http://127.0.0.1:19300')
 await page.goto(origin+'/gb-en/products/swappie-iphone-17e?v_id=variant_01M51RN7H7HPV9RH69T32QMD2Q')
 await page.getByTestId('add-product-button').click()
 await expect(page.getByTestId('checkout-button')).toBeVisible()
 await page.getByTestId('checkout-button').click()
 for(const [id,value] of Object.entries({'shipping-first-name-input':'Isolated','shipping-last-name-input':'Refund','shipping-email-input':email,'confirm-email-input':email,'shipping-phone-input':'+441234567890','shipping-address-input':'1 Sandbox Street','shipping-postal-code-input':'SW1A 1AA','shipping-city-input':'London'}))await page.getByTestId(id).fill(value)
 await page.getByTestId('submit-address-button').click()
 await expect(page).toHaveURL(/step=delivery/)
 await page.getByRole('radio').first().check()
 await page.getByTestId('submit-delivery-option-button').click()
 await expect(page).toHaveURL(/step=payment/)
 await page.getByTestId('submit-payment-button').click()
 await expect(page.getByTestId('submit-order-button')).toBeEnabled({timeout:60000})
 // Retain the real checkout's cookies and local payment intent for recovery.
 await context.storageState({path:resolve(logs,'refund-payment-state.json')})
 writeFileSync(resolve(logs,'refund-payment-localstorage.json'),JSON.stringify(await page.evaluate(()=>({...localStorage}))))
 await page.getByTestId('submit-order-button').click()
 await page.waitForURL(url=>url.hostname.endsWith('mollie.com'),{timeout:60000})
 writeFileSync(resolve(logs,'refund-hosted-url.txt'),page.url())
 await page.getByRole('button',{name:/PayPal/}).click({timeout:60000})
 await page.locator('input[name="final_state"][value="paid"]').check({timeout:60000})
 await page.getByRole('button',{name:/Continue/}).click()
 await page.waitForURL(origin+'/gb-en/order/**/confirmed',{timeout:90000})
 await expect(page.getByTestId('order-complete-container')).toBeVisible()
 const orderID=new URL(page.url()).pathname.split('/').at(-2)
 assert.match(orderID,/^order_[0-9a-f-]{36}$/)
 assert.notEqual(orderID,'order_8bffc6dc-112e-4ed5-8a32-57421bc2ae4d')
 writeFileSync(resolve(logs,'refund-new-paid-order.json'),JSON.stringify({order_id:orderID,real_mollie_test_payment:true,logged_in_payment:true}))
 // Actual account ownership must be attached by checkout, without fixture SQL.
 await expect.poll(async()=>(await context.request.get(origin+'/api/store/customer/orders/'+orderID)).status()).toBe(200)
 await page.goto(origin+'/gb-en/account/orders/details/'+orderID)
 await expect(page.getByTestId('order-details-container')).toBeVisible()
 return orderID
}

export async function resumeLocalRefund({admin,page,context,origin,orderID,request,operationID}){
 const panel=admin.getByTestId('order-after-sales').locator(`tr[data-row-key="${request.id}"]`)
 await panel.getByRole('button',{name:'Refund history and actions',exact:true}).click()
 const drawer=admin.getByRole('dialog')
 await expect(drawer.getByText('789.00',{exact:false}).first()).toBeVisible()
 const read=async()=>{
  const response=await admin.request.get(`http://127.0.0.1:19200/api/admin/orders/${orderID}/after-sales/${request.id}/refunds`)
  assert.equal(response.status(),200)
  const body=await response.json();assert.equal(body.operations.length,1)
  const operation=body.operations[0];assert.equal(operation.operation_id,operationID)
  assert.equal(operation.amount,'789.00')
  return operation
 }
 // The separate worker restores the durable instruction. This function never
 // submits another refund or changes a SQL/provider state to force success.
 let operation=await read()
 if(!operation.applied){
  await page.waitForTimeout(12000)
  operation=await read()
 }
 const posted=operation.state==='refunded'&&operation.applied
 if(posted)await expect(drawer.getByText('Posted',{exact:true})).toBeVisible()
 else{
  assert.ok(['pending','queued','processing','uncertain'].includes(operation.state))
  assert.equal(operation.applied,false)
  await expect(drawer.getByRole('button',{name:'Submit refund instruction',exact:true})).toBeDisabled()
 }
 await drawer.locator('.ant-drawer-close').click()
 await panel.locator('.ant-select-selector').click();await admin.locator('.ant-select-item-option[title="Completed"]').click()
 await panel.getByRole('button',{name:'Save',exact:true}).click()
 const reply=admin.waitForResponse(r=>r.request().method()==='POST'&&r.url().endsWith('/orders/'+orderID+'/after-sales'))
 await admin.getByRole('button',{name:'Confirm update',exact:true}).click()
 assert.equal((await reply).status(),posted?200:409)
 await expect(panel.locator('.ant-tag')).toHaveText(posted?'Completed':'Processing')
 if(!posted){
  await page.goto(origin+'/gb-en/account/orders/details/'+orderID)
  await expect(page.getByTestId('order-details-container')).toBeVisible()
  await expect(page.getByTestId('timeline-refund')).toHaveCount(0)
  const response=await context.request.get(origin+'/api/store/customer/orders/'+orderID+'/after-sales')
  assert.equal(response.status(),200)
  assert.equal((await response.json()).requests.find(r=>r.id===request.id).status,'processing')
 }
 return {completed:posted,state:operation.state}
}
