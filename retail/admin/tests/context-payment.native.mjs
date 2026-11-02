import assert from 'node:assert/strict'
import {writeFile} from 'node:fs/promises'
import {join} from 'node:path'
import {chromium,expect} from '@playwright/test'

const origin=process.env.CONTEXT_ASTRO_ORIGIN,api=process.env.CONTEXT_API_ORIGIN,key=process.env.CONTEXT_FIXTURE_KEY,output=process.env.CONTEXT_BROWSER_OUTPUT
assert(/^http:\/\/127\.0\.0\.1:\d+$/.test(origin)&&/^http:\/\/127\.0\.0\.1:\d+$/.test(api)&&key?.length===64&&output)
const contextKey='retail:cart-context-intent:v1',paymentKey='retail:checkout-payment-intent:v1'
const cases=[],errors=[]
const browser=await chromium.launch({headless:true,...(process.env.CONTEXT_BROWSER_EXECUTABLE?{executablePath:process.env.CONTEXT_BROWSER_EXECUTABLE}:{})})
let page
async function control(path,body){const r=await fetch(api+'/__fixture/'+path,{method:path==='facts'?'GET':'POST',headers:{'X-Fixture-Key':key,'Content-Type':'application/json'},...(body?{body:JSON.stringify(body)}:{})});assert.equal(r.status,200,'fixture '+path);return path==='facts'||path==='new'?r.json():null}
async function open(){await page.locator('astro-island[component-url*="gin-header"]:not([ssr])').waitFor({state:'attached'});await page.getByRole('button',{name:/^(Country and language|国家与语言)$/,exact:true}).click();await expect(page.getByRole('dialog')).toBeVisible()}
async function journal(k){return page.evaluate(k=>JSON.parse(localStorage.getItem(k)),k)}
async function current(){return page.evaluate(async()=>{const r=await fetch('/api/store/cart');if(!r.ok)throw Error('cart '+r.status);return r.json()})}
async function accountCase(context,seed,scenario,lang,width){
 const attach=scenario==='unbound-unchanged',restore=['unbound-restore','unbound-create-unknown'].includes(scenario),unknown=scenario==='unbound-delete-unknown',fault=scenario==='unbound-receipt-fault',changed=scenario.startsWith('unbound-')&&!attach,success=true
 const accountURL=origin+'/dk-'+lang+'/account',transferKey='retail:customer-cart-transfer:v1',inputs=[]
 async function login(email){
  await page.goto(accountURL);await expect(page.getByTestId('login-page')).toBeVisible()
  await page.getByTestId('email-input').fill(email);await page.getByTestId('password-input').fill(seed.password)
  await page.getByTestId('sign-in-button').click()
 }
 if(scenario==='same-account'){
  await login(seed.account_email);await expect(page.getByTestId('customer-authenticated')).toBeAttached()
  await control('edited-email')
  // Preserve cart possession and the payment journal: no logout operation is
  // issued, since production logout deliberately clears both capabilities.
  await context.clearCookies({name:'retail_customer_session'});await context.clearCookies({name:'retail_customer_csrf'})
 }
 await page.goto(origin+'/dk-'+lang+'/checkout?step=payment')
 await expect(page.getByTestId('submit-payment-button')).toBeEnabled();await page.getByTestId('submit-payment-button').click()
 await expect(page.getByTestId('submit-order-button')).toBeVisible()
 const payment=await journal(paymentKey),before=await control('facts')
 assert.equal(before.creates,1);assert.equal(before.deletes,0)
 if(attach)await control('first-attachment-audit')
 if(scenario==='unbound-precision')await control('precision-price')
 else if(!attach)await control('price') // A same-account no-op must not refresh prices.
 if(restore)await control('mode/'+(scenario==='unbound-restore'?'restore':'create-unknown'))
 if(unknown)await control('mode/delete-unknown')
 if(fault)await control('transfer-receipt-fault/yes')
 let lost=false
 await page.route('**/customer/cart-transfer',async route=>{
  inputs.push(route.request().postDataJSON());const r=await route.fetch()
  assert.equal(r.status(),restore||fault?503:200)
  if(!restore&&!unknown&&!fault&&!lost){lost=true;await route.abort('failed')}else await route.fulfill({response:r})
 })
 await login(seed.account_email);await expect(page.getByTestId('login-error-message')).toBeVisible()
 if(scenario==='unbound-lost-restore'){
  const original=await journal(transferKey);assert(original)
  await page.route('**/customer/cart-transfer/restore',async route=>{inputs.push(route.request().postDataJSON());const r=await route.fetch();assert.equal(r.status(),200);assert.equal((await r.json()).state,'saved');await route.fulfill({response:r})})
  await page.getByTestId('customer-cart-restore').click();await expect(page.getByTestId('customer-authenticated')).toBeAttached()
  const after=await control('facts');assert.equal(after.version,before.version+1);assert.equal(after.account_owners,1);assert.equal(after.transfer_receipts,1);assert.equal(after.deletes,1);assert.equal(after.creates,1);assert.equal(after.checkout,null);assert.equal(after.refresh,null);assert.equal(after.protected_unchanged,true)
  assert.equal(await journal(paymentKey),null);assert.equal(await journal(transferKey),null);assert.equal(inputs.length,2);assert.deepEqual(inputs[0],inputs[1])
  cases.push({scenario,lang,width,real_database_facts:after,lost_success_restore_acknowledged:true});await control('case-done');return
 }
 if(restore||unknown||fault){
  const frozen=await control('facts');assert.equal(frozen.version,before.version);assert.equal(frozen.account_owners,0);assert.equal(frozen.transfer_receipts,0);assert.equal(frozen.deletes,1);assert(frozen.refresh)
  const original=await journal(transferKey);assert(original)
  await control('transfer-scope-audit',original.command)
  await page.reload();await expect(page.getByTestId('customer-cart-restore')).toBeVisible()
  assert.equal((await control('facts')).deletes,1)
  if(unknown||fault){await page.unroute('**/customer/cart-transfer');if(fault)await control('transfer-receipt-fault/no');else await control('mode/canceled');await page.reload()}
  else{
   await page.getByTestId('customer-cart-restore').click()
   if(scenario==='unbound-create-unknown'){
    await expect(page.getByTestId('customer-cart-restore')).toBeEnabled();await expect(page.getByRole('alert').last()).toBeVisible()
    assert.equal((await control('facts')).creates,2)
    await page.reload();await expect(page.getByTestId('customer-cart-restore')).toBeEnabled()
    assert.equal((await control('facts')).creates,2);await control('mode/normal');await page.getByTestId('customer-cart-restore').click()
   }
  }
  await expect(page.getByTestId('customer-authenticated')).toBeAttached()
  const after=await control('facts');assert.equal(after.version,before.version+1);assert.equal(after.refresh,null);assert.equal(after.protected_unchanged,true);assert.equal(after.deletes,1)
  assert.equal(after.account_owners,restore?0:1);assert.equal(after.transfer_receipts,restore?0:1);assert.equal(after.creates,restore?2:1)
  assert.equal(after.email,restore?'owned@example.invalid':seed.account_email);assert.equal(await journal(transferKey),null)
  if(restore){const rebound=await journal(paymentKey);assert.equal(rebound.operationID,original.command.operation_id);assert.equal(rebound.version,'5');assert(after.checkout)}else{assert.equal(await journal(paymentKey),null);assert.equal(after.checkout,null)}
  assert(inputs.every(input=>JSON.stringify(input)===JSON.stringify(inputs[0])))
  await page.screenshot({path:join(output,`${scenario}-${lang}-${width}.png`)});cases.push({scenario,lang,width,real_database_facts:after,same_uuid_payload:true,restored_original_cart:restore});await control('case-done');return
 }
 if(success){
  const pending=await journal(transferKey);assert(pending);assert.equal(pending.command.version,String(before.version))
  await page.reload()
  await expect(page.getByTestId('customer-authenticated')).toBeAttached()
  assert(inputs.length>=2);assert(inputs.every(input=>JSON.stringify(input)===JSON.stringify(inputs[0])));assert.equal(await journal(transferKey),null)
  await context.clearCookies({name:'retail_customer_session'});await context.clearCookies({name:'retail_customer_csrf'})
  await page.unroute('**/customer/cart-transfer')
  await login(seed.other_email);await expect(page.getByTestId('login-error-message')).toBeVisible()
 }
 const after=await control('facts')
 assert.equal(after.version,before.version+(attach||changed?1:0));assert.equal(after.checkout,changed?null:before.checkout);assert.equal(after.refresh,null)
 assert.equal(after.account_owners,success?1:0)
 assert.equal(after.transfer_receipts,before.transfer_receipts+(success?1:0))
 assert.equal(after.email,scenario==='same-account'?'edited-checkout@example.invalid':attach||changed?seed.account_email:'owned@example.invalid')
 assert.equal(after.creates,1);assert.equal(after.deletes,changed?1:0);assert.equal(after.protected_unchanged,true)
 assert.deepEqual(await journal(paymentKey),changed?null:payment)
 if(attach){assert.equal(after.attempts[0].InputVersion,4);assert.equal(after.attempts[0].CartVersion,5);assert.equal(after.attempts[0].EffectiveCartVersion,6)}
 else assert.deepEqual(after.attempts,before.attempts)
 if(scenario==='same-account')await control('frozen-transfer-audit')
 await page.screenshot({path:join(output,`${scenario}-${lang}-${width}.png`)})
 cases.push({scenario,lang,width,real_database_facts:after,payment_journal_unchanged:!changed,lost_response_replayed:lost,foreign_account_denied:true})
 await control('case-done')
}
try{
 const matrix=process.env.CONTEXT_MATRIX==='1',scenarios=JSON.parse(process.env.CONTEXT_SCENARIOS)
 for(const lang of matrix?['en','zh']:['en'])for(const width of matrix?[390,1440]:[390])for(const scenario of scenarios){
  const replacement=scenario.startsWith('replacement-'),restoring=['restore','create-unknown'].includes(scenario)||replacement
  const context=await browser.newContext({viewport:{width,height:900},reducedMotion:'reduce'})
  page=await context.newPage();page.setDefaultTimeout(20000);page.on('pageerror',e=>errors.push(e.message))
  const writes=[];page.on('request',r=>{if(r.method()==='POST'&&r.url().includes('/context-payment'))writes.push({path:new URL(r.url()).pathname,key:r.headers()['idempotency-key'],body:r.postDataJSON()})})
  const seed=await control('new'),url=origin+'/dk-'+lang+'/about?joint=payment#contact'
  await context.addCookies([{name:'retail_cart_possession',value:seed.token,url:origin,sameSite:'Strict'},{name:'retail_cart_csrf',value:seed.csrf,url:origin,sameSite:'Strict'}])
  if(scenario==='same-account'||scenario.startsWith('unbound-')){await accountCase(context,seed,scenario,lang,width);await context.close();continue}
  await page.goto(origin+'/dk-'+lang+'/checkout?step=payment')
  await expect(page.getByTestId('submit-payment-button')).toBeEnabled()
  await page.getByTestId('submit-payment-button').click()
  await expect(page.getByTestId('submit-order-button')).toBeVisible()
  const initialIntent=await journal(paymentKey),initial=initialIntent.operationID
  assert.equal(initialIntent.version,'4')
  await page.goto(url)
  if(scenario!=='unchanged')await control('price')
  if(['restore','delete-unknown','create-unknown','paid'].includes(scenario)||replacement)await control('mode/'+(replacement?'create-unknown':scenario==='paid'?'delete-unknown':scenario))
  let lost=false
  if(['unchanged','changed'].includes(scenario))await page.route('**/context-payment',async route=>{const r=await route.fetch();assert.equal(r.status(),200);if(!lost){lost=true;await route.abort('failed')}else await route.fulfill({response:r})})
  await open();await page.getByRole('dialog').getByRole('button',{name:'Danmark — Chinese Simplified (China)',exact:true}).click()
  await expect(page.getByRole('dialog').getByRole('alert')).toBeVisible()
  const pending=await journal(contextKey);assert(pending);assert.equal(pending.kind,'payment-save');assert.equal(pending.version,'5')
  const beforeReload=await control('facts')
  await page.reload();await open()
  const restore=page.getByRole('button',{name:lang==='zh'?'恢复之前的设置':'Restore previous preferences',exact:true})
  await expect(restore).toBeVisible()
  if(restoring){
   if(scenario==='restore')await page.route('**/context-payment/restore',async route=>{const r=await route.fetch();assert.equal(r.status(),200);if(!lost){lost=true;await route.abort('failed')}else await route.fulfill({response:r})})
   await restore.click()
   if(scenario==='restore'){
    await expect(page.getByRole('dialog').getByRole('alert')).toBeVisible();await page.reload();await open();await restore.click()
   }
   if(scenario==='create-unknown'||replacement){
    await expect(page.getByRole('dialog').getByRole('alert')).toBeVisible()
    assert.equal((await journal(contextKey)).kind,'payment-restore')
    const uncertain=await control('facts');assert.equal(uncertain.creates,2);assert.equal(uncertain.deletes,1)
    await page.reload();await open();await restore.click();await expect(page.getByRole('dialog').getByRole('alert')).toBeVisible()
    assert.equal((await control('facts')).creates,2)
    await control('mode/'+(replacement?scenario:'normal'))
    if(replacement)await page.route('**/context-payment/restore',async route=>{const r=await route.fetch();assert.equal(r.status(),200);if(!lost){lost=true;await route.abort('failed')}else await route.fulfill({response:r})})
    await restore.click()
    if(replacement){await expect(page.getByRole('dialog').getByRole('alert')).toBeVisible();await page.reload();await open();await restore.click()}
   }
   await expect(page.getByRole('dialog')).toHaveCount(0)
   assert.equal(page.url(),url)
   const binding=await journal(paymentKey);assert.equal(binding.operationID,pending.operationID);assert.equal(binding.version,'5')
   if(replacement){
    const observed=await page.evaluate(async id=>{const csrf=decodeURIComponent(document.cookie.split('; ').find(s=>s.startsWith('retail_cart_csrf=')).split('=')[1]);const r=await fetch(`/api/store/carts/${id}/payment-status`,{method:'POST',headers:{'X-CSRF-Token':csrf,'Content-Type':'application/json'},body:'{}'});return {status:r.status,body:await r.json()}},seed.cart_id)
    assert.equal(observed.status,200)
    assert.equal(observed.body.state,scenario==='replacement-paid'?'paid_pending_order':'authorized')
    // Replaying the payment preparation cannot offer an already paid checkout
    // URL or dispatch another CREATE. Order completion is a separate boundary.
    await page.goto(origin+'/dk-'+lang+'/checkout?step=payment')
    await expect(page.getByTestId('payment-recover')).toBeVisible()
    await expect(page.getByTestId('submit-order-button')).toHaveCount(0)
    await page.getByTestId('payment-recover').click()
    await expect(page.getByTestId('payment-method-error-message')).toBeVisible()
    await expect(page.getByTestId('submit-order-button')).toHaveCount(0)
   }
  }else if(scenario==='paid'){
   await control('mode/paid')
   await page.getByRole('dialog').getByRole('button',{name:'Danmark — Chinese Simplified (China)',exact:true}).click()
   await expect(page.getByRole('dialog').getByRole('alert')).toBeVisible()
   await restore.click();await expect(page.getByRole('dialog').getByRole('alert')).toBeVisible()
   const frozen=await control('facts');assert.equal(frozen.protected_unchanged,true);assert.equal(frozen.version,5);assert.equal(frozen.receipts,0);assert.equal(frozen.deletes,1);assert.equal(frozen.creates,1);assert(frozen.checkout&&frozen.refresh)
   assert.equal((await journal(contextKey)).kind,'payment-restore');assert.deepEqual(await journal(paymentKey),initialIntent)
   assert.equal(page.url(),url);assert.equal(new Set(writes.map(w=>w.key)).size,1)
   cases.push({lang,width,scenario,real_database_facts:frozen,paid_keeps_frozen:true,same_uuid_payload:true})
   await control('case-done');await context.close();continue
  }else{
   if(scenario==='delete-unknown'){
    assert.equal(beforeReload.deletes,1);assert.equal(beforeReload.receipts,0)
    await page.getByRole('dialog').getByRole('button',{name:'Danmark — Chinese Simplified (China)',exact:true}).click()
    await expect(page.getByRole('dialog').getByRole('alert')).toBeVisible();assert.equal((await control('facts')).deletes,1)
    await control('mode/canceled')
   }
   await page.getByRole('dialog').getByRole('button',{name:'Danmark — Chinese Simplified (China)',exact:true}).click()
   await expect(page).toHaveURL(origin+'/dk-zh/about?joint=payment#contact')
   if(scenario==='unchanged')assert.deepEqual(await journal(paymentKey),{cartID:seed.cart_id,version:'4',operationID:initial})
   else assert.equal(await journal(paymentKey),null)
  }
  assert.equal(await journal(contextKey),null)
  const facts=await control('facts');assert.equal(facts.protected_unchanged,true);assert.equal(facts.version,6);assert.equal(facts.refresh,null)
  assert.equal(facts.creates,restoring?2:1)
  assert.equal(facts.deletes,scenario==='unchanged'?0:1)
  assert.equal(facts.receipts,restoring?0:1)
  assert.equal(facts.attempts[0].InputVersion,4);assert.equal(facts.attempts[0].CartVersion,5)
  if(scenario==='unchanged')assert.equal(facts.attempts[0].EffectiveCartVersion,6)
  assert(writes.length>=2);assert.equal(new Set(writes.map(w=>w.key)).size,1);assert(writes.every(w=>JSON.stringify(w.body)===JSON.stringify(writes[0].body)))
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true)
  await page.screenshot({path:join(output,`${scenario}-${lang}-${width}.png`)})
  cases.push({lang,width,scenario,real_database_facts:facts,context_posts:writes.length,same_uuid_payload:true,lost_success_response:lost,refresh_recovery:true})
  await control('case-done');await context.close()
 }
 assert.deepEqual(errors,[])
 await writeFile(join(output,'browser-result.json'),JSON.stringify({cases,errors,api_mocking:false,actual_gin_mysql_redis_astro:true},null,2)+'\n')
}catch(e){if(page)await page.screenshot({path:join(output,'failure.png')}).catch(()=>{});await writeFile(join(output,'browser-failure.json'),JSON.stringify({error:String(e),cases,errors},null,2));throw e}finally{await browser.close()}
