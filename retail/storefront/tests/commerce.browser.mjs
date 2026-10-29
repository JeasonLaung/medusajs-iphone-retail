import assert from 'node:assert/strict'
import {createRequire} from 'node:module'
import {fileURLToPath} from 'node:url'
import {spawn} from 'node:child_process'
import {once} from 'node:events'
const require=createRequire(new URL('../../admin/package.json',import.meta.url)),{chromium,expect}=require('@playwright/test')
const target=new URL(process.env.STOREFRONT_TEST_GIN??'')
assert.equal(target.protocol,'http:');assert.equal(target.hostname,'127.0.0.1');assert.ok(target.port)
let server,browser
try{
 const origin='http://127.0.0.1:19200'
 server=spawn(process.execPath,['scripts/start.mjs'],{env:{...process.env,HOST:'127.0.0.1',PORT:'19200',GIN_API_ORIGIN:target.origin,STOREFRONT_ORIGIN:origin,ASTRO_TELEMETRY_DISABLED:'1'},stdio:['ignore','ignore','pipe'],windowsHide:true})
 let stderr='';server.stderr.on('data',data=>{stderr=(stderr+data).slice(-4000)})
 server.on('error',()=>{})
 let ready=false
 for(let i=0;i<100;i++){
  assert.equal(server.exitCode,null,'Owned production Astro failed to start')
  try{if((await fetch(origin+'/health')).status===200){ready=true;break}}catch{}
  await new Promise(resolve=>setTimeout(resolve,100))
 }
 assert.ok(ready,'Owned production Astro not ready')
 const warm=await fetch(origin+'/dk-en/products/browser-add-product',{signal:AbortSignal.timeout(60000)})
 assert.equal(warm.status,200);assert.match(await warm.text(),/Browser phone/)
 browser=await chromium.launch({headless:true,...(process.env.ADMIN_TEST_BROWSER?{executablePath:process.env.ADMIN_TEST_BROWSER}:{})})
 const context=await browser.newContext({viewport:{width:390,height:900}}),page=await context.newPage(),errors=[]
 page.on('pageerror',e=>{errors.push(e.message);console.error('Commerce page error',e.message)});page.on('console',m=>{if(m.type()==='error')console.error('Commerce console',m.text())});page.setDefaultTimeout(15000)
 // Initialization dependency failure must remain an error, not become an empty
 // cart with an enabled purchase button. No backend response is forged happy.
 await page.route('**/api/store/cart',r=>r.fulfill({status:503,json:{}}))
 const failedInitialization=page.waitForResponse(r=>r.url()===origin+'/api/store/cart',{timeout:30000})
 const foreign=await context.request.post(origin+'/api/store/cart-sessions',{headers:{Origin:'https://foreign.example','Content-Type':'application/json'},data:'{}'})
 assert.equal(foreign.status(),403)
 const unsupported=await context.request.get(origin+'/api/store/cart?store=other')
 assert.equal(unsupported.status(),404)
 await page.goto(origin+'/dk-en/products/browser-add-product')
 assert.equal((await failedInitialization).status(),503)
 await expect(page.getByRole('status')).toContainText('Could not add this item')
 await expect(page.getByTestId('add-product-button')).toBeDisabled()
 await page.unroute('**/api/store/cart')
 const versions=[]
 for(const width of [390,1440]){
  await page.setViewportSize({width,height:900})
  await page.goto(origin+'/dk-en/products/browser-add-product')
  const button=page.getByTestId('add-product-button')
  await expect(button).toBeEnabled()
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
  let release
  const gate=new Promise(resolve=>{release=resolve})
  await page.route('**/line-items',async route=>{await gate;await route.continue()})
  const request=page.waitForRequest(r=>r.url().endsWith('/line-items'))
  await button.click()
  const sent=await request
  assert.equal(sent.headers().origin,origin)
  assert.deepEqual(Object.keys(sent.postDataJSON()).sort(),['quantity','variant_id','version'])
  await expect(button).toHaveText('Adding…');await expect(button).toBeDisabled()
  release()
  await expect(page).toHaveURL(origin+'/dk-en/cart')
  await expect(page.getByRole('heading',{name:'Cart',exact:true})).toBeVisible()
  await expect(page.getByTestId('product-row')).toHaveCount(1)
  await expect(page.getByTestId('nav-cart-link')).toContainText(width===390?'1':'2')
  await expect(page.getByText('Available to order',{exact:true})).toBeVisible()
  await expect(page.getByTestId('product-select-button')).toHaveAttribute('max','4')
  await expect(page.getByTestId('checkout-button')).toHaveCount(0)
  await page.unroute('**/line-items')
  const current=await page.evaluate(async()=>{const r=await fetch('/api/store/cart',{cache:'no-store'});if(r.status!==200)throw Error('Current failed');return r.json()})
  versions.push(current.cart.version)
  const amounts=await page.evaluate(async id=>{const r=await fetch(`/api/store/carts/${id}/amounts`,{cache:'no-store'});if(r.status!==200)throw Error('Amounts failed');return r.json()},current.cart.id)
  assert.equal(amounts.lines.length,1)
  assert.equal(amounts.lines[0].quantity,width===390?'1':'2')
  assert.equal(amounts.totals.total,width===390?'100.000000000000000000001':'160.000000000000000000002')
  assert.equal(await page.evaluate(()=>localStorage.getItem('retail.cart.intent.v1')),null)
 }
 assert.deepEqual(versions,['2','3']);assert.deepEqual(errors,[])
 await page.getByRole('button',{name:'Increase quantity',exact:true}).click()
 await expect(page.getByTestId('product-select-button')).toHaveValue('3')
 await expect(page.getByTestId('cart-total')).toHaveAttribute('data-value','240.000000000000000000003')
 await expect(page.getByTestId('nav-cart-link')).toHaveAttribute('aria-label','Cart (3)')
 await page.screenshot({path:fileURLToPath(new URL('../../../logs/gin-migration/commerce-cart-live.png',import.meta.url)),fullPage:true})
 const lost=[]
 const drop=async route=>{
  const request=route.request();assert.equal(request.method(),'POST')
  const response=await route.fetch();assert.equal(response.status(),200)
  lost.push({key:request.headers()['idempotency-key'],body:request.postDataJSON()})
  await route.abort('failed')
 }
 await page.route('**/line-items/*',drop)
 await page.getByRole('button',{name:'Increase quantity',exact:true}).click()
 await expect(page.getByTestId('cart-recover-button')).toBeVisible()
 assert.equal(lost.length,1);assert.equal(lost[0].body.quantity,'4')
 await page.unroute('**/line-items/*')
 await page.reload()
 await expect(page.getByTestId('cart-recover-button')).toBeVisible()
 await expect(page.getByRole('button',{name:'Increase quantity',exact:true})).toBeDisabled()
 const recoveryGap=await page.evaluate(()=>document.querySelector('[data-testid="cart-container"]').getBoundingClientRect().top-document.querySelector('[data-testid="cart-recover-button"]').getBoundingClientRect().bottom)
 assert.ok(recoveryGap>=0&&recoveryGap<=48,'recovery feedback must remain next to the cart')
 await page.screenshot({path:fileURLToPath(new URL('../../../logs/gin-migration/commerce-cart-recovery.png',import.meta.url)),fullPage:true})
 assert.equal(await page.evaluate(()=>JSON.parse(localStorage.getItem('retail.cart.intent.v1')).kind),'set')
 const recovery=page.waitForRequest(r=>r.method()==='POST'&&r.url().includes('/line-items/'))
 await page.getByTestId('cart-recover-button').click()
 const replay=await recovery
 assert.equal(replay.headers()['idempotency-key'],lost[0].key);assert.deepEqual(replay.postDataJSON(),lost[0].body)
 await expect(page.getByTestId('cart-recover-button')).toHaveCount(0)
 await expect(page.getByTestId('nav-cart-link')).toHaveAttribute('aria-label','Cart (4)')
 await expect(page.getByTestId('product-select-button')).toHaveValue('4')
 await expect(page.getByTestId('cart-total')).toHaveAttribute('data-value','320.000000000000000000004')
 await expect(page.getByRole('button',{name:'Increase quantity',exact:true})).toBeDisabled()
 await page.route('**/api/store/products/browser-add-product?*',r=>r.fulfill({status:503,json:{}}))
 const unavailableObservation=page.waitForResponse(r=>r.url().includes('/api/store/products/browser-add-product?'))
 await page.reload()
 assert.equal((await unavailableObservation).status(),503)
 await expect(page.getByText('Availability will be checked at checkout.',{exact:true})).toBeVisible()
 await expect(page.getByTestId('product-delete-button')).toBeEnabled()
 await expect(page.getByTestId('cart-total')).toHaveAttribute('data-value','320.000000000000000000004')
 await page.unroute('**/api/store/products/browser-add-product?*')
 await page.route('**/line-items/*',drop)
 await page.getByTestId('product-delete-button').click()
 await expect(page.getByTestId('cart-recover-button')).toBeVisible()
 assert.equal(lost.length,2);assert.equal(lost[1].body.quantity,'0')
 await page.unroute('**/line-items/*');await page.reload()
 await expect(page.getByTestId('empty-cart-message')).toBeVisible()
 await expect(page.getByTestId('cart-recover-button')).toBeVisible()
 const removalRecovery=page.waitForRequest(r=>r.method()==='POST'&&r.url().includes('/line-items/'))
 await page.getByTestId('cart-recover-button').click()
 const removalReplay=await removalRecovery
 assert.equal(removalReplay.headers()['idempotency-key'],lost[1].key);assert.deepEqual(removalReplay.postDataJSON(),lost[1].body)
 await expect(page.getByTestId('cart-recover-button')).toHaveCount(0)
 await expect(page.getByTestId('empty-cart-message')).toBeVisible()
 await expect(page.getByTestId('nav-cart-link')).toHaveAttribute('aria-label','Cart (0)')
 assert.equal(await page.evaluate(()=>localStorage.getItem('retail.cart.intent.v1')),null)
 await page.screenshot({path:fileURLToPath(new URL('../../../logs/gin-migration/commerce-cart-empty.png',import.meta.url)),fullPage:true})
 assert.deepEqual(errors,[])
 await context.close()
 console.log('CART_COMMERCE_BUTTON_BROWSER '+JSON.stringify({foreign_origin_and_query_rejected:true,live_header_count:true,formal_private_cart_bridge:true,actual_astro_react_button:true,actual_gin_mysql_redis:true,initialization_failure_disabled:true,viewports:[390,1440],loading_state:true,real_navigation:true,reload_merge_tier_price:true,original_cart_component:true,current_variant_stock:true,failed_observation_preserves_amount_and_removal:true,quantity_command:true,lost_quantity_and_removal_response_recovery:true,exact_original_operation_keys:true,explicit_recovery_only:true,noncredential_intent_cleared:true,production_page_mounted:true,full_cart_accepted:false}))
}finally{await browser?.close();if(server&&server.exitCode===null){const ended=once(server,'exit');server.kill();await ended}}
