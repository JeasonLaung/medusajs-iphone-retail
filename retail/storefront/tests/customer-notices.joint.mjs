import assert from 'node:assert/strict'
import {createRequire} from 'node:module'
import {dev} from 'astro'
import {envField} from 'astro/config'
import {realpathSync} from 'node:fs'
import {fileURLToPath} from 'node:url'
import react from '@astrojs/react'
const require=createRequire(new URL('../../admin/package.json',import.meta.url)),{chromium,expect}=require('@playwright/test')
const origin='http://127.0.0.1:19383',target=process.env.ADMIN_TEST_API_TARGET
assert.match(target??'',/^http:\/\/127\.0\.0\.1:[0-9]+$/)
assert.match(process.env.CUSTOMER_BRIDGE_KEY??'',/^[0-9a-f]{64}$/)
process.env.GIN_API_ORIGIN=target
process.env.STOREFRONT_ORIGIN=origin
let server,browser
try{
 server=await dev({root:new URL('./notices-joint-fixture/',import.meta.url),configFile:false,output:'server',integrations:[react()],devToolbar:{enabled:false},server:{host:'127.0.0.1',port:19383},logLevel:'error',env:{schema:Object.fromEntries(['CUSTOMER_BRIDGE_KEY','GIN_API_ORIGIN','STOREFRONT_ORIGIN'].map(k=>[k,envField.string({context:'server',access:'secret'})]))},vite:{resolve:{dedupe:['react','react-dom']},server:{strictPort:true,fs:{allow:[fileURLToPath(new URL('../',import.meta.url)),realpathSync(fileURLToPath(new URL('../node_modules',import.meta.url)))]}}}})
 assert.equal(server.address.port,19383)
 browser=await chromium.launch({headless:true,executablePath:process.env.ADMIN_TEST_BROWSER||'C:/Program Files/Google/Chrome/Application/chrome.exe'})
 const context=await browser.newContext({reducedMotion:'reduce'}),page=await context.newPage(),errors=[]
 page.on('pageerror',e=>errors.push(e.message))
 // No route interception or synthetic API responses in this rehearsal.
 await page.goto(origin)
 await expect(page.getByTestId('email-input')).toBeVisible()
 await expect(page.getByTestId('notification-bell')).toHaveCount(0)
 async function login(email){
  await page.getByTestId('email-input').fill(email)
  await page.getByTestId('password-input').fill('artificial-customer-notice-password')
  await page.getByTestId('sign-in-button').click()
  await expect(page.getByTestId('customer-authenticated')).toBeVisible()
 }
 await login('replacement-notice@example.invalid')
 await expect(page.getByTestId('notification-badge')).toHaveText('4')
 const cookies=await context.cookies()
 assert.equal(cookies.find(c=>c.name==='retail_customer_session')?.httpOnly,true)
 const raw=await page.evaluate(async()=>{const r=await fetch('/api/store/customer/notifications?offset=0');return {status:r.status,cache:r.headers.get('cache-control'),body:await r.json()}})
 assert.equal(raw.status,200);assert.match(raw.cache,/no-store/);assert.equal(raw.body.count,4)
 assert.deepEqual(Object.keys(raw.body.notifications[0]).sort(),['action','created_at','id','kind','read_at','reference','resource_id'])
 assert.equal(raw.body.notifications.filter(n=>n.kind==='shipment.created').length,2)
 assert.equal((await context.request.get(target+'/api/store/customer/notifications')).status(),403)
 assert.equal((await context.request.get(origin+'/api/store/customer/notifications',{headers:{'x-forwarded-for':'127.0.0.2'}})).status(),403)
 const csrfDenied=await page.evaluate(async()=>{const r=await fetch('/api/store/customer/notifications',{method:'POST',headers:{'Content-Type':'application/json'},body:'{}'});return r.status})
 assert.equal(csrfDenied,403)
 await page.getByTestId('logout-button').first().click()
 await expect(page.getByTestId('notification-bell')).toHaveCount(0)
 await expect(page.getByTestId('email-input')).toBeVisible()
 await login('other-notice@example.invalid')
 await expect(page.getByTestId('notification-bell')).toBeVisible()
 await page.getByTestId('notification-bell').click()
 await expect(page.getByRole('dialog').getByRole('listitem')).toHaveCount(0)
 const other=await page.evaluate(async()=>{const r=await fetch('/api/store/customer/notifications?offset=0');return r.json()})
 assert.equal(other.count,0);assert.equal(other.unread,0)
 const ownIDs=raw.body.notifications.map(n=>n.id)
 const foreignRead=await page.evaluate(async(id)=>{const csrf=document.cookie.split(';').map(s=>s.trim()).find(s=>s.startsWith('retail_customer_csrf=')).split('=')[1];const r=await fetch('/api/store/customer/notifications',{method:'POST',headers:{'Content-Type':'application/json','X-CSRF-Token':csrf},body:JSON.stringify({id})});return r.status},ownIDs[0])
 assert.equal(foreignRead,200)
 await page.getByRole('button',{name:'Close notifications',exact:true}).click()
 await page.getByTestId('logout-button').first().click()
 await expect(page.getByTestId('email-input')).toBeVisible()
 await login('replacement-notice@example.invalid')
 await expect(page.getByTestId('notification-badge')).toHaveText('4')
 await page.getByTestId('notification-bell').click()
 const dialog=page.getByRole('dialog');await expect(dialog.getByRole('listitem')).toHaveCount(4)
 await dialog.getByRole('button',{name:'Mark as read',exact:true}).first().click()
 await expect(page.getByTestId('notification-badge')).toHaveText('3')
 await page.reload();await expect(page.getByTestId('notification-badge')).toHaveText('3')
 await page.getByTestId('notification-bell').click()
 await page.getByRole('button',{name:'Mark all as read',exact:true}).click()
 await expect(page.getByTestId('notification-badge')).toHaveCount(0)
 await page.getByRole('button',{name:'View details',exact:true}).first().click()
 await page.waitForURL(origin+'/dk-en/account/orders/allocation-order')
 await page.goto(origin);await expect(page.getByTestId('customer-authenticated')).toBeVisible()
 await page.getByTestId('logout-button').first().click()
 await expect(page.getByTestId('notification-bell')).toHaveCount(0)
 assert.deepEqual(errors,[])
 console.log('CUSTOMER_NOTICES_JOINT '+JSON.stringify({synthetic_api:false,production_login_form:true,production_header_bell:true,production_signed_bridge:true,real_gin_mysql_redis:true,unread:4,single_read_survives_reload:true,all_read:true,detail_navigation:true,logout_hides_bell:true,account_switch_isolated:true,foreign_read_noop:true,direct_unsigned_denied:true,forwarded_ip_denied:true,csrf_denied:true,browser_errors:0,full_storefront_accepted:false}))
}finally{await browser?.close();await server?.stop()}
