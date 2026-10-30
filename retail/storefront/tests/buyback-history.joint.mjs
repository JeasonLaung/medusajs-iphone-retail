import assert from 'node:assert/strict'
import {createRequire} from 'node:module'
import {mkdir} from 'node:fs/promises'
import {realpathSync} from 'node:fs'
import {fileURLToPath} from 'node:url'
import {dev} from 'astro'
const require=createRequire(new URL('../../admin/package.json',import.meta.url)),{chromium,expect:baseExpect}=require('@playwright/test'),expect=baseExpect.configure({timeout:30000})
const origin='http://127.0.0.1:19383',target=process.env.ADMIN_TEST_API_TARGET
assert.match(target??'',/^http:\/\/127\.0\.0\.1:[0-9]+$/)
assert.match(process.env.CUSTOMER_BRIDGE_KEY??'',/^[0-9a-f]{64}$/)
process.env.GIN_API_ORIGIN=target
process.env.STOREFRONT_ORIGIN=origin
const returning=process.env.BUYBACK_INSPECTION_DECISION==='decline'
const output=new URL('../../../logs/gin-migration/astro-buyback-'+(returning?'return':'settlement')+'/',import.meta.url),errors=[]
let server,browser
try{
 // Load actual production routes, layout, account and signed bridge; no API interception.
 server=await dev({root:new URL('../',import.meta.url),server:{host:'127.0.0.1',port:19383},integrations:[{name:'joint-admin-route',hooks:{'astro:config:setup':({injectRoute})=>injectRoute({pattern:'/buyback',entrypoint:fileURLToPath(new URL('./buyback-admin.astro',import.meta.url))})}}],vite:{resolve:{dedupe:['react','react-dom']},server:{strictPort:true,fs:{allow:[fileURLToPath(new URL('../../',import.meta.url)),realpathSync(fileURLToPath(new URL('../node_modules',import.meta.url))),realpathSync(fileURLToPath(new URL('../../admin/node_modules',import.meta.url)))]},proxy:{'/api/admin':{target}}}},logLevel:'error'})
 assert.equal(server.address.port,19383)
 browser=await chromium.launch({headless:true,executablePath:process.env.ADMIN_TEST_BROWSER||'C:/Program Files/Google/Chrome/Application/chrome.exe'})
 const context=await browser.newContext({viewport:{width:1440,height:1000},reducedMotion:'reduce'}),page=await context.newPage()
 page.on('pageerror',e=>errors.push(e.message))
 await page.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','en'))
 await page.goto(origin+'/buyback');await page.getByLabel('Email',{exact:true}).fill('browser@example.invalid');await page.getByLabel('Password',{exact:true}).fill('artificial-browser-password');await page.getByRole('button',{name:'Sign in',exact:true}).click();await expect(page.getByRole('button',{name:'Sign out',exact:true})).toBeVisible()
 await page.getByRole('menuitem',{name:'Selling requests',exact:true}).click()
 await page.getByLabel('Search selling requests',{exact:true}).fill('historical-browser-request')
 await page.getByLabel('Search selling requests',{exact:true}).press('Enter')
 await page.getByRole('button',{name:'historical-browser-request',exact:true}).click()
 const drawer=page.locator('.ant-drawer-content')
 await expect(drawer.getByText('Historical selling record, available for reference. Legacy customer accounts were not migrated.',{exact:true})).toBeVisible()
 await expect(drawer.getByText('HISTORICAL-PAY',{exact:true})).toBeVisible()
 await expect(drawer.getByRole('button',{name:/Record completed payment|Confirm return delivered|Send final offer|Link inspection device/})).toHaveCount(0)
 await expect(drawer.locator('img.ant-image-img')).toHaveCount(1)
 await expect.poll(()=>drawer.locator('img.ant-image-img').evaluate(img=>img.complete&&img.naturalWidth>0)).toBe(true)
 await drawer.getByRole('button',{name:'Refresh request',exact:true}).click()
 await expect(drawer.getByText('HISTORICAL-PAY',{exact:true})).toBeVisible()
 await page.setViewportSize({width:390,height:900})
 await expect(drawer.getByText('Historical selling record, available for reference. Legacy customer accounts were not migrated.',{exact:true})).toBeVisible()
 await page.setViewportSize({width:1440,height:1000})
 await page.getByRole('button',{name:'Close',exact:true}).click()
 await page.getByLabel('Search selling requests',{exact:true}).fill('historical-claim-request');await page.getByLabel('Search selling requests',{exact:true}).press('Enter')
 await page.getByRole('button',{name:'historical-claim-request',exact:true}).click()
 await drawer.getByRole('button',{name:'Verify and link new customer',exact:true}).click()
 const modal=page.locator('.ant-modal:visible')
 await modal.getByLabel('Verified new customer account ID',{exact:true}).fill('cust_buyback-browser')
 await modal.getByLabel('Verification evidence',{exact:true}).fill('Artificial identity checked with original application evidence')
 await modal.getByRole('button',{name:'Confirm verified ownership',exact:true}).click()
 await expect(page.locator('.ant-modal:visible')).toHaveCount(0)
 await expect(drawer.getByRole('button',{name:'Verify and link new customer',exact:true})).toHaveCount(0)
 const customer=await context.newPage();await customer.setViewportSize({width:390,height:900});await customer.goto(origin+'/dk-en/account');await customer.getByTestId('email-input').fill('buyback-browser@example.invalid');await customer.getByTestId('password-input').fill('artificial-buyback-password');await customer.getByTestId('sign-in-button').click();await expect(customer.getByTestId('mobile-account-nav').getByTestId('logout-button')).toBeVisible()
 const response=await customer.evaluate(async()=>{const r=await fetch('/api/store/customer/buyback/requests/historical-claim-request');return {status:r.status,body:await r.json()}})
 assert.equal(response.status,200);assert.equal(response.body.request.status,'awaiting_photos')
 await customer.goto(origin+'/dk-en/account/buyback/historical-claim-request')
 await expect(customer.getByTestId('buyback-request-details')).toBeVisible()
 await customer.getByRole('button',{name:'Cancel request',exact:true}).click();await customer.getByRole('dialog').getByRole('button',{name:'Confirm',exact:true}).click()
 await customer.reload()
 const closed=await customer.evaluate(async()=>{const r=await fetch('/api/store/customer/buyback/requests/historical-claim-request');return r.json()})
 assert.equal(closed.request.status,'canceled');assert.ok(closed.request.closed_at)
 assert.deepEqual(errors,[])
 console.log('BUYBACK_HISTORY_BROWSER '+JSON.stringify({real_admin:true,history_list_search:true,history_detail:true,private_photo_decoded:true,refresh_persisted:true,commands_hidden:true,human_claim:true,claimed_customer_read:true,claimed_customer_action_persisted:true,mobile:true,browser_errors:0}))
}catch(e){console.error('HISTORY_BROWSER_FAILED');throw e}finally{await browser?.close();await server?.stop()}
