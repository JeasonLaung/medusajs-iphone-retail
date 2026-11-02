// Native Windows repair/exchange subflows; reuse established joint scenarios.
import assert from 'node:assert/strict'
import {execFileSync} from 'node:child_process'
import {readFileSync,writeFileSync} from 'node:fs'
import {mkdir} from 'node:fs/promises'
import {resolve} from 'node:path'
import {pathToFileURL,fileURLToPath} from 'node:url'
import {chromium,expect as baseExpect} from '../../admin/node_modules/@playwright/test/index.mjs'
const expect=baseExpect.configure({timeout:15000}),origin='http://127.0.0.1:19300',adminOrigin='http://127.0.0.1:19200'
import {requestCheckoutAfterSales,repairCheckoutOrder,verifyCheckoutRepair} from '../../admin/tests/checkout-repair.joint.mjs'
import {exchangeCheckoutOrder,verifyCheckoutExchange} from '../../admin/tests/checkout-exchange.joint.mjs'
import {refundCheckoutOrder,verifyCheckoutRefund} from '../../admin/tests/checkout-refund.joint.mjs'
import {prepareLocalRefundPayment,resumeLocalRefund} from './local-refund-payment.live.mjs'
import {fulfillLocalWarehouse} from './local-warehouse.live.mjs'
import {verifyCheckoutShipment} from '../../admin/tests/checkout-fulfillment.joint.mjs'
import {settleLocalSupplement} from './local-supplement.live.mjs'
const logs=process.env.ACCOUNT_TEST_LOGS,owner=process.env.ACCOUNT_TEST_OWNER,customerOwner=process.env.ACCOUNT_TEST_CUSTOMER_OWNER??owner,password='isolated-account-'+customerOwner+'-password',email=`local-account-${customerOwner}-1@example.invalid`
assert.match(owner??'',/^[0-9a-f]{12}$/)
const output=pathToFileURL(logs+'/'),errors=[],returning=process.env.ACCOUNT_TEST_SCENARIO==='buyback-return',shop=origin,segment='gb-en',index=1
const supplement=process.env.ACCOUNT_TEST_SCENARIO==='supplement'
const exchange=['exchange','supplement'].includes(process.env.ACCOUNT_TEST_SCENARIO)
const refund=process.env.ACCOUNT_TEST_SCENARIO==='refund'
const warehouse=['warehouse','paid-warehouse'].includes(process.env.ACCOUNT_TEST_SCENARIO)
const resumePaid=process.env.ACCOUNT_TEST_RESUME_PAID==='true'
const existingRefund=process.env.ACCOUNT_TEST_REFUND_EXISTING==='true'
let refundCompleted=false,refundState
const browser=await chromium.launch({headless:true,executablePath:resolve(process.env.LOCALAPPDATA,'ms-playwright/chromium-1234/chrome-win64/chrome.exe')})
try{
 const context=await browser.newContext({viewport:{width:390,height:900},reducedMotion:'reduce'}),page=await context.newPage()
 page.on('pageerror',e=>errors.push(e.message))
 async function read(path){const r=await context.request.get(origin+'/api/store/customer/'+path);return {status:r.status(),body:await r.json()}}
  const login=async(status=200)=>{
   await page.getByTestId('email-input').fill(email)
   await page.getByTestId('password-input').fill(password)
   const reply=page.waitForResponse(r=>new URL(r.url()).pathname==='/api/store/customer/auth/login')
   await page.getByTestId('sign-in-button').click()
   assert.equal((await reply).status(),status)
  }
  await page.goto(`${shop}/${segment}/account`,{waitUntil:'domcontentloaded'})
  await expect(page.getByTestId('login-page')).toBeVisible()
  if(!resumePaid){
  await page.getByTestId('register-button').click()
  const form=page.getByTestId('register-page')
  for(const [id,value] of Object.entries({'first-name-input':'Isolated','last-name-input':'Customer','email-input':email,'password-input':password}))await form.getByTestId(id).fill(value)
  const registration=page.waitForResponse(r=>new URL(r.url()).pathname==='/api/store/customer/auth/register')
  await form.getByTestId('register-button').click()
  assert.equal((await registration).status(),202)
  await expect(page.getByTestId('register-verification-message')).toBeVisible()
  await page.goto(`${shop}/${segment}/account`,{waitUntil:'domcontentloaded'})
  await login(403)
  assert.equal((await context.cookies(shop)).some(c=>c.name==='retail_customer_session'),false)
  const linkFile=resolve(logs,`verification-${index}.txt`)
  execFileSync(process.env.ACCOUNT_TEST_PYTHON,[resolve('../scripts/test-local-customer-account.py'),'--verification-link',process.env.ACCOUNT_TEST_DB,email,linkFile],{stdio:'pipe'})
  const verified=page.waitForResponse(r=>new URL(r.url()).pathname==='/api/store/customer/auth/verify')
  await page.goto(readFileSync(linkFile,'utf8'),{waitUntil:'domcontentloaded'})
  assert.equal((await verified).status(),200)
  }
  await page.goto(`${shop}/${segment}/account/orders`,{waitUntil:'domcontentloaded'})
  await login()
  await expect(page.getByTestId('customer-authenticated')).toBeVisible()
  if(!resumePaid)await expect(page.getByTestId('no-orders-container')).toBeVisible()
  // The copied real Mollie guest order must not become this account's property.
  assert.equal((await context.request.get(shop+'/api/store/customer/orders/order_8bffc6dc-112e-4ed5-8a32-57421bc2ae4d')).status(),404)

 const orderID=resumePaid?JSON.parse(readFileSync(resolve(logs,'refund-new-paid-order.json'),'utf8')).order_id:refund?await prepareLocalRefundPayment({page,context,origin,email,logs}):'order_8bffc6dc-112e-4ed5-8a32-57421bc2ae4d',sn='ISOLATED-REPAIR-001',tracking='ISOLATED-REPAIR-OUTBOUND-001'
 if(resumePaid)assert.equal((await context.request.get(origin+'/api/store/customer/orders/'+orderID)).status(),200)
 if(!refund&&!resumePaid)execFileSync(process.env.ACCOUNT_TEST_PYTHON,[resolve('../scripts/test-local-customer-account.py'),'--prepare-repair-owner',process.env.ACCOUNT_TEST_DB,email],{stdio:'pipe'})
 const credentials=readFileSync(resolve('../../.private-files/local/admin-credentials.txt'),'utf8')
 const credential=name=>credentials.split(/\r?\n/).find(x=>new RegExp('^'+name+'\\s*[:=]','i').test(x))?.replace(/^[^:=]+[:=]\s*/,'').trim()
 const admin=await context.newPage();admin.on('pageerror',e=>errors.push(e.message));await admin.setViewportSize({width:1440,height:1000})
 await admin.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','en'))
 await admin.goto(adminOrigin+'/orders/'+orderID)
 await admin.getByLabel('Email',{exact:true}).fill(credential('Email'));await admin.getByLabel('Password',{exact:true}).fill(credential('Password'));await admin.getByRole('button',{name:'Sign in',exact:true}).click()
 await expect(admin.getByRole('button',{name:'Sign out',exact:true})).toBeVisible()
 await admin.goto(adminOrigin+'/orders/'+orderID)
 if(!existingRefund){
 await expect(admin.getByRole('button',{name:'Allocate devices',exact:true})).toBeVisible()
 await admin.getByRole('button',{name:'Allocate devices',exact:true}).click()
 let dialog=admin.getByRole('dialog')
 await dialog.getByRole('row').filter({hasText:sn}).getByRole('checkbox').check()
 const allocation=admin.waitForResponse(r=>r.request().method()==='POST'&&r.url().endsWith('/allocation'))
 await dialog.getByRole('button',{name:'Save device reservation',exact:true}).click()
 assert.equal((await allocation).status(),200)
 await expect(dialog.getByText('isolated-repair-device',{exact:true})).toBeVisible()
 await dialog.getByRole('button',{name:'Cancel',exact:true}).click()
 if(warehouse){
 const result=await fulfillLocalWarehouse({browser,admin,origin:adminOrigin,orderID,owner,sn,tracking,errors})
 await page.goto(origin+'/gb-en/account/orders/details/'+orderID)
 await verifyCheckoutShipment({page,context,shop:origin,orderID,result,expect})
 assert.deepEqual(errors,[])
 writeFileSync(resolve(logs,'browser-result.json'),JSON.stringify({passed:true,scenario:resumePaid?'paid-warehouse':'warehouse',order_id:orderID,explicit_fixture_ownership:!resumePaid,new_paid_order_attachment_tested:resumePaid,paid_checkpoint_continuation:resumePaid,...result,customer_tracking_and_notice:true,browser_errors:0,real_mail_transfer_carrier:false},null,2))
 }else{
 await admin.getByRole('button',{name:'Create device parcel',exact:true}).click()
 dialog=admin.getByRole('dialog');await dialog.getByRole('row').filter({hasText:sn}).getByRole('checkbox').check()
 await dialog.getByRole('button',{name:'Pack and deduct stock',exact:true}).click()
 await expect(dialog.getByText('Parcel created',{exact:true})).toBeVisible()
 await dialog.getByRole('button',{name:/^Cancel$/}).click()
 await admin.getByRole('button',{name:'Parcel operations',exact:true}).click()
 dialog=admin.getByRole('dialog');await dialog.getByLabel('Tracking number',{exact:true}).fill(tracking)
 await dialog.getByLabel('Tracking URL',{exact:true}).fill('https://tracking.example.invalid/'+tracking)
 await dialog.getByRole('button',{name:'Ship entire parcel',exact:true}).click()
 await expect(dialog.getByText('Parcel operation saved',{exact:true})).toBeVisible()
 await admin.reload()
 }
 }
 if(!warehouse){
 await page.goto(origin+'/gb-en/account/orders/details/'+orderID)
 await expect(page.getByTestId('order-details-container')).toBeVisible()
 await expect(page.getByTestId('shipment')).toContainText(tracking)
 const result={sn,tracking}
 const checkpoint=existingRefund?JSON.parse(readFileSync(resolve(logs,'refund-existing.json'),'utf8')):null
 const request=existingRefund?{id:checkpoint.request_id,copy:JSON.parse(readFileSync(resolve('src/components/account/translations/en.json'),'utf8'))}:await requestCheckoutAfterSales({page,orderID,result,segment:'gb-en',type:refund?'return':exchange?'exchange':'repair'})
 if(existingRefund){
 const resumed=await resumeLocalRefund({admin,page,context,origin,orderID,request,operationID:checkpoint.operation_id})
 refundCompleted=resumed.completed;refundState=resumed.state
 if(refundCompleted)await verifyCheckoutRefund({page,context,shop:origin,orderID,result,request,segment:'gb-en',expectedUnread:4})
 }else if(refund){
 const accepted=await refundCheckoutOrder({page:admin,origin:adminOrigin,orderID,request,result,index:1,amount:'789.00',allowPending:true})
 if(accepted.pending){
 await admin.getByRole('dialog').locator('.ant-drawer-close').click()
 const resumed=await resumeLocalRefund({admin,page,context,origin,orderID,request,operationID:accepted.operation_id})
 refundCompleted=resumed.completed;refundState=resumed.state
 }else{refundCompleted=true;refundState='refunded'}
 if(refundCompleted)await verifyCheckoutRefund({page,context,shop:origin,orderID,result,request,segment:'gb-en',expectedUnread:4})
 }else if(exchange){
 if(supplement)execFileSync(process.env.ACCOUNT_TEST_PYTHON,[resolve('../scripts/test-local-customer-account.py'),'--supplement-tax',process.env.ACCOUNT_TEST_DB],{stdio:'pipe'})
 const dispatchReplacement=async(nextSN,nextTracking)=>{
  await admin.getByRole('button',{name:'Create device parcel',exact:true}).click()
  const packing=admin.getByRole('dialog')
  await packing.getByRole('row').filter({hasText:nextSN}).getByRole('checkbox').check()
  await packing.getByRole('button',{name:'Pack and deduct stock',exact:true}).click()
  await expect(packing.getByText('Parcel created',{exact:true})).toBeVisible()
  const fulfillment=(await packing.locator('.ant-alert-description').innerText()).trim()
  assert.match(fulfillment,/^ful_/)
  await packing.getByRole('button',{name:'Cancel',exact:true}).click()
  const parcel=admin.locator(`[data-fulfillment-id="${fulfillment}"]`)
  await parcel.getByRole('button',{name:'Parcel operations',exact:true}).click()
  const shipping=admin.getByRole('dialog')
  await shipping.getByLabel('Tracking number',{exact:true}).fill(nextTracking)
  await shipping.getByLabel('Tracking URL',{exact:true}).fill('https://tracking.example.invalid/'+nextTracking)
  await shipping.getByRole('button',{name:'Ship entire parcel',exact:true}).click()
  await expect(shipping.getByText('Parcel operation saved',{exact:true})).toBeVisible()
  return fulfillment
 }
 const replacement=await exchangeCheckoutOrder({page:admin,origin:adminOrigin,orderID,request,result,index:1,dispatchReplacement,...(supplement?{expectedDifference:'7.79',settlePayment:()=>settleLocalSupplement({page,context,origin,orderID,logs})}:{})})
 await verifyCheckoutExchange({page,context,shop:origin,orderID,result,request,replacement,segment:'gb-en',expectedUnread:supplement?5:4})
 }else{
 const returned=await repairCheckoutOrder({page:admin,origin:adminOrigin,orderID,request,result,index:1})
 await verifyCheckoutRepair({page,context,shop:origin,orderID,result,request,tracking:returned,expectedUnread:3})
 }
 assert.deepEqual(errors,[])
 writeFileSync(resolve(logs,'browser-result.json'),JSON.stringify({passed:true,scenario:supplement?'supplement':refund?'refund':exchange?'exchange':'repair',request_id:request.id,order_id:orderID,explicit_fixture_ownership:!refund&&!resumePaid,new_paid_order_attachment_tested:refund||resumePaid,real_mollie_sandbox_supplement:supplement,real_mollie_sandbox_refund:refund,refund_completed:refundCompleted,refund_state:refundState,pending_refund_completion_denied:refund&&!refundCompleted,actual_device_allocation_packing_shipping:true,customer_request_admin_receipt_inspection_dispatch_completion:!refund||refundCompleted,customer_repair_tracking:!exchange&&!refund,customer_original_and_replacement_tracking:exchange,lost_reply_original_command_recovery:!existingRefund,browser_errors:0,real_mail_transfer_carrier:false},null,2))
 }
}catch(e){for(const [i,p]of browser.contexts().flatMap(c=>c.pages()).entries()){await p.screenshot({path:resolve(logs,`failure-${i}.png`),fullPage:true}).catch(()=>{});console.error('FAILURE',p.url(),await p.locator('body').innerText().catch(()=>''))}throw e}finally{await browser.close()}
