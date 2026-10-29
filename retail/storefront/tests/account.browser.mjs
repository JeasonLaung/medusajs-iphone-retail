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
let verified=false,authenticated=false,registered=false,verifications=0,customerUnavailable=false,child,browser
const cookie=(kind,value=token)=>`retail_customer_${kind}=${value}; Path=/; Max-Age=43200; Expires=${new Date(Date.now()+43200000).toUTCString()}; ${kind==='csrf'?'':'HttpOnly; '}SameSite=Lax`
const clear=kind=>`retail_customer_${kind}=; Path=/; Max-Age=0; ${kind==='csrf'?'':'HttpOnly; '}SameSite=Lax`
const api=createServer(async(req,res)=>{
 let body={},status=200,cookies=[]
 const path=new URL(req.url,'http://fixture.invalid').pathname
 if(path.startsWith('/api/store/customer/')){
  let raw='';for await(const part of req)raw+=part
  let input={};try{input=JSON.parse(raw||'{}')}catch{}
  switch(path){
   case '/api/store/customer/auth/challenge':body={csrf_token:csrf};cookies=[cookie('prelogin')];break
   case '/api/store/customer/me':if(customerUnavailable){status=503;body={error:{code:'unavailable'}}}else if(authenticated)body={customer:account};else{status=401;body={error:{code:'customer.unauthorized'}}}break
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
 await expect(page.getByTestId('customer-authenticated')).toContainText('temporarily unavailable')
 assert.ok(!(await page.content()).includes('artificial-password'))
 await page.getByRole('button',{name:'Log out',exact:true}).click();await expect(page.getByTestId('login-page')).toBeVisible()
 await expect(page.locator('header[data-signed-in]')).toHaveAttribute('data-signed-in','false')
 await expect(second.getByTestId('login-page')).toBeVisible()
 await expect(second.locator('header[data-signed-in]')).toHaveAttribute('data-signed-in','false')
 const marker=await page.evaluate(()=>localStorage.getItem('retail.customer.change.v1'))
 assert.match(marker,/^[0-9a-f-]{36}$/);assert.ok(!marker.includes(account.email))
 await second.close()
 await page.goto(shop+'/dk-zh/account');await expect(page.getByTestId('login-page')).toContainText('登录')
 await page.goto(shop+'/dk-en/verify-account');await expect(page.getByTestId('verify-account-page')).toContainText('invalid')
 assert.deepEqual(errors,[])
 console.log('ASTRO_ACCOUNT_BROWSER '+JSON.stringify({formal_routes:true,synthetic_api:true,login_register_switch:true,pending_mail_honest:true,verify_once:true,verify_no_login:true,login_current_customer:true,logout:true,en_zh:true,widths:[390,1440],browser_errors:0,header_login_logout:true,cross_tab_logout:true,dependency_failure_not_logout:true,opaque_storage_marker:true,real_gin_mysql:false,source_pixel_comparison:false,dashboard_complete:false,mail_delivery:false}))
}finally{
 if(browser)await browser.close()
 if(child&&child.exitCode===null){child.kill();await once(child,'exit')}
 await new Promise(resolve=>api.close(resolve))
}
