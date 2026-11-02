import assert from 'node:assert/strict'
import {writeFile} from 'node:fs/promises'
import {join} from 'node:path'
import {chromium,expect} from '@playwright/test'

const origin=process.env.CONTEXT_ASTRO_ORIGIN,api=process.env.CONTEXT_API_ORIGIN,key=process.env.CONTEXT_FIXTURE_KEY,output=process.env.CONTEXT_BROWSER_OUTPUT
assert(/^http:\/\/127\.0\.0\.1:\d+$/.test(origin)&&/^http:\/\/127\.0\.0\.1:\d+$/.test(api)&&key?.length===64&&output)
const browser=await chromium.launch({headless:true,...(process.env.CONTEXT_BROWSER_EXECUTABLE?{executablePath:process.env.CONTEXT_BROWSER_EXECUTABLE}:{})})
const cases=[],errors=[],paymentKey='retail:checkout-payment-intent:v1',contextKey='retail:cart-context-intent:v1',transferKey='retail:customer-cart-transfer:v1'
async function control(path,body){const r=await fetch(api+'/__fixture/'+path,{method:path==='facts'?'GET':'POST',headers:{'X-Fixture-Key':key,'Content-Type':'application/json'},...(body?{body:JSON.stringify(body)}:{})});assert.equal(r.status,200,'fixture '+path);return ['facts','new','stage-paypal-context'].includes(path)?r.json():null}
try{
 for(const provider of ['pp_paypal_paypal','pp_paypal_card_paypal_card'])for(const scenario of ['save','restore','login'])for(const [lang,width] of [['en',390],['zh',1440]]){
  const seed=await control('new'),context=await browser.newContext({viewport:{width,height:1000}}),page=await context.newPage(),creates=[],writes=[]
  page.on('pageerror',e=>errors.push(e.message))
  await context.addCookies([{name:'retail_cart_possession',value:seed.token,url:origin,sameSite:'Strict'},{name:'retail_cart_csrf',value:seed.csrf,url:origin,sameSite:'Strict'}])
  await page.route('https://www.paypal.com/sdk/js?**',async route=>route.fulfill({contentType:'application/javascript',body:`(()=>{window[document.currentScript.dataset.namespace]={Buttons(){let el;return {isEligible:()=>true,async render(root){el=document.createElement('div');el.textContent='External PayPal wallet';root.append(el)},async close(){el?.remove()}}},CardFields(){const field=()=>({async render(root){const frame=document.createElement('iframe');frame.title='External PayPal card field';frame.style.cssText='width:100%;height:48px;border:0';root.append(frame)},async close(){}});return {isEligible:()=>true,getState:async()=>({isFormValid:false}),NameField:field,NumberField:field,ExpiryField:field,CVVField:field,async submit(){throw Error('must not pay in context subflow')}}}}})()`}))
  await page.route('**/payment-attempts',async route=>{creates.push({body:route.request().postDataJSON(),key:route.request().headers()['idempotency-key']});const r=await route.fetch();assert.equal(r.status(),200);await route.fulfill({response:r})})
  await page.goto(origin+'/dk-'+lang+'/checkout?step=payment')
  await expect(page.getByTestId('payment-method-option')).toHaveCount(3)
  const name=provider==='pp_paypal_paypal'?'PayPal':lang==='zh'?'借记卡 / 信用卡（PayPal）':'Debit / credit card (PayPal)'
  await page.getByRole('radio',{name,exact:true}).click();await page.getByTestId('submit-payment-button').click()
  await expect(page.getByTestId(provider==='pp_paypal_paypal'?'paypal-wallet':'paypal-card')).toBeVisible()
  if(provider==='pp_paypal_card_paypal_card')await expect(page.getByTestId('paypal-card-submit')).toBeEnabled()
  const original=await page.evaluate(k=>JSON.parse(localStorage.getItem(k)),paymentKey),before=await control('facts')
  assert.equal(original.providerID,provider);assert.equal(before.paypal_creates,1);assert.equal(before.version,5)
  let lost=false,staged=false
  if(scenario==='login'){
   await page.route('**/customer/cart-transfer',async route=>{writes.push(route.request().postDataJSON());const r=await route.fetch();assert.equal(r.status(),200);if(!lost){lost=true;await route.abort('failed')}else await route.fulfill({response:r})})
   await page.goto(origin+'/dk-'+lang+'/account');await expect(page.getByTestId('login-page')).toBeVisible()
   await page.getByTestId('email-input').fill(seed.account_email);await page.getByTestId('password-input').fill(seed.password);await page.getByTestId('sign-in-button').click()
   await expect(page.getByTestId('login-error-message')).toBeVisible();assert(await page.evaluate(k=>localStorage.getItem(k),transferKey))
   await page.reload();await expect(page.getByTestId('customer-authenticated')).toBeAttached();assert.equal(await page.evaluate(k=>localStorage.getItem(k),transferKey),null)
  }else{
   await page.route('**/context-payment',async route=>{
    const body=route.request().postDataJSON(),operationID=route.request().headers()['idempotency-key'];writes.push({body,operationID})
    if(scenario==='restore'&&!staged){staged=true;const result=await control('stage-paypal-context',{...body,operation_id:operationID});assert.equal(result.effects,0);await route.fulfill({status:503,contentType:'application/json',headers:{'Cache-Control':'private, no-store'},body:'{"error":"owned staged checkpoint"}'});return}
    const r=await route.fetch();assert.equal(r.status(),200);if(!lost){lost=true;await route.abort('failed')}else await route.fulfill({response:r})
   })
   const target=lang==='en'?'zh':'en',about=origin+'/dk-'+lang+'/about?context=paypal#contact'
   const open=async()=>{await page.locator('astro-island[component-url*="gin-header"]:not([ssr])').waitFor({state:'attached'});await page.getByRole('button',{name:/^(Country and language|国家与语言)$/,exact:true}).click();await expect(page.getByRole('dialog')).toBeVisible()}
   const choose=async()=>page.getByRole('dialog').getByRole('button',{name:target==='zh'?'Danmark — Chinese Simplified (China)':'Danmark — English (United Kingdom)',exact:true}).click()
   await page.goto(about);await open();await choose();await expect(page.getByRole('dialog').getByRole('alert')).toBeVisible()
   const pending=await page.evaluate(k=>JSON.parse(localStorage.getItem(k)),contextKey);assert.equal(pending.version,'5')
   await page.reload();await open()
   if(scenario==='restore'){
    await page.route('**/context-payment/restore',async route=>{writes.push({body:route.request().postDataJSON(),operationID:route.request().headers()['idempotency-key']});const r=await route.fetch();assert.equal(r.status(),200);if(!lost){lost=true;await route.abort('failed')}else await route.fulfill({response:r})})
    await page.getByRole('dialog').getByRole('button',{name:/^(Restore previous preferences|恢复之前的设置)$/,exact:true}).click();await expect(page.getByRole('dialog').getByRole('alert')).toBeVisible()
    await page.reload();await open();await page.getByRole('dialog').getByRole('button',{name:/^(Restore previous preferences|恢复之前的设置)$/,exact:true}).click();await expect(page.getByRole('dialog')).toHaveCount(0);assert.equal(page.url(),about)
   }else{await choose();await expect(page).toHaveURL(origin+'/dk-'+target+'/about?context=paypal#contact')}
   assert.equal(await page.evaluate(k=>localStorage.getItem(k),contextKey),null)
   assert(writes.every(w=>w.operationID===pending.operationID&&JSON.stringify(w.body)===JSON.stringify(writes[0].body)))
  }
  assert(lost);assert(writes.length>=2)
  assert.deepEqual(await page.evaluate(k=>JSON.parse(localStorage.getItem(k)),paymentKey),original)
  // Reopen the actual payment page after the effective cart version advanced.
  await page.goto(origin+'/dk-'+lang+'/checkout?step=payment');await expect(page.getByTestId(provider==='pp_paypal_paypal'?'paypal-wallet':'paypal-card')).toBeVisible()
  if(provider==='pp_paypal_card_paypal_card')await expect(page.getByTestId('paypal-card-submit')).toBeEnabled()
  await expect(page.getByTestId('payment-method-summary')).toHaveText(name)
  const after=await control('facts');assert.equal(after.version,6);assert.equal(after.checkout,original.operationID);assert.equal(after.refresh,null);assert.equal(after.paypal_creates,1);assert.equal(after.paypal_settlements,0);assert.equal(after.deletes,0);assert.equal(after.creates,0);assert.equal(after.live_sessions,1);assert.equal(after.protected_unchanged,true);assert.equal(after.attempts[0].InputVersion,4);assert.equal(after.attempts[0].CartVersion,5);assert.equal(after.attempts[0].EffectiveCartVersion,6)
  assert.equal(after.collections[0].CurrencyCode,'eur');assert.equal(JSON.parse(after.collections[0].RawAmount).value,'158.75');assert.equal(after.account_owners,scenario==='login'?1:0);assert.equal(after.transfer_receipts,scenario==='login'?1:0)
  assert(creates.every(w=>w.key===original.operationID&&w.body.version==='4'&&w.body.provider_id===provider))
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true)
  await page.screenshot({path:join(output,`paypal-context-${provider}-${scenario}-${lang}-${width}.png`)})
  cases.push({provider,scenario,lang,width,original,writes,creates,before,after,lost_response_recovered:true,external_sdk_provider_artificial:true,full_order_flow_executed:false});await control('case-done');await context.close()
 }
}finally{await browser.close();await writeFile(join(output,'browser-result.json'),JSON.stringify({cases,errors},null,2)+'\n')}
assert.deepEqual(errors,[])
