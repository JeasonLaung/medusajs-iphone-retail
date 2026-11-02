import assert from 'node:assert/strict'
import {writeFile} from 'node:fs/promises'
import {join} from 'node:path'
import {chromium,expect} from '@playwright/test'

const origin=process.env.CONTEXT_ASTRO_ORIGIN,output=process.env.CONTEXT_BROWSER_OUTPUT
if(!/^http:\/\/127\.0\.0\.1:\d+$/.test(origin??'')||!output)throw Error('Owned context checkout fixture required')
const errors=[],cases=[]
let browser,page
async function api(path,body,csrf,key){
 return page.evaluate(async({path,body,csrf,key})=>{
  const r=await fetch(path,{method:body===undefined?'GET':'POST',credentials:'same-origin',cache:'no-store',headers:{Accept:'application/json',...(body===undefined?{}:{'Content-Type':'application/json'}),...(csrf?{'X-CSRF-Token':csrf}:{}),...(key?{'Idempotency-Key':key}:{})},...(body===undefined?{}:{body:JSON.stringify(body)})})
  const value=await r.json();if(![200,201].includes(r.status))throw Error('Fixture HTTP '+r.status+': '+JSON.stringify(value));return value
 },{path,body,csrf,key})
}
async function open(){await page.getByRole('button',{name:/^(Country and language|国家与语言)$/}).click();await expect(page.getByRole('dialog')).toBeVisible()}
async function choose(name){await page.getByRole('dialog').getByRole('button',{name,exact:true}).click()}
async function fit(){assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),'page overflow')}
try{
 browser=await chromium.launch({channel:'chrome',headless:true})
 for(const initial of ['en','zh'])for(const width of [390,1440]){
  const context=await browser.newContext({viewport:{width,height:900},reducedMotion:'reduce'})
  page=await context.newPage();page.setDefaultTimeout(20000);page.on('pageerror',e=>errors.push(e.message))
  const posts=[];page.on('request',r=>{if(r.method()==='POST'&&r.url().endsWith('/context'))posts.push({key:r.headers()['idempotency-key'],body:r.postDataJSON()})})
  const suffix='/about?selection=a%2Bb&sort=price#contact'
  await page.goto(origin+'/dk-'+initial+suffix)
  const scope=await api('/api/store/cart-sessions',{})
  const created=await api('/api/store/carts',{region_id:'fixture-region',locale:initial==='zh'?'zh-CN':'en-GB'},scope.csrf_token,crypto.randomUUID())
  const id=created.cart.id,path='/api/store/carts/'+id,csrf=created.csrf_token
  let result=await api(path+'/line-items',{version:'1',variant_id:'browser-add-v',quantity:'1'},csrf,crypto.randomUUID())
  const address={first_name:'Owned',last_name:'Buyer',company:'',address_1:'Artificial street',address_2:'',city:'Copenhagen',country_code:'dk',province:'',postal_code:'0012',phone:'+45 111'}
  result=await api(path+'/contacts',{version:result.version,email:'context-browser@example.invalid',shipping_address:address,billing_address:address},csrf,crypto.randomUUID())
  result=await api(path+'/shipping-methods',{version:result.version,option_id:'context-delivery-option'},csrf,crypto.randomUUID())
  result=await api(path+'/promotions',{version:result.version,codes:['CONTEXT_EUR10']},csrf,crypto.randomUUID())
  assert.equal(result.version,'5')
  const before=await api(path+'/amounts'),beforeContacts=await api(path+'/contacts')
  assert.equal(before.totals.total,'121.25');assert.equal(before.shipping_methods.length,1)
  const method=before.shipping_methods[0].id
  await page.reload();await open()
  const other=initial==='en'?'zh':'en'
  await choose('Deutschland — '+(other==='zh'?'Chinese Simplified (China)':'English (United Kingdom)'))
  await expect(page).toHaveURL(origin+'/de-'+other+suffix)
  const same=await api(path+'/amounts')
  assert.equal(same.version,'6');assert.equal(same.totals.total,'121.25');assert.equal(same.shipping_methods[0].id,method)
  assert.deepEqual((await api(path+'/contacts')).shipping_address,beforeContacts.shipping_address)
  let drop=true
  await page.route('**/api/store/carts/*/context',async route=>{
   const response=await route.fetch();assert.equal(response.status(),200)
   if(drop){drop=false;await route.abort('failed')}else await route.fulfill({response})
  })
  await open();await choose('France — Chinese Simplified (China)');await expect(page.getByRole('alert')).toBeVisible()
  await expect(page).toHaveURL(origin+'/de-'+other+suffix)
  assert.equal(await page.evaluate(()=>JSON.parse(localStorage.getItem('retail:cart-context-intent:v1')).version),'6')
  await page.reload();await open();await choose('France — Chinese Simplified (China)')
  await expect(page).toHaveURL(origin+'/fr-zh'+suffix)
  assert.deepEqual(posts[1],posts[2])
  const dollars=await api(path+'/amounts'),promotions=await api(path+'/promotions')
  assert.equal(dollars.version,'7');assert.equal(dollars.currency_code,'usd');assert.equal(dollars.totals.total,'36.9600000000000000000011')
  assert.equal(dollars.shipping_methods.length,0)
  assert.deepEqual(promotions.promotions.map(p=>p.code),['CONTEXT_USD20'])
  const foreign=await api(path+'/contacts')
  assert.equal(foreign.shipping_address.country_code,'fr');assert.equal(foreign.shipping_address.address_1,'')
  assert.deepEqual(foreign.billing_address,beforeContacts.billing_address)
  // The real checkout must render a fillable draft, even with a stale payment
  // step in the URL. Complete it through the original address form.
  await page.goto(origin+'/fr-zh/checkout?step=payment')
  await expect(page.getByTestId('submit-address-button')).toBeVisible()
  await expect(page).toHaveURL(origin+'/fr-zh/checkout?step=address')
  await expect(page.locator('[name="shipping_address.country_code"]')).toHaveValue('fr')
  await expect(page.locator('[name="shipping_address.address_1"]')).toHaveValue('')
  await expect(page.getByTestId('shipping-email-input')).toHaveValue('context-browser@example.invalid')
  await fit();await page.screenshot({path:join(output,`context-checkout-${initial}-${width}-draft.png`)})
  for(const [key,value] of Object.entries({first_name:'Owned',last_name:'Buyer',address_1:'Completed French street',city:'Paris',postal_code:'75001',phone:'+33 111'}))await page.locator(`[name="shipping_address.${key}"]`).fill(value)
  await page.getByTestId('submit-address-button').click()
  await expect(page).toHaveURL(origin+'/fr-zh/checkout?step=delivery')
  const completed=await api(path+'/contacts')
  assert.equal(completed.version,'8');assert.equal(completed.shipping_address.address_1,'Completed French street')
  assert.deepEqual(completed.billing_address,beforeContacts.billing_address)
  await page.goto(origin+'/fr-zh'+suffix)
  await fit();await open();await page.screenshot({path:join(output,`context-checkout-${initial}-${width}-usd.png`)})
  await choose('Deutschland — English (United Kingdom)');await expect(page).toHaveURL(origin+'/de-en'+suffix)
  const euros=await api(path+'/amounts'),final=await api(path+'/contacts')
  assert.equal(euros.version,'9');assert.equal(euros.totals.total,'100');assert.equal(euros.shipping_methods.length,0)
  assert.equal(final.shipping_address,null);assert.deepEqual(final.billing_address,beforeContacts.billing_address)
  assert.deepEqual((await api(path+'/promotions')).promotions,[])
  assert.equal(await page.evaluate(()=>localStorage.getItem('retail:cart-context-intent:v1')),null)
  assert.equal(posts.length,4);assert.equal(new Set(posts.map(p=>p.key)).size,3)
  await fit();await page.screenshot({path:join(output,`context-checkout-${initial}-${width}-final.png`)})
  cases.push({initial,width,cartID:id,setup_actual_private_http:true,existing_delivery_and_discount:true,same_region_preserves_method_and_address:true,foreign_region_clears_delivery_tax_and_manual_currency_discount:true,new_currency_automatic_discount:true,lost_success_exact_retry:true,draft_checkout_address_filled_via_original_form:true,incomplete_draft_forces_address_step:true,billing_preserved:true,exact_usd_total:dollars.totals.total,final_total:euros.totals.total,final_version:'9',unique_operations:3,context_posts:4})
  await context.close();page=undefined
 }
 assert.deepEqual(errors,[])
 await writeFile(join(output,'browser-result.json'),JSON.stringify({actual_gin_mysql_redis_astro:true,api_mocking:false,checkout_combination:true,fault_injection:'Discard actual successful context POST response only',cases,errors},null,2)+'\n')
 console.log(JSON.stringify({passed:true,cases:cases.length,errors:errors.length}))
}catch(e){
 if(page)await page.screenshot({path:join(output,'failure.png')}).catch(()=>{})
 await writeFile(join(output,'browser-failure.json'),JSON.stringify({error:String(e),cases,errors},null,2)+'\n');throw e
}finally{await browser?.close()}
