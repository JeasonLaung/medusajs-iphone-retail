import assert from 'node:assert/strict'
import {createRequire} from 'node:module'
import {dev} from 'astro'
import react from '@astrojs/react'

// Reuse the repository's pinned browser runner. This fixture owns its own Astro
// test root/random loopback listener and cannot shadow production src/pages.
const adminRequire=createRequire(new URL('../../admin/package.json',import.meta.url))
const {chromium,expect}=adminRequire('@playwright/test')
let server,browser
try {
 server=await dev({root:new URL('./selection-fixture/',import.meta.url),configFile:false,output:'server',integrations:[react()],devToolbar:{enabled:false},server:{host:'127.0.0.1',port:0},logLevel:'error'})
 const origin=`http://127.0.0.1:${server.address.port}`
 browser=await chromium.launch({headless:true,...(process.env.ADMIN_TEST_BROWSER?{executablePath:process.env.ADMIN_TEST_BROWSER}:{})})
 const context=await browser.newContext(),page=await context.newPage(),errors=[],documentRequests=[]
 page.on('pageerror',error=>errors.push(error.message))
 page.on('console',message=>{if(message.type()==='error')errors.push(message.text())})
 page.on('request',request=>{if(request.resourceType()==='document')documentRequests.push(request.url())})
 const variant=async(id)=>{await expect(page.getByTestId('actions-variant')).toHaveText(id);await expect(page.getByTestId('gallery-variant')).toHaveText(id)}
 const ready=async()=>{for(const kind of ['actions','gallery'])await expect(page.getByTestId(kind)).toHaveAttribute('data-ready','true')}
 const start='/dk-en/products/swappie-iphone-13?campaign=a&campaign=b&v_id=v128#specs'
 await page.goto(origin+start);await ready();await variant('v128')
 await expect(page.getByTestId('gallery-image')).toHaveText('/images/128.svg')
 await expect(page.getByTestId('price')).toHaveText('€9,007,199,254,740,993.01')
 const length=await page.evaluate(()=>history.length),initialDocuments=documentRequests.length
 await page.evaluate(()=>history.replaceState({fixture:'preserve-state'},'',location.href))
 await page.getByRole('button',{name:'Select 256',exact:true}).click()
 await variant('v256');assert.equal(new URL(page.url()).pathname,'/dk-en/products/iphone-13-256gb-blue-S85')
 assert.equal(new URL(page.url()).search,'?campaign=a&campaign=b');assert.equal(new URL(page.url()).hash,'#specs')
 await expect(page.getByTestId('gallery-image')).toHaveText('/images/256.svg')
 await expect(page.getByTestId('price')).toHaveText('€9,007,199,254,740,993.02')
 await expect(page).toHaveTitle('iPhone 13 256 | EuroRefurb')
 await expect(page.locator('link[rel="canonical"]')).toHaveAttribute('href','https://shop.example/dk-en/products/iphone-13-256gb-blue-S85')
 assert.equal(await page.evaluate(()=>history.length),length)
 assert.deepEqual(await page.evaluate(()=>history.state),{fixture:'preserve-state'})
 assert.equal(documentRequests.length,initialDocuments)
 const before=page.url()
 await page.getByRole('button',{name:'Select 1024',exact:true}).click();await variant('v256');assert.equal(page.url(),before)
 await page.getByRole('button',{name:'Select unknown',exact:true}).click();assert.equal(page.url(),before)
 // Native back/forward dispatch popstate; both independent islands must update.
 await page.evaluate(()=>{history.pushState({fixture:'entry-two'},'','/dk-en/products/iphone-13-128gb-blue-S85?campaign=c#images');dispatchEvent(new PopStateEvent('popstate'))})
 await variant('v128');await page.goBack();await variant('v256');await page.goForward();await variant('v128')
 assert.equal(documentRequests.length,initialDocuments)
 await page.reload();await ready();await variant('v128')
 // A no-slug selection remains selected in memory despite the old slug path.
 // Reload intentionally returns to the URL's higher-priority slug, as before.
 await page.getByRole('button',{name:'Select 512',exact:true}).click();await variant('v512')
 assert.equal(new URL(page.url()).pathname,'/dk-en/products/iphone-13-128gb-blue-S85')
 assert.equal(new URL(page.url()).searchParams.get('v_id'),'v512')
 await expect(page.getByTestId('gallery-image')).toHaveText('/images/512.svg')
 await page.reload();await ready();await variant('v128')
 // The no-slug rule keeps pathname and adds v_id. A matching pathname slug has
 // priority on reload; this path is therefore tested on a model URL as before.
 await page.goto(origin+'/dk-en/products/swappie-iphone-13?campaign=a#images');await ready();await variant('v128')
 await expect(page.getByTestId('gallery-image')).toHaveText('/images/model.svg')
 const modelDocuments=documentRequests.length
 await page.getByRole('button',{name:'Select 512',exact:true}).click();await variant('v512')
 assert.equal(new URL(page.url()).pathname,'/dk-en/products/swappie-iphone-13');assert.equal(new URL(page.url()).search,'?campaign=a&v_id=v512')
 await expect(page.getByTestId('price')).toHaveText('€0.00');await expect(page.getByTestId('gallery-image')).toHaveText('/images/512.svg')
 assert.equal(documentRequests.length,modelDocuments)
 await page.reload();await ready();await variant('v512')
 // Direct slug outranks v_id and survives a real page reload/hydration.
 await page.goto(origin+'/dk-en/products/iphone-13-256gb-blue-S85?v_id=v128');await ready();await variant('v256')
 await page.goto(origin+'/dk-en/products/swappie-iphone-13?v_id=v1024');await ready();await variant('v1024')
 await page.goto(origin+'/dk-en/products/swappie-iphone-13?v_id=unknown');await ready();await variant('v128')
 assert.deepEqual(errors,[])
 await context.close()
 console.log('ASTRO_SELECTION_BROWSER '+JSON.stringify({test_root_separate:true,two_hydrated_islands:true,exact_currency_in_browser:true,selection_replace_state_no_document_request:true,history_state_and_length_preserved:true,query_and_hash_preserved:true,title_and_canonical_synchronized:true,gallery_synchronized:true,native_back_forward:true,reload_and_deep_link:true,no_slug_v_id:true,unavailable_candidate_unchanged:true,unknown_v_id_default:true,hydration_errors:0}))
} finally {
 if(browser)await browser.close()
 if(server)await server.stop()
}
