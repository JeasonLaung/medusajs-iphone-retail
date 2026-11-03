// Real product page and React commerce adapter; controlled private-cart replies.
// No cart/order/stock/payment mutations reach the shared development backend.
import assert from 'node:assert/strict'
import {randomUUID,createHash} from 'node:crypto'
import {mkdir} from 'node:fs/promises'
import {chromium,expect} from '../../admin/node_modules/@playwright/test/index.mjs'
import {initialVariant} from '../src/lib/product-variants.ts'

const origin=process.env.STOREFRONT_RECOVERY_ORIGIN??'http://127.0.0.1:9300'
assert.equal(new URL(origin).hostname,'127.0.0.1')
const browser=await chromium.launch({headless:true,executablePath:process.env.ADMIN_TEST_BROWSER??'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe'})
const key='retail.cart.intent.v1',csrf='b'.repeat(64),cases=[]
const totals=['subtotal','total','tax_total','original_subtotal','original_total','original_tax_total','discount_subtotal','discount_tax_total','discount_total']
const cartTotals=[...totals,'item_total','item_subtotal','item_tax_total','item_discount_total','original_item_total','original_item_subtotal','original_item_tax_total','shipping_total','shipping_subtotal','shipping_tax_total','shipping_discount_total','original_shipping_total','original_shipping_subtotal','original_shipping_tax_total','credit_line_total','credit_line_subtotal','credit_line_tax_total']
try{
 for(const segment of ['gb-en','gb-zh'])for(const width of [390,1440])for(const scenario of ['expired','committed-response-lost','recovery-outage','server-expired']){
  const context=await browser.newContext({viewport:{width,height:1000},reducedMotion:'reduce'}),page=await context.newPage(),errors=[],writes=[]
  page.on('pageerror',e=>errors.push(e.message))
  let cart=null,regionID,currency,variantID,denyCreate=scenario==='server-expired',denyRead=false,scopeCalls=0
  const capabilities={items:false,checkout:false}
  const amounts=()=>({cart_id:cart.id,version:cart.version,currency_code:currency,totals:Object.fromEntries(cartTotals.map(k=>[k,'0'])),lines:cart.version==='1'?[]:[{id:'line_recovery',variant_id:variantID,unit_price:'1',quantity:'1',is_tax_inclusive:false,totals:Object.fromEntries(totals.map(k=>[k,'0']))}],shipping_methods:[],pricing_recalculated:false,checkout:false})
  await page.route('**/api/store/**',async route=>{
   const request=route.request(),path=new URL(request.url()).pathname
   const reply=(status,json)=>route.fulfill({status,json,headers:{'Cache-Control':'private, no-store'}})
   if(path==='/api/store/cart')return reply(denyRead?503:cart?200:401,cart?{cart,capabilities}:{})
   if(path.endsWith('/amounts'))return reply(200,amounts())
   if(request.method()!=='POST')return route.continue()
   writes.push({path,key:request.headers()['idempotency-key'],body:request.postDataJSON()})
   if(path==='/api/store/cart-sessions'){
    scopeCalls++
    await context.addCookies([{name:'retail_cart_scope_csrf',value:csrf,url:origin},{name:'retail_cart_csrf',value:csrf,url:origin}])
    return reply(200,{csrf_token:csrf,reused:false,expires_at:'2099-01-01T00:00:00Z'})
   }
   if(path==='/api/store/carts'){
    if(denyCreate){denyCreate=false;return reply(401,{})}
    cart={id:'cart_recovery',region_id:regionID,currency_code:currency,sales_channel_id:'channel_recovery',locale:segment.endsWith('-zh')?'zh-CN':'en-GB',version:'1'}
    return reply(201,{cart,capabilities,csrf_token:csrf,expires_at:'2099-01-01T00:00:00Z',replayed:false})
   }
   if(path.endsWith('/line-items')){
    variantID=request.postDataJSON().variant_id;cart.version='2'
    return reply(200,{cart_id:cart.id,version:'2',line_id:'line_recovery',quantity:'1',replayed:false})
   }
   throw Error('Unexpected private write '+path)
  })
  // The navigation target is controlled too; cart rendering has separate tests.
  await page.route(origin+'/'+segment+'/cart',route=>route.fulfill({contentType:'text/html',body:'<!doctype html><title>Recovery navigation target</title><h1>Cart</h1>'}))
  const url=origin+'/'+segment+'/products/swappie-iphone-17e'
  await page.goto(url)
  const button=page.getByTestId('add-product-button')
  await expect(button).toBeEnabled()
  const props=JSON.parse(await page.locator('astro-island[component-url*="gin-product-actions"]').getAttribute('props'))
  const decode=([type,value])=>type===1?value.map(decode):value&&typeof value==='object'?Object.fromEntries(Object.entries(value).map(([k,v])=>[k,decode(v)])):value
  regionID=props.regionID[1];currency=props.currency[1];variantID=initialVariant(decode(props.product),new URL(url).pathname,null,props.initialVariantID[1])?.id
  assert.ok(variantID)
  const original={format:1,kind:'create',regionID,segment,variantID,quantity:'1',operationID:randomUUID(),scopeBinding:createHash('sha256').update(scenario==='server-expired'?csrf:'expired-csrf').digest('hex')}
  if(scenario==='server-expired')await context.addCookies([{name:'retail_cart_scope_csrf',value:csrf,url:origin}])
  if(scenario==='committed-response-lost'){
   cart={id:'cart_recovery',region_id:regionID,currency_code:currency,sales_channel_id:'channel_recovery',locale:segment.endsWith('-zh')?'zh-CN':'en-GB',version:'1'}
   await context.addCookies([{name:'retail_cart_csrf',value:csrf,url:origin}])
  }
  await page.evaluate(({key,original})=>localStorage.setItem(key,JSON.stringify(original)),{key,original})
  await page.reload();await expect(button).toBeEnabled()
  await button.click()
  await expect(button).toHaveText(segment.endsWith('-zh')?'恢复购物车':'Restore cart')
  await expect(page.getByRole('status').first()).toContainText(segment.endsWith('-zh')?'上次的购物车创建操作无法继续':'Your previous cart setup could not be continued')
  assert.equal(writes.length,scenario==='server-expired'?1:0)
  if(scenario==='recovery-outage'){
   denyRead=true;await button.click()
   await expect(page.getByRole('status').first()).toContainText(segment.endsWith('-zh')?'购物车暂时无法恢复':'Your cart could not be restored')
   assert.deepEqual(await page.evaluate(key=>JSON.parse(localStorage.getItem(key)),key),original)
   assert.equal(writes.length,0);denyRead=false
  }
  await button.click()
  await expect(button).toHaveText(segment.endsWith('-zh')?'加入购物车':'Add to cart')
  await expect(page.getByRole('status').first()).toContainText(segment.endsWith('-zh')?'购物车已就绪':'Your cart is ready')
  assert.equal(writes.filter(r=>r.path.endsWith('/line-items')).length,0)
  assert.equal(scopeCalls,scenario==='committed-response-lost'?0:1)
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
  if(scenario==='expired'){
   await mkdir(new URL('../../../logs/gin-migration/cart-recovery/',import.meta.url),{recursive:true})
   await page.screenshot({path:new URL(`../../../logs/gin-migration/cart-recovery/${segment}-${width}.png`,import.meta.url).pathname.replace(/^\/([A-Za-z]:)/,'$1'),fullPage:true})
  }
  await button.click()
  try{await expect(page).toHaveURL(origin+'/'+segment+'/cart')}
  catch(error){console.error(JSON.stringify({segment,width,scenario,writes,status:await page.getByRole('status').allTextContents(),pending:await page.evaluate(key=>localStorage.getItem(key),key)}));throw error}
  assert.equal(writes.filter(r=>r.path.endsWith('/line-items')).length,1)
  assert.equal(writes.filter(r=>r.path==='/api/store/carts').length,scenario==='committed-response-lost'?0:scenario==='server-expired'?2:1)
  assert.equal(await page.evaluate(key=>localStorage.getItem(key),key),null)
  assert.deepEqual(errors,[])
  cases.push({segment,width,scenario});await context.close()
 }
 console.log(JSON.stringify({controlled_cart_transport:true,shared_business_writes:false,cases}))
}finally{await browser.close()}
