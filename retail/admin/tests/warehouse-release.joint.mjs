import assert from 'node:assert/strict'
import {expect} from '@playwright/test'

export async function releaseAsWarehouse({browser,admin,page,origin,parcelID,errors}){
 const parcel=await (await admin.request.get(origin+`/api/admin/orders/allocation-order/fulfillments/${parcelID}/operations`)).json()
 const directory=await (await admin.request.get(origin+'/api/admin/warehouse-users')).json(),location=directory.locations.find(l=>l.id===parcel.location_id)
 assert.ok(location)
 await page.goto(origin+'/warehouse-users');await page.getByRole('button',{name:'New warehouse user',exact:true}).click()
 const editor=page.getByRole('dialog')
 await editor.getByLabel('Name',{exact:true}).fill('Warehouse release operator')
 await editor.getByLabel('Email',{exact:true}).fill('warehouse-release@example.invalid')
 await editor.getByLabel('Password',{exact:true}).fill('artificial-warehouse-release-password')
 await editor.getByText(`${location.name} (${location.id})`,{exact:true}).click();await editor.locator('.ant-transfer-operation button').first().click()
 for(const name of ['View fulfillment records','Allocate warehouse parcels']){await editor.getByRole('checkbox',{name,exact:true}).click();await expect(editor.getByRole('checkbox',{name,exact:true})).toBeChecked()}
 await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(editor).not.toBeVisible()
 const worker=await browser.newContext({viewport:{width:390,height:1000},reducedMotion:'reduce'})
 try{
  await worker.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','en'))
  const p=await worker.newPage();p.on('pageerror',e=>errors.push(e.message))
  await p.goto(origin+'/account');await p.getByLabel('Email',{exact:true}).fill('warehouse-release@example.invalid');await p.getByLabel('Password',{exact:true}).fill('artificial-warehouse-release-password');await p.getByRole('button',{name:'Sign in',exact:true}).click();await expect(p.getByRole('button',{name:'Sign out',exact:true})).toBeVisible()
  await p.goto(origin+'/warehouse-fulfillments');await p.getByTestId('warehouse-fulfillments').getByRole('button',{name:parcelID,exact:true}).click()
  const release=p.getByTestId('warehouse-release'),requests=[]
  await expect(release.getByText('ALLOC-A',{exact:true})).toBeVisible()
  await p.route('**/release/*',async route=>{requests.push(route.request().postDataJSON());const response=await route.fetch();assert.equal(response.status(),200);if(requests.length===1)await route.abort('failed');else await route.fulfill({response})})
  await release.getByRole('button',{name:'Release device allocation',exact:true}).click()
  const modal=p.locator('.ant-modal-content')
  await expect(modal.getByText('This cancels the entire parcel and restores stock. The selected device becomes available; other devices remain reserved for the order.',{exact:true})).toBeVisible()
  assert.equal(requests.length,0)
  await modal.getByRole('button',{name:'Save',exact:true}).click()
  await expect(modal.getByRole('button',{name:'Retry this save',exact:true})).toBeVisible()
  await modal.getByRole('button',{name:'Retry this save',exact:true}).click()
  await expect(modal).not.toBeVisible()
  assert.equal(requests.length,2);assert.deepEqual(requests[0],requests[1])
  await expect(p.getByTestId('warehouse-fulfillments').getByRole('button',{name:parcelID,exact:true})).toHaveCount(0)
  assert.equal((await worker.request.get(origin+'/api/admin/orders')).status(),403)
  assert.equal(await p.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
 }finally{await worker.close()}
 await page.goto(origin+'/orders/allocation-order')
 await page.getByRole('button',{name:'Allocate devices',exact:true}).click()
 const drawer=page.getByRole('dialog')
 await drawer.getByRole('row').filter({has:page.getByText('ALLOC-A',{exact:true})}).getByRole('checkbox').check()
 await drawer.getByRole('button',{name:'Save device reservation',exact:true}).click()
  await expect(drawer.getByText('allocation-a',{exact:true}).first()).toBeVisible()
 await drawer.getByRole('button',{name:'Cancel',exact:true}).click()
}
