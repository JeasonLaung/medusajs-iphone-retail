import assert from 'node:assert/strict'
import {createServer} from 'node:http'
import {createHmac,createHash} from 'node:crypto'
import {spawn} from 'node:child_process'
import {once} from 'node:events'
import {createRequire} from 'node:module'
import {mkdir} from 'node:fs/promises'
import {fileURLToPath} from 'node:url'
import {setTimeout as delay} from 'node:timers/promises'
import {confirmationFixture} from './order-confirmation.fixture.ts'
const require=createRequire(new URL('../../admin/package.json',import.meta.url)),{chromium,expect}=require('@playwright/test')
const host='127.0.0.1',port=19386,shop=`http://${host}:${port}`,token='a'.repeat(64),csrf='b'.repeat(64),key='71'.repeat(32),order='order_created',errors=[]
const account={id:'cust_payment',email:'buyer@example.invalid',first_name:'Buyer',last_name:'Customer',phone:'',verified_at:'2026-10-30T00:00:00Z'}
const region={id:'region',name:'Europe',currency_code:'eur',automatic_taxes:false,countries:[{iso_2:'dk',iso_3:'dnk',num_code:'208',name:'Denmark',display_name:'Denmark',region_id:'region'}]}
const consent={expected_version:3,collection_id:'paycol_extra',amount:'6.00',currency_code:'eur'}
let records=[],inputs=[],cancellations=0,lose=true,blocked=false,locale='en',paid=false,detailReads=0,customerWrites=0,child,browser
const receipt=()=>records.at(-1)
const api=createServer(async(req,res)=>{
 try{
 const path=new URL(req.url,'http://fixture').pathname;let status=200,body={}
 if(path.startsWith('/api/store/customer/')){
  let text='';for await(const chunk of req)text+=chunk
  const fields=['retail-customer-bridge-v1',req.headers['x-retail-bridge-time'],req.headers['x-retail-client-ip'],req.method,req.url,req.headers.host,req.headers.origin??'',req.headers.cookie??'',req.headers['x-csrf-token']??'',createHash('sha256').update(text).digest('hex')]
  assert.equal(req.headers['x-retail-bridge-proof'],createHmac('sha256',Buffer.from(key,'hex')).update(fields.join('\n')).digest('hex'))
  assert.equal(req.headers.cookie,`retail_customer_session=${token}; retail_customer_csrf=${csrf}`)
  assert.equal(req.headers.authorization,undefined)
  if(req.method==='POST'){customerWrites++;assert.equal(req.headers['x-csrf-token'],csrf)}
  if(path.endsWith('/me'))body={customer:account}
  else if(path.endsWith('/addresses'))body={addresses:[]}
  else if(path.endsWith('/orders/'+order+'/progress'))body={order_id:order,currency_code:'eur',events:[{id:'placed',kind:'placed',occurred_at:'2026-10-30T00:00:00Z'}],shipments:[]}
  else if(path.endsWith('/orders/'+order)){
   detailReads++;body=structuredClone(confirmationFixture);body.locale=locale;body.amounts.version=3;body.amounts.totals.total='164.75';body.amounts.totals.original_total='164.75';body.amounts.totals.tax_total='37.75';body.amounts.totals.original_tax_total='37.75';Object.assign(body.amounts.lines[0].totals,{total:'156',original_total:'156',tax_total:'36',original_tax_total:'36'});body.amounts.summary={transaction_total:paid?'164.75':'158.75',pending_difference:paid?'0':'6'}
  }else if(path.endsWith('/orders/'+order+'/payments')&&req.method==='GET'){
   if(blocked)status=503
   else body={order_id:order,order_version:3,currency_code:'eur',pending_difference:paid?'0':'6',next:!receipt()||receipt().can_retry?{...consent,...(receipt()?{previous_operation_id:receipt().operation_id}:{})}:null,payments:records}
  }else if(path.endsWith('/orders/'+order+'/payments')&&req.method==='POST'){
   const input=JSON.parse(text);inputs.push(input)
   assert.equal(input.expected_version,'3');assert.equal(input.amount,'6.00');assert.equal(input.collection_id,consent.collection_id);assert.equal(input.currency_code,'eur')
   let saved=records.find(r=>r.operation_id===input.operation_id)
   if(!saved){assert.ok(!receipt()||receipt().can_retry);if(receipt())assert.equal(input.previous_operation_id,receipt().operation_id);saved={operation_id:input.operation_id,...(input.previous_operation_id?{previous_operation_id:input.previous_operation_id}:{}),order_id:order,order_version:3,collection_id:consent.collection_id,amount:'6.00',currency_code:'eur',state:'pending',applied:false,can_cancel:true,can_retry:false,checkout_url:null};records.push(saved)}
   else assert.deepEqual(input,inputs.find(i=>i.operation_id===input.operation_id))
   if(lose){lose=false;blocked=true;req.socket.destroy();return}
   const {checkout_url,...minimal}=saved;status=202;body={payment:minimal}
  }else if(path.endsWith('/cancel')&&req.method==='POST'){
   const id=path.split('/').at(-2);assert.equal(id,receipt().operation_id);assert.ok(receipt().can_cancel);assert.match(JSON.parse(text).operation_id,/^[0-9a-f-]{36}$/);cancellations++;Object.assign(receipt(),{state:'canceled',can_cancel:false,can_retry:true});const {checkout_url,...minimal}=receipt();body={payment:minimal}
  }else if(path.includes('/payments/')){
   const id=path.split('/').at(-1),row=records.find(r=>r.operation_id===id);if(!row)status=404;else body={payment:row}
  }else status=404
  res.writeHead(status,{'Content-Type':'application/json','Cache-Control':'private, no-store'});res.end(JSON.stringify(body));return
 }
 switch(path){
 case '/api/store/regions':body={regions:[region],count:1};break
 case '/api/store/locales':body={locales:[],count:0};break
 case '/api/store/product-categories':body={product_categories:[],count:0,limit:2,offset:0};break
 case '/api/store/collections':body={collections:[],count:0,limit:6,offset:0};break
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
 browser=await chromium.launch({headless:true,executablePath:process.env.ADMIN_TEST_BROWSER||'/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'})
 const output=new URL('../../../logs/gin-migration/order-payment-ui/',import.meta.url);await mkdir(output,{recursive:true})
 for(const segment of ['dk-en','dk-zh'])for(const width of [390,1440]){
  records=[];inputs=[];cancellations=0;lose=true;blocked=false;paid=false;detailReads=0;customerWrites=0;locale=segment.endsWith('zh')?'zh-CN':'en'
  const context=await browser.newContext({viewport:{width,height:1000},reducedMotion:'reduce'}),page=await context.newPage();page.on('pageerror',e=>errors.push(e.message))
  await context.addCookies([{name:'retail_customer_session',value:token,url:shop},{name:'retail_customer_csrf',value:csrf,url:shop}])
  await page.goto(`${shop}/${segment}/account/orders/details/${order}`)
  await expect(page.getByTestId('supplemental-prepare')).toBeVisible();await expect(page.getByTestId('supplemental-due')).toContainText('6')
  await expect(page.getByTestId('cart-total')).toHaveAttribute('data-value','164.75');await expect(page.getByTestId('product-price')).toContainText('156')
  await page.getByTestId('supplemental-prepare').click()
  await expect(page.getByRole('button',{name:locale==='en'?'Retry the same request':'重试同一请求',exact:true})).toBeVisible()
  assert.equal(records.length,1);assert.equal(inputs.length,1)
  blocked=false
  await page.getByRole('button',{name:locale==='en'?'Retry the same request':'重试同一请求',exact:true}).click()
  await expect(page.getByRole('button',{name:locale==='en'?'Cancel unsent payment':'取消未发送的补款',exact:true})).toBeVisible()
  assert.equal(inputs.length,2);assert.equal(inputs[0].operation_id,inputs[1].operation_id)
  await page.reload()
  await expect(page.getByRole('button',{name:locale==='en'?'Cancel unsent payment':'取消未发送的补款',exact:true})).toBeVisible()
  await expect(page.getByTestId('supplemental-prepare')).toHaveCount(0)
  assert.equal(inputs.length,2);assert.equal(records.length,1)
  await page.getByRole('button',{name:locale==='en'?'Cancel unsent payment':'取消未发送的补款',exact:true}).click()
  await expect(page.getByTestId('supplemental-prepare')).toBeVisible()
  await page.getByTestId('supplemental-prepare').click();await expect(page.getByTestId('supplemental-payment-receipt')).toHaveCount(2)
  assert.equal(cancellations,1);assert.equal(records.length,2);assert.notEqual(records[0].operation_id,records[1].operation_id);assert.equal(inputs.at(-1).previous_operation_id,records[0].operation_id)
  Object.assign(receipt(),{state:'open',can_cancel:false,checkout_url:'https://www.mollie.com/checkout/owned'})
  await page.getByRole('button',{name:locale==='en'?'Refresh payment status':'刷新付款状态',exact:true}).click()
  await expect(page.getByTestId('supplemental-checkout-link')).toBeVisible()
  const returning=shop+'/api/mollie-return?cart_id=retired_cart&order_id='+order+'&order_payment_id='+receipt().operation_id
  await context.route('https://www.mollie.com/**',route=>route.fulfill({contentType:'text/html',body:`<a href="${returning.replaceAll('&','&amp;')}">Return to store</a>`}))
  await page.getByTestId('supplemental-checkout-link').click();await expect(page).toHaveURL('https://www.mollie.com/checkout/owned')
  Object.assign(receipt(),{state:'paid',applied:false,checkout_url:null})
  const writesBeforeReturn=customerWrites
  await page.getByRole('link',{name:'Return to store'}).click()
  await expect(page).toHaveURL(new RegExp('/'+segment+'/account/orders/details/'+order+'#order-supplemental-payment'))
  await expect(page.getByTestId('supplemental-payment-receipt').last()).toHaveAttribute('data-applied','false')
  assert.equal(customerWrites,writesBeforeReturn)
  await page.evaluate(()=>window.scrollTo(0,0));await delay(100)
  await page.screenshot({path:fileURLToPath(new URL(`${segment}-${width}-pending.png`,output)),fullPage:true})
  paid=true;Object.assign(receipt(),{applied:true})
  await page.getByRole('button',{name:locale==='en'?'Refresh payment status':'刷新付款状态',exact:true}).click()
  await expect(page.getByTestId('supplemental-payment-receipt').last()).toHaveAttribute('data-applied','true')
  await expect(page.getByTestId('supplemental-payment-receipt').last()).toContainText(locale==='en'?'Additional payment received':'补款已收妥')
  await expect(page.getByTestId('payment-amount')).toContainText('158.75')
  await expect(page.getByTestId('supplemental-checkout-link')).toHaveCount(0)
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
  assert.ok(detailReads<8,'cash refresh loop')
  await page.evaluate(()=>window.scrollTo(0,0));await delay(100)
  await page.screenshot({path:fileURLToPath(new URL(`${segment}-${width}-paid.png`,output)),fullPage:true})
  await context.close()
 }
 assert.deepEqual(errors,[])
 console.log('ORDER_PAYMENT_BROWSER_REHEARSAL '+JSON.stringify({built_astro:true,signed_formal_customer_routes:true,languages:['en','zh'],widths:[390,1440],exact_consent:true,lost_post_same_uuid:true,reload_recovers_existing_instruction:true,explicit_canceled_predecessor:true,no_cart_bearer:true,read_only_provider_return:true,paid_not_success_until_applied:true,original_and_supplemental_amounts:true,no_refresh_loop:true,no_overflow:true,browser_errors:0,real_gin_mysql:false,independent_payment_cli:false,real_provider:false}))
}finally{await browser?.close();if(child&&child.exitCode===null){child.kill();await once(child,'exit')}await new Promise(resolve=>api.close(resolve))}
