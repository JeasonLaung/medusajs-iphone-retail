import assert from 'node:assert/strict'
import {writeFileSync} from 'node:fs'
import {resolve} from 'node:path'
import {expect as baseExpect} from '../../admin/node_modules/@playwright/test/index.mjs'
const expect=baseExpect.configure({timeout:30000})

export async function settleLocalSupplement({page,context,origin,orderID,logs}){
 assert.equal(origin,'http://127.0.0.1:19300')
 await page.goto(origin+'/gb-en/account/orders/details/'+orderID)
 await expect(page.getByTestId('supplemental-prepare')).toBeVisible()
 const requests=[],pattern='**/api/store/customer/orders/'+orderID+'/payments'
 await page.route(pattern,async route=>{
  if(route.request().method()!=='POST'){await route.continue();return}
  requests.push(route.request().postDataJSON());const response=await route.fetch();assert.equal(response.status(),202)
  if(requests.length===1)await route.abort('failed');else await route.fulfill({response})
 })
 try{
  await page.getByTestId('supplemental-prepare').click()
  await page.getByRole('button',{name:'Retry the same request',exact:true}).click()
  await expect(page.getByTestId('supplemental-payment-receipt')).toHaveCount(1)
  assert.equal(requests.length,2);assert.deepEqual(requests[0],requests[1]);assert.equal(requests[0].amount,'7.79')
 }finally{await page.unrouteAll({behavior:'wait'})}
 writeFileSync(resolve(logs,'supplement-operation.json'),JSON.stringify({order_id:orderID,operation_id:requests[0].operation_id,amount:'7.79'}))
 await page.reload()
 // Independent production worker creates and observes the hosted payment.
 await expect(page.getByTestId('supplemental-checkout-link')).toBeVisible({timeout:90000})
 await page.getByTestId('supplemental-checkout-link').click()
 await page.waitForURL(url=>url.hostname.endsWith('mollie.com'),{timeout:60000})
 await page.getByRole('button',{name:/PayPal/}).click({timeout:60000})
 await page.locator('input[name="final_state"][value="paid"]').check({timeout:60000})
 await page.getByRole('button',{name:/Continue/}).click()
 await page.waitForURL(url=>url.origin===origin&&url.pathname==='/gb-en/account/orders/details/'+orderID,{timeout:90000})
 await expect(page.getByTestId('supplemental-payment-receipt')).toHaveAttribute('data-applied','true',{timeout:90000})
 const r=await context.request.get(origin+'/api/store/customer/orders/'+orderID+'/payments');assert.equal(r.status(),200)
 const view=await r.json();assert.equal(view.payments.length,1);assert.equal(view.payments[0].amount,'7.79');assert.equal(view.payments[0].applied,true);assert.equal(view.pending_difference,'0')
 writeFileSync(resolve(logs,'supplement-paid.json'),JSON.stringify({passed:true,operation_id:requests[0].operation_id,order_id:orderID,real_mollie_test_payment:true,lost_reply_same_operation:true,worker_applied:true}))
}
