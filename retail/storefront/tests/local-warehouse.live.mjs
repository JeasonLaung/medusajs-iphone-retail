import assert from 'node:assert/strict'
import {expect as baseExpect} from '../../admin/node_modules/@playwright/test/index.mjs'
const expect=baseExpect.configure({timeout:15000})

export async function fulfillLocalWarehouse({browser,admin,origin,orderID,owner,sn,tracking,errors}){
 assert.equal(origin,'http://127.0.0.1:19200')
 const locationID='sloc_01M4FBP3NDPZHJKVV3VY682QTN'
 const pack=async()=>{
  await admin.getByRole('button',{name:'Create device parcel',exact:true}).click()
  const drawer=admin.getByRole('dialog')
  await drawer.getByRole('row').filter({hasText:sn}).getByRole('checkbox').check()
  await drawer.getByRole('button',{name:'Pack and deduct stock',exact:true}).click()
  await expect(drawer.getByText('Parcel created',{exact:true})).toBeVisible()
  const id=(await drawer.locator('.ant-alert-description').innerText()).trim();assert.match(id,/^ful_/)
  await drawer.getByRole('button',{name:'Cancel',exact:true}).click();return id
 }
 const canceled=await pack()
 const directory=await admin.request.get(origin+'/api/admin/warehouse-users');assert.equal(directory.status(),200)
 const locations=(await directory.json()).locations,location=locations.find(l=>l.id===locationID),other=locations.find(l=>l.id!==locationID)
 assert.ok(location);assert.ok(other)
 const email=`local-warehouse-${owner}@example.invalid`,password='isolated-warehouse-'+owner+'-password'
 await admin.goto(origin+'/warehouse-users');await admin.getByRole('button',{name:'New warehouse user',exact:true}).click()
 const editor=admin.getByRole('dialog')
 for(const [name,value] of Object.entries({Name:'Isolated warehouse operator',Email:email,Password:password}))await editor.getByLabel(name,{exact:true}).fill(value)
 await editor.getByText(`${location.name} (${location.id})`,{exact:true}).click()
 await editor.locator('.ant-transfer-operation button').first().click()
 for(const name of ['View fulfillment records','Ship warehouse parcels','Allocate warehouse parcels']){
  await editor.getByRole('checkbox',{name,exact:true}).click()
  await expect(editor.getByRole('checkbox',{name,exact:true})).toBeChecked()
 }
 await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(editor).not.toBeVisible()
 const worker=await browser.newContext({viewport:{width:390,height:1000},reducedMotion:'reduce'})
 try{
  await worker.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','en'))
  const page=await worker.newPage();page.on('pageerror',e=>errors.push(e.message))
  await page.goto(origin+'/account')
  await page.getByLabel('Email',{exact:true}).fill(email);await page.getByLabel('Password',{exact:true}).fill(password)
  await page.getByRole('button',{name:'Sign in',exact:true}).click();await expect(page.getByRole('button',{name:'Sign out',exact:true})).toBeVisible()
  for(const path of ['/api/admin/orders','/api/admin/orders/'+orderID,'/api/admin/warehouse-users'])assert.equal((await worker.request.get(origin+path)).status(),403)
  assert.equal((await worker.request.get(origin+'/api/admin/warehouse/locations/'+other.id+'/fulfillments?q=&offset=0')).status(),403)
  // The shared European location is the only authorized scope.
  const access=await worker.request.get(origin+'/api/admin/warehouse/access');assert.equal(access.status(),200)
  const scope=await access.json();assert.deepEqual(scope.locations.map(l=>l.id),[locationID])
  const parcel=async id=>{
   await page.goto(origin+'/warehouse-fulfillments')
   await page.getByTestId('warehouse-fulfillments').getByRole('button',{name:id,exact:true}).click()
   return page.getByRole('dialog')
  }
  const retryLost=async(pattern,root,submit,complete)=>{
   const requests=[]
   await page.route(pattern,async route=>{
    if(route.request().method()!=='POST'){await route.continue();return}
    requests.push(route.request().postDataJSON());const response=await route.fetch();assert.equal(response.status(),200)
    if(requests.length===1)await route.abort('failed');else await route.fulfill({response})
   })
   try{
    await submit();await expect(root.getByRole('button',{name:'Retry this save',exact:true})).toBeVisible()
    await root.getByRole('button',{name:'Retry this save',exact:true}).click();await complete()
    assert.equal(requests.length,2);assert.deepEqual(requests[0],requests[1])
   }finally{await page.unrouteAll({behavior:'wait'})}
  }
  let drawer=await parcel(canceled)
  const allocation=drawer.getByTestId('warehouse-allocation')
  await allocation.getByRole('row').filter({hasText:sn}).getByRole('checkbox').uncheck()
  await allocation.getByRole('row').filter({hasText:'CHECKOUT-DEVICE-5'}).getByRole('checkbox').check()
  await retryLost('**/allocation',allocation,()=>allocation.getByRole('button',{name:'Save',exact:true}).click(),()=>expect(allocation.getByRole('row').filter({hasText:'CHECKOUT-DEVICE-5'}).getByRole('checkbox')).toBeChecked())
  // Restore the original identity without deducting quantity stock again.
  await allocation.getByRole('row').filter({hasText:'CHECKOUT-DEVICE-5'}).getByRole('checkbox').uncheck()
  await allocation.getByRole('row').filter({hasText:sn}).getByRole('checkbox').check()
  const saved=page.waitForResponse(r=>r.request().method()==='POST'&&r.url().endsWith('/allocation'))
  await allocation.getByRole('button',{name:'Save',exact:true}).click();assert.equal((await saved).status(),200)
  // Refresh all parcel panels so release uses the current revision.
  drawer=await parcel(canceled)
  await drawer.getByTestId('warehouse-release').getByRole('button',{name:'Release device allocation',exact:true}).click()
  const modal=page.locator('.ant-modal-content')
  await retryLost('**/release/*',modal,()=>modal.getByRole('button',{name:'Save',exact:true}).click(),()=>expect(modal).not.toBeVisible())
  await expect(page.getByTestId('warehouse-fulfillments').getByRole('button',{name:canceled,exact:true})).toHaveCount(0)
  await admin.goto(origin+'/orders/'+orderID);await admin.getByRole('button',{name:'Allocate devices',exact:true}).click()
  const reserve=admin.getByRole('dialog');await reserve.getByRole('row').filter({hasText:sn}).getByRole('checkbox').check()
  const reserved=admin.waitForResponse(r=>r.request().method()==='POST'&&r.url().endsWith('/allocation'))
  await reserve.getByRole('button',{name:'Save device reservation',exact:true}).click();assert.equal((await reserved).status(),200)
  await expect(reserve.getByRole('row').filter({hasText:'isolated-repair-device'}).filter({hasText:'Reserved'})).toBeVisible()
  await reserve.getByRole('button',{name:'Cancel',exact:true}).click()
  const shipped=await pack();assert.notEqual(shipped,canceled)
  drawer=await parcel(shipped);const shipping=drawer.getByTestId('warehouse-shipment')
  await shipping.getByLabel('Tracking number',{exact:true}).fill(tracking)
  await shipping.getByLabel('Tracking URL',{exact:true}).fill('https://tracking.example.invalid/'+tracking)
  await retryLost('**/shipment',shipping,()=>shipping.getByRole('button',{name:'Ship entire parcel',exact:true}).click(),()=>expect(drawer.getByText(tracking,{exact:true})).toBeVisible())
  await expect(shipping.getByRole('button',{name:'Ship entire parcel',exact:true})).toBeDisabled()
  drawer=await parcel(shipped);await expect(drawer.getByText(tracking,{exact:true})).toBeVisible()
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
  return {sn,tracking,canceled,shipped,warehouse_account_created:true,warehouse_scope_enforced:true,warehouse_allocation_release_and_shipment:true,lost_reply_exact_command_recovery:true}
 }finally{await worker.close()}
}
