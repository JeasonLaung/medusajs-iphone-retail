import assert from 'node:assert/strict'
import {createServer} from 'node:http'
import {spawn} from 'node:child_process'
import {once} from 'node:events'
import {createRequire} from 'node:module'
import {fileURLToPath} from 'node:url'
import {setTimeout as delay} from 'node:timers/promises'
const require=createRequire(new URL('../../admin/package.json',import.meta.url)),{chromium,expect}=require('@playwright/test')
const host='127.0.0.1',port=19375,shop=`http://${host}:${port}`
const region={id:'region',name:'Europe',currency_code:'eur',automatic_taxes:true,countries:[{iso_2:'dk',iso_3:'dnk',num_code:'208',name:'Denmark',display_name:'Denmark',region_id:'region'}]}
const rowKeys=['subtotal','total','tax_total','original_subtotal','original_total','original_tax_total','discount_subtotal','discount_tax_total','discount_total']
const cartKeys=[...rowKeys,'item_total','item_subtotal','item_tax_total','item_discount_total','original_item_total','original_item_subtotal','original_item_tax_total','shipping_total','shipping_subtotal','shipping_tax_total','shipping_discount_total','original_shipping_total','original_shipping_subtotal','original_shipping_tax_total','credit_line_total','credit_line_subtotal','credit_line_tax_total']
const address={first_name:'Saved',last_name:'Customer',company:'',address_1:'Saved street',address_2:'',city:'Copenhagen',country_code:'dk',province:'',postal_code:'0012',phone:'+45 111'}
const saved={...address,id:'addr_12345678-1234-4234-8234-123456789012',is_default_billing:false,is_default_shipping:true,created_at:'2026-10-29T00:00:00Z',updated_at:'2026-10-29T00:00:00Z'}
let version='2',contacts={email:'',shipping_address:null,billing_address:null},child,browser,failed=false,readFailure=false,loseResponse=false,empty=false
let paymentState='pending',paymentStatusPosts=0;const receipts=new Map(),posts=[],errors=[],shippingPosts=[],shippingReceipts=new Map();let selected=null,loseShippingResponse=false,shippingReadFailure=false
const totals=Object.fromEntries(cartKeys.map(k=>[k,['total','original_total','item_total','original_item_total'].includes(k)?'150':['tax_total','original_tax_total','item_tax_total','original_item_tax_total'].includes(k)?'30':k.includes('subtotal')&&!k.includes('shipping')&&!k.includes('discount')&&!k.includes('credit')?'120':'0']))
const lineTotals=Object.fromEntries(rowKeys.map(k=>[k,totals[k]]))
const delivery=()=>({cart_id:'cart',version,currency_code:'eur',options:[{id:'standard',profile_id:'profile',name:'Home delivery',description:'Tracked delivery',kind:'shipping',amount:'7',is_tax_inclusive:false,insufficient_inventory:false},{id:'pickup',profile_id:'profile',name:'Store pickup',description:'Collect in store',kind:'pickup',amount:'0',is_tax_inclusive:false,insufficient_inventory:false},{id:'unavailable',profile_id:'profile',name:'Unavailable',description:'',kind:'shipping',amount:'10',is_tax_inclusive:false,insufficient_inventory:true}],selected_option_ids:selected?[selected]:[],complete:!!selected})
const displayedTotals=()=>selected==='standard'?{...totals,total:'158.75',original_total:'158.75',tax_total:'31.75',original_tax_total:'31.75',subtotal:'127',original_subtotal:'127',shipping_total:'8.75',shipping_subtotal:'7',shipping_tax_total:'1.75',original_shipping_total:'8.75',original_shipping_subtotal:'7',original_shipping_tax_total:'1.75'}:totals
const detail=()=>({cart_id:'cart',version,...contacts})
const view=()=>({amounts:{cart_id:'cart',version,currency_code:'eur',totals:displayedTotals(),lines:empty?[]:[{id:'line',variant_id:'variant',unit_price:'120',quantity:'1',is_tax_inclusive:false,totals:lineTotals}],shipping_methods:selected?[{id:'method',name:selected==='standard'?'Home delivery':'Store pickup',amount:selected==='standard'?'7':'0',is_tax_inclusive:false,totals:{...lineTotals,total:selected==='standard'?'8.75':'0',subtotal:selected==='standard'?'7':'0',tax_total:selected==='standard'?'1.75':'0'}}]:[],pricing_recalculated:false,checkout:false},items:empty?[]:[{id:'line',title:'Phone',product_id:'p',product_title:'Phone',product_handle:'phone',thumbnail:'/iphone-thumbnails/iphone-16e/white.webp',variant_title:'128GB',option_values:{Storage:'128GB'}}]})
const api=createServer(async(req,res)=>{
 try{
 const url=new URL(req.url,'http://fixture.invalid');let body={},status=200
 if(url.pathname.startsWith('/api/store/cart'))assert.equal(req.headers.host,`127.0.0.1:${port}`)
 switch(url.pathname){
 case '/api/store/regions':if(failed){status=503;break}body={regions:[region],count:1};break
 case '/api/store/locales':body={locales:[],count:0};break
 case '/api/store/product-categories':body={product_categories:[],count:0,limit:2,offset:0};break
 case '/api/store/collections':body={collections:[],count:0,limit:Number(url.searchParams.get('limit')||100),offset:0};break
 case '/api/store/cart':if(!req.headers.cookie?.includes('retail_cart_possession=')){status=401;break}body={cart:{id:'cart',region_id:'region',sales_channel_id:'channel',currency_code:'eur',locale:'en',version},capabilities:{items:false,checkout:false}};break
 case '/api/store/carts/cart/view':if(readFailure){status=503;break}body=view();break
 case '/api/store/carts/cart/delivery-options':if(shippingReadFailure){status=503;break}body=delivery();break
 case '/api/store/carts/cart/payment-status':{
  assert.equal(req.method,'POST');assert.equal(req.headers.origin,shop);assert.equal(req.headers['x-csrf-token'],'a'.repeat(64));let raw='';for await(const chunk of req)raw+=chunk;assert.equal(raw,'{}');paymentStatusPosts++;body={cart_id:'cart',version,state:paymentState};break
 }
 case '/api/store/carts/cart/shipping-methods':{
  assert.equal(req.headers.origin,shop);assert.equal(req.headers['x-csrf-token'],'a'.repeat(64))
  let raw='';for await(const chunk of req)raw+=chunk;const input=JSON.parse(raw),key=req.headers['idempotency-key'];shippingPosts.push({key,input})
  assert.deepEqual(Object.keys(input),['version','option_id']);const old=shippingReceipts.get(key)
  if(old){assert.deepEqual(input,old.input);body={...old.result,replayed:true}}
  else{assert.equal(input.version,version);assert.ok(['standard','pickup'].includes(input.option_id));version=String(BigInt(version)+1n);selected=input.option_id;body={cart_id:'cart',version,option_id:selected,replayed:false};shippingReceipts.set(key,{input,result:body})}
  if(loseShippingResponse){loseShippingResponse=false;status=503};break
 }
 case '/api/store/carts/cart/contacts':
  if(req.method==='GET'){body=detail();break}
  {assert.equal(req.headers.origin,shop);assert.equal(req.headers['x-csrf-token'],'a'.repeat(64));assert.ok(req.headers.cookie.includes('retail_cart_possession='))
  let raw='';for await(const chunk of req)raw+=chunk;const input=JSON.parse(raw),key=req.headers['idempotency-key'];posts.push({key,input})
  assert.equal(input.shipping_address.postal_code,'0012');assert.equal(input.shipping_address.id,undefined);assert.equal(input.customer_id,undefined)
  const old=receipts.get(key)
  if(old){assert.deepEqual(input,old.input);body={...old.result,replayed:true}}
  else{assert.equal(input.version,version);version=String(BigInt(version)+1n);contacts={email:input.email,shipping_address:input.shipping_address,billing_address:input.billing_address};body={cart_id:'cart',version,replayed:false};receipts.set(key,{input,result:body})}
  if(loseResponse){loseResponse=false;status=503}
  break}
 case '/api/store/customer/me':body={customer:{id:'cust_12345678-1234-4234-8234-123456789012',email:'saved@example.invalid',first_name:'Saved',last_name:'Customer',phone:'',verified_at:'2026-10-29T00:00:00Z'}};break
 case '/api/store/customer/addresses':body={addresses:[saved]};break
 default:status=404
 }
 res.writeHead(status,{'Content-Type':'application/json',...(url.pathname.includes('/cart')||url.pathname.includes('/customer')?{'Cache-Control':'private, no-store'}:{})});res.end(JSON.stringify(body))
 }catch(error){errors.push(error.message);res.writeHead(500);res.end('{}')}
})
try{
 const probe=createServer();probe.listen(port,host);await once(probe,'listening');await new Promise(resolve=>probe.close(resolve))
 api.listen(0,host);await once(api,'listening');const apiOrigin=`http://${host}:${api.address().port}`
 child=spawn(process.execPath,['scripts/start.mjs'],{env:{...process.env,HOST:host,PORT:String(port),GIN_API_ORIGIN:apiOrigin,STOREFRONT_ORIGIN:shop,CUSTOMER_BRIDGE_KEY:'b'.repeat(64)},stdio:'ignore'})
 let ready=false;for(let i=0;i<100;i++){if(child.exitCode!==null)throw Error('Owned Astro stopped');try{if((await fetch(shop+'/health')).ok){ready=true;break}}catch{}await delay(100)}assert.ok(ready)
 assert.equal((await fetch(shop+'/dk-en/checkout')).status,404)
 const response=await fetch(shop+'/dk-en/checkout',{headers:{Cookie:`retail_cart_possession=${'b'.repeat(64)}`}});assert.equal(response.status,200);assert.equal(response.headers.get('cache-control'),'private, no-store');const html=await response.text();assert.ok(!html.includes('saved@example.invalid'));assert.ok(!html.includes('Saved street'));assert.match(html,/data-testid="checkout-container"/)
 empty=true;const emptyResponse=await fetch(shop+'/dk-en/checkout',{redirect:'manual',headers:{Cookie:`retail_cart_possession=${'b'.repeat(64)}`}});assert.equal(emptyResponse.status,307);assert.equal(new URL(emptyResponse.headers.get('location'),shop).pathname,'/dk-en/cart');empty=false
 readFailure=true;assert.equal((await fetch(shop+'/dk-en/checkout',{headers:{Cookie:`retail_cart_possession=${'b'.repeat(64)}`}})).status,503);readFailure=false
 assert.equal((await fetch(shop+'/xx-en/checkout')).status,404);failed=true;assert.equal((await fetch(shop+'/dk-en/checkout')).status,503);failed=false
 browser=await chromium.launch({headless:true,executablePath:process.env.ADMIN_TEST_BROWSER||'C:/Program Files/Google/Chrome/Application/chrome.exe'})
 for(const language of ['en','zh'])for(const width of [390,1440]){
  const context=await browser.newContext({reducedMotion:'reduce',viewport:{width,height:1100}}),page=await context.newPage();context.setDefaultTimeout(12000)
  await context.addCookies([{name:'retail_cart_possession',value:'b'.repeat(64),url:shop,httpOnly:true},{name:'retail_cart_csrf',value:'a'.repeat(64),url:shop}]);page.on('pageerror',e=>errors.push(e.message))
  selected=null;shippingPosts.length=0;shippingReceipts.clear();version='2';contacts={email:'',shipping_address:null,billing_address:null};receipts.clear();posts.length=0
  await page.goto(shop+`/dk-${language}/checkout?step=address`);await expect(page.getByTestId('shipping-first-name-input')).toBeVisible();await expect(page.getByTestId('shipping-email-input')).toHaveValue('saved@example.invalid')
  const selector=page.locator('select').filter({has:page.locator('option[value="'+saved.id+'"]')});await expect(selector).toBeVisible();await selector.selectOption(saved.id)
  await expect(page.getByTestId('shipping-first-name-input')).toHaveValue('Saved');await expect(page.getByTestId('shipping-postal-code-input')).toHaveValue('0012')
  await page.getByTestId('shipping-email-input').fill('saved@example.invalid');await page.getByTestId('confirm-email-input').fill('different@example.invalid')
  await page.getByTestId('submit-address-button').click();await expect(page.getByTestId('address-error-message')).toBeVisible();assert.equal(posts.length,0)
  await page.getByTestId('confirm-email-input').fill('SAVED@EXAMPLE.INVALID')
  await page.getByTestId('billing-address-checkbox').uncheck()
  for(const [testID,value] of Object.entries({'billing-first-name-input':'Billing','billing-last-name-input':'Customer','billing-address-input':'Billing street','billing-postal-code-input':'0012','billing-city-input':'Copenhagen'}))await page.getByTestId(testID).fill(value)
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
  await page.screenshot({path:fileURLToPath(new URL(`../../../logs/gin-migration/checkout-${language}-${width}.png`,import.meta.url)),fullPage:true})
  loseResponse=true;await page.getByTestId('submit-address-button').click();await expect.poll(()=>posts.length).toBe(1);await expect(page.getByTestId('submit-address-button')).toBeEnabled();await expect(page.getByTestId('address-error-message')).toBeVisible()
  const journal=await page.evaluate(()=>localStorage.getItem('retail:checkout-contact-intent:v1'));assert.ok(journal);assert.ok(!journal.includes('Saved street'));assert.ok(!journal.includes('saved@example.invalid'))
  await page.reload();await expect(page.getByTestId('shipping-postal-code-input')).toHaveValue('0012');await expect(page.getByTestId('billing-first-name-input')).toHaveValue('Billing')
  await page.getByTestId('submit-address-button').click();await expect(page).toHaveURL(/step=delivery/);await expect(page.getByTestId('checkout-total')).toHaveText('€150.00');assert.equal(posts.length,2);assert.equal(posts[0].key,posts[1].key);assert.deepEqual(posts[0].input,posts[1].input)
  assert.equal(await page.evaluate(()=>localStorage.getItem('retail:checkout-contact-intent:v1')),null)
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
  assert.equal(contacts.billing_address.first_name,'Billing')
  await expect(page.getByTestId('delivery-option-radio')).toHaveCount(3);await expect(page.locator('input[value="unavailable"]')).toBeDisabled()
  loseShippingResponse=true;await page.locator('input[value="standard"]').click();await expect(page.getByTestId('delivery-recover')).toBeEnabled();assert.equal(shippingPosts.length,1)
  await page.reload();await expect(page.getByTestId('delivery-recover')).toBeEnabled();await page.getByTestId('delivery-recover').click();await expect(page.getByTestId('delivery-recover')).toHaveCount(0)
  assert.equal(shippingPosts.length,2);assert.deepEqual(shippingPosts[0],shippingPosts[1]);await expect(page.locator('input[value="standard"]')).toBeChecked();await expect(page.getByTestId('checkout-total')).toHaveText('€158.75')
  shippingReadFailure=true;await page.locator('input[value="pickup"]').click();await expect(page.getByTestId('delivery-recover')).toBeEnabled();assert.equal(shippingPosts.length,3)
  shippingReadFailure=false;await page.reload();await expect(page.getByTestId('delivery-recover')).toBeEnabled();await page.getByTestId('delivery-recover').click();await expect(page.getByTestId('delivery-recover')).toHaveCount(0)
  assert.equal(shippingPosts.length,3);await expect(page.locator('input[value="pickup"]')).toBeChecked();await expect(page.getByTestId('submit-delivery-option-button')).toBeDisabled()
  assert.equal(await page.evaluate(()=>localStorage.getItem('retail:checkout-shipping-intent:v1')),null)
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
  await page.screenshot({path:fileURLToPath(new URL(`../../../logs/gin-migration/delivery-${language}-${width}.png`,import.meta.url)),fullPage:true})

  await page.getByRole('link',{name:language==='zh'?'修改':'Edit',exact:true}).click();await expect(page.getByTestId('shipping-first-name-input')).toHaveValue('Saved')
  await expect(page.getByTestId('back-to-cart-link')).toHaveAttribute('href',`/dk-${language}/cart`)
  paymentState='paid_pending_order';await page.goto(shop+'/api/mollie-return?cart_id=cart');await expect(page).toHaveURL(new RegExp('/dk-en/payment-result'))
  await expect(page.getByTestId('payment-result')).toHaveAttribute('data-status','pending');await expect(page.getByTestId('payment-result')).not.toHaveAttribute('data-status','confirmed')
  for(const [state,expected] of [['authorized','pending'],['failed','failed'],['requires_review','pending']]){
   paymentState=state;await page.goto(shop+`/dk-${language}/payment-result?cart_id=cart`);await expect(page.getByTestId('payment-result')).toHaveAttribute('data-status',expected)
  }
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
  await page.screenshot({path:fileURLToPath(new URL(`../../../logs/gin-migration/payment-result-${language}-${width}.png`,import.meta.url)),fullPage:true})
  const before=paymentStatusPosts;await page.goto(shop+`/dk-${language}/payment-result?cart_id=foreign-cart`);await expect(page.getByTestId('payment-result')).toHaveAttribute('data-status','invalid');assert.equal(paymentStatusPosts,before)
  assert.equal((await fetch(shop+'/api/mollie-return?cart_id=foreign-cart',{headers:{Cookie:`retail_cart_possession=${'b'.repeat(64)}`}})).status,404)

  await context.close()
 }
 const context=await browser.newContext(),page=await context.newPage();page.on('pageerror',e=>errors.push(e.message))
 await context.addCookies([{name:'retail_cart_possession',value:'b'.repeat(64),url:shop,httpOnly:true},{name:'retail_cart_csrf',value:'a'.repeat(64),url:shop}])
 selected=null;shippingPosts.length=0;shippingReceipts.clear();version='2';contacts={email:'',shipping_address:null,billing_address:null};receipts.clear();posts.length=0
 await page.goto(shop+'/dk-en/checkout?step=address');await expect(page.getByTestId('shipping-first-name-input')).toBeVisible()
 await page.locator('select').filter({has:page.locator('option[value="'+saved.id+'"]')}).selectOption(saved.id)
 await expect(page.getByTestId('billing-address-checkbox')).toBeChecked()
 readFailure=true;await page.getByTestId('submit-address-button').click();await expect(page.getByTestId('contact-recover')).toBeVisible();assert.equal(posts.length,1)
 assert.equal(contacts.shipping_address.address_1,contacts.billing_address.address_1)
 readFailure=false;await page.reload();await expect(page.getByTestId('contact-recover')).toBeVisible();await page.getByTestId('contact-recover').click()
 await expect(page).toHaveURL(/step=delivery/);await expect(page.getByTestId('contact-recover')).toHaveCount(0);assert.equal(posts.length,1)
 await context.close()
 assert.deepEqual(errors,[])
 console.log('ASTRO_CHECKOUT_CONTACT_BROWSER '+JSON.stringify({formal_astro:true,synthetic_api:true,real_gin_mysql:false,original_css_and_address_markup:true,private_values_absent_from_ssr:true,no_cart_404:true,empty_cart_307:true,private_read_failure_503:true,saved_address_copy:true,email_confirmation_before_transport:true,separate_billing:true,leading_postal_zero:true,lost_response_same_operation_after_reload:true,rehydrated_contact_values:true,acknowledged_step_delivery:true,private_journal_descriptor:true,acknowledged_read_failure_read_only_recovery:true,same_billing_copy:true,exact_summary:true,edit_and_back_links:true,en_zh:true,widths:[390,1440],overflow:false,browser_errors:0,payment_or_order_complete:false,delivery_selection:true,delivery_lost_response_same_operation:true,delivery_acknowledged_read_only_recovery:true,unavailable_inventory_disabled:true,payment_still_closed:true,source_pixel_comparison:false,payment_result_page:true,paid_waits_for_order:true,authorized_not_paid:true,failed_return:true,foreign_cart_no_provider_poll:true,stable_return_callback:true}))
}finally{
 if(browser)await browser.close();if(child&&child.exitCode===null){child.kill();await once(child,'exit')};await new Promise(resolve=>api.close(resolve))
}
