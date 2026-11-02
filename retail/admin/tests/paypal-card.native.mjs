import assert from 'node:assert/strict'
import {writeFile} from 'node:fs/promises'
import {join} from 'node:path'
import {chromium,expect} from '@playwright/test'

const origin=process.env.CONTEXT_ASTRO_ORIGIN,api=process.env.CONTEXT_API_ORIGIN,key=process.env.CONTEXT_FIXTURE_KEY,output=process.env.CONTEXT_BROWSER_OUTPUT
assert(/^http:\/\/127\.0\.0\.1:\d+$/.test(origin)&&/^http:\/\/127\.0\.0\.1:\d+$/.test(api)&&key?.length===64&&output)
const browser=await chromium.launch({headless:true,...(process.env.CONTEXT_BROWSER_EXECUTABLE?{executablePath:process.env.CONTEXT_BROWSER_EXECUTABLE}:{})})
const cases=[],errors=[],paymentKey='retail:checkout-payment-intent:v1'
async function control(path){const r=await fetch(api+'/__fixture/'+path,{method:path==='facts'?'GET':'POST',headers:{'X-Fixture-Key':key}});assert.equal(r.status,200);return ['facts','new'].includes(path)?r.json():null}
try{
 for(const [lang,width] of [['en',390],['en',1440],['zh',390],['zh',1440]]){
  const seed=await control('new'),context=await browser.newContext({viewport:{width,height:1000}}),page=await context.newPage(),writes=[]
  page.on('pageerror',e=>errors.push(e.message))
  await context.addCookies([{name:'retail_cart_possession',value:seed.token,url:origin,sameSite:'Strict'},{name:'retail_cart_csrf',value:seed.csrf,url:origin,sameSite:'Strict'}])
  // Only the external SDK is simulated, using the same server order ID callback.
  await page.route('https://www.paypal.com/sdk/js?**',async route=>{
   const u=new URL(route.request().url());assert.equal(u.searchParams.get('client-id'),'artificial_client');assert.equal(u.searchParams.get('currency'),'EUR');assert.equal(u.searchParams.get('intent'),'capture');assert.equal(u.searchParams.get('components'),'card-fields')
   await route.fulfill({contentType:'application/javascript',body:`(()=>{const script=document.currentScript;if(script.dataset.clientToken!=='artificial.browser-token')throw Error('missing browser token');window[script.dataset.namespace]={CardFields(options){let valid=false;const field=name=>({async render(root){const input=document.createElement('input');input.placeholder=name;input.setAttribute('aria-label',name);input.oninput=()=>{valid=input.value==='valid'};root.append(input);if(name==='Card number'){for(const [label,callback] of [['Cancel',()=>options.onCancel()],['Wrong approval',()=>options.onApprove({orderID:'AAAAAAAAAAAAAAAAA'})]]){const b=document.createElement('button');b.textContent=label;b.onclick=callback;root.append(b)}}},async close(){}});return {isEligible:()=>true,NameField:()=>field('Cardholder name'),NumberField:()=>field('Card number'),ExpiryField:()=>field('Expiry date'),CVVField:()=>field('Security code'),getState:async()=>({isFormValid:valid}),async submit(input){if(!valid)throw Error('invalid form');if(input.billingAddress.countryCode!=='DK')throw Error('billing country');const orderID=await options.createOrder();await options.onApprove({orderID})}}}}})()`})
  })
  let lostCreate=false,lostSettle=false,lostComplete=false
  for(const [endpoint,kind] of [['payment-attempts','create'],['paypal-settlement','settle'],['complete','complete']])await page.route('**/'+endpoint,async route=>{
   if(route.request().method()!=='POST'){await route.continue();return}
   const request={kind,key:route.request().headers()['idempotency-key'],body:route.request().postDataJSON()},r=await route.fetch();assert.equal(r.status(),200,endpoint);writes.push(request);console.log(JSON.stringify({endpoint,status:r.status(),response:await r.json()}))
   const lost=kind==='create'?!lostCreate:kind==='settle'?!lostSettle:!lostComplete
   if(kind==='create')lostCreate=true;else if(kind==='settle')lostSettle=true;else lostComplete=true
   if(lost)await route.abort('failed');else await route.fulfill({response:r})
  })
  await page.goto(origin+'/dk-'+lang+'/checkout?step=payment')
  const beforeToken=await page.evaluate(async({id,csrf})=>{
   const statuses=[]
   for(const [target,body,auth] of [[id,{},csrf],[id,{amount:'1'},csrf],[id,{},'0'.repeat(64)],['cart_foreign',{},csrf]]){
    const r=await fetch(`/api/store/carts/${target}/paypal-client-token`,{method:'POST',headers:{'Content-Type':'application/json','X-CSRF-Token':auth},body:JSON.stringify(body)});statuses.push(r.status)
   }
   return statuses
  },{id:seed.cart_id,csrf:seed.csrf});assert.deepEqual(beforeToken,[409,400,403,404]);assert.equal((await control('facts')).tokens,0)
  await expect(page.getByTestId('payment-method-option')).toHaveCount(3)
  await page.getByRole('radio',{name:lang==='zh'?'借记卡 / 信用卡（PayPal）':'Debit / credit card (PayPal)',exact:true}).click()
  await page.getByTestId('submit-payment-button').click();await expect(page.getByTestId('payment-recover')).toBeVisible()
  const original=await page.evaluate(k=>JSON.parse(localStorage.getItem(k)),paymentKey);assert.equal(original.providerID,'pp_paypal_card_paypal_card');assert.equal(original.version,'4')
  await page.reload();await expect(page.getByTestId('paypal-card-submit')).toBeEnabled({timeout:15000})
  await page.screenshot({path:join(output,`paypal-card-form-${lang}-${width}.png`)})
  await page.getByRole('button',{name:'Cancel',exact:true}).click();await expect(page.getByTestId('paypal-card').getByRole('alert')).toBeVisible();assert.equal((await control('facts')).settlements,0)
  await page.getByRole('button',{name:'Wrong approval',exact:true}).click();assert.equal((await control('facts')).settlements,0)
  await page.getByTestId('paypal-card-submit').click();await expect(page.getByTestId('paypal-card').getByRole('alert')).toBeVisible();assert.equal((await control('facts')).settlements,0);await page.getByRole('textbox',{name:'Card number',exact:true}).fill('valid');await page.getByTestId('paypal-card-submit').click();await expect(page.getByTestId('paypal-card').getByRole('alert')).toBeVisible()
  assert.equal((await control('facts')).settlements,1)
  await page.getByTestId('paypal-reconcile').click();await expect(page).toHaveURL(/\/payment-result\?/)
  // Completion response was also lost. The result page must recover the receipt
  // of the now closed cart, rather than generate another payment or order.
  await expect.poll(()=>lostComplete).toBe(true);await page.reload()
  await expect(page.getByTestId('order-complete-container')).toBeVisible({timeout:20000})
  await expect(page.getByTestId('payment-method')).toHaveText(lang==='zh'?'借记卡 / 信用卡（PayPal）':'Debit / credit card (PayPal)')
  await expect(page.getByTestId('payment-amount')).toContainText('158.75')
  assert.equal(await page.evaluate(k=>localStorage.getItem(k),paymentKey),null)
  const facts=await control('facts');assert.equal(facts.tokens,1);assert.equal(facts.creates,1);assert.equal(facts.settlements,1);assert.equal(facts.orders,1);assert.equal(facts.receipts,1);assert.equal(facts.captures,1);assert.equal(facts.cash.ProviderID,'pp_paypal_card_paypal_card');assert.equal(facts.cash.CurrencyCode,'eur');assert.equal(Number(facts.cash.Amount),158.75)
  const create=writes.filter(w=>w.kind==='create');assert(create.length>=2);assert(create.every(w=>w.key===original.operationID&&JSON.stringify(w.body)===JSON.stringify(create[0].body)));assert.equal(create[0].body.provider_id,'pp_paypal_card_paypal_card')
  assert(writes.filter(w=>w.kind==='settle').every(w=>JSON.stringify(w.body)==='{}'))
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true)
  await page.screenshot({path:join(output,`paypal-card-${lang}-${width}.png`)})
  cases.push({lang,width,facts,writes,lost_create_settlement_completion:true,cancel_wrong_order_invalid_form_no_capture:true,external_sdk_provider_artificial:true});await control('case-done');await context.close()
 }
}finally{await browser.close();await writeFile(join(output,'browser-result.json'),JSON.stringify({cases,errors},null,2)+'\n')}
assert.deepEqual(errors,[])
