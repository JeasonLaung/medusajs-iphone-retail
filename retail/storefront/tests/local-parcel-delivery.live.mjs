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
try{
 const output=execFileSync(process.env.WINDOWS_PARCEL_DELIVERY_FIXTURE,['-test.run=^TestWindowsParcelDelivery$','-test.v'],{env:{...process.env,WINDOWS_DELIVERY_ORDER:orderID},timeout:90000,windowsHide:true})
 writeFileSync(resolve(logs,'parcel-delivery-guards.log'),output)
}catch(e){writeFileSync(resolve(logs,'parcel-delivery-guards.log'),Buffer.concat([e.stdout??Buffer.alloc(0),e.stderr??Buffer.alloc(0)]));throw Error('Owned delivery guards failed; inspect parcel-delivery-guards.log')}
const browser=await chromium.launch({headless:true,executablePath:resolve(process.env.LOCALAPPDATA,'ms-playwright/chromium-1234/chrome-win64/chrome.exe')}),errors=[]
try{
 const context=await browser.newContext({viewport:{width:390,height:900},reducedMotion:'reduce'}),page=await context.newPage(),admin=await context.newPage()
 for(const p of [page,admin])p.on('pageerror',e=>errors.push(e.message))
 await page.goto(shop+'/gb-en/account/orders/details/'+orderID)
 await page.getByTestId('email-input').fill(`local-account-${owner}-1@example.invalid`);await page.getByTestId('password-input').fill('isolated-account-'+owner+'-password');await page.getByTestId('sign-in-button').click()
 await expect(page.getByTestId('order-details-container')).toBeVisible();await expect(page.getByTestId('timeline-delivered')).toHaveCount(0)
 const progress=await (await context.request.get(shop+'/api/store/customer/orders/'+orderID+'/progress')).json()
 assert.equal(progress.shipments.length,1);assert.equal(progress.shipments[0].delivered_at,null)
 const parcel=progress.shipments[0].id,labels=progress.shipments[0].labels
 const credentials=readFileSync(resolve('../../.private-files/local/admin-credentials.txt'),'utf8')
 const credential=name=>credentials.split(/\r?\n/).find(x=>new RegExp('^'+name+'\\s*[:=]','i').test(x))?.replace(/^[^:=]+[:=]\s*/,'').trim()
 await admin.setViewportSize({width:1440,height:1000});await admin.addInitScript(()=>{if(!localStorage.getItem('eurorefurb.admin.language'))localStorage.setItem('eurorefurb.admin.language','en')})
 await admin.goto(origin+'/orders/'+orderID);await admin.getByLabel('Email',{exact:true}).fill(credential('Email'));await admin.getByLabel('Password',{exact:true}).fill(credential('Password'));await admin.getByRole('button',{name:'Sign in',exact:true}).click();await expect(admin.getByRole('button',{name:'Sign out',exact:true})).toBeVisible()
 const open=async()=>{await admin.goto(origin+'/orders/'+orderID);await admin.locator(`[data-fulfillment-id="${parcel}"]`).getByRole('button',{name:'Parcel operations',exact:true}).click()}
 await open();let drawer=admin.getByRole('dialog');await expect(drawer.getByRole('radio',{name:'Confirm delivery',exact:true})).toBeChecked()
 await expect(drawer.getByRole('radio',{name:'Ship entire parcel',exact:true})).toBeDisabled();await expect(drawer.getByRole('radio',{name:'Cancel parcel',exact:true})).toBeDisabled()
 const commands=[],replies=[]
 await admin.route('**/fulfillments/'+parcel+'/operations',async route=>{
  if(route.request().method()!=='POST')return route.continue()
  commands.push(route.request().postDataJSON())
  if(commands.length===1){const responses=await Promise.all(Array.from({length:6},()=>route.fetch()));for(const r of responses){assert.equal(r.status(),200);replies.push(await r.json())}await route.abort('failed')}
  else{const response=await route.fetch();assert.equal(response.status(),200);assert.equal((await response.json()).replayed,true);await route.fulfill({response})}
 })
 await drawer.getByRole('button',{name:'Confirm delivery',exact:true}).click();await admin.getByRole('button',{name:'Confirm delivery',exact:true}).last().click()
 await expect(drawer.getByText('Save result is unconfirmed. Retry the same save with its original contents, or close to reload the product.',{exact:true})).toBeVisible()
 await drawer.getByRole('button',{name:'Retry this save',exact:true}).click();await expect(drawer.getByText('Parcel operation saved',{exact:true})).toBeVisible()
 await admin.unrouteAll({behavior:'wait'});assert.equal(commands.length,2);assert.deepEqual(commands[0],commands[1]);assert.equal(replies.filter(r=>!r.replayed).length,1);assert.equal(replies.filter(r=>r.replayed).length,5)
 await expect(drawer.getByRole('button',{name:'Confirm delivery',exact:true})).toBeDisabled()
 const current=await (await admin.request.get(origin+'/api/admin/orders/'+orderID+'/fulfillments/'+parcel+'/operations')).json()
 const csrf=(await context.cookies(origin)).find(c=>c.name==='retail_admin_csrf')?.value
 const rejectHeaders={'Content-Type':'application/json','Origin':origin,...(csrf?{'X-CSRF-Token':decodeURIComponent(csrf)}:{})}
 for(const action of ['deliver','ship','cancel']){
  const response=await admin.request.post(origin+'/api/admin/orders/'+orderID+'/fulfillments/'+parcel+'/operations',{headers:rejectHeaders,data:{operation_id:crypto.randomUUID(),expected_revision:current.revision,action,tracking_number:action==='ship'?'INVALID-SECOND-SHIP':'',tracking_url:''}})
  assert.equal(response.status(),409)
 }
 await admin.screenshot({path:resolve(logs,'delivery-admin-en.png'),fullPage:true})
 await drawer.getByRole('button',{name:'Cancel',exact:true}).click()
 await admin.evaluate(()=>localStorage.setItem('eurorefurb.admin.language','zh'));await admin.reload();await admin.locator(`[data-fulfillment-id="${parcel}"]`).getByRole('button',{name:'包裹操作',exact:true}).click();drawer=admin.getByRole('dialog');await expect(drawer.getByRole('button',{name:'确认包裹送达',exact:true})).toBeDisabled();await admin.screenshot({path:resolve(logs,'delivery-admin-zh.png'),fullPage:true});await drawer.locator('.ant-drawer-close').click()
 await admin.evaluate(()=>localStorage.setItem('eurorefurb.admin.language','en'));await admin.reload()
 for(const segment of ['gb-en','gb-zh']){
  await page.goto(shop+'/'+segment+'/account/orders/details/'+orderID);await expect(page.getByTestId('timeline-delivered')).toHaveCount(1)
  const r=await context.request.get(shop+'/api/store/customer/orders/'+orderID+'/progress');assert.equal(r.status(),200);const after=await r.json();assert.equal(after.shipments.length,1);assert.ok(after.shipments[0].delivered_at);assert.deepEqual(after.shipments[0].labels,labels);assert.equal(after.events.filter(e=>e.kind==='delivered').length,1)
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true);await page.screenshot({path:resolve(logs,'delivery-customer-'+segment+'.png'),fullPage:true})
 }
 const notice=await (await context.request.get(shop+'/api/store/customer/notifications?offset=0')).json();assert.equal(notice.notifications.filter(n=>n.kind==='delivery.created').length,1)
 const mail=await verifyCustomerEventMail({admin,origin,logs,expect,id:orderID,customerOwner:owner,expectedKinds:['delivery.created'],safety:false})
 assert.deepEqual(errors,[])
 writeFileSync(resolve(logs,'browser-result.json'),JSON.stringify({passed:true,scenario:'delivery-mail',order_id:orderID,fulfillment_id:parcel,operation_id:commands[0].operation_id,...mail,guards:JSON.parse(readFileSync(resolve(logs,'parcel-delivery-guards.json'),'utf8')),six_concurrent_one_fresh_five_replayed:true,lost_response_same_command:true,customer_en_zh_mobile:true,admin_en_zh:true,original_tracking_retained:true,no_payment_or_shipment_replayed:true,browser_errors:0},null,2))
}catch(e){for(const [i,p]of browser.contexts().flatMap(c=>c.pages()).entries())await p.screenshot({path:resolve(logs,`delivery-failure-${i}.png`),fullPage:true}).catch(()=>{});throw e}finally{await browser.close()}
