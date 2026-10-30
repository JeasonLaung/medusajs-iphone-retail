import assert from 'node:assert/strict'
import {expect} from '@playwright/test'

// Called after production Admin has reserved, packed, cancelled, repacked and
// shipped actual devices. No API response interception or direct SQL mutation.
export async function checkWarehouseFulfillments({browser,admin,page,origin,parcelID,canceledID,errors}){
 const response=await admin.request.get(origin+`/api/admin/orders/allocation-order/fulfillments/${parcelID}/operations`)
 assert.equal(response.status(),200)
 const parcel=await response.json(),location=parcel.location_id
 const directory=await (await admin.request.get(origin+'/api/admin/warehouse-users')).json()
 const bound=directory.locations.find(l=>l.id===location),other=directory.locations.find(l=>l.id!==location)
 assert.ok(bound&&other)
 await page.goto(origin+'/warehouse-users')
 await page.getByRole('button',{name:'New warehouse user',exact:true}).click()
 const editor=page.getByRole('dialog')
 await editor.getByLabel('Name',{exact:true}).fill('Parcel warehouse operator')
 await editor.getByLabel('Email',{exact:true}).fill('parcel-warehouse@example.invalid')
 await editor.getByLabel('Password',{exact:true}).fill('artificial-parcel-warehouse-password')
 await editor.getByText(`${bound.name} (${bound.id})`,{exact:true}).click()
 await editor.locator('.ant-transfer-operation button').first().click()
 for(const name of ['Allocate warehouse parcels','Ship warehouse parcels'])await expect(editor.getByRole('checkbox',{name,exact:true})).not.toBeChecked()
 await editor.getByRole('checkbox',{name:'View fulfillment records',exact:true}).click()
 await expect(editor.getByRole('checkbox',{name:'View fulfillment records',exact:true})).toBeChecked()
 await editor.getByRole('button',{name:'Save',exact:true}).click()
 await expect(editor).not.toBeVisible()
 for(const language of ['en','zh-CN'])for(const width of [390,1440]){
  const worker=await browser.newContext({viewport:{width,height:1000},reducedMotion:'reduce'})
  try{
   await worker.addInitScript(language=>localStorage.setItem('eurorefurb.admin.language',language),language)
   const p=await worker.newPage();p.on('pageerror',e=>errors.push(e.message))
   const zh=language==='zh-CN'
   await p.goto(origin+'/account')
   await p.getByLabel(zh?'邮箱':'Email',{exact:true}).fill('parcel-warehouse@example.invalid')
   await p.getByLabel(zh?'密码':'Password',{exact:true}).fill('artificial-parcel-warehouse-password')
   await p.getByRole('button',{name:zh?'登录':'Sign in',exact:true}).click()
   await expect(p.getByRole('button',{name:zh?'退出登录':'Sign out',exact:true})).toBeVisible()
   await p.goto(origin+'/warehouse-fulfillments')
   const panel=p.getByTestId('warehouse-fulfillments')
   await expect(panel.getByRole('radio',{name:bound.name,exact:true})).toBeVisible()
   await expect(panel.getByRole('radio',{name:other.name,exact:true})).toHaveCount(0)
   await expect(panel.getByRole('button',{name:canceledID,exact:true})).toHaveCount(0)
   await panel.getByRole('button',{name:parcelID,exact:true}).click()
   const detail=p.locator('.ant-drawer-content')
   await expect(detail.getByTestId('warehouse-shipment')).toHaveCount(0)
   for(const text of ['ALLOC-A','ALLOC-B','TRACK-JOINT-001'])await expect(detail.getByText(text,{exact:true})).toBeVisible()
   assert.equal(await p.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
   const scoped=`/api/admin/warehouse/locations/${location}/orders/allocation-order/fulfillments/`
   assert.equal((await worker.request.get(origin+scoped+canceledID)).status(),404)
   assert.equal((await worker.request.get(origin+`/api/admin/warehouse/locations/${other.id}/fulfillments`)).status(),403)
   assert.equal((await worker.request.get(origin+'/api/admin/orders')).status(),403)
   const raw=await worker.request.get(origin+scoped+parcelID)
   assert.equal(raw.status(),200)
   for(const key of ['cost_cents','payment_collections','raw_quantity','imei','inventory_item_id'])assert.ok(!(await raw.text()).includes(key))
   if(language==='zh-CN'&&width===1440){
    await page.getByRole('button',{name:'Parcel warehouse operator',exact:true}).click()
    await editor.getByRole('checkbox',{name:'View fulfillment records',exact:true}).click()
    await expect(editor.getByRole('checkbox',{name:'View fulfillment records',exact:true})).not.toBeChecked()
    await editor.getByRole('button',{name:'Save',exact:true}).click();await expect(editor).not.toBeVisible()
    // Saving account grants revokes the old login session as well as access.
    assert.equal((await worker.request.get(origin+scoped+parcelID)).status(),401)
    await p.reload();await expect(p.getByLabel('密码',{exact:true})).toBeVisible()
    await p.getByLabel('邮箱',{exact:true}).fill('parcel-warehouse@example.invalid')
    await p.getByLabel('密码',{exact:true}).fill('artificial-parcel-warehouse-password')
    await p.getByRole('button',{name:'登录',exact:true}).click()
    await expect(p.getByRole('button',{name:'退出登录',exact:true})).toBeVisible()
    assert.equal((await worker.request.get(origin+scoped+parcelID)).status(),403)
    await p.goto(origin+'/warehouse-fulfillments');await expect(p.getByText('没有访问此功能的权限',{exact:true})).toBeVisible()
   }
  }finally{await worker.close()}
 }
 return {platform_packing_to_new_warehouse_user:true,real_scoped_parcel_and_tracking:true,canceled_hidden:true,cross_warehouse_and_full_store_denied:true,read_grant_revoked_via_admin:true,scoped_private_projection:true,warehouse_languages:['en','zh-CN'],warehouse_widths:[390,1440]}
}
