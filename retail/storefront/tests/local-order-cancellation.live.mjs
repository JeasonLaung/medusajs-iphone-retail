import assert from 'node:assert/strict'
import {readFileSync,writeFileSync} from 'node:fs'
import {execFileSync} from 'node:child_process'
import {resolve} from 'node:path'
import {chromium,expect as baseExpect} from '../../admin/node_modules/@playwright/test/index.mjs'
import {verifyCustomerEventMail} from './local-buyback-mail.live.mjs'
const expect=baseExpect.configure({timeout:15000}),shop='http://127.0.0.1:19300',origin='http://127.0.0.1:19200'
const logs=process.env.ACCOUNT_TEST_LOGS,owner=process.env.ACCOUNT_TEST_CUSTOMER_OWNER
assert.match(owner??'',/^[0-9a-f]{12}$/)
const {order_id:orderID}=JSON.parse(readFileSync(resolve(logs,'refund-new-paid-order.json'),'utf8'))
const fixture=phase=>{
 try{writeFileSync(resolve(logs,'cancellation-'+phase+'.log'),execFileSync(process.env.WINDOWS_CANCELLATION_FIXTURE,['-test.run=^TestWindowsOrderCancellation$','-test.v'],{env:{...process.env,WINDOWS_CANCELLATION_ORDER:orderID,WINDOWS_CANCELLATION_PHASE:phase},timeout:90000,windowsHide:true}))}
 catch(e){writeFileSync(resolve(logs,'cancellation-'+phase+'.log'),Buffer.concat([e.stdout??Buffer.alloc(0),e.stderr??Buffer.alloc(0)]));throw Error('Owned cancellation phase failed: '+phase)}
}
const fault=phase=>execFileSync(process.env.ACCOUNT_TEST_PYTHON,[resolve('../../retail/scripts/test-local-customer-account.py'),'--cancellation-fault','retail_account_'+process.env.ACCOUNT_TEST_OWNER,orderID,phase],{env:process.env,windowsHide:true,timeout:30000})
fixture('guards')
const browser=await chromium.launch({headless:true,executablePath:resolve(process.env.LOCALAPPDATA,'ms-playwright/chromium-1234/chrome-win64/chrome.exe')}),errors=[]
try{
 const context=await browser.newContext({viewport:{width:390,height:900},reducedMotion:'reduce'}),page=await context.newPage(),admin=await context.newPage()
 for(const p of [page,admin])p.on('pageerror',e=>errors.push(e.message))
 await page.goto(shop+'/gb-en/account/orders/details/'+orderID)
 await page.getByTestId('email-input').fill(`local-account-${owner}-1@example.invalid`);await page.getByTestId('password-input').fill('isolated-account-'+owner+'-password');await page.getByTestId('sign-in-button').click();await expect(page.getByTestId('order-details-container')).toBeVisible()
 const credentials=readFileSync(resolve('../../.private-files/local/admin-credentials.txt'),'utf8'),credential=name=>credentials.split(/\r?\n/).find(x=>new RegExp('^'+name+'\\s*[:=]','i').test(x))?.replace(/^[^:=]+[:=]\s*/,'').trim()
 await admin.setViewportSize({width:1440,height:1000});await admin.addInitScript(()=>{if(!localStorage.getItem('eurorefurb.admin.language'))localStorage.setItem('eurorefurb.admin.language','en')})
 await admin.goto(origin+'/orders/'+orderID);await admin.getByLabel('Email',{exact:true}).fill(credential('Email'));await admin.getByLabel('Password',{exact:true}).fill(credential('Password'));await admin.getByRole('button',{name:'Sign in',exact:true}).click();await expect(admin.getByRole('button',{name:'Sign out',exact:true})).toBeVisible()
 await admin.goto(origin+'/orders/'+orderID);await admin.getByRole('button',{name:'Cancel order',exact:true}).click()
 let drawer=admin.getByRole('dialog');await expect(drawer.getByRole('button',{name:'Submit order cancellation',exact:true})).toBeEnabled()
 const route=origin+'/api/admin/orders/'+orderID+'/cancellation',roots=[],replies=[]
 await admin.route(route,async r=>{if(r.request().method()!=='POST')return r.continue();roots.push(r.request().postDataJSON());const responses=await Promise.all(Array.from({length:6},()=>r.fetch()));for(const response of responses){assert.equal(response.status(),202);replies.push(await response.json())}await r.abort('failed')})
 await drawer.getByLabel('Cancellation reason',{exact:true}).fill('Customer requested cancellation — original payment refund.')
 await drawer.getByRole('button',{name:'Submit order cancellation',exact:true}).click();await admin.getByRole('button',{name:'Confirm cancellation',exact:true}).click()
 await expect(drawer.getByRole('button',{name:'Retry same cancellation operation',exact:true})).toBeVisible();await admin.unroute(route);await drawer.getByRole('button',{name:'Retry same cancellation operation',exact:true}).click()
 await expect(drawer.getByText('Cancellation in progress',{exact:true})).toBeVisible();assert.equal(roots.length,1);assert.equal(replies.length,6);assert.equal(new Set(replies.map(x=>x.operation_id)).size,1);assert.equal(new Set(replies.map(x=>x.refunds[0].operation_id)).size,1)
 fixture('failed');await drawer.getByRole('button',{name:'Reload cancellation status',exact:true}).click();await expect(drawer.getByRole('button',{name:'Retry failed refund',exact:true})).toBeVisible()
 const retryRoute=route+'/refunds/'+replies[0].refunds[0].operation_id+'/retry',retryCommands=[]
 await admin.route(retryRoute,async r=>{retryCommands.push(r.request().postDataJSON());const responses=await Promise.all(Array.from({length:6},()=>r.fetch()));for(const response of responses)assert.equal(response.status(),202);await r.abort('failed')})
 await drawer.getByRole('button',{name:'Retry failed refund',exact:true}).click();await admin.getByRole('button',{name:'OK',exact:true}).click();await expect(drawer.getByRole('button',{name:'Retry same cancellation operation',exact:true})).toBeVisible();await admin.unroute(retryRoute);await drawer.getByRole('button',{name:'Retry same cancellation operation',exact:true}).click();await expect(drawer.getByText('Cancellation in progress',{exact:true})).toBeVisible();assert.equal(retryCommands.length,1)
 fixture('lost');fixture('pending')
 const csrf=(await context.cookies(origin)).find(c=>c.name==='retail_admin_csrf')?.value,headers={'Content-Type':'application/json','Origin':origin,'X-CSRF-Token':decodeURIComponent(csrf??'')}
 const held=await (await admin.request.get(route)).json();assert.equal(held.state,'processing');assert.equal(held.refunds.length,2)
 const illegal=await admin.request.post(route,{headers,data:{...roots[0],operation_id:crypto.randomUUID()}});assert.equal(illegal.status(),409)
 const replayChanged=await admin.request.post(route,{headers,data:{...roots[0],reason:'Changed reason'}});assert.equal(replayChanged.status(),409)
 fault('refund-on');fixture('refunded');fault('refund-off');fixture('apply')
 fixture('devices');fault('cancel-on');fixture('finish-failure');fault('cancel-off');fixture('finish')
 await drawer.getByRole('button',{name:'Reload cancellation status',exact:true}).click();await expect(drawer.getByText('Order canceled',{exact:true})).toBeVisible();await expect(drawer.getByText(roots[0].reason,{exact:true})).toBeVisible();await admin.screenshot({path:resolve(logs,'cancellation-admin-en.png'),fullPage:true});await drawer.locator('.ant-drawer-close').click()
 await admin.evaluate(()=>localStorage.setItem('eurorefurb.admin.language','zh'));await admin.reload();await admin.getByRole('button',{name:'取消整单',exact:true}).click();drawer=admin.getByRole('dialog');await expect(drawer.getByText('订单已取消',{exact:true})).toBeVisible();await admin.screenshot({path:resolve(logs,'cancellation-admin-zh.png'),fullPage:true});await drawer.locator('.ant-drawer-close').click();await admin.evaluate(()=>localStorage.setItem('eurorefurb.admin.language','en'));await admin.reload()
 for(const segment of ['gb-en','gb-zh']){await page.goto(shop+'/'+segment+'/account/orders/details/'+orderID);await expect(page.getByTestId('timeline-canceled')).toHaveCount(1);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true);await page.screenshot({path:resolve(logs,'cancellation-customer-'+segment+'.png'),fullPage:true})}
 const notices=await(await context.request.get(shop+'/api/store/customer/notifications?offset=0')).json();for(const kind of ['order.canceled','refund.created'])assert.equal(notices.notifications.filter(n=>n.kind===kind).length,1)
 const mail=await verifyCustomerEventMail({admin,origin,logs,expect,id:orderID,customerOwner:owner,expectedKinds:['refund.created','order.canceled'],safety:false})
 assert.deepEqual(errors,[])
 const phases=Object.fromEntries(['guards','failed','lost','pending','refunded','apply','devices','finish-failure','finish'].map(p=>[p,JSON.parse(readFileSync(resolve(logs,'cancellation-'+p+'.json'),'utf8'))]))
 writeFileSync(resolve(logs,'browser-result.json'),JSON.stringify({passed:true,scenario:'cancellation',order_id:orderID,operation_id:roots[0].operation_id,phases,...mail,six_concurrent_admissions_one_root_one_child:true,lost_root_and_retry_replies_recovered:true,failed_refund_explicit_successor:true,uncertain_refund_get_only:true,verified_refund_cached_after_ledger_rollback:true,stock_and_cancel_reason_atomic:true,customer_en_zh_mobile:true,admin_en_zh:true,no_new_checkout_or_payment:true,browser_errors:errors.length},null,2))
}catch(e){for(const[i,p]of browser.contexts().flatMap(c=>c.pages()).entries())await p.screenshot({path:resolve(logs,`cancellation-failure-${i}.png`),fullPage:true}).catch(()=>{});throw e}finally{await browser.close()}
