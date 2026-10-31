import {refundCheckoutOrder} from './checkout-refund.joint.mjs'
import {exchangeCheckoutOrder} from './checkout-exchange.joint.mjs'
import assert from 'node:assert/strict'
import {enableCheckoutTracking,repairCheckoutOrder} from './checkout-repair.joint.mjs'
import {createServer} from 'vite'
import {expect} from '@playwright/test'
import {fileURLToPath} from 'node:url'

// An opt-in bridge between the real Astro checkout and the existing Admin
// packing/warehouse controls. No business responses or SQL facts are fabricated.
export async function startCheckoutFulfillment({browser,target,errors}){
 const origin='http://127.0.0.1:19383'
 process.env.ADMIN_TEST_API_TARGET=target
 let server,admin,worker
 try{
  server=await createServer({root:fileURLToPath(new URL('..',import.meta.url)),configFile:fileURLToPath(new URL('../vite.config.ts',import.meta.url)),server:{port:19383,strictPort:true,host:'127.0.0.1'},logLevel:'error'});await server.listen()
  async function login(context,email,password){
   await context.addInitScript(()=>{if(!localStorage.getItem('eurorefurb.admin.language'))localStorage.setItem('eurorefurb.admin.language','en')})
   const p=await context.newPage();p.on('pageerror',e=>errors.push(e.message))
   await p.goto(origin+'/account');await p.getByLabel('Email',{exact:true}).fill(email);await p.getByLabel('Password',{exact:true}).fill(password);await p.getByRole('button',{name:'Sign in',exact:true}).click();await expect(p.getByRole('button',{name:'Sign out',exact:true})).toBeVisible();return p
  }
  admin=await browser.newContext({viewport:{width:1440,height:1000},reducedMotion:'reduce'});admin.setDefaultTimeout(12000)
  const page=await login(admin,'browser@example.invalid','artificial-browser-password')
  if(process.env.CHECKOUT_REFUND==='1'||process.env.CHECKOUT_REPAIR==='1'||process.env.CHECKOUT_EXCHANGE==='1')await enableCheckoutTracking(page,origin)
  const response=await admin.request.get(origin+'/api/admin/warehouse-users');assert.equal(response.status(),200)
  const directory=await response.json(),location=directory.locations.find(l=>l.id==='add-location');assert.ok(location)
  await page.goto(origin+'/warehouse-users');await page.getByRole('button',{name:'New warehouse user',exact:true}).click()
  const editor=page.getByRole('dialog')
  await editor.getByLabel('Name',{exact:true}).fill('Checkout warehouse operator');await editor.getByLabel('Email',{exact:true}).fill('checkout-warehouse@example.invalid');await editor.getByLabel('Password',{exact:true}).fill('artificial-checkout-warehouse-password')
  await editor.getByText(`${location.name} (${location.id})`,{exact:true}).click();await editor.locator('.ant-transfer-operation button').first().click()
  for(const name of ['View fulfillment records','Ship warehouse parcels','Allocate warehouse parcels']){await editor.getByRole('checkbox',{name,exact:true}).click();await expect(editor.getByRole('checkbox',{name,exact:true})).toBeChecked()}
  await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(editor).not.toBeVisible()
  worker=await browser.newContext({viewport:{width:390,height:1000},reducedMotion:'reduce'});worker.setDefaultTimeout(12000)
  const warehouse=await login(worker,'checkout-warehouse@example.invalid','artificial-checkout-warehouse-password')
  assert.equal((await worker.request.get(origin+'/api/admin/orders')).status(),403)
  async function allocate(orderID,sn){
   await page.goto(origin+'/orders/'+orderID);await page.getByRole('button',{name:'Allocate devices',exact:true}).click()
   const d=page.getByRole('dialog');await d.getByRole('row').filter({has:page.getByText(sn,{exact:true})}).getByRole('checkbox').check()
   const saved=page.waitForResponse(r=>r.request().method()==='POST'&&r.url().endsWith('/allocation'))
   await d.getByRole('button',{name:'Save device reservation',exact:true}).click();assert.equal((await saved).status(),200)
   await expect(d.getByText(sn.toLowerCase(),{exact:true}).first()).toBeVisible()
   await d.getByRole('button',{name:'Cancel',exact:true}).click()
  }
  async function pack(sn){
   await page.getByRole('button',{name:'Create device parcel',exact:true}).click();const d=page.getByRole('dialog')
   await d.getByRole('row').filter({has:page.getByText(sn,{exact:true})}).getByRole('checkbox').check()
   await d.getByRole('button',{name:'Pack and deduct stock',exact:true}).click();await expect(d.getByText('Parcel created',{exact:true})).toBeVisible()
   const id=(await d.locator('.ant-alert-description').innerText()).trim();assert.match(id,/^ful_/);await d.getByRole('button',{name:'Cancel',exact:true}).click();return id
  }
  async function parcel(id){await warehouse.goto(origin+'/warehouse-fulfillments');await warehouse.getByTestId('warehouse-fulfillments').getByRole('button',{name:id,exact:true}).click();return warehouse.locator('.ant-drawer-content')}
  async function retryLost(p,pattern,action,root,completed){
   const requests=[]
   await p.route(pattern,async route=>{if(route.request().method()!=='POST'){await route.continue();return}requests.push(route.request().postDataJSON());const r=await route.fetch();assert.equal(r.status(),200);if(requests.length===1)await route.abort('failed');else await route.fulfill({response:r})})
   try{await action();await expect(root.getByRole('button',{name:'Retry this save',exact:true})).toBeVisible();await root.getByRole('button',{name:'Retry this save',exact:true}).click();await completed();assert.equal(requests.length,2);assert.deepEqual(requests[0],requests[1])}finally{await p.unrouteAll({behavior:'wait'})}
  }
  return {
   async fulfill(orderID,index){
    const sn='CHECKOUT-DEVICE-'+index,tracking='CHECKOUT-TRACK-'+index
    await allocate(orderID,sn);const canceled=await pack(sn);let d=await parcel(canceled)
    await expect(d.getByTestId('warehouse-release').getByText(sn,{exact:true})).toBeVisible()
    await d.getByTestId('warehouse-release').getByRole('button',{name:'Release device allocation',exact:true}).click()
    const modal=warehouse.locator('.ant-modal-content')
    await retryLost(warehouse,'**/release/*',()=>modal.getByRole('button',{name:'Save',exact:true}).click(),modal,()=>expect(modal).not.toBeVisible())
    await expect(modal).not.toBeVisible()
    await allocate(orderID,sn);const shipped=await pack(sn);assert.notEqual(shipped,canceled);d=await parcel(shipped)
    const shipping=d.getByTestId('warehouse-shipment')
    await shipping.getByLabel('Tracking number',{exact:true}).fill(tracking);await shipping.getByLabel('Tracking URL',{exact:true}).fill('https://tracking.example.invalid/'+tracking)
    await retryLost(warehouse,'**/shipment',()=>shipping.getByRole('button',{name:'Ship entire parcel',exact:true}).click(),shipping,()=>expect(d.getByText(tracking,{exact:true})).toBeVisible())
    await expect(d.getByText(tracking,{exact:true})).toBeVisible();await expect(shipping.getByRole('button',{name:'Ship entire parcel',exact:true})).toBeDisabled()
    await warehouse.reload();await warehouse.getByTestId('warehouse-fulfillments').getByRole('button',{name:shipped,exact:true}).click();await expect(d.getByText(tracking,{exact:true})).toBeVisible()
    assert.equal(await warehouse.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
    assert.equal((await worker.request.get(origin+'/api/admin/orders/'+orderID)).status(),403)
    return {sn,tracking,canceled,shipped}
   },
   async refund(orderID,request,result,index){return refundCheckoutOrder({page,origin,orderID,request,result,index})},
   async exchange(orderID,request,result,index,settlePayment){return exchangeCheckoutOrder({page,origin,orderID,request,result,index,pack,warehouse,parcel,retryLost,settlePayment})},
   async repair(orderID,request,result,index){return repairCheckoutOrder({page,origin,orderID,request,result,index})},
   async close(){await worker.close();await admin.close();await server.close()}
  }
 }catch(e){await worker?.close();await admin?.close();await server?.close();throw e}
}

export async function verifyCheckoutShipment({page,context,shop,orderID,result,expect}){
 await page.reload();await expect(page.getByTestId('shipment')).toHaveCount(1)
 await expect(page.getByTestId('shipment')).toContainText(result.tracking)
 await expect(page.getByTestId('shipment').locator('a')).toHaveAttribute('href','https://tracking.example.invalid/'+result.tracking)
 await expect(page.getByTestId('timeline-shipped')).toHaveCount(1)
 const devicesResponse=await context.request.get(shop+'/api/store/customer/orders/'+orderID+'/devices');assert.equal(devicesResponse.status(),200)
 const devices=await devicesResponse.json();assert.equal(devices.devices.length,1);assert.equal(devices.devices[0].serial_number,result.sn);assert.equal(devices.devices[0].status,'shipped');assert.equal(devices.devices[0].tracking_number,result.tracking)
 const section=page.getByTestId('after-sales-section')
 await section.getByRole('button').first().click();await section.getByRole('checkbox').first().check()
 await expect(section.getByText(new RegExp(result.sn))).toBeVisible();await section.getByRole('button').first().click()
 await expect(page.getByTestId('notification-badge')).toHaveText('1')
 await page.getByTestId('notification-bell').click()
 const notices=await context.request.get(shop+'/api/store/customer/notifications?offset=0');assert.equal(notices.status(),200)
 const body=await notices.json();assert.equal(body.count,1);assert.equal(body.notifications[0].kind,'shipment.created');assert.equal(body.notifications[0].resource_id,orderID)
 const dialog=page.getByRole('dialog'),zh=new URL(page.url()).pathname.includes('/dk-zh/')
 await dialog.getByRole('button',{name:zh?'标记已读':'Mark as read',exact:true}).click();await expect(page.getByTestId('notification-badge')).toHaveCount(0)
 await dialog.getByRole('button',{name:zh?'关闭消息通知':'Close notifications',exact:true}).click()
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
}
