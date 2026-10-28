import assert from 'node:assert/strict'
import {createRequire} from 'node:module'
import {dev} from 'astro'
import react from '@astrojs/react'
const require=createRequire(new URL('../../admin/package.json',import.meta.url)),{chromium,expect}=require('@playwright/test')
const target=new URL(process.env.STOREFRONT_TEST_GIN??'')
assert.equal(target.protocol,'http:');assert.equal(target.hostname,'127.0.0.1');assert.ok(target.port)
let server,browser
try{
 server=await dev({root:new URL('./commerce-fixture/',import.meta.url),configFile:false,output:'server',integrations:[react()],devToolbar:{enabled:false},server:{host:'127.0.0.1',port:19200},vite:{server:{strictPort:true,proxy:{'/api':{target:target.origin,changeOrigin:false}}},resolve:{dedupe:['react','react-dom']}},logLevel:'error'})
 const origin='http://127.0.0.1:19200'
 const warm=await fetch(origin+'/dk-en/products/browser-add-product',{signal:AbortSignal.timeout(60000)})
 assert.equal(warm.status,200);assert.match(await warm.text(),/Owned commerce fixture/)
 browser=await chromium.launch({headless:true,...(process.env.ADMIN_TEST_BROWSER?{executablePath:process.env.ADMIN_TEST_BROWSER}:{})})
 const context=await browser.newContext({viewport:{width:390,height:900}}),page=await context.newPage(),errors=[]
 page.on('pageerror',e=>{errors.push(e.message);console.error('Commerce page error',e.message)});page.on('console',m=>{if(m.type()==='error')console.error('Commerce console',m.text())});page.setDefaultTimeout(15000)
 await page.goto(origin+'/dk-en/products/browser-add-product?closed')
 await expect(page.getByTestId('add-product-button')).toBeDisabled()
 // Initialization dependency failure must remain an error, not become an empty
 // cart with an enabled purchase button. No backend response is forged happy.
 await page.route('**/api/store/cart',r=>r.fulfill({status:503,json:{}}))
 const failedInitialization=page.waitForResponse(r=>r.url()===origin+'/api/store/cart',{timeout:30000})
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
  await expect(page.getByRole('heading',{name:'Owned cart destination'})).toBeVisible()
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
 await context.close()
 console.log('CART_COMMERCE_BUTTON_BROWSER '+JSON.stringify({actual_astro_react_button:true,actual_gin_mysql_redis:true,initialization_failure_disabled:true,explicit_closed_disabled:true,viewports:[390,1440],loading_state:true,real_navigation:true,reload_merge_tier_price:true,noncredential_intent_cleared:true,production_page_mounted:false,full_cart_accepted:false}))
}finally{await browser?.close();if(server)await server.stop()}
