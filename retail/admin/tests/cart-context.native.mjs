import assert from 'node:assert/strict'
import {writeFile} from 'node:fs/promises'
import {join} from 'node:path'
import {chromium,expect} from '@playwright/test'

const origin=process.env.CONTEXT_ASTRO_ORIGIN,output=process.env.CONTEXT_BROWSER_OUTPUT
if(!/^http:\/\/127\.0\.0\.1:\d+$/.test(origin??'')||!output)throw Error('Owned context fixture required')
const closed=process.env.CONTEXT_GATES_CLOSED==='1'
const errors=[],cases=[]
let browser,page
async function api(path,body,csrf,key){
 return page.evaluate(async({path,body,csrf,key})=>{
  const r=await fetch(path,{method:body===undefined?'GET':'POST',credentials:'same-origin',cache:'no-store',headers:{Accept:'application/json',...(body===undefined?{}:{'Content-Type':'application/json'}),...(csrf?{'X-CSRF-Token':csrf}:{}),...(key?{'Idempotency-Key':key}:{})},...(body===undefined?{}:{body:JSON.stringify(body)})})
  const value=await r.json();if(r.status!==200&&r.status!==201)throw Error('Fixture HTTP '+r.status+': '+JSON.stringify(value));return value
 },{path,body,csrf,key})
}
const cookie=async()=>{const a=await page.context().cookies();return a.find(v=>v.name==='_retail_locale')}
async function open(){await page.getByRole('button',{name:/^(Country and language|国家与语言)$/}).click();await expect(page.getByRole('dialog')).toBeVisible()}
async function choose(name){await page.getByRole('dialog').getByRole('button',{name,exact:true}).click()}
async function fit(){assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),'page overflow')}
try{
 browser=await chromium.launch({channel:'chrome',headless:true})
 for(const initial of ['en','zh'])for(const width of [390,1440]){
  const context=await browser.newContext({viewport:{width,height:900},reducedMotion:'reduce'})
  page=await context.newPage();page.setDefaultTimeout(20000);page.on('pageerror',e=>errors.push(e.message))
  const posts=[];page.on('request',r=>{if(r.method()==='POST'&&r.url().endsWith('/context'))posts.push({key:r.headers()['idempotency-key'],body:r.postDataJSON()})})
  const other=initial==='en'?'zh':'en',language=other==='zh'?'Chinese Simplified (China)':'English (United Kingdom)',locale=other==='zh'?'zh-CN':'en-GB'
  const suffix='/about?selection=a%2Bb&sort=price#contact'
  await page.goto(origin+'/dk-'+initial+suffix)
  await open();await fit();await page.screenshot({path:join(output,`preferences-${initial}-${width}-initial.png`)})
  await choose('Deutschland — '+language)
  await expect(page).toHaveURL(origin+'/de-'+other+suffix)
  assert.equal(posts.length,0);assert.ok([401,404].includes((await page.request.get(origin+'/api/store/cart')).status()))
  assert.equal((await cookie()).value,locale)
  await page.reload();await expect(page.locator('html')).toHaveAttribute('lang',other==='zh'?'zh-CN':'en')
  assert.equal((await cookie()).sameSite,'Strict')
  if(closed){
   assert.equal(posts.length,0);await fit()
   cases.push({initial,width,no_cart_no_mutation:true,commerce_gates_closed:true,route_cookie_preserved:true})
   await context.close();page=undefined;continue
  }
  // Prepare one real draft cart/item through the same browser/private bridge.
  // The interaction being tested below is the rendered original selector.
  const scope=await api('/api/store/cart-sessions',{})
  const created=await api('/api/store/carts',{region_id:'fixture-region',locale},scope.csrf_token,crypto.randomUUID())
  const cartID=created.cart.id
  await api('/api/store/carts/'+cartID+'/line-items',{version:'1',variant_id:'browser-add-v',quantity:'1'},created.csrf_token,crypto.randomUUID())
  await page.reload();await page.waitForLoadState('networkidle')
  let dropPost=true,dropRead=false,armRead=false
  await page.route('**/api/store/carts/**',async route=>{
   const request=route.request(),url=new URL(request.url())
   if(request.method()==='POST'&&url.pathname.endsWith('/context')){
    const response=await route.fetch();assert.equal(response.status(),200)
    if(dropPost){dropPost=false;await route.abort('failed');return}
    if(armRead&&request.postDataJSON().region_id==='fixture-region'){dropRead=true;armRead=false}
    await route.fulfill({response});return
   }
   if(dropRead&&url.pathname.endsWith('/amounts')){const response=await route.fetch();assert.equal(response.status(),200);dropRead=false;await route.abort('failed');return}
   await route.continue()
  })
  const beforeURL=page.url(),beforeCookie=(await cookie()).value
  await open();await choose('France — Chinese Simplified (China)')
  await expect(page.getByRole('alert')).toBeVisible()
  assert.equal(page.url(),beforeURL);assert.equal((await cookie()).value,beforeCookie)
  const unknown=await page.evaluate(()=>JSON.parse(localStorage.getItem('retail:cart-context-intent:v1')))
  assert.equal(unknown.kind,'save');assert.equal(unknown.version,'2')
  assert.equal((await api('/api/store/cart')).cart.currency_code,'usd')
  await page.reload();await open();await choose('France — Chinese Simplified (China)')
  await expect(page).toHaveURL(origin+'/fr-zh'+suffix)
  assert.equal(posts.length,2);assert.deepEqual(posts[0],posts[1])
  const dollars=await api('/api/store/carts/'+cartID+'/amounts')
  assert.equal(dollars.version,'3');assert.equal(dollars.currency_code,'usd');assert.equal(dollars.totals.total,'46.2000000000000000000011')
  assert.equal((await cookie()).value,'zh-CN')
  await open();await expect(page.getByRole('dialog').getByRole('button',{name:'France — Chinese Simplified (China)',exact:true})).toHaveAttribute('aria-pressed','true');await fit()
  await page.screenshot({path:join(output,`preferences-${initial}-${width}-usd.png`)})
  // Acknowledged write followed by a lost real amounts response: after reload
  // the saved refresh stage must perform no second context POST.
  armRead=true;await choose('Deutschland — English (United Kingdom)')
  await expect(page.getByRole('alert')).toBeVisible()
  assert.equal(page.url(),origin+'/fr-zh'+suffix);assert.equal((await cookie()).value,'zh-CN')
  assert.equal(await page.evaluate(()=>JSON.parse(localStorage.getItem('retail:cart-context-intent:v1')).kind),'refresh')
  const count=posts.length
  await page.reload();await open();await choose('Deutschland — English (United Kingdom)')
  await expect(page).toHaveURL(origin+'/de-en'+suffix);assert.equal(posts.length,count)
  const euros=await api('/api/store/carts/'+cartID+'/amounts')
  assert.equal(euros.version,'4');assert.equal(euros.currency_code,'eur');assert.equal(euros.totals.total,'100.000000000000000000001')
  await page.goBack();await expect(page).toHaveURL(origin+'/fr-zh'+suffix)
  assert.equal((await cookie()).value,'zh-CN');assert.equal(posts.length,count)
  // The source URL remains en while the explicit da preference/cart persists.
  await open();await choose('Danmark — Danish (Denmark)')
  await expect(page).toHaveURL(origin+'/dk-en'+suffix)
  assert.equal((await cookie()).value,'da-DK');await page.reload();assert.equal((await cookie()).value,'da-DK')
  const final=(await api('/api/store/cart')).cart
  assert.equal(final.id,cartID);assert.equal(final.version,'5');assert.equal(final.locale,'da-DK');assert.equal(final.region_id,'fixture-region')
  assert.equal(await page.evaluate(()=>localStorage.getItem('retail:cart-context-intent:v1')),null)
  assert.equal(posts.length,4);assert.equal(new Set(posts.map(v=>v.key)).size,3)
  await fit();await page.screenshot({path:join(output,`preferences-${initial}-${width}-final.png`)})
  cases.push({initial,width,cartID,no_cart_no_mutation:true,post_success_lost_and_same_retry:true,acknowledged_read_loss_no_second_post:true,exact_usd_tax:dollars.totals.total,exact_eur_total:euros.totals.total,back_uses_route_language:true,explicit_locale:'da-DK',unique_operations:3,context_posts:4,final_version:'5'})
  await context.close();page=undefined
 }
 assert.deepEqual(errors,[])
 await writeFile(join(output,closed?'browser-closed-result.json':'browser-result.json'),JSON.stringify({actual_gin_mysql_redis_astro:true,api_mocking:false,fault_injection:'Discard actual successful POST/amounts responses only',cases,errors},null,2)+'\n')
 console.log(JSON.stringify({passed:true,cases:cases.length,errors:errors.length}))
}catch(e){
 if(page)await page.screenshot({path:join(output,'failure.png')}).catch(()=>{})
 await writeFile(join(output,closed?'browser-closed-failure.json':'browser-failure.json'),JSON.stringify({error:String(e),cases,errors},null,2)+'\n');throw e
}finally{await browser?.close()}
