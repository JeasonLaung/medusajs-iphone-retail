import assert from 'node:assert/strict'
import {createServer} from 'node:http'
import {spawn} from 'node:child_process'
import {once} from 'node:events'
import {createRequire} from 'node:module'
import {fileURLToPath} from 'node:url'
import {setTimeout as delay} from 'node:timers/promises'
const require=createRequire(new URL('../../admin/package.json',import.meta.url)),{chromium,expect}=require('@playwright/test')
const host='127.0.0.1',port=19371,shop=`http://${host}:${port}`,token='a'.repeat(64),csrf='b'.repeat(64)
const account={id:'cust_browser',email:'new@example.invalid',first_name:'New',last_name:'Customer',phone:'',verified_at:'2026-10-29T00:00:00Z'}
const region={id:'region',name:'Europe',currency_code:'eur',automatic_taxes:false,countries:[{iso_2:'dk',iso_3:'dnk',num_code:'208',name:'Denmark',display_name:'Denmark',region_id:'region'}]}
let verified=false,authenticated=false,registered=false,verifications=0,customerUnavailable=false,profileFailure=false,addressFailure=false,deleteFailure=false,addressSequence=0,child,browser
const addresses=[]
const cookie=(kind,value=token)=>`retail_customer_${kind}=${value}; Path=/; Max-Age=43200; Expires=${new Date(Date.now()+43200000).toUTCString()}; ${kind==='csrf'?'':'HttpOnly; '}SameSite=Lax`
const clear=kind=>`retail_customer_${kind}=; Path=/; Max-Age=0; ${kind==='csrf'?'':'HttpOnly; '}SameSite=Lax`
const api=createServer(async(req,res)=>{
 let body={},status=200,cookies=[]
 const path=new URL(req.url,'http://fixture.invalid').pathname
 if(path.startsWith('/api/store/customer/')){
  let raw='';for await(const part of req)raw+=part
  let input={};try{input=JSON.parse(raw||'{}')}catch{}
  if(path==='/api/store/customer/addresses'||path.startsWith('/api/store/customer/addresses/')){
   const id=path.split('/')[5],removing=path.endsWith('/delete')
   if(!authenticated){status=401;body={error:{code:'unauthorized'}}}
   else if(req.method==='GET'){if(addressFailure){status=503;body={error:{code:'unavailable'}}}else body={addresses}}
   else if(addressFailure||removing&&deleteFailure){status=503;body={error:{code:'unavailable'}}}
   else if(removing){const index=addresses.findIndex(address=>address.id===id);if(index<0){status=404;body={error:{code:'missing'}}}else{addresses.splice(index,1);body={success:true}}}
   else {
    const fields={...input};for(const key of ['is_default_billing','is_default_shipping'])if(key in fields)fields[key]=fields[key]==='true'
    for(const key of ['is_default_billing','is_default_shipping'])if(fields[key]===true)for(const address of addresses)address[key]=false
    let address=id?addresses.find(address=>address.id===id):null
    if(!id){address={id:'addr_00000000-0000-4000-8000-'+String(++addressSequence).padStart(12,'0'),first_name:'',last_name:'',company:'',address_1:'',address_2:'',city:'',postal_code:'',province:'',country_code:'dk',phone:'',is_default_billing:false,is_default_shipping:false,created_at:'2026-10-29T00:00:00Z',updated_at:'2026-10-29T00:00:00Z'};addresses.push(address)}
    if(!address){status=404;body={error:{code:'missing'}}}else{Object.assign(address,fields);body={address}}
   }
  }else switch(path){
   case '/api/store/customer/auth/challenge':body={csrf_token:csrf};cookies=[cookie('prelogin')];break
   case '/api/store/customer/me':if(req.method==='POST'){if(profileFailure){status=503;body={error:{code:'unavailable'}}}else{Object.assign(account,input);body={customer:account}}}else if(customerUnavailable){status=503;body={error:{code:'unavailable'}}}else if(authenticated)body={customer:account};else{status=401;body={error:{code:'customer.unauthorized'}}}break
   case '/api/store/customer/auth/register':registered=true;assert.equal(input.email,account.email);status=202;body={state:'verification_required',delivery:'pending',email:account.email};cookies=[clear('prelogin')];break
   case '/api/store/customer/auth/verify':verifications++;assert.equal(input.token,token);verified=true;body={success:true};cookies=[clear('prelogin')];break
   case '/api/store/customer/auth/login':
    cookies=[clear('prelogin')]
    if(!verified){status=403;body={state:'verification_required',error:{code:'customer.verification_required'}}}
    else{authenticated=true;body={customer:account,csrf_token:csrf};cookies.push(cookie('session'),cookie('csrf',csrf))}break
   case '/api/store/customer/auth/logout':authenticated=false;body={success:true};cookies=['session','csrf','prelogin'].map(clear);break
   default:status=404
  }
  res.writeHead(status,{'Content-Type':'application/json','Cache-Control':'private, no-store','Set-Cookie':cookies});res.end(JSON.stringify(body));return
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
})
try{
 const probe=createServer();probe.listen(port,host);await once(probe,'listening');await new Promise(resolve=>probe.close(resolve))
 api.listen(0,host);await once(api,'listening');const address=api.address();assert.ok(address&&typeof address!=='string')
 child=spawn(process.execPath,['scripts/start.mjs'],{env:{...process.env,HOST:host,PORT:String(port),GIN_API_ORIGIN:`http://${host}:${address.port}`,STOREFRONT_ORIGIN:shop,CUSTOMER_BRIDGE_KEY:'71'.repeat(32)},stdio:'ignore'})
 let ready=false;for(let i=0;i<100;i++){if(child.exitCode!==null)throw new Error('Owned Astro stopped');try{if((await fetch(shop+'/health')).ok){ready=true;break}}catch{}await delay(100)}assert.ok(ready)
 browser=await chromium.launch({headless:true,executablePath:process.env.ADMIN_TEST_BROWSER||'C:/Program Files/Google/Chrome/Application/chrome.exe'})
 const context=await browser.newContext(),page=await context.newPage(),errors=[]
 page.on('pageerror',error=>errors.push(error.message))
 for(const width of [390,1440]){
  await page.setViewportSize({width,height:1000});await page.goto(shop+'/dk-en/account')
  await expect(page.getByTestId('login-page')).toBeVisible()
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
  await page.screenshot({path:fileURLToPath(new URL(`../../../logs/gin-migration/account-login-${width}.png`,import.meta.url)),fullPage:true})
 }
 await page.getByTestId('register-button').click()
 await expect(page.getByTestId('register-page')).toBeVisible()
 await page.getByTestId('first-name-input').fill('New');await page.getByTestId('last-name-input').fill('Customer')
 await page.getByTestId('email-input').fill(account.email);await page.getByTestId('password-input').fill('artificial-password')
 await page.getByTestId('register-page').getByTestId('register-button').click()
 await expect(page.getByTestId('register-verification-message')).toContainText('waiting to be sent')
 await expect(page.getByTestId('resend-verification-button')).toBeDisabled();assert.equal(registered,true)
 await page.goto(shop+'/dk-en/verify-account?token='+token)
 await expect(page.getByTestId('verify-account-page')).toContainText('verified',{timeout:15000})
 assert.equal(verifications,1);assert.equal(authenticated,false)
 await page.getByTestId('verify-account-page').getByRole('link').click()
 await expect(page.getByTestId('login-page')).toBeVisible()
 await page.getByTestId('email-input').fill(account.email);await page.getByTestId('password-input').fill('artificial-password');await page.getByTestId('sign-in-button').click()
 await expect(page.getByTestId('customer-email')).toHaveText(account.email)
 await expect(page.locator('header[data-signed-in]')).toHaveAttribute('data-signed-in','true')
 await expect(page.getByTestId('account-nav')).toBeVisible()
 await expect(page.getByTestId('account-nav').locator('.ant-menu-item-selected')).toContainText('Overview')
 for(const [id,path] of [['overview-link','/account'],['profile-link','/account/profile'],['addresses-link','/account/addresses'],['orders-link','/account/orders'],['buyback-link','/account/buyback']]){
  await expect(page.getByTestId('account-nav').getByTestId(id)).toHaveAttribute('href','/dk-en'+path)
 }
 await expect(page.getByTestId('customer-profile-completion')).toHaveAttribute('data-value','50')
 await expect(page.getByTestId('addresses-count')).toHaveAttribute('data-value','0')
 await expect(page.getByTestId('overview-orders-unavailable')).toBeVisible()
 await expect(page.getByTestId('no-orders-message')).toHaveCount(0)
 await page.locator('[data-testid="profile-link"]:visible').click()
 await expect(page.getByTestId('profile-page-wrapper')).toBeVisible()
 const nameEditor=page.getByTestId('account-name-editor')
 await nameEditor.getByTestId('edit-button').click()
 await nameEditor.getByTestId('first-name-input').fill('Changed')
 await nameEditor.getByTestId('save-button').click()
 await expect(nameEditor.getByTestId('current-info')).toContainText('Changed Customer')
 await expect(nameEditor.getByTestId('success-message')).toBeVisible()
 const phoneEditor=page.getByTestId('account-phone-editor')
 await phoneEditor.getByTestId('edit-button').click()
 await phoneEditor.getByTestId('phone-input').fill('+45 111')
 profileFailure=true
 await phoneEditor.getByTestId('save-button').click()
 await expect(phoneEditor.getByTestId('error-message')).toBeVisible()
 assert.equal(account.phone,'')
 profileFailure=false
 await phoneEditor.getByTestId('save-button').click()
 await expect(phoneEditor.getByTestId('current-info')).toHaveText('+45 111')
 await page.reload()
 await expect(page.getByTestId('account-name-editor').getByTestId('current-info')).toContainText('Changed Customer')
 await expect(page.getByTestId('account-phone-editor').getByTestId('current-info')).toHaveText('+45 111')
 await expect(page.getByTestId('profile-email-readonly')).toHaveText(account.email)
 await page.locator('[data-testid="addresses-link"]:visible').click()
 await expect(page.getByTestId('addresses-page-wrapper')).toBeVisible()
 await page.getByTestId('add-address-button').click()
 let modal=page.getByTestId('add-address-modal')
 await modal.getByTestId('cancel-button').click()
 assert.equal(addresses.length,0)
 await page.getByTestId('add-address-button').click()
 modal=page.getByTestId('add-address-modal')
 await modal.getByTestId('first-name-input').fill('New');await modal.getByTestId('last-name-input').fill('Customer')
 await modal.getByTestId('address-1-input').fill('Street 1');await modal.getByTestId('postal-code-input').fill('0012');await modal.getByTestId('city-input').fill('Copenhagen')
 await modal.getByTestId('country-select').click()
 await modal.getByRole('combobox').fill('Sweden')
 await expect(page.getByRole('option',{name:'Sweden',exact:true})).toBeAttached()
 await modal.getByRole('combobox').fill('Denmark')
 await page.getByTitle('Denmark',{exact:true}).click()
 addressFailure=true
 await modal.getByTestId('save-button').click();await expect(modal.getByTestId('address-error')).toBeVisible()
 assert.equal(addresses.length,0)
 await expect(modal.getByTestId('first-name-input')).toHaveValue('New')
 await expect(modal.locator('select[name=country_code]')).toHaveValue('dk')
 addressFailure=false
 await modal.getByTestId('save-button').click()
 await expect(modal).not.toBeVisible()
 await expect(page.getByTestId('address-container')).toHaveCount(1)
 await expect(page.getByTestId('address-postal-city')).toContainText('0012, Copenhagen')
 await page.getByTestId('address-edit-button').click()
 modal=page.getByTestId('edit-address-modal')
 await modal.getByTestId('address-1-input').fill('Changed Street 2')
 await modal.getByTestId('save-button').click();await expect(modal).not.toBeVisible()
 await expect(page.getByTestId('address-address')).toHaveText('Changed Street 2')
 await page.reload();await expect(page.getByTestId('address-address')).toHaveText('Changed Street 2')
 for(const width of [390,1440]){
  await page.setViewportSize({width,height:1000})
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
  await page.screenshot({path:fileURLToPath(new URL(`../../../logs/gin-migration/account-addresses-${width}.png`,import.meta.url)),fullPage:true})
 }
 deleteFailure=true
 await page.getByTestId('address-delete-button').click();await expect(page.getByTestId('addresses-page-wrapper').getByRole('alert')).toBeVisible()
 await expect(page.getByTestId('address-container')).toHaveCount(1)
 deleteFailure=false
 await page.getByTestId('address-delete-button').click();await expect(page.getByTestId('address-container')).toHaveCount(0)
 await page.locator('[data-testid="profile-link"]:visible').click()
 const billing=page.getByTestId('account-billing-address-editor')
 await billing.getByTestId('edit-button').click()
 await billing.getByTestId('billing-first-name-input').fill('Billing');await billing.getByTestId('billing-last-name-input').fill('Customer')
 await billing.getByTestId('billing-phone-input').fill('+45 123')
 await billing.getByTestId('billing-address-1-input').fill('Billing Street 3');await billing.getByTestId('billing-postcal-code-input').fill('0123');await billing.getByTestId('billing-city-input').fill('Copenhagen')
 await billing.getByTestId('billing-country-code-select').click();await billing.getByRole('combobox').fill('Denmark');await page.getByTitle('Denmark',{exact:true}).click()
 await billing.getByTestId('save-button').click();await expect(billing.getByTestId('current-info').first()).toContainText('Billing Street 3')
 assert.equal(addresses.length,1);assert.equal(addresses[0].is_default_billing,true)
 await page.reload();await expect(page.getByTestId('account-billing-address-editor').getByTestId('current-info').first()).toContainText('Billing Street 3')
 await page.getByTestId('account-billing-address-editor').getByTestId('edit-button').click()
 await page.getByTestId('billing-address-1-input').fill('Updated Billing 4')
 await page.getByTestId('account-billing-address-editor').getByTestId('save-button').click()
 await expect(page.getByTestId('account-billing-address-editor').getByTestId('current-info').first()).toContainText('Updated Billing 4')
 assert.equal(addresses.length,1)
 await page.locator('[data-testid="overview-link"]:visible').click()
 await expect(page.getByTestId('customer-profile-completion')).toHaveAttribute('data-value','100')
 await expect(page.getByTestId('addresses-count')).toHaveAttribute('data-value','1')
 addressFailure=true
 await page.evaluate(()=>window.dispatchEvent(new Event('retail:customer-changed')))
 await expect(page.getByTestId('overview-address-error')).toBeVisible()
 await expect(page.getByTestId('addresses-count')).toHaveCount(0)
 addressFailure=false
 await page.getByTestId('overview-address-error').getByRole('button',{name:'Retry',exact:true}).click()
 await expect(page.getByTestId('addresses-count')).toHaveAttribute('data-value','1')
 await page.screenshot({path:fileURLToPath(new URL('../../../logs/gin-migration/account-overview-1440.png',import.meta.url)),fullPage:true})
 await page.setViewportSize({width:390,height:1000})
 await expect(page.getByTestId('mobile-account-nav')).toBeVisible()
 await expect(page.getByTestId('account-nav')).not.toBeVisible()
 await expect(page.getByTestId('customer-profile-completion')).not.toBeVisible()
 await page.locator('[data-testid="profile-link"]:visible').click()
 await expect(page.getByTestId('account-main-link')).toBeVisible()
 await page.getByTestId('account-main-link').click()
 await expect(page.getByTestId('mobile-account-nav').getByText('Hello Changed',{exact:true})).toBeVisible()
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
 await page.screenshot({path:fileURLToPath(new URL('../../../logs/gin-migration/account-overview-390.png',import.meta.url)),fullPage:true})
 await page.goto(shop+'/dk-zh/account')
 await expect(page.getByTestId('mobile-account-nav').getByTestId('buyback-link')).toHaveText('我的回收申请')
 await page.setViewportSize({width:1440,height:1000})
 await expect(page.getByTestId('customer-profile-completion')).toHaveAttribute('data-value','100')
 await expect(page.getByTestId('addresses-count')).toHaveAttribute('data-value','1')
 await expect(page.getByTestId('account-nav').locator('.ant-menu-item-selected')).toContainText('概览')
 await page.screenshot({path:fileURLToPath(new URL('../../../logs/gin-migration/account-overview-zh-1440.png',import.meta.url)),fullPage:true})
 await page.goto(shop+'/dk-zh/account/addresses')
 await expect(page.getByTestId('addresses-page-wrapper')).toBeVisible()
 await expect(page.getByTestId('address-address')).toHaveText('Updated Billing 4')
 await page.setViewportSize({width:390,height:1000})
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
 await page.screenshot({path:fileURLToPath(new URL('../../../logs/gin-migration/account-addresses-zh-390.png',import.meta.url)),fullPage:true})
 await page.goto(shop+'/dk-en/account/profile')
 const second=await context.newPage();await second.goto(shop+'/dk-en/account')
 await expect(second.getByTestId('customer-email')).toHaveText(account.email)
 await expect(second.locator('header[data-signed-in]')).toHaveAttribute('data-signed-in','true')
 customerUnavailable=true
 await page.evaluate(()=>window.dispatchEvent(new Event('retail:customer-changed')))
 await expect(page.getByTestId('customer-email')).not.toBeVisible()
 await expect(page.locator('header[data-signed-in]')).toHaveAttribute('data-signed-in','true')
 customerUnavailable=false
 await page.getByRole('button',{name:'Retry',exact:true}).click()
 await expect(page.getByTestId('customer-email')).toHaveText(account.email)
 await expect(page.getByTestId('profile-page-wrapper')).toBeVisible()
 assert.ok(!(await page.content()).includes('artificial-password'))
 await page.getByTestId('account-main-link').click()
 await page.locator('[data-testid="logout-button"]:visible').click();await expect(page.getByTestId('login-page')).toBeVisible()
 await expect(page.locator('header[data-signed-in]')).toHaveAttribute('data-signed-in','false')
 await expect(second.getByTestId('login-page')).toBeVisible()
 await expect(second.locator('header[data-signed-in]')).toHaveAttribute('data-signed-in','false')
 const marker=await page.evaluate(()=>localStorage.getItem('retail.customer.change.v1'))
 assert.match(marker,/^[0-9a-f-]{36}$/);assert.ok(!marker.includes(account.email))
 await second.close()
 await page.goto(shop+'/dk-zh/account');await expect(page.getByTestId('login-page')).toContainText('登录')
 await page.goto(shop+'/dk-en/verify-account');await expect(page.getByTestId('verify-account-page')).toContainText('invalid')
 assert.deepEqual(errors,[])
 console.log('ASTRO_ACCOUNT_BROWSER '+JSON.stringify({formal_routes:true,synthetic_api:true,login_register_switch:true,pending_mail_honest:true,verify_once:true,verify_no_login:true,login_current_customer:true,logout:true,en_zh:true,widths:[390,1440],browser_errors:0,header_login_logout:true,cross_tab_logout:true,dependency_failure_not_logout:true,opaque_storage_marker:true,profile_name_phone_saved:true,profile_failure_recovery:true,profile_reload_persisted:true,email_readonly:true,address_create_edit_delete:true,address_failure_recovery:true,address_reload_persisted:true,delete_failure_visible:true,cancel_no_write:true,original_europe_country_options:true,billing_create_update_persisted:true,address_chinese_mobile:true,real_gin_mysql:false,source_pixel_comparison:false,original_account_navigation:true,overview_real_address_stats:true,address_stats_failure_not_zero:true,mobile_account_back:true,dashboard_complete:false,mail_delivery:false}))
}finally{
 if(browser)await browser.close()
 if(child&&child.exitCode===null){child.kill();await once(child,'exit')}
 await new Promise(resolve=>api.close(resolve))
}
