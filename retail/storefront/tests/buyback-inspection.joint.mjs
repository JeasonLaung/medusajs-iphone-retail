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
const returning=process.env.BUYBACK_INSPECTION_DECISION==='decline'
const output=new URL('../../../logs/gin-migration/astro-buyback-'+(returning?'return':'settlement')+'/',import.meta.url),errors=[]
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
 const list=await read('buyback/requests?offset=0'),request=list.body.requests.find(r=>r.status==='received');assert.ok(request)
 const id=request.id
 const admin=await context.newPage();await admin.setViewportSize({width:1440,height:1000});admin.on('pageerror',e=>errors.push(e.message))
 await admin.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','en'))
 await admin.goto(origin+'/buyback');await admin.getByLabel('Email',{exact:true}).fill('browser@example.invalid');await admin.getByLabel('Password',{exact:true}).fill('artificial-browser-password');await admin.getByRole('button',{name:'Sign in',exact:true}).click();await expect(admin.getByRole('button',{name:'Sign out',exact:true})).toBeVisible()
 await admin.getByRole('menuitem',{name:'Selling requests',exact:true}).click();await admin.getByRole('button',{name:id,exact:true}).click()
 const drawer=admin.locator('.ant-drawer-content')
 await expect(drawer.getByRole('button',{name:'Send final offer',exact:true})).toBeDisabled()
 await drawer.getByRole('button',{name:'Link inspection device',exact:true}).click()
 let modal=admin.locator('.ant-modal:visible');await expect(modal.getByRole('combobox')).toBeEnabled();await admin.bringToFront();await modal.getByRole('combobox').focus();await modal.getByRole('combobox').press('ArrowDown');await modal.getByRole('combobox').press('Enter');await modal.getByRole('button',{name:'Confirm',exact:true}).click();await expect(admin.locator('.ant-modal:visible')).toHaveCount(0)
 await expect(drawer.getByText('Linked device: inspection-browser-device',{exact:true})).toBeVisible()
 await page.bringToFront();const linked=await read('buyback/requests/'+id);assert.equal(linked.body.request.items[0].device_id,'inspection-browser-device');assert.equal(linked.body.request.status,'received')
 const linkedNotices=await read('notifications?offset=0');assert.equal(linkedNotices.body.notifications.filter(n=>n.action==='link_device').length,0)
 await admin.bringToFront();await drawer.getByRole('button',{name:'Send final offer',exact:true}).click();modal=admin.locator('.ant-modal:visible');await modal.getByRole('button',{name:'Confirm',exact:true}).click();await expect(modal.locator('.ant-form-item-explain-error')).toHaveCount(2)
 await modal.getByLabel('Amount (EUR)',{exact:true}).fill('270.01');await modal.getByLabel('Inspection notes',{exact:true}).fill('Display inspected; minor frame wear.');await modal.getByRole('button',{name:'Confirm',exact:true}).click();await expect(admin.locator('.ant-modal:visible')).toHaveCount(0)
 await expect(drawer.getByText('Awaiting final-offer decision',{exact:true}).first()).toBeVisible()
 await mkdir(output,{recursive:true});await admin.screenshot({path:fileURLToPath(new URL('final-offer-admin.png',output)),fullPage:true})
 await page.bringToFront();await page.goto(origin+'/dk-en/account/buyback/'+id);await expect(page.getByRole('button',{name:'Accept final offer',exact:true})).toBeVisible()
 const offered=await read('buyback/requests/'+id);assert.equal(offered.body.request.final_amount,270.01);assert.equal(offered.body.request.items[0].inspection_notes,'Display inspected; minor frame wear.')
 const notice=await read('notifications?offset=0');assert.equal(notice.body.notifications.filter(n=>n.action==='final_offer').length,1)
 await page.screenshot({path:fileURLToPath(new URL('final-offer-customer-mobile.png',output)),fullPage:true})
 await page.getByRole('button',{name:returning?'Decline and request return':'Accept final offer',exact:true}).click();await page.getByRole('dialog').getByRole('button',{name:'Confirm',exact:true}).click();await expect(page.getByRole('dialog')).toHaveCount(0)
 await page.reload();await expect(page.getByText(returning?'Return requested':'Accepted / awaiting payment',{exact:true}).first()).toBeVisible()
 const accepted=await read('buyback/requests/'+id);assert.equal(accepted.body.request.final_amount,270.01);assert.equal(accepted.body.request.items[0].device_id,'inspection-browser-device')
 await admin.bringToFront();await drawer.getByRole('button',{name:'Refresh request',exact:true}).click();await expect(drawer.getByText(returning?'Return requested':'Accepted / awaiting payment',{exact:true}).first()).toBeVisible();await expect(drawer.getByRole('button',{name:'Send final offer',exact:true})).toHaveCount(0)

 if(returning){
  await drawer.getByRole('button',{name:'Record return shipment',exact:true}).click();modal=admin.locator('.ant-modal:visible');await modal.getByLabel('Return tracking number',{exact:true}).fill('RETURN-JOINT');await modal.getByRole('button',{name:'Confirm',exact:true}).click();await expect(admin.locator('.ant-modal:visible')).toHaveCount(0)
  await page.bringToFront();await page.reload();await expect(page.getByText('Returning',{exact:true}).first()).toBeVisible();await expect(page.getByText('RETURN-JOINT',{exact:true})).toBeVisible()
  await admin.bringToFront();await drawer.getByRole('button',{name:'Confirm return delivered',exact:true}).click();await admin.locator('.ant-modal:visible').getByRole('button',{name:'Confirm',exact:true}).click();await expect(admin.locator('.ant-modal:visible')).toHaveCount(0)
 }else{
  await drawer.getByRole('button',{name:'Record completed payment',exact:true}).click();modal=admin.locator('.ant-modal:visible');await modal.getByRole('button',{name:'Confirm',exact:true}).click();await expect(modal.locator('.ant-form-item-explain-error')).toHaveCount(1);await modal.getByLabel('Payment reference',{exact:true}).fill('TRANSFER-JOINT');await modal.getByRole('button',{name:'Confirm',exact:true}).click();await expect(admin.locator('.ant-modal:visible')).toHaveCount(0)
 }
 await page.bringToFront();await page.reload();await expect(page.getByText(returning?'Returned':'Paid',{exact:true}).first()).toBeVisible()
 const finished=await read('buyback/requests/'+id);assert.equal(finished.body.request.status,returning?'returned':'paid');assert.ok(finished.body.request.closed_at);assert.equal(finished.body.request[returning?'return_tracking':'payment_reference'],returning?'RETURN-JOINT':'TRANSFER-JOINT')
 const finalNotices=await read('notifications?offset=0');for(const action of(returning?['mark_returned']:['mark_paid']))assert.equal(finalNotices.body.notifications.filter(n=>n.action===action).length,1)
 await page.screenshot({path:fileURLToPath(new URL('closed-customer-mobile.png',output)),fullPage:true})
 await admin.bringToFront();await drawer.getByRole('button',{name:'Refresh request',exact:true}).click();await expect(drawer.getByText(returning?'Returned':'Paid',{exact:true}).first()).toBeVisible();await expect(drawer.getByRole('button',{name:'Record completed payment',exact:true})).toHaveCount(0)
 await drawer.getByRole('button',{name:'Close',exact:true}).click();await expect(drawer).not.toBeVisible()
 const language=admin.getByRole('combobox',{name:'Language',exact:true});await admin.locator('.ant-select').filter({has:language}).locator('.ant-select-selector').click();await language.press('ArrowUp');await language.press('Enter');await expect(admin.getByRole('heading',{name:'回收申请',exact:true})).toBeVisible()
 await admin.setViewportSize({width:390,height:900});assert.ok(await admin.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));await admin.getByRole('button',{name:id,exact:true}).click();await expect(drawer.getByRole('heading',{name:'检测与最终报价',exact:true})).toBeVisible();await expect(drawer.getByText('最终报价：EUR 270.01 · Display inspected; minor frame wear.',{exact:true})).toBeVisible()
 await admin.screenshot({path:fileURLToPath(new URL('accepted-admin-zh-mobile.png',output)),fullPage:true})
 assert.deepEqual(errors,[])
 console.log('BUYBACK_INSPECTION_JOINT '+JSON.stringify({synthetic_api:false,real_admin_and_customer:true,device_link_persisted:true,link_notice_suppressed:true,final_offer_persisted:true,customer_decision:returning?'declined':'accepted',closed_state:returning?'returned':'paid',closed_at_persisted:true,payment_reference_or_return_tracking_persisted:true,closing_notices_once:true,offer_notice_exactly_once:true,final_amount:270.01,admin_zh_mobile:true,browser_errors:0}))
}catch(e){for(const [i,p]of(browser?.contexts().flatMap(c=>c.pages())??[]).entries()){console.error('FAILURE_PAGE',p.url(),await p.locator('body').innerText().catch(()=>''));await mkdir(output,{recursive:true});await p.screenshot({path:fileURLToPath(new URL('failure-'+i+'.png',output)),fullPage:true}).catch(()=>{})}throw e}finally{await browser?.close();await server?.stop()}
