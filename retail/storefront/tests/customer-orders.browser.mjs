import assert from 'node:assert/strict'
import {createServer} from 'node:http'
import {createHmac,createHash} from 'node:crypto'
import {spawn} from 'node:child_process'
import {once} from 'node:events'
import {createRequire} from 'node:module'
import {mkdir,writeFile} from 'node:fs/promises'
import {fileURLToPath} from 'node:url'
import {setTimeout as delay} from 'node:timers/promises'
import {confirmationFixture} from './order-confirmation.fixture.ts'
const require=createRequire(new URL('../../admin/package.json',import.meta.url)),{chromium,expect}=require('@playwright/test')
const host='127.0.0.1',port=19376,shop=`http://${host}:${port}`,token='a'.repeat(64),csrf='b'.repeat(64),key='71'.repeat(32),errors=[]
const account={id:'cust_orders',email:'buyer@example.invalid',first_name:'Buyer',last_name:'Customer',phone:'',verified_at:'2026-10-30T00:00:00Z'}
const region={id:'region',name:'Europe',currency_code:'eur',automatic_taxes:false,countries:[{iso_2:'dk',iso_3:'dnk',num_code:'208',name:'Denmark',display_name:'Denmark',region_id:'region'}]}
const summary={id:'order_created',display_id:'1001',custom_display_id:confirmationFixture.custom_display_id,status:'pending',created_at:confirmationFixture.created_at,currency_code:'eur',total:'158.75',original_total:'158.75',items:[{id:'order_line',title:'Phone',thumbnail:confirmationFixture.items[0].thumbnail,quantity:'1',sku:'PHONE-128'}]}
let owned=false,lose=true,posts=0,listFailure=false,detailFailure=false,recoveryBlocked=false,authenticated=true,empty=false,child,browser
const api=createServer(async(req,res)=>{
 try{
 const path=new URL(req.url,'http://fixture').pathname;let status=200,body={}
 if(path.startsWith('/api/store/customer/')){
  let text='';for await(const chunk of req)text+=chunk
  const fields=['retail-customer-bridge-v1',req.headers['x-retail-bridge-time'],req.headers['x-retail-client-ip'],req.method,req.url,req.headers.host,req.headers.origin??'',req.headers.cookie??'',req.headers['x-csrf-token']??'',createHash('sha256').update(text).digest('hex')]
  assert.equal(req.headers['x-retail-bridge-proof'],createHmac('sha256',Buffer.from(key,'hex')).update(fields.join('\n')).digest('hex'))
  const attaching=path.endsWith('/attach');assert.equal((req.headers.cookie??'').includes('retail_cart_possession='),attaching)
  if(!authenticated){status=401;body={error:{code:'customer.unauthorized'}}}
  else if(path.endsWith('/me'))body={customer:account}
  else if(path.endsWith('/addresses'))body={addresses:[]}
  else if(path.endsWith('/attach')){posts++;assert.equal(req.method,'POST');assert.equal(text,'{}');assert.equal(req.headers['x-csrf-token'],csrf);owned=true;if(lose){lose=false;recoveryBlocked=true;req.socket.destroy();return}body={order_id:'order_created'}}
  else if(path.endsWith('/orders')){if(listFailure){status=503}else body={orders:owned&&!empty?[summary]:[],count:owned&&!empty?1:0,limit:20,offset:0}}
  else if(path.endsWith('/orders/order_created')){if(detailFailure||recoveryBlocked)status=503;else if(!owned)status=404;else body=confirmationFixture}
  else status=404
  res.writeHead(status,{'Content-Type':'application/json','Cache-Control':'private, no-store'});res.end(JSON.stringify(body));return
 }
 switch(path){
 case '/api/store/regions':body={regions:[region],count:1};break
 case '/api/store/locales':body={locales:[],count:0};break
 case '/api/store/product-categories':body={product_categories:[],count:0,limit:2,offset:0};break
 case '/api/store/collections':body={collections:[],count:0,limit:6,offset:0};break
 case '/api/store/orders/order_created/confirmation':body=confirmationFixture;res.writeHead(200,{'Content-Type':'application/json','Cache-Control':'private, no-store'});res.end(JSON.stringify(body));return
 case '/api/store/cart':status=401;break
 default:status=404
 }
 res.writeHead(status,{'Content-Type':'application/json','Cache-Control':'no-store'});res.end(JSON.stringify(body))
 }catch(e){errors.push(String(e));res.writeHead(500);res.end('{}')}
})
try{
 const probe=createServer();probe.listen(port,host);await once(probe,'listening');await new Promise(resolve=>probe.close(resolve))
 api.listen(0,host);await once(api,'listening');const address=api.address();assert.ok(address&&typeof address!=='string')
 child=spawn(process.execPath,['scripts/start.mjs'],{env:{...process.env,HOST:host,PORT:String(port),GIN_API_ORIGIN:`http://${host}:${address.port}`,STOREFRONT_ORIGIN:shop,CUSTOMER_BRIDGE_KEY:key},stdio:'ignore'})
 let ready=false;for(let i=0;i<100;i++){if(child.exitCode!==null)throw Error('Astro exited');try{if((await fetch(shop+'/health')).ok){ready=true;break}}catch{}await delay(100)}assert.ok(ready)
 browser=await chromium.launch({headless:true,executablePath:process.env.ADMIN_TEST_BROWSER||'C:/Program Files/Google/Chrome/Application/chrome.exe'})
 for(const segment of ['dk-en','dk-zh'])for(const width of [390,1440]){
  owned=false;lose=true;recoveryBlocked=false;posts=0;empty=false;authenticated=true;detailFailure=false;listFailure=false
  const context=await browser.newContext({viewport:{width,height:1000},reducedMotion:'reduce'}),page=await context.newPage();page.on('pageerror',e=>errors.push(e.message))
  await context.addCookies([{name:'retail_customer_session',value:token,url:shop},{name:'retail_customer_csrf',value:csrf,url:shop},{name:'retail_cart_possession',value:token,url:shop}])
  await page.goto(shop+`/${segment}/order/order_created/confirmed`);await expect(page.getByTestId('order-complete-container')).toBeVisible();await expect(page.getByTestId('order-account-attachment-error')).toBeVisible();assert.equal(posts,1)
  recoveryBlocked=false;await page.getByTestId('order-account-attachment-error').getByRole('button').click();await expect(page.getByTestId('order-account-attachment-error')).toHaveCount(0);assert.equal(posts,1)
  // Subsequent private reads deliberately exclude the independent cart cookie.
  await page.goto(shop+`/${segment}/account/orders`);await expect(page.getByTestId('order-card')).toBeVisible();await expect(page.getByTestId('item-sku')).toContainText('PHONE-128');await expect(page.getByTestId('order-amount')).toContainText('158.75')
  await page.getByTestId('order-details-link').click();await expect(page.getByTestId('order-details-container')).toBeVisible();await expect(page.getByTestId('shipping-address-summary')).toContainText('Saved street');await expect(page.getByTestId('cart-total')).toHaveAttribute('data-value','158.75')
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
  const out=new URL('../../../logs/gin-migration/customer-orders/',import.meta.url);await mkdir(out,{recursive:true});await page.screenshot({path:fileURLToPath(new URL(`${segment}-${width}.png`,out)),fullPage:true})
  detailFailure=true;await page.reload();await expect(page.getByTestId('order-details-container')).toHaveCount(0);await expect(page.getByTestId('customer-authenticated').getByRole('button',{name:segment==='dk-en'?'Retry':/重\s*试/})).toBeVisible();detailFailure=false;await page.getByTestId('customer-authenticated').getByRole('button',{name:segment==='dk-en'?'Retry':/重\s*试/}).click();await expect(page.getByTestId('order-details-container')).toBeVisible()
  await page.goto(shop+`/${segment}/account`);if(width===1440)await expect(page.getByTestId('order-wrapper')).toBeVisible()
  listFailure=true;await page.goto(shop+`/${segment}/account/orders`);await expect(page.getByTestId('order-card')).toHaveCount(0);await expect(page.getByTestId('orders-page-wrapper').getByRole('button',{name:segment==='dk-en'?'Retry':/重\s*试/})).toBeVisible();listFailure=false;await page.getByTestId('orders-page-wrapper').getByRole('button',{name:segment==='dk-en'?'Retry':/重\s*试/}).click();await expect(page.getByTestId('order-card')).toBeVisible()
  empty=true;await page.reload();await expect(page.getByTestId('no-orders-container')).toBeVisible()
  authenticated=false;await page.reload();await expect(page.getByTestId('customer-authenticated')).toHaveCount(0)
  await context.close()
 }
 assert.deepEqual(errors,[])
 const report={formal_astro_routes:true,synthetic_api:true,gin_mysql_browser_joint:false,hmac_full_request_verified:true,dual_cookie_attach_only:true,lost_attach_response_recovered_without_second_post:true,account_list_and_details:true,recent_orders:true,empty_error_retry_and_session_expiry:true,exact_total:'158.75',languages:['en','zh'],widths:[390,1440],page_overflow:false,browser_errors:0}
 await writeFile(new URL('../../../logs/gin-migration/customer-orders/report.json',import.meta.url),JSON.stringify(report,null,2));console.log('CUSTOMER_ORDERS_BROWSER '+JSON.stringify(report))
}finally{await browser?.close();if(child&&child.exitCode===null){child.kill();await once(child,'exit')}await new Promise(resolve=>api.close(resolve))}
