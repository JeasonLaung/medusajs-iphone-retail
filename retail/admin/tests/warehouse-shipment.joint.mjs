import assert from 'node:assert/strict'
import {expect} from '@playwright/test'

export async function shipAsWarehouse({browser,admin,page,origin,parcelID,errors}){
 const parcel=await (await admin.request.get(origin+`/api/admin/orders/allocation-order/fulfillments/${parcelID}/operations`)).json()
 const directory=await (await admin.request.get(origin+'/api/admin/warehouse-users')).json(),location=directory.locations.find(l=>l.id===parcel.location_id)
 assert.ok(location)
 await page.goto(origin+'/warehouse-users');await page.getByRole('button',{name:'New warehouse user',exact:true}).click()
 const editor=page.getByRole('dialog')
 await editor.getByLabel('Name',{exact:true}).fill('Warehouse shipping operator')
 await editor.getByLabel('Email',{exact:true}).fill('warehouse-shipping@example.invalid')
 await editor.getByLabel('Password',{exact:true}).fill('artificial-warehouse-shipping-password')
 await editor.getByText(`${location.name} (${location.id})`,{exact:true}).click();await editor.locator('.ant-transfer-operation button').first().click()
 for(const name of ['View fulfillment records','Ship warehouse parcels']){await editor.getByRole('checkbox',{name,exact:true}).click();await expect(editor.getByRole('checkbox',{name,exact:true})).toBeChecked()}
 await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(editor).not.toBeVisible()
 const worker=await browser.newContext({viewport:{width:390,height:1000},reducedMotion:'reduce'})
 try{
  await worker.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','en'))
  const p=await worker.newPage();p.on('pageerror',e=>errors.push(e.message))
  await p.goto(origin+'/account');await p.getByLabel('Email',{exact:true}).fill('warehouse-shipping@example.invalid');await p.getByLabel('Password',{exact:true}).fill('artificial-warehouse-shipping-password');await p.getByRole('button',{name:'Sign in',exact:true}).click();await expect(p.getByRole('button',{name:'Sign out',exact:true})).toBeVisible()
  await p.goto(origin+'/warehouse-fulfillments');await p.getByTestId('warehouse-fulfillments').getByRole('button',{name:parcelID,exact:true}).click()
  const detail=p.locator('.ant-drawer-content'),shipping=detail.getByTestId('warehouse-shipment')
  await expect(detail.getByText('ALLOC-A',{exact:true})).toBeVisible();await expect(detail.getByText('ALLOC-B',{exact:true})).toBeVisible()
  await shipping.getByLabel('Tracking number',{exact:true}).fill('TRACK-JOINT-001')
  await shipping.getByLabel('Tracking URL',{exact:true}).fill('https://tracking.example.invalid/TRACK-JOINT-001')
  const requests=[]
  await p.route('**/shipment',async route=>{
   if(route.request().method()!=='POST'){await route.continue();return}
   requests.push(route.request().postDataJSON())
   const response=await route.fetch();assert.equal(response.status(),200)
   if(requests.length===1)await route.abort('failed');else await route.fulfill({response})
  })
  await shipping.getByRole('button',{name:'Ship entire parcel',exact:true}).click()
  await expect(shipping.getByRole('button',{name:'Retry this save',exact:true})).toBeVisible()
  await expect(shipping.getByLabel('Tracking number',{exact:true})).toBeDisabled()
  await shipping.getByRole('button',{name:'Retry this save',exact:true}).click()
  await expect(detail.getByText('TRACK-JOINT-001',{exact:true})).toBeVisible()
  assert.equal(requests.length,2);assert.deepEqual(requests[0],requests[1])
  assert.equal((await worker.request.get(origin+'/api/admin/orders')).status(),403)
  assert.equal(await p.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
  await p.reload();await p.getByTestId('warehouse-fulfillments').getByRole('button',{name:parcelID,exact:true}).click();await expect(detail.getByText('TRACK-JOINT-001',{exact:true})).toBeVisible()
  await expect(detail.getByRole('button',{name:'Ship entire parcel',exact:true})).toBeDisabled()
  await worker.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','zh-CN'));await p.setViewportSize({width:1440,height:1000});await p.reload()
  await p.getByTestId('warehouse-fulfillments').getByRole('button',{name:parcelID,exact:true}).click()
  await expect(detail.getByRole('button',{name:'整包发货',exact:true})).toBeDisabled()
  await expect(detail.getByLabel('物流单号',{exact:true})).toBeDisabled()
 }finally{await worker.close()}
 await page.goto(origin+'/orders/allocation-order')
}
