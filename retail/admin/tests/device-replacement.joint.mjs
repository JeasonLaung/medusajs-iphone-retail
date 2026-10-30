import assert from 'node:assert/strict'
import {createServer} from 'vite'
import {chromium,expect} from '@playwright/test'
let server,browser
const origin='http://127.0.0.1:19383',errors=[]
if(!process.env.ADMIN_TEST_API_TARGET)throw Error('Owned API target required')
try{
 server=await createServer({server:{port:19383,strictPort:true,host:'127.0.0.1'},logLevel:'error'});await server.listen()
 browser=await chromium.launch({headless:true,executablePath:process.env.ADMIN_TEST_BROWSER||'C:/Program Files/Google/Chrome/Application/chrome.exe'})
 const context=await browser.newContext({reducedMotion:'reduce'});await context.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','en'))
 const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message));await page.goto(origin+'/account');await page.getByLabel('Email',{exact:true}).fill('browser@example.invalid');await page.getByLabel('Password',{exact:true}).fill('artificial-browser-password');await page.getByRole('button',{name:'Sign in',exact:true}).click();await expect(page.getByRole('button',{name:'Sign out',exact:true})).toBeVisible()
 await page.goto(origin+'/orders/allocation-order');await page.getByRole('button',{name:'Allocate devices',exact:true}).click();let drawer=page.getByRole('dialog')
 await expect(drawer.getByText('ALLOC-A',{exact:true})).toBeVisible();await expect(drawer.getByText('ALLOC-FOREIGN',{exact:true})).toHaveCount(0)
 for(const sn of ['ALLOC-A','ALLOC-B'])await drawer.getByRole('row').filter({has:page.getByText(sn,{exact:true})}).getByRole('checkbox').check()
 await drawer.getByRole('button',{name:'Save device reservation',exact:true}).click();await expect(drawer.getByText('allocation-a',{exact:true})).toBeVisible();await drawer.getByRole('button',{name:'Cancel',exact:true}).click()
 async function pack(sns){await page.getByRole('button',{name:'Create device parcel',exact:true}).click();const d=page.getByRole('dialog');for(const sn of sns)await d.getByRole('row').filter({has:page.getByText(sn,{exact:true})}).getByRole('checkbox').check();await d.getByRole('button',{name:'Pack and deduct stock',exact:true}).click();await expect(d.getByText('Parcel created',{exact:true})).toBeVisible();const id=(await d.locator('.ant-alert-description').innerText()).trim();assert.match(id,/^ful_/);await d.getByRole('button',{name:'Cancel',exact:true}).click();return id}
 async function openParcel(id){await page.locator('[data-fulfillment-id="'+id+'"]').getByRole('button',{name:'Parcel operations',exact:true}).click();return page.getByRole('dialog')}
 const first=await pack(['ALLOC-A']);drawer=await openParcel(first);await drawer.getByRole('radio',{name:'Cancel parcel',exact:true}).check();await drawer.getByRole('button',{name:'Cancel parcel',exact:true}).click();await page.getByRole('button',{name:'Confirm parcel cancellation',exact:true}).click();await expect(drawer.getByText('Parcel operation saved',{exact:true})).toBeVisible();await expect(drawer.getByRole('button',{name:'Cancel parcel',exact:true})).toBeDisabled();await drawer.getByRole('button',{name:'Cancel',exact:true}).click()
 const second=await pack(['ALLOC-A','ALLOC-B']);drawer=await openParcel(second);await drawer.getByLabel('Tracking number',{exact:true}).fill('TRACK-JOINT-001');await drawer.getByLabel('Tracking URL',{exact:true}).fill('https://tracking.example.invalid/TRACK-JOINT-001');await drawer.getByRole('button',{name:'Ship entire parcel',exact:true}).click();await expect(drawer.getByText('Parcel operation saved',{exact:true})).toBeVisible();await expect(drawer.getByRole('button',{name:'Ship entire parcel',exact:true})).toBeDisabled();await drawer.getByRole('button',{name:'Cancel',exact:true}).click()
 await page.reload();await expect(page.getByText('TRACK-JOINT-001',{exact:true})).toBeVisible();const tracking=page.getByRole('link',{name:'View tracking',exact:true});await expect(tracking).toHaveAttribute('href','https://tracking.example.invalid/TRACK-JOINT-001')
 drawer=await openParcel(second);await expect(drawer.getByText('ALLOC-A',{exact:true})).toBeVisible();await expect(drawer.getByText('ALLOC-B',{exact:true})).toBeVisible();await expect(drawer.getByRole('button',{name:'Ship entire parcel',exact:true})).toBeDisabled()
 await page.setViewportSize({width:390,height:844});assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
 await context.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','zh-CN'));await page.reload();await page.locator('[data-fulfillment-id="'+second+'"]').getByRole('button',{name:'包裹操作',exact:true}).click();await expect(page.getByRole('dialog').getByRole('button',{name:'整包发货',exact:true})).toBeDisabled();assert.deepEqual(errors,[])

 await context.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','en'));await page.goto(origin+'/orders/allocation-order')
 const panel=page.getByTestId('order-after-sales');await expect(panel.getByText('Pending',{exact:true})).toBeVisible()
 await panel.getByRole('button',{name:'Save',exact:true}).click();await page.getByRole('button',{name:'Confirm update',exact:true}).click();await expect(panel.locator('.ant-tag')).toHaveText('Processing')
 await panel.getByRole('button',{name:'Receive device',exact:true}).click();drawer=page.getByRole('dialog')
 await drawer.getByLabel('Originally shipped device',{exact:true}).focus();await drawer.getByLabel('Originally shipped device',{exact:true}).press('ArrowDown');await drawer.getByLabel('Originally shipped device',{exact:true}).press('Enter')
 await drawer.getByLabel('Receiving source / warehouse',{exact:true}).focus();await drawer.getByLabel('Receiving source / warehouse',{exact:true}).press('ArrowDown');await drawer.getByLabel('Receiving source / warehouse',{exact:true}).press('Enter')
 const scan=drawer.getByLabel('Scan serial number / IMEI / EID',{exact:true});await scan.fill('WRONG-MACHINE');await drawer.getByRole('button',{name:'Confirm device receipt',exact:true}).click();await expect(drawer.getByRole('button',{name:'Retry this receipt',exact:true})).toBeVisible()
 await drawer.getByRole('button',{name:'Reload',exact:true}).click();await expect(scan).toBeEnabled()
 await drawer.getByLabel('Originally shipped device',{exact:true}).focus();await drawer.getByLabel('Originally shipped device',{exact:true}).press('ArrowDown');await drawer.getByLabel('Originally shipped device',{exact:true}).press('Enter')
 await drawer.getByLabel('Receiving source / warehouse',{exact:true}).focus();await drawer.getByLabel('Receiving source / warehouse',{exact:true}).press('ArrowDown');await drawer.getByLabel('Receiving source / warehouse',{exact:true}).press('Enter');await scan.fill('ALLOC-A')
 const bodies=[];let lost=false
 await page.route('**/api/admin/orders/allocation-order/after-sales/asr-browser-return/device-returns',async route=>{if(route.request().method()!=='POST')return route.continue();bodies.push(route.request().postDataJSON());if(!lost){lost=true;const response=await route.fetch();assert.equal(response.status(),200);return route.abort('failed')};return route.continue()})
 await drawer.getByRole('button',{name:'Confirm device receipt',exact:true}).click();await expect(drawer.getByRole('button',{name:'Retry this receipt',exact:true})).toBeVisible();await expect(scan).toBeDisabled()
 await drawer.getByRole('button',{name:'Retry this receipt',exact:true}).click();await expect(drawer.getByText('Device received',{exact:true})).toBeVisible();assert.equal(bodies.length,2);assert.deepEqual(bodies[0],bodies[1])
 await page.reload();await panel.getByRole('button',{name:'Receive device',exact:true}).click();drawer=page.getByRole('dialog');await expect(drawer.getByRole('cell',{name:'returned',exact:true})).toBeVisible();await expect(drawer.getByRole('cell',{name:'inspection',exact:true})).toBeVisible();await expect(drawer.getByRole('button',{name:'Confirm device receipt',exact:true})).toBeDisabled()
 await page.setViewportSize({width:390,height:844});await expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)

 await drawer.getByRole('button',{name:'Service device',exact:true}).click();let service=page.getByRole('dialog').last()
 await expect(service.getByText('inspection',{exact:true})).toBeVisible()
 // Persist the replacement through production Gin, then lose its successful
 // response to verify that the real browser retries the immutable operation.
 await service.getByRole('button',{name:'Assign replacement device',exact:true}).click();let replacement=page.getByRole('dialog').last()
 await replacement.getByRole('combobox',{name:'Replacement device',exact:true}).click();await page.locator('.ant-select-item-option').filter({hasText:'ALLOC-C'}).click()
 const replacementBodies=[];let replacementLost=false
 await page.route('**/api/admin/orders/allocation-order/after-sales/asr-browser-return/devices/*/replacement',async route=>{if(route.request().method()!=='POST')return route.continue();replacementBodies.push(route.request().postDataJSON());if(!replacementLost){replacementLost=true;const response=await route.fetch();assert.equal(response.status(),200);return route.abort('failed')};return route.continue()})
 await replacement.getByRole('button',{name:'Confirm replacement',exact:true}).click();await expect(replacement.getByRole('button',{name:'Retry this replacement',exact:true})).toBeVisible();await expect(replacement.getByRole('combobox',{name:'Replacement device',exact:true})).toBeDisabled()
 await replacement.getByRole('button',{name:'Retry this replacement',exact:true}).click();await expect(replacement.getByText('Replacement device assigned',{exact:true})).toBeVisible();assert.equal(replacementBodies.length,2);assert.deepEqual(replacementBodies[0],replacementBodies[1])
 await expect(replacement.getByText('Pending order difference',{exact:true}).locator('..')).toContainText('0')
 await page.reload()
 const third=await pack(['ALLOC-C']);drawer=await openParcel(third);await drawer.getByLabel('Tracking number',{exact:true}).fill('EXCHANGE-JOINT-001');await drawer.getByLabel('Tracking URL',{exact:true}).fill('https://tracking.example.invalid/EXCHANGE-JOINT-001');await drawer.getByRole('button',{name:'Ship entire parcel',exact:true}).click();await expect(drawer.getByText('Parcel operation saved',{exact:true})).toBeVisible()
 await page.reload();await expect(page.getByText('EXCHANGE-JOINT-001',{exact:true})).toBeVisible()
 // Receiving into quarantine does not restock the original. Inspect it separately
 // before completing the after-sales request, as in the existing business flow.
 await panel.getByRole('button',{name:'Receive device',exact:true}).click();drawer=page.getByRole('dialog');await drawer.getByRole('button',{name:'Service device',exact:true}).click();service=page.getByRole('dialog').last()
 // Use the existing product/SKU picker, preserving the original SKU for a repaired machine.
 await service.getByRole('button',{name:'Select inspected SKU',exact:true}).click();const picker=page.getByRole('dialog').last()
 await picker.getByLabel('Search products',{exact:true}).fill('Device fixture');await picker.getByLabel('Search products',{exact:true}).press('Enter')
 await picker.getByRole('combobox',{name:'Product',exact:true}).click();await page.locator('.ant-select-item-option').filter({has:page.locator('.ant-select-item-option-content').getByText('Device fixture',{exact:true})}).click()
 await picker.getByRole('combobox',{name:'SKU',exact:true}).click();await page.locator('.ant-select-item-option[title="ALLOCATION-SKU Device variant"]').click()
 await picker.getByRole('button',{name:'Use this SKU',exact:true}).click()
 await service.getByLabel('Actual battery health',{exact:true}).fill('90');await service.getByLabel('Inspection notes',{exact:true}).fill('Original machine repaired and inspected with photo evidence')
 await service.locator('input[type="file"]').first().setInputFiles({name:'inspection.png',mimeType:'image/png',buffer:Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jB9kAAAAASUVORK5CYII=','base64')})
 await expect(service.locator('.ant-upload-list-item-done')).toHaveCount(1)
 await service.getByRole('button',{name:'Save inspection',exact:true}).click();await expect(service.getByText('Device operation saved',{exact:true})).toBeVisible();await expect(service.getByText('available',{exact:true})).toBeVisible()
 await page.reload();await expect(panel.locator('.ant-tag')).toHaveText('Processing');await panel.locator('.ant-select-selector').click();await page.locator('.ant-select-item-option[title="Completed"]').click();await panel.getByRole('button',{name:'Save',exact:true}).click();await page.getByRole('button',{name:'Confirm update',exact:true}).click();await expect(panel.locator('.ant-tag')).toHaveText('Completed')
 await page.setViewportSize({width:390,height:844});await expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true)
 assert.deepEqual(errors,[])
 console.log('DEVICE_REPLACEMENT_JOINT '+JSON.stringify({real_browser_react_gin_mysql_redis:true,api_mocking:false,preallocate_pack_cancel_repack_ship:true,review_and_scan_return:true,native_exchange_new_item:true,zero_difference_without_unpaid_collection:true,replacement_pack_and_ship:true,original_inspection_with_photo:true,lost_successful_replies:true,immutable_replacement_retry:true,complete_after_shipment_and_inspection:true,mobile_no_page_overflow:true,browser_errors:0}))
}finally{await browser?.close();await server?.close()}
