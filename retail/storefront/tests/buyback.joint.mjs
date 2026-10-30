import assert from 'node:assert/strict'
import {createRequire} from 'node:module'
import {mkdir} from 'node:fs/promises'
import {realpathSync} from 'node:fs'
import {fileURLToPath} from 'node:url'
import {dev} from 'astro'
const require=createRequire(new URL('../../admin/package.json',import.meta.url)),{chromium,expect:baseExpect}=require('@playwright/test'),expect=baseExpect.configure({timeout:30000})
const origin='http://127.0.0.1:19383',target=process.env.ADMIN_TEST_API_TARGET
assert.match(target??'',/^http:\/\/127\.0\.0\.1:[0-9]+$/)
assert.match(process.env.CUSTOMER_BRIDGE_KEY??'',/^[0-9a-f]{64}$/)
process.env.GIN_API_ORIGIN=target
process.env.STOREFRONT_ORIGIN=origin
const output=new URL('../../../logs/gin-migration/astro-buyback-joint/',import.meta.url),errors=[]
let server,browser
try{
 // Load actual production routes, layout, account and signed bridge; no API interception.
 server=await dev({root:new URL('../',import.meta.url),server:{host:'127.0.0.1',port:19383},integrations:[{name:'joint-admin-route',hooks:{'astro:config:setup':({injectRoute})=>injectRoute({pattern:'/buyback',entrypoint:fileURLToPath(new URL('./buyback-admin.astro',import.meta.url))})}}],vite:{resolve:{dedupe:['react','react-dom']},server:{strictPort:true,fs:{allow:[fileURLToPath(new URL('../../',import.meta.url)),realpathSync(fileURLToPath(new URL('../node_modules',import.meta.url))),realpathSync(fileURLToPath(new URL('../../admin/node_modules',import.meta.url)))]},proxy:{'/api/admin':{target}}}},logLevel:'error'})
 assert.equal(server.address.port,19383)
 browser=await chromium.launch({headless:true,executablePath:process.env.ADMIN_TEST_BROWSER||'C:/Program Files/Google/Chrome/Application/chrome.exe'})
 const context=await browser.newContext({viewport:{width:390,height:900},reducedMotion:'reduce'}),page=await context.newPage()
 page.on('pageerror',e=>{errors.push(e.message);console.error('PAGE_ERROR',e.message)})
 async function login(email){
  await page.goto(origin+'/dk-en/account')
  await page.getByTestId('email-input').fill(email)
  await page.getByTestId('password-input').fill('artificial-buyback-password')
  await page.getByTestId('sign-in-button').click()
  await expect(page.getByTestId('mobile-account-nav').getByTestId('logout-button')).toBeVisible()
 }
 async function read(path){return page.evaluate(async path=>{const r=await fetch('/api/store/customer/'+path);return {status:r.status,body:await r.json()}},path)}
 await login('buyback-browser@example.invalid')
 assert.equal((await context.cookies()).find(c=>c.name==='retail_customer_session')?.httpOnly,true)
 async function questionnaire(){
 await page.goto(origin+'/dk-en/sell/iphone/model')
 await page.getByRole('combobox',{name:'Select device model'}).click();await page.getByText('iPhone 13',{exact:true}).click()
 const next=()=>page.getByRole('button',{name:'Continue',exact:true}).click()
 await next();await page.getByRole('radio',{name:'256GB',exact:true}).check();await next()
 await page.locator('#battery-capacity').fill('90');await next()
 for(let i=0;i<5;i++)await page.getByRole('button',{name:'Yes',exact:true}).click()
 await next();await page.getByRole('checkbox',{name:'Everything works',exact:true}).check();await next()
 await page.getByRole('checkbox',{name:'No cracks, chips or heavy scratches',exact:true}).check();await next()
 for(const stage of ['screen','frame','back']){await expect(page).toHaveURL(new RegExp('/'+stage+'$'));await page.getByRole('radio').last().check();await next()}
 }
 await questionnaire()
 async function submit(){
  const form=page.getByTestId('buyback-submission');await expect(form).toBeVisible()
  for(const [field,value]of Object.entries({name:'Browser Customer',phone:'123456',address:'Street 1',city:'Copenhagen',postal:'1000',country:'DK'}))await page.locator('#'+field).fill(value)
  for(const checkbox of await form.getByRole('checkbox').all())await checkbox.check()
  await form.locator('button[type=submit]').click()
  await expect(page).toHaveURL(/\/account\/buyback\/bbr_[0-9a-f-]+$/)
  await expect(page.getByTestId('buyback-request-details')).toBeVisible()
  return page.url().split('/').at(-1)
 }
 const id=await submit(),detail=await read('buyback/requests/'+id)
 assert.equal(detail.status,200);assert.equal(detail.body.request.reference_amount,294)
 const item=detail.body.request.items[0]
 await page.locator(`[id="${id}-${item.id}-serial_number"] input`).fill('sn-joint')
 await page.locator(`[id="${id}-${item.id}-imei"] input`).fill('000000000000001')
 const png=await page.evaluate(()=>{const c=document.createElement('canvas');c.width=c.height=20;const x=c.getContext('2d');x.fillStyle='#1677FF';x.fillRect(0,0,20,20);return c.toDataURL('image/png').split(',')[1]})
 for(const type of ['front','back']){
  const saved=page.waitForResponse(r=>r.url()===origin+'/api/store/customer/buyback/requests/'+id+'/photos'&&r.request().method()==='POST')
  await page.locator(`[id="${id}-${item.id}-${type}"] input[type=file]`).setInputFiles({name:'phone.png',mimeType:'image/png',buffer:Buffer.from(png,'base64')})
  assert.equal((await saved).status(),201)
  await expect(page.locator(`[id="${id}-${item.id}-${type}"] .ant-tag`)).toHaveText('Awaiting review')
 }
 await expect.poll(()=>page.locator('[data-testid=buyback-request-details] .ant-image img').evaluateAll(images=>images.filter(i=>i.complete&&i.naturalWidth>0).length)).toBe(2)
 await page.getByTestId('buyback-request-details').locator('button').filter({hasText:'Submit photos for review'}).click()
 await expect(page.getByText('Photos under review',{exact:true}).first()).toBeVisible()
 await page.reload();await expect(page.getByText('Photos under review',{exact:true}).first()).toBeVisible()
 const persisted=await read('buyback/requests/'+id)
 assert.equal(persisted.body.request.status,'photo_review');assert.equal(persisted.body.request.items[0].serial_number,'SN-JOINT');assert.equal(persisted.body.request.photos.length,2)
 await expect.poll(()=>page.locator('[data-testid=buyback-request-details] .ant-image img').evaluateAll(images=>images.filter(i=>i.complete&&i.naturalWidth>0).length)).toBe(2)
 const photo=persisted.body.request.photos[0].id
 await mkdir(output,{recursive:true});await page.mouse.move(0,0);await page.screenshot({path:fileURLToPath(new URL('photo-review-mobile.png',output)),fullPage:true})
 // Actual Ant Design admin loaded via a fixture route; production API, login and permissions.
 const admin=await context.newPage();await admin.setViewportSize({width:1440,height:1000});admin.on('pageerror',e=>{errors.push(e.message);console.error('ADMIN_PAGE_ERROR',e.message)})
 await admin.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','en'))
 await admin.goto(origin+'/buyback')
 await admin.getByLabel('Email',{exact:true}).fill('browser@example.invalid')
 await admin.getByLabel('Password',{exact:true}).fill('artificial-browser-password')
 await admin.getByRole('button',{name:'Sign in',exact:true}).click()
 await expect(admin.getByRole('button',{name:'Sign out',exact:true})).toBeVisible()
 await admin.getByRole('menuitem',{name:'Selling requests',exact:true}).click()
 await admin.getByRole('button',{name:id,exact:true}).click()
 const drawer=admin.locator('.ant-drawer-content')
 await expect(drawer.getByText('SN: SN-JOINT · IMEI: 000000000000001',{exact:true})).toBeVisible()
 await expect.poll(()=>drawer.locator('.ant-image img').evaluateAll(images=>images.filter(i=>i.complete&&i.naturalWidth>0).length)).toBe(2)
 // Pending photos cannot authorize shipping.
 await drawer.getByRole('button',{name:'Approve shipping',exact:true}).click()
 let modal=admin.locator('.ant-modal:visible')
 await modal.getByLabel('Shipping instructions',{exact:true}).fill('Pack carefully. Send to fixture warehouse.')
 await modal.getByRole('button',{name:'Confirm',exact:true}).click()
 await expect(modal.getByText('Approve all required photos first',{exact:true})).toBeVisible()
 await modal.getByRole('button',{name:'Cancel',exact:true}).click();await expect(admin.locator('.ant-modal:visible')).toHaveCount(0)
 // A retake requires a reason and sends the customer back to awaiting photos.
 await drawer.getByRole('button',{name:'Request retake',exact:true}).first().click()
 modal=admin.locator('.ant-modal:visible')
 await modal.getByRole('button',{name:'Confirm',exact:true}).click();await expect(modal.locator('.ant-form-item-explain-error')).toBeVisible()
 await modal.getByLabel('Note',{exact:true}).fill('Please retake the front photo in better light.')
 await modal.getByRole('button',{name:'Confirm',exact:true}).click();await expect(admin.getByRole('dialog')).toHaveCount(1)
 await page.bringToFront();await page.reload();await expect(page.getByText('Awaiting photos',{exact:true}).first()).toBeVisible()
 const notices=await read('notifications?offset=0');assert.equal(notices.body.notifications.filter(n=>n.kind==='buyback.updated'&&n.action==='review_photo').length,1)
 // Replace only the rejected front photo through the original customer control.
 const replaced=page.waitForResponse(r=>r.url()===origin+'/api/store/customer/buyback/requests/'+id+'/photos'&&r.request().method()==='POST')
 await page.locator(`[id="${id}-${item.id}-front"] input[type=file]`).setInputFiles({name:'retake.png',mimeType:'image/png',buffer:Buffer.from(png,'base64')});assert.equal((await replaced).status(),201)
 await expect(page.locator(`[id="${id}-${item.id}-front"] .ant-tag`)).toHaveText('Awaiting review')
 await page.getByTestId('buyback-request-details').locator('button').filter({hasText:'Submit photos for review'}).click();await expect(page.getByText('Photos under review',{exact:true}).first()).toBeVisible()
 await admin.bringToFront();await drawer.getByRole('button',{name:'Refresh request',exact:true}).click()
 for(let i=0;i<2;i++){
  await drawer.getByRole('button',{name:'Approve',exact:true}).nth(i).click();await admin.locator('.ant-modal:visible').getByRole('button',{name:'Confirm',exact:true}).click();await expect(admin.getByRole('dialog')).toHaveCount(1)
 }
 await drawer.getByRole('button',{name:'Approve shipping',exact:true}).click();modal=admin.locator('.ant-modal:visible');await modal.getByLabel('Shipping instructions',{exact:true}).fill('Pack carefully. Send to fixture warehouse.');await modal.getByRole('button',{name:'Confirm',exact:true}).click();await expect(admin.getByRole('dialog')).toHaveCount(1)
 await page.bringToFront();await page.reload();await expect(page.getByText('Approved for shipping',{exact:true}).first()).toBeVisible()
 await page.locator('form input').fill('TRACK-JOINT');await page.getByRole('button',{name:'Confirm shipped',exact:true}).click();await expect(page.getByText('In transit',{exact:true}).first()).toBeVisible()
 await admin.bringToFront();await drawer.getByRole('button',{name:'Refresh request',exact:true}).click();await drawer.getByRole('button',{name:'Mark received',exact:true}).click();await admin.locator('.ant-modal:visible').getByRole('button',{name:'Confirm',exact:true}).click();await expect(admin.getByRole('dialog')).toHaveCount(1)
 await page.reload();await expect(page.getByText('Received / inspecting',{exact:true}).first()).toBeVisible()
 const notified=await read('notifications?offset=0');assert.deepEqual(notified.body.notifications.filter(n=>n.kind==='buyback.updated').map(n=>n.action).sort(),['mark_received'])
 await admin.bringToFront()
 await admin.screenshot({path:fileURLToPath(new URL('admin-received-desktop.png',output)),fullPage:true})
 await drawer.getByRole('button',{name:'Close',exact:true}).click()
 await expect(drawer).not.toBeVisible()
 const language=admin.getByRole('combobox',{name:'Language',exact:true});await admin.locator('.ant-select').filter({has:language}).locator('.ant-select-selector').click();await language.press('ArrowUp');await language.press('Enter')
 await expect(admin.getByRole('heading',{name:'回收申请',exact:true})).toBeVisible()
 await admin.setViewportSize({width:390,height:900});assert.ok(await admin.evaluate(()=>document.documentElement.scrollWidth<=innerWidth))
 await admin.getByRole('button',{name:id,exact:true}).click();await expect(admin.locator('.ant-drawer-content').getByText('已收货，检测中',{exact:true}).first()).toBeVisible()
 await expect(admin.locator('.ant-drawer-content').getByText('电池健康',{exact:true})).toBeVisible()
 await expect(admin.locator('.ant-drawer-content').getByText('已通过',{exact:true})).toHaveCount(2)
 await admin.screenshot({path:fileURLToPath(new URL('admin-received-zh-mobile.png',output)),fullPage:true});await admin.close()
 // A second actual form submission exercises cancellation and durable closure.
 await questionnaire()
 const canceled=await submit()
 assert.notEqual(id,canceled)
 await page.getByRole('button',{name:'Cancel request',exact:true}).click()
 await page.getByRole('dialog').getByRole('button',{name:'Confirm',exact:true}).click()
 await expect(page.getByRole('dialog')).toHaveCount(0)
 await page.reload();const closed=await read('buyback/requests/'+canceled)
 assert.equal(closed.body.request.status,'canceled');assert.ok(closed.body.request.closed_at)
 await page.goto(origin+'/dk-en/account/buyback');await expect(page.locator('article')).toHaveCount(2)
 const csrfDenied=await page.evaluate(async id=>(await fetch('/api/store/customer/buyback/requests/'+id,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({action:'cancel'})})).status,id)
 assert.equal(csrfDenied,403)
 assert.equal((await context.request.get(target+'/api/store/customer/buyback/requests/'+id)).status(),403)
 await page.goto(origin+'/dk-en/account');await page.getByTestId('mobile-account-nav').getByTestId('logout-button').click();await expect(page.getByTestId('email-input')).toBeVisible()
 await login('buyback-other@example.invalid')
 assert.equal((await read('buyback/requests/'+id)).status,404)
 assert.equal((await read('buyback/requests/'+id+'/photos/'+photo)).status,404)
 const foreign=await read('buyback/requests?offset=0');assert.equal(foreign.body.count,0)
 await page.goto(origin+'/dk-en/account/buyback/'+id);await expect(page.locator('.ant-alert-error')).toBeVisible();assert.equal(await page.getByTestId('buyback-request-details').count(),0)
 assert.deepEqual(errors,[])
 console.log('BUYBACK_JOINT '+JSON.stringify({synthetic_api:false,production_routes:true,original_questionnaire:true,real_login:true,real_gin_mysql_redis:true,created:2,private_photos:3,admin_retake_replacement_approval_shipping_receipt:true,transactional_unread_customer_notices:1,admin_en_zh_and_mobile:true,photo_previews_decoded_after_reload:true,submit_photos_survives_reload:true,cancel_closed_at_survives_reload:true,account_switch_isolated:true,foreign_photo_denied:true,unsigned_and_csrf_denied:true,browser_errors:0,platform_review_payment_return_accepted:false}))
}catch(e){await mkdir(output,{recursive:true});for(const [i,p]of (browser?.contexts().flatMap(c=>c.pages())??[]).entries()){await p.screenshot({path:fileURLToPath(new URL('failure-'+i+'.png',output)),fullPage:true}).catch(()=>{});console.error('FAILURE_BUTTONS',await p.locator('button').evaluateAll(nodes=>nodes.map(n=>({text:n.textContent,disabled:n.disabled,html:n.outerHTML.slice(0,600),hidden:n.closest('[aria-hidden="true"]')?.outerHTML.slice(0,300)}))).catch(()=>[]));console.error('FAILURE_MODAL',await p.locator('.ant-modal').evaluateAll(nodes=>nodes.map(n=>({html:n.outerHTML.slice(0,5000),ancestors:[n.parentElement?.outerHTML.slice(0,300)]}))).catch(()=>[]));console.error('FAILURE_PAGE',p.url(),await p.locator('body').innerText().catch(()=>''))}throw e}finally{await browser?.close();await server?.stop()}
