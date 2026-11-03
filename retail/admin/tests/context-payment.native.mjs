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
async function retiredEditsCase(seed,lang,width){
 const target=lang==='en'?'zh':'en',segment='dk-'+target,steps=[],requests=[]
 const amounts=()=>page.evaluate(async id=>{const r=await fetch(`/api/store/carts/${id}/amounts`);if(!r.ok)throw Error('amounts '+r.status);return r.json()},seed.cart_id)
 const check=async(version,total)=>{
  const a=await amounts(),f=await control('facts');assert.equal(a.version,String(version));assert.equal(a.totals.total,total);assert.equal(f.version,version);assert.equal(f.protected_unchanged,true);assert.equal(f.collections.length,1);assert.equal(JSON.parse(f.collections[0].RawAmount).value,total);assert.equal(f.collections[0].CurrencyCode,'eur');assert.equal(f.deletes,1)
  steps.push({amounts:a,facts:f});return f
 }
 await page.goto(origin+'/dk-'+lang+'/checkout?step=payment');await expect(page.getByTestId('submit-payment-button')).toBeEnabled();await page.getByTestId('submit-payment-button').click();await expect(page.getByTestId('submit-order-button')).toBeVisible()
 const original=await journal(paymentKey)
 await control('price');await page.goto(origin+'/dk-'+lang+'/about');await open();await page.getByRole('dialog').getByRole('button',{name:target==='zh'?'Danmark — Chinese Simplified (China)':'Danmark — English (United Kingdom)',exact:true}).click();await expect(page).toHaveURL(origin+'/'+segment+'/about')
 assert.equal(await journal(paymentKey),null);assert.equal((await check(6,'160')).checkout,null)
 let lostItem=false
 await page.route('**/line-items/*',async route=>{requests.push({kind:'item',key:route.request().headers()['idempotency-key'],body:route.request().postDataJSON()});const r=await route.fetch();assert.equal(r.status(),200);if(!lostItem){lostItem=true;await route.abort('failed')}else await route.fulfill({response:r})})
 await page.goto(origin+'/'+segment+'/cart');await expect(page.getByTestId('product-select-button')).toBeEnabled();await page.getByTestId('product-select-button').fill('2');await page.getByTestId('product-select-button').press('Enter');await expect(page.getByTestId('cart-recover-button')).toBeVisible()
 await page.reload();await expect(page.getByTestId('cart-recover-button')).toBeEnabled();await page.getByTestId('cart-recover-button').click();await expect(page.getByTestId('product-select-button')).toHaveValue('2');await expect(page.getByTestId('cart-recover-button')).toHaveCount(0)
 assert.equal((await check(7,'311.25')).live_sessions,0);assert.equal(await journal('retail.cart.intent.v1'),null)
 let lostPromo=false
 await page.route('**/promotions',async route=>{if(route.request().method()!=='POST'){await route.continue();return}requests.push({kind:'promo',key:route.request().headers()['idempotency-key'],body:route.request().postDataJSON()});const r=await route.fetch();assert.equal(r.status(),200);if(!lostPromo){lostPromo=true;await route.abort('failed')}else await route.fulfill({response:r})})
 await page.goto(origin+'/'+segment+'/checkout?step=payment');await expect(page.getByTestId('discount-input')).toBeEnabled();await page.getByTestId('discount-input').fill('OWNED10');await page.getByTestId('discount-apply-button').click();await expect(page.getByTestId('discount-recover')).toBeVisible()
 await page.reload();await expect(page.getByTestId('discount-recover')).toBeEnabled();await page.getByTestId('discount-recover').click();await expect(page.getByTestId('discount-code-tag')).toContainText('OWNED10');await expect(page.getByTestId('discount-recover')).toHaveCount(0)
 assert.equal((await check(8,'281')).live_sessions,0);assert.equal(await journal('retail:checkout-promotion-intent:v1'),null)
 let lostPayment=false
 await page.route('**/payment-attempts',async route=>{requests.push({kind:'payment',key:route.request().headers()['idempotency-key'],body:route.request().postDataJSON()});const r=await route.fetch();assert.equal(r.status(),200);if(!lostPayment){lostPayment=true;await route.abort('failed')}else await route.fulfill({response:r})})
 await expect(page.getByTestId('submit-payment-button')).toBeEnabled();await page.getByTestId('submit-payment-button').click()
 await expect(page.getByTestId('payment-recover')).toBeVisible();const pending=await journal(paymentKey);assert.equal(pending.version,'8');assert.notEqual(pending.operationID,original.operationID)
 // Existing checkout automatically resumes its persisted payment command.
 await page.reload();await expect(page.getByTestId('submit-order-button')).toBeVisible()
 const final=await check(9,'281');assert.equal(final.creates,2);assert.equal(final.live_sessions,1);assert.equal(final.checkout,pending.operationID);assert.equal(final.attempts[0].InputVersion,4);assert.equal(final.attempts[0].CartVersion,5);assert.equal(final.attempts[1].InputVersion,8);assert.equal(final.attempts[1].CartVersion,9)
 assert.deepEqual(final.provider_events,[{Kind:'create',Amount:'158.75',Currency:'eur'},{Kind:'delete',Amount:'158.75',Currency:'eur'},{Kind:'create',Amount:'281',Currency:'eur'}])
 for(const kind of ['item','promo','payment']){const group=requests.filter(r=>r.kind===kind);assert(group.length>=2,kind+' must replay');assert(group.every(r=>r.key===group[0].key&&JSON.stringify(r.body)===JSON.stringify(group[0].body)))}
 await page.screenshot({path:join(output,`retired-edits-${lang}-${width}.png`)})
 cases.push({scenario:'retired-edits',lang,width,steps,requests,lost_item_promotion_payment_responses:true,same_uuid_per_command:true,full_order_flow_executed:false});await control('case-done')
}
async function currencyCase(seed,scenario,lang,width,writes){
 const restoring=scenario==='currency-restore',target=lang==='zh'?'Chinese Simplified (China)':'English (United Kingdom)'
 await page.goto(origin+'/dk-'+lang+'/checkout?step=payment');await expect(page.getByTestId('submit-payment-button')).toBeEnabled();await page.getByTestId('submit-payment-button').click();await expect(page.getByTestId('submit-order-button')).toBeVisible()
 const original=await journal(paymentKey),url=origin+'/dk-'+lang+'/about?joint=currency#contact'
 const amounts=()=>page.evaluate(async id=>{const r=await fetch(`/api/store/carts/${id}/amounts`);if(!r.ok)throw Error('amounts '+r.status);return r.json()},seed.cart_id)
 const before=await amounts();assert.equal(before.currency_code,'eur');assert.equal(before.totals.total,'158.75')
 await page.goto(url)
 if(restoring)await control('mode/restore')
 let lost=false
 if(!restoring)await page.route('**/context-payment',async route=>{const r=await route.fetch();assert.equal(r.status(),200);if(!lost){lost=true;await route.abort('failed')}else await route.fulfill({response:r})})
 await open();await page.getByRole('dialog').getByRole('button',{name:'Canada — '+target,exact:true}).click();await expect(page.getByRole('dialog').getByRole('alert')).toBeVisible()
 const pending=await journal(contextKey);assert.equal(pending.version,'5');assert.equal(pending.currency,'cad');assert.equal(pending.regionID,'region-cad')
 if(restoring){
  const frozen=await control('facts');assert.equal(frozen.version,5);assert.equal(frozen.deletes,1);assert.equal(frozen.refresh,pending.operationID);assert.deepEqual(await amounts(),before)
 }
 await page.reload();await open()
 if(restoring){
  await page.route('**/context-payment/restore',async route=>{const r=await route.fetch();assert.equal(r.status(),200);if(!lost){lost=true;await route.abort('failed')}else await route.fulfill({response:r})})
  await page.getByRole('dialog').getByRole('button',{name:/^(Restore previous preferences|恢复之前的设置)$/,exact:true}).click();await expect(page.getByRole('dialog').getByRole('alert')).toBeVisible()
  await page.reload();await open();await page.getByRole('dialog').getByRole('button',{name:/^(Restore previous preferences|恢复之前的设置)$/,exact:true}).click();await expect(page.getByRole('dialog')).toHaveCount(0)
  assert.equal(page.url(),url);assert.deepEqual(await journal(paymentKey),{cartID:seed.cart_id,version:'5',operationID:pending.operationID})
 }else{
  await page.getByRole('dialog').getByRole('button',{name:'Canada — '+target,exact:true}).click();await expect(page).toHaveURL(origin+'/ca-'+lang+'/about?joint=currency#contact');assert.equal(await journal(paymentKey),null)
 }
 const after=await amounts(),cart=(await current()).cart,facts=await control('facts')
 assert.equal(after.currency_code,restoring?'eur':'cad');assert.equal(after.totals.total,'158.75');assert.equal(after.version,'6');assert.equal(cart.region_id,restoring?'region':'region-cad');assert.equal(cart.version,'6')
 assert.equal(facts.version,6);assert.equal(facts.deletes,1);assert.equal(facts.creates,restoring?2:1);assert.equal(facts.refresh,null);assert.equal(facts.receipts,restoring?0:1);assert.equal(facts.protected_unchanged,true);assert.equal(facts.attempts[0].InputVersion,4);assert.equal(facts.attempts[0].CartVersion,5)
 assert.equal(facts.collections.length,1);assert.equal(facts.collections[0].CurrencyCode,restoring?'eur':'cad');assert.equal(JSON.parse(facts.collections[0].RawAmount).value,'158.75');assert.equal(facts.live_sessions,restoring?1:0)
 assert.deepEqual(facts.provider_events,[{Kind:'create',Amount:'158.75',Currency:'eur'},{Kind:'delete',Amount:'158.75',Currency:'eur'},...(restoring?[{Kind:'create',Amount:'158.75',Currency:'eur'}]:[])])
 if(restoring){assert.equal(facts.checkout,pending.operationID);assert.equal(facts.attempts[1].InputVersion,5);assert.equal(facts.attempts[1].CartVersion,6)}else{assert.equal(facts.checkout,null);assert.equal(after.shipping_methods.length,0);assert.equal(after.totals.tax_total,'0')}
 assert.equal(await journal(contextKey),null);assert(writes.length>=2);assert(writes.every(w=>w.key===writes[0].key&&JSON.stringify(w.body)===JSON.stringify(writes[0].body)))
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true)
 await page.screenshot({path:join(output,`${scenario}-${lang}-${width}.png`)})
 cases.push({scenario,lang,width,before,after,cart,real_database_facts:facts,equal_numeric_total_different_currency:true,lost_response_replayed:lost,original_payment_operation:original.operationID});await control('case-done')
}
async function continuousCase(seed,lang,width,writes){
 await page.goto(origin+'/dk-'+lang+'/checkout?step=payment')
 await expect(page.getByTestId('submit-payment-button')).toBeEnabled();await page.getByTestId('submit-payment-button').click();await expect(page.getByTestId('submit-order-button')).toBeVisible()
 const original=await journal(paymentKey);assert.equal(original.version,'4')
 const first=lang==='en'?'zh':'en',second=lang,steps=[]
 await page.goto(origin+'/dk-'+lang+'/about?joint=continuous#contact')
 const choose=async target=>page.getByRole('dialog').getByRole('button',{name:target==='zh'?'Danmark — Chinese Simplified (China)':'Danmark — English (United Kingdom)',exact:true}).click()
 const check=async(v,creates,deletes,receipts,binding)=>{
  const facts=await control('facts'),cart=await current();assert.equal(cart.cart.version,String(v));assert.equal(facts.version,v);assert.equal(facts.creates,creates);assert.equal(facts.deletes,deletes);assert.equal(facts.receipts,receipts);assert.equal(facts.refresh,null);assert.equal(facts.protected_unchanged,true)
  assert.deepEqual(await journal(paymentKey),binding);assert.equal(await journal(contextKey),null)
  assert.equal(facts.attempts[0].InputVersion,4);assert.equal(facts.attempts[0].CartVersion,5)
  steps.push({cart:cart.cart,facts})
 }
 for(const [i,target] of [first,second].entries()){
  let lost=false
  await page.route('**/context-payment',async route=>{const r=await route.fetch();assert.equal(r.status(),200);if(!lost){lost=true;await route.abort('failed')}else await route.fulfill({response:r})})
  await open();await choose(target);await expect(page.getByRole('dialog').getByRole('alert')).toBeVisible()
  const pending=await journal(contextKey);assert.equal(pending.version,String(5+i))
  await page.reload();await open();await choose(target);await expect(page).toHaveURL(origin+'/dk-'+target+'/about?joint=continuous#contact')
  await check(6+i,1,0,1+i,original);assert.equal(steps.at(-1).facts.attempts[0].EffectiveCartVersion,6+i)
  await page.unroute('**/context-payment')
 }
 await control('price');await control('mode/restore')
 await open();await choose(first);await expect(page.getByRole('dialog').getByRole('alert')).toBeVisible()
 const pending=await journal(contextKey);assert.equal(pending.version,'7');assert.equal(pending.kind,'payment-save')
 const frozen=await control('facts');assert.equal(frozen.version,7);assert.equal(frozen.deletes,1);assert.equal(frozen.receipts,2);assert.equal(frozen.checkout,original.operationID);assert.equal(frozen.refresh,pending.operationID)
 await page.reload();await open();let lostRestore=false
 await page.route('**/context-payment/restore',async route=>{const r=await route.fetch();assert.equal(r.status(),200);if(!lostRestore){lostRestore=true;await route.abort('failed')}else await route.fulfill({response:r})})
 await page.getByRole('dialog').getByRole('button',{name:/^(Restore previous preferences|恢复之前的设置)$/,exact:true}).click();await expect(page.getByRole('dialog').getByRole('alert')).toBeVisible()
 await page.reload();await open();await page.getByRole('dialog').getByRole('button',{name:/^(Restore previous preferences|恢复之前的设置)$/,exact:true}).click();await expect(page.getByRole('dialog')).toHaveCount(0)
 const replacement={cartID:seed.cart_id,version:'7',operationID:pending.operationID}
 await check(8,2,1,2,replacement);assert.equal(steps.at(-1).facts.checkout,pending.operationID);assert.equal(steps.at(-1).facts.attempts[1].InputVersion,7);assert.equal(steps.at(-1).facts.attempts[1].CartVersion,8)
 await page.unroute('**/context-payment/restore');await control('original-price');await control('mode/normal')
 await open();await choose(first);await expect(page).toHaveURL(origin+'/dk-'+first+'/about?joint=continuous#contact')
 await check(9,2,1,3,replacement);assert.equal(steps.at(-1).facts.attempts[1].EffectiveCartVersion,9)
 const observed=await page.evaluate(async id=>{const csrf=decodeURIComponent(document.cookie.split('; ').find(s=>s.startsWith('retail_cart_csrf=')).split('=')[1]);const r=await fetch(`/api/store/carts/${id}/payment-status`,{method:'POST',headers:{'X-CSRF-Token':csrf,'Content-Type':'application/json'},body:'{}'});return {status:r.status,body:await r.json()}},seed.cart_id)
 assert.equal(observed.status,200);assert.deepEqual(observed.body,{cart_id:seed.cart_id,version:'9',state:'pending'})
 const final=await control('facts');assert.equal(final.observes,1);assert.equal(final.creates,2);assert.equal(final.deletes,1);assert.equal(final.protected_unchanged,true)
 const grouped=Object.groupBy(writes,w=>w.key);assert.equal(Object.keys(grouped).length,4)
 for(const group of Object.values(grouped))assert(group.every(w=>JSON.stringify(w.body)===JSON.stringify(group[0].body)))
 assert.deepEqual(Object.values(grouped).map(g=>g[0].body.version),['5','6','7','8'])
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true)
 await page.screenshot({path:join(output,`continuous-${lang}-${width}.png`)})
 cases.push({scenario:'continuous',lang,width,steps,observed,final,unchanged_saves_then_replacement_then_save:true,lost_save_and_restore_responses:true,same_uuid_per_operation:true,full_order_flow_executed:false});await control('case-done')
}
async function groupCase(context,seed,scenario,lang,width,writes){
 const restoring=scenario==='group-restore',target=lang==='en'?'zh':'en'
 const amounts=async()=>{const r=await context.request.get(origin+'/api/store/carts/'+seed.cart_id+'/amounts');assert.equal(r.status(),200);return r.json()}
 await control('group/member')
 await page.goto(origin+'/dk-'+lang+'/account');await page.getByTestId('email-input').fill(seed.account_email);await page.getByTestId('password-input').fill(seed.password);await page.getByTestId('sign-in-button').click();await expect(page.getByTestId('customer-authenticated')).toBeAttached()
 assert.equal((await amounts()).lines[0].unit_price,'100')
 await page.goto(origin+'/dk-'+lang+'/checkout?step=payment');await expect(page.getByTestId('discount-input')).toBeEnabled();await page.getByTestId('discount-input').fill('MEMBER10');await page.getByTestId('discount-apply-button').click();await expect(page.getByTestId('discount-code-tag')).toContainText('MEMBER10')
 assert.equal((await amounts()).totals.total,'121.25')
 await expect(page.getByTestId('submit-payment-button')).toBeEnabled();await page.getByTestId('submit-payment-button').click();await expect(page.getByTestId('submit-order-button')).toBeVisible()
 const original=await journal(paymentKey),before=await amounts(),frozen=await control('facts');assert.equal(original.version,'6');assert.equal(frozen.version,7);assert.equal(frozen.creates,1)
 await control('group/removed');assert.deepEqual(await amounts(),before)
 const blocked=await context.request.post(origin+'/api/store/carts/'+seed.cart_id+'/line-items/'+before.lines[0].id,{headers:{Origin:origin,'X-CSRF-Token':seed.csrf,'Idempotency-Key':crypto.randomUUID()},data:{version:'7',quantity:'2'}});assert.equal(blocked.status(),409);assert.deepEqual(await amounts(),before)
 // Replaying the original create acknowledges its frozen quote; it must not
 // silently create another channel payment at the new non-member amount.
 const replay=await context.request.post(origin+'/api/store/carts/'+seed.cart_id+'/payment-attempts',{headers:{Origin:origin,'X-CSRF-Token':seed.csrf,'Idempotency-Key':original.operationID},data:{version:original.version}});assert.equal(replay.status(),200);assert.equal((await control('facts')).creates,1);assert.deepEqual(await amounts(),before)
 if(restoring)await control('mode/restore')
 const startURL=origin+'/dk-'+lang+'/about?joint=groups#contact';await page.goto(startURL)
 let lost=false
 await page.route('**/context-payment',async route=>{const r=await route.fetch();assert.equal(r.status(),restoring?409:200);if(!restoring&&!lost){lost=true;await route.abort('failed')}else await route.fulfill({response:r})})
 const choose=()=>page.getByRole('dialog').getByRole('button',{name:target==='zh'?'Danmark — Chinese Simplified (China)':'Danmark — English (United Kingdom)',exact:true}).click()
 await open();await choose();await expect(page.getByRole('dialog').getByRole('alert')).toBeVisible();const pending=await journal(contextKey);assert.equal(pending.version,'7')
 await page.reload();await open()
 if(restoring){
  await page.route('**/context-payment/restore',async route=>{const r=await route.fetch();assert.equal(r.status(),200);if(!lost){lost=true;await route.abort('failed')}else await route.fulfill({response:r})})
  const restore=()=>page.getByRole('button',{name:lang==='zh'?'恢复之前的设置':'Restore previous preferences',exact:true}).click()
  await restore();await expect(page.getByRole('dialog').getByRole('alert')).toBeVisible();await page.reload();await open();await restore();await expect.poll(()=>journal(contextKey)).toBeNull();await expect(page.getByRole('dialog')).toHaveCount(0);await expect(page).toHaveURL(startURL)
 }else{await choose();await expect(page).toHaveURL(origin+'/dk-'+target+'/about?joint=groups#contact')}
 const recovered=await amounts(),facts=await control('facts');assert.equal(recovered.version,'8');assert.equal(recovered.lines[0].unit_price,restoring?'100':'120');assert.equal(recovered.totals.discount_total,restoring?'12.5':'0');assert.equal(recovered.totals.total,restoring?'121.25':'158.75');assert.equal(facts.deletes,1);assert.equal(facts.creates,restoring?2:1);assert.equal(facts.receipts,restoring?0:1);assert.equal(facts.refresh,null);assert.equal(facts.protected_unchanged,true)
 assert.equal(await journal(contextKey),null);assert(lost);assert(writes.length>=2);assert(writes.every(w=>w.key===pending.operationID&&JSON.stringify(w.body)===JSON.stringify(writes[0].body)))
 let nextSaveWrites=[]
 if(restoring){
  const binding=await journal(paymentKey);assert.equal(binding.operationID,pending.operationID);assert.equal(binding.version,'7')
  // Compensation preserves the original authenticated quote. A NEW explicit
  // save, rather than compensation replay, adopts current membership/prices.
  await page.unroute('**/context-payment')
  await page.route('**/context-payment',async route=>{nextSaveWrites.push({key:route.request().headers()['idempotency-key'],body:route.request().postDataJSON()});const r=await route.fetch();assert.equal(r.status(),200);await route.fulfill({response:r})})
  await open();await choose();await expect(page).toHaveURL(origin+'/dk-'+target+'/about?joint=groups#contact')
  assert.equal(nextSaveWrites.length,1);assert.notEqual(nextSaveWrites[0].key,pending.operationID);assert.equal(nextSaveWrites[0].body.version,'8');assert.equal(await journal(contextKey),null)
 }
 {
  assert.equal(await journal(paymentKey),null);await page.goto(origin+'/dk-'+target+'/checkout?step=payment');await expect(page.getByTestId('submit-payment-button')).toBeEnabled();await page.getByTestId('submit-payment-button').click();await expect(page.getByTestId('submit-order-button')).toBeVisible();assert.equal((await journal(paymentKey)).version,restoring?'9':'8')
 }
 const after=await amounts(),final=await control('facts');assert.equal(after.lines[0].unit_price,restoring?'122':'120');assert.equal(after.totals.discount_total,'0');assert.equal(after.totals.total,restoring?'161.25':'158.75');assert.equal(final.version,restoring?10:9);assert.equal(final.creates,restoring?3:2);assert.equal(final.deletes,restoring?2:1);assert.equal(final.live_sessions,1);assert.equal(final.protected_unchanged,true);assert.equal(final.account_owners,1)
 assert.deepEqual(final.provider_events,[{Kind:'create',Amount:'121.25',Currency:'eur'},{Kind:'delete',Amount:'121.25',Currency:'eur'},...(restoring?[{Kind:'create',Amount:'121.25',Currency:'eur'},{Kind:'delete',Amount:'121.25',Currency:'eur'}]:[]),{Kind:'create',Amount:restoring?'161.25':'158.75',Currency:'eur'}])
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true);await page.screenshot({path:join(output,`${scenario}-${lang}-${width}.png`)})
 cases.push({scenario,lang,width,before,recovered,after,nextSaveWrites,real_database_facts:final,frozen_quote_preserved_until_explicit_refresh:true,compensation_preserves_original_quote:restoring,frozen_item_write_denied:true,original_create_replay_no_duplicate:true,removed_group_discount_cleared_on_new_save:true,lost_response_same_operation_recovered:true,full_order_flow_executed:false});await control('case-done')
}
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
async function failedPaymentCase(seed,scenario,lang,width,writes){
 const amounts=()=>page.evaluate(async id=>{const r=await fetch(`/api/store/carts/${id}/amounts`);if(!r.ok)throw Error('amounts '+r.status);return r.json()},seed.cart_id)
 await page.goto(origin+'/dk-'+lang+'/checkout?step=payment')
 await expect(page.getByTestId('submit-payment-button')).toBeEnabled()
 await page.getByTestId('submit-payment-button').click()
 await expect(page.getByTestId('submit-order-button')).toBeVisible()
 const original=await journal(paymentKey),before=await amounts()
 await control('mode/'+scenario.slice(6))
 await page.goto(origin+'/dk-'+lang+'/payment-result?cart_id='+seed.cart_id)
 const result=page.getByTestId('payment-result'),retry=page.getByRole('button',{name:lang==='zh'?'返回结账':'Return to checkout',exact:true})
 await expect(result).toHaveAttribute('data-status','failed');await expect(retry).toBeEnabled()
 await control('context-receipt-fault/on')
 await retry.click();await expect(page.getByRole('alert')).toBeVisible()
 const intent=await journal(contextKey),frozen=await control('facts')
 assert.equal(intent.kind,'payment-retry');assert.equal(intent.version,'5')
 assert.equal(frozen.version,5);assert.equal(frozen.checkout,original.operationID);assert.equal(frozen.refresh,intent.operationID);assert.equal(frozen.live_sessions,1);assert.equal(frozen.receipts,0);assert.equal(frozen.deletes,1);assert.equal(frozen.creates,1);assert(frozen.protected_unchanged)
 await control('context-receipt-fault/off')
 let lost=false
 await page.route('**/context-payment',async route=>{const response=await route.fetch();assert.equal(response.status(),200);const body=await response.json();assert.equal(body.state,'saved');assert.equal(body.completion.payment,null);lost=true;await route.abort('failed')})
 await retry.click();await expect.poll(()=>lost).toBe(true);await expect(page.getByRole('alert')).toBeVisible()
 const retired=await control('facts')
 assert.equal(retired.version,6);assert.equal(retired.checkout,null);assert.equal(retired.refresh,null);assert.equal(retired.live_sessions,0);assert.equal(retired.receipts,1);assert.equal(retired.deletes,1);assert.equal(retired.creates,1);assert(retired.protected_unchanged)
 assert.deepEqual(await journal(contextKey),intent);assert.deepEqual(await journal(paymentKey),original)
 await page.unroute('**/context-payment');await page.reload()
 await expect(page).toHaveURL(origin+'/dk-'+lang+'/checkout?step=payment')
 await expect(page.getByTestId('submit-payment-button')).toBeEnabled()
 assert.equal(await journal(contextKey),null);assert.equal(await journal(paymentKey),null)
 const after=await amounts();assert.deepEqual(after.totals,before.totals);assert.equal(after.currency_code,before.currency_code)
 await page.getByTestId('submit-payment-button').click();await expect(page.getByTestId('submit-order-button')).toBeVisible()
 const replacement=await journal(paymentKey),facts=await control('facts')
 assert.equal(replacement.version,'6');assert.notEqual(replacement.operationID,original.operationID)
 assert.equal(facts.version,7);assert.equal(facts.creates,2);assert.equal(facts.deletes,1);assert.equal(facts.live_sessions,1);assert.equal(facts.receipts,1);assert.equal(facts.attempts.length,2);assert(facts.protected_unchanged)
 assert(writes.length>=3&&writes.every(w=>w.key===intent.operationID&&w.body.version==='5'&&JSON.stringify(w.body)===JSON.stringify(writes[0].body)))
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),true)
 await page.screenshot({path:join(output,`${scenario}-${lang}-${width}.png`)})
 cases.push({scenario,lang,width,late_failure_atomic:true,lost_success_recovered:true,same_uuid:true,original:original.operationID,replacement:replacement.operationID,before,after,real_database_facts:facts})
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
  if(scenario.startsWith('retry-')){await failedPaymentCase(seed,scenario,lang,width,writes);await context.close();continue}
  if(scenario.startsWith('group-')){await groupCase(context,seed,scenario,lang,width,writes);await context.close();continue}
  if(scenario==='retired-edits'){await retiredEditsCase(seed,lang,width);await context.close();continue}
  if(scenario.startsWith('currency-')){await currencyCase(seed,scenario,lang,width,writes);await context.close();continue}
  if(scenario==='continuous'){await continuousCase(seed,lang,width,writes);await context.close();continue}
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
  // In the Chinese matrix the target URL can equal the current URL. The URL
  // assertion then finishes before the async save clears its durable intent.
  await expect.poll(()=>journal(contextKey)).toBeNull()
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
