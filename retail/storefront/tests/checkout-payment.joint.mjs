import assert from 'node:assert/strict'

// Same real registered customer and order. Only the external hosted page is replaced.
export async function settleCheckoutExchangePayment({page,context,shop,segment,orderID,cart,control,expect}){
 const zh=segment.endsWith('-zh'),inputs=[],pattern='**/api/store/customer/orders/'+orderID+'/payments'
 await page.reload();await expect(page.getByTestId('supplemental-prepare')).toBeVisible()
 await expect(page.getByTestId('supplemental-due')).toContainText('6')
 await expect(page.getByTestId('cart-total')).toHaveAttribute('data-value','164.75')
 await expect(page.getByTestId('payment-amount')).toContainText('158.75')
 await page.route(pattern,async route=>{
  if(route.request().method()!=='POST'){await route.continue();return}
  inputs.push(route.request().postDataJSON());const r=await route.fetch();assert.equal(r.status(),202)
  if(inputs.length===1)await route.abort('failed');else await route.fulfill({response:r})
 })
 try{
  await page.getByTestId('supplemental-prepare').click()
  const retry=page.getByRole('button',{name:zh?'重试同一请求':'Retry the same request',exact:true})
  await expect(retry).toBeVisible();await retry.click();await expect(page.getByTestId('supplemental-payment-receipt')).toHaveCount(1)
  assert.equal(inputs.length,2);assert.deepEqual(inputs[0],inputs[1]);assert.equal(inputs[0].amount,'6.00')
 }finally{await page.unroute(pattern)}
 assert.equal((await context.cookies(shop)).some(c=>c.name==='retail_cart_possession'||c.name==='retail_cart_csrf'),false)
 const operation=inputs[0].operation_id
 await page.reload();await expect(page.getByTestId('supplemental-prepare')).toHaveCount(0)
 await expect(page.getByTestId('supplemental-checkout-link')).toBeVisible({timeout:60000})
 const view=await (await context.request.get(shop+'/api/store/customer/orders/'+orderID+'/payments/'+operation)).json()
 assert.equal(view.payment.applied,false);assert.equal(view.payment.amount,'6.00')
 const returning=shop+'/api/mollie-return?cart_id='+cart.id+'&order_id='+orderID+'&order_payment_id='+operation,hosted='https://www.mollie.com/checkout/supplement/'+operation
 await page.route(hosted,route=>route.fulfill({contentType:'text/html',body:`<a href="${returning.replaceAll('&','&amp;')}">Return to shop</a>`}))
 await page.getByTestId('supplemental-checkout-link').click();await expect(page).toHaveURL(hosted)
 assert.equal((await control('supplement-paid/'+operation)).status,200)
 const writes=[];const observe=request=>{if(request.method()==='POST'&&new URL(request.url()).pathname.startsWith('/api/store/customer/'))writes.push(request.url())};page.on('request',observe)
 await page.getByRole('link',{name:'Return to shop',exact:true}).click()
 await expect(page).toHaveURL(new RegExp('/'+segment+'/account/orders/details/'+orderID+'#order-supplemental-payment'))
 await expect(page.getByTestId('supplemental-payment-receipt')).toHaveAttribute('data-applied','true',{timeout:60000})
 await expect(page.getByTestId('supplemental-payment-receipt')).toContainText(zh?'补款已收妥':'Additional payment received')
 await expect(page.getByTestId('payment-amount')).toContainText('158.75')
 const notices=await (await context.request.get(shop+'/api/store/customer/notifications?offset=0')).json()
 assert.equal(notices.notifications.filter(n=>n.kind==='payment.received').length,1)
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
 page.off('request',observe);assert.deepEqual(writes,[])
 await page.unroute(hosted)
}
