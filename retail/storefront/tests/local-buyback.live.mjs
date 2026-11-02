// Native Windows buyback main flow; reuses the established joint scenarios.
import assert from 'node:assert/strict'
import {intakeBuybackDevice} from './local-buyback-intake.live.mjs'
import {verifyBuybackMail} from './local-buyback-mail.live.mjs'
import {execFileSync} from 'node:child_process'
import {readFileSync,writeFileSync} from 'node:fs'
import {mkdir} from 'node:fs/promises'
import {resolve} from 'node:path'
import {pathToFileURL,fileURLToPath} from 'node:url'
import {chromium,expect as baseExpect} from '../../admin/node_modules/@playwright/test/index.mjs'
const expect=baseExpect.configure({timeout:15000}),origin='http://127.0.0.1:19300',adminOrigin='http://127.0.0.1:19200'
const logs=process.env.ACCOUNT_TEST_LOGS,owner=process.env.ACCOUNT_TEST_OWNER,password='isolated-account-'+owner+'-password',email=`local-account-${owner}-1@example.invalid`
assert.match(owner??'',/^[0-9a-f]{12}$/)
const output=pathToFileURL(logs+'/'),errors=[],returning=['buyback-return','buyback-intake-return'].includes(process.env.ACCOUNT_TEST_SCENARIO),shop=origin,segment='gb-en',index=1
const actualIntake=['buyback-intake','buyback-intake-return','buyback-mail'].includes(process.env.ACCOUNT_TEST_SCENARIO)
let intakeFacts={},deviceID='isolated-buyback-device'
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
  await page.goto(`${shop}/${segment}/account/orders`,{waitUntil:'domcontentloaded'})
  await login()
  await expect(page.getByTestId('customer-authenticated')).toBeVisible()
  await expect(page.getByTestId('no-orders-container')).toBeVisible()
  // The copied real Mollie guest order must not become this account's property.
  assert.equal((await context.request.get(shop+'/api/store/customer/orders/order_8bffc6dc-112e-4ed5-8a32-57421bc2ae4d')).status(),404)
 async function questionnaire(){
 await page.goto(origin+'/gb-en/sell/iphone/model')
 await page.getByRole('combobox',{name:'Select device model'}).fill('iPhone 13');await page.getByText('iPhone 13',{exact:true}).click()
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
 const credentials=readFileSync(resolve('../../.private-files/local/admin-credentials.txt'),'utf8')
 const credential=name=>credentials.split(/\r?\n/).find(x=>new RegExp('^'+name+'\\s*[:=]','i').test(x))?.replace(/^[^:=]+[:=]\s*/,'').trim()
 const admin=await context.newPage();await admin.setViewportSize({width:1440,height:1000});admin.on('pageerror',e=>{errors.push(e.message);console.error('ADMIN_PAGE_ERROR',e.message)})
 await admin.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','en'))
 await admin.goto(adminOrigin+'/buyback')
 await admin.getByLabel('Email',{exact:true}).fill(credential('Email'))
 await admin.getByLabel('Password',{exact:true}).fill(credential('Password'))
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
 await expect(drawer.getByRole('button',{name:'Send final offer',exact:true})).toBeDisabled()
 if(actualIntake){intakeFacts=await intakeBuybackDevice({context,origin:adminOrigin,owner,expect,png,errors});deviceID=intakeFacts.intake_device_id;await admin.bringToFront();await drawer.getByRole('button',{name:'Refresh request',exact:true}).click()}
 await drawer.getByRole('button',{name:'Link inspection device',exact:true}).click()
 modal=admin.locator('.ant-modal:visible');await expect(modal.getByRole('combobox')).toBeEnabled();await admin.bringToFront();await modal.getByRole('combobox').focus();await modal.getByRole('combobox').press('ArrowDown');await modal.getByRole('combobox').press('Enter');await modal.getByRole('button',{name:'Confirm',exact:true}).click();await expect(admin.locator('.ant-modal:visible')).toHaveCount(0)
 await expect(drawer.getByText('Linked device: '+deviceID,{exact:true})).toBeVisible()
 await page.bringToFront();const linked=await read('buyback/requests/'+id);assert.equal(linked.body.request.items[0].device_id,deviceID);assert.equal(linked.body.request.status,'received')
 const linkedNotices=await read('notifications?offset=0');assert.equal(linkedNotices.body.notifications.filter(n=>n.action==='link_device').length,0)
 await admin.bringToFront();await drawer.getByRole('button',{name:'Send final offer',exact:true}).click();modal=admin.locator('.ant-modal:visible');await modal.getByRole('button',{name:'Confirm',exact:true}).click();await expect(modal.locator('.ant-form-item-explain-error')).toHaveCount(2)
 await modal.getByLabel('Amount (EUR)',{exact:true}).fill('270.01');await modal.getByLabel('Inspection notes',{exact:true}).fill('Display inspected; minor frame wear.');await modal.getByRole('button',{name:'Confirm',exact:true}).click();await expect(admin.locator('.ant-modal:visible')).toHaveCount(0)
 await expect(drawer.getByText('Awaiting final-offer decision',{exact:true}).first()).toBeVisible()
 await mkdir(output,{recursive:true});await admin.screenshot({path:fileURLToPath(new URL('final-offer-admin.png',output)),fullPage:true})
 await page.bringToFront();await page.goto(origin+'/gb-en/account/buyback/'+id);await expect(page.getByRole('button',{name:'Accept final offer',exact:true})).toBeVisible()
 const offered=await read('buyback/requests/'+id);assert.equal(offered.body.request.final_amount,270.01);assert.equal(offered.body.request.items[0].inspection_notes,'Display inspected; minor frame wear.')
 const notice=await read('notifications?offset=0');assert.equal(notice.body.notifications.filter(n=>n.action==='final_offer').length,1)
 await page.screenshot({path:fileURLToPath(new URL('final-offer-customer-mobile.png',output)),fullPage:true})
 await page.getByRole('button',{name:returning?'Decline and request return':'Accept final offer',exact:true}).click();await page.getByRole('dialog').getByRole('button',{name:'Confirm',exact:true}).click();await expect(page.getByRole('dialog')).toHaveCount(0)
 await page.reload();await expect(page.getByText(returning?'Return requested':'Accepted / awaiting payment',{exact:true}).first()).toBeVisible()
 const accepted=await read('buyback/requests/'+id);assert.equal(accepted.body.request.final_amount,270.01);assert.equal(accepted.body.request.items[0].device_id,deviceID)
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

 if(process.env.ACCOUNT_TEST_SCENARIO==='buyback-mail')Object.assign(intakeFacts,await verifyBuybackMail({admin,origin:adminOrigin,logs,expect,id}))
 assert.deepEqual(errors,[])
 writeFileSync(resolve(logs,'browser-result.json'),JSON.stringify({passed:true,scenario:process.env.ACCOUNT_TEST_SCENARIO,...intakeFacts,original_questionnaire:true,registered_verified_customer:true,customer_photos_admin_review_shipping_received:true,device_link_final_offer_decision_closing:true,closed_state:returning?'returned':'paid',real_transfer:false,real_carrier:false,real_email_delivery:false,business_api_mocking:false,browser_errors:0,request_id:id},null,2))
}catch(e){for(const [i,p]of browser.contexts().flatMap(c=>c.pages()).entries()){await p.screenshot({path:resolve(logs,`failure-${i}.png`),fullPage:true}).catch(()=>{});console.error('FAILURE',p.url(),await p.locator('body').innerText().catch(()=>''))}throw e}finally{await browser.close()}
