import assert from 'node:assert/strict'
import {createRequire} from 'node:module'
import {mkdir,writeFile} from 'node:fs/promises'
import {fileURLToPath} from 'node:url'
import {dev} from 'astro'
import react from '@astrojs/react'
import {legacyBuyback} from './legacy-buyback.mjs'
import {requestFixture} from './buyback-fixture-data.ts'
import {getBuybackCopy} from '../src/components/sell/buyback-copy.ts'
const require=createRequire(new URL('../../admin/package.json',import.meta.url)),{chromium,expect:baseExpect}=require('@playwright/test'),expect=baseExpect.configure({timeout:30000})
const output=new URL('../../../logs/gin-migration/astro-buyback-requests/',import.meta.url),cases=[],errors=[]
let server,browser
try{
 server=await dev({root:new URL('./buyback-fixture/',import.meta.url),configFile:false,output:'server',integrations:[react()],devToolbar:{enabled:false},server:{host:'127.0.0.1',port:0},vite:{plugins:[legacyBuyback()],resolve:{dedupe:['react','react-dom']}},logLevel:'error'})
 browser=await chromium.launch({headless:true,executablePath:process.env.ADMIN_TEST_BROWSER||'C:/Program Files/Google/Chrome/Application/chrome.exe'})
 const origin=`http://127.0.0.1:${server.address.port}`
 await mkdir(output,{recursive:true})
 async function fixture({locale='en',width=390,state='awaiting_photos',list=false,logged=true,compare=''}={}){
  const context=await browser.newContext({viewport:{width,height:900},reducedMotion:'reduce'})
  await context.addCookies([{name:'retail_customer_csrf',value:'b'.repeat(64),url:origin}])
  const page=await context.newPage();page.on('pageerror',e=>{errors.push(e.message);console.error('PAGE_ERROR',e.message)})
  await page.route('https://**/*',r=>r.abort())
  const current=requestFixture(),calls={uploads:[],commands:[],offsets:[],reads:0};current.status=state
  if(state==='ready_to_ship')current.shipping_instructions='Wrap the phone carefully and send to our warehouse.'
  if(['final_offer','accepted','paid'].includes(state))current.final_amount=125
  const photoBytes=new Map()
  let fail=false,stale=false
  await page.route('**/api/store/customer/me',r=>r.fulfill({status:logged?200:401,json:logged?{customer:{id:'cust_fixture',email:current.email,first_name:'Test',last_name:'Customer',phone:current.phone,verified_at:current.created_at}}:{}}))
  await page.route('**/api/store/customer/buyback/requests**',async route=>{
   const request=route.request(),url=new URL(request.url()),tail=url.pathname.split('/requests')[1]
   if(request.method()==='POST'){
    assert.equal(request.headers()['x-csrf-token'],'b'.repeat(64))
    const body=request.postDataJSON()
    if(tail.endsWith('/photos')){
     calls.uploads.push(body);assert.equal(body.mime_type,'image/webp');assert.equal(body.item_id,'bbi_fixture');assert.ok(body.content.length<4194304)
     const p={id:'bbp_'+calls.uploads.length,request_id:current.id,item_id:body.item_id,photo_type:body.photo_type,uploaded_at:current.created_at,review_status:'pending',review_note:'',mime_type:'image/webp'}
     photoBytes.set(p.id,{content:body.content,mime_type:body.mime_type})
     current.photos=current.photos.filter(v=>v.photo_type!==p.photo_type);current.photos.push(p)
     return route.fulfill({status:201,json:{photo:p}})
    }
    calls.commands.push(body)
    if(stale)return route.fulfill({status:409,json:{error:{code:'BUYBACK_OFFER_CHANGED'}}})
    current.status=({submit_photos:'photo_review',mark_shipped:'in_transit',accept_offer:'accepted',decline_offer:'return_requested',cancel:'canceled'})[body.action]
    if(body.action==='submit_photos')for(const item of current.items){const input=body.items.find(v=>v.item_id===item.id);item.serial_number=input.serial_number;item.imei=input.imei}
    if(body.action==='mark_shipped')current.tracking_number=body.tracking_number
    if(body.action==='cancel')current.closed_at=current.updated_at
    current.history.push({at:current.updated_at,action:body.action,actor:'customer',status:current.status,note:''})
    return route.fulfill({json:{request:current}})
   }
   if(tail.includes('/photos/')){calls.reads++;const photo=photoBytes.get(tail.split('/').at(-1));assert.ok(photo);return route.fulfill({json:photo})}
   if(fail)return route.fulfill({status:503,json:{error:{code:'unavailable'}}})
   if(tail===''){
    const offset=Number(url.searchParams.get('offset'));calls.offsets.push(offset)
    const rows=Array.from({length:23},(_,i)=>({...structuredClone(current),id:current.id.slice(0,-2)+String(i).padStart(2,'0')}))
    return route.fulfill({json:{requests:rows.slice(offset,offset+20),count:23,offset,limit:20,summary:{recycled:3,processing:23,received:[{currency_code:'EUR',amount:375}]}}})
   }
   return route.fulfill({json:{request:current}})
  })
  const segment='dk-'+locale
  await page.goto(`${origin}/${segment}/account/buyback${list?'':'/'+current.id}${compare?'?compare='+compare:''}`)
  return {page,context,current,calls,copy:getBuybackCopy(locale),setFail:v=>fail=v,setStale:v=>stale=v}
 }
 // Original list pagination, total counters, page retry and selected account navigation.
 {
  const {page,context,copy,calls,setFail}=await fixture({list:true,width:1440})
  await expect(page.getByRole('heading',{name:copy.title,exact:true})).toBeVisible();await expect(page.locator('article')).toHaveCount(20)
  await page.locator('.ant-pagination-item-2').click();await expect(page.locator('article')).toHaveCount(3);assert.deepEqual(calls.offsets,[0,20])
  await expect(page.getByText('€375.00',{exact:true})).toBeVisible()
  setFail(true);await page.getByRole('button',{name:copy.refresh,exact:true}).click();await expect(page.locator('.ant-alert-error')).toBeVisible()
  setFail(false);await page.getByRole('button',{name:copy.retry,exact:true}).click();await expect(page.locator('article')).toHaveCount(3)
  await expect(page.locator('[data-testid=buyback-link]').first()).toHaveAttribute('href','/dk-en/account/buyback')
  cases.push('23-request original list, pagination, global totals, retry and account nav');await context.close()
 }
 // Exercise browser compression of a real image through the unchanged Upload control.
 {
  const {page,context,current,copy,calls}=await fixture()
  const details=page.getByTestId('buyback-request-details');await expect(details).toBeVisible()
  await page.getByRole('button',{name:copy.submitPhotos,exact:true}).click();await expect(page.getByText(copy.serialRequired,{exact:true})).toBeVisible()
  const serial=page.locator(`[id="${current.id}-bbi_fixture-serial_number"] input`),imei=page.locator(`[id="${current.id}-bbi_fixture-imei"] input`)
  await serial.fill('sn-test');await imei.fill('000000000000001')
  await page.getByRole('button',{name:copy.submitPhotos,exact:true}).click();await expect(page.getByText(copy.photoRequired,{exact:true})).toBeVisible()
  const image=Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jffkAAAAASUVORK5CYII=','base64')
  for(const [i,type] of ['front','back','edges'].entries()){
   await page.locator(`[id="${current.id}-bbi_fixture-${type}"] input[type=file]`).setInputFiles({name:'phone.png',mimeType:'image/png',buffer:image})
   await expect.poll(()=>calls.uploads.length).toBe(i+1)
   await expect(page.locator(`[id="${current.id}-bbi_fixture-${type}"] .ant-tag`)).toHaveText(copy.reviews.pending)
  }
  await expect.poll(()=>page.locator('[data-testid=buyback-request-details] .ant-image img').evaluateAll(images=>images.filter(i=>i.complete&&i.naturalWidth>0).length)).toBe(3)
  assert.equal(calls.reads,3);await page.mouse.move(0,0)
  await page.screenshot({path:fileURLToPath(new URL('en-awaiting-mobile.png',output)),fullPage:true})
  await page.getByRole('button',{name:copy.submitPhotos,exact:true}).click();await expect(page.getByText(copy.statuses.photo_review,{exact:true}).first()).toBeVisible()
  assert.equal(calls.commands.length,1);assert.deepEqual(calls.commands[0],{action:'submit_photos',items:[{item_id:'bbi_fixture',serial_number:'SN-TEST',imei:'000000000000001'}]})
  assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth))
  cases.push('identity/required-photo validation, real PNG-to-WebP compression, three slots, CSRF submission and timeline');await context.close()
 }
 for(const action of ['accept_offer','decline_offer','cancel']){
  const {page,context,copy,calls,setStale}=await fixture({state:action==='cancel'?'awaiting_photos':'final_offer',locale:action==='decline_offer'?'zh':'en',width:1440})
  await expect(page.getByTestId('buyback-request-details')).toBeVisible()
  const label=action==='accept_offer'?copy.accept:action==='decline_offer'?copy.decline:copy.cancel
  await page.getByRole('button',{name:label,exact:true}).click();await expect(page.getByRole('dialog')).toBeVisible()
  if(action==='accept_offer'){
   setStale(true);await page.getByRole('button',{name:new RegExp('^'+[...copy.confirmChoice].join('\\s*')+'$')}).click();await expect(page.getByText(copy.errors.BUYBACK_OFFER_CHANGED,{exact:true})).toBeVisible();await expect(page.getByRole('dialog')).toBeVisible();setStale(false)
  }
  await page.getByRole('button',{name:new RegExp('^'+[...copy.confirmChoice].join('\\s*')+'$')}).click();await expect(page.getByRole('dialog')).toHaveCount(0)
  assert.equal(calls.commands.at(-1).action,action);if(action==='accept_offer')assert.equal(calls.commands.at(-1).final_amount,125)
  cases.push(action+' original confirmation and response state'+(action==='accept_offer'?' with stale-offer recovery':''));await context.close()
 }
 {
  const {page,context,copy,calls}=await fixture({state:'ready_to_ship',locale:'zh'})
  await expect(page.getByTestId('buyback-request-details')).toBeVisible()
  // Packaging/sealed controls are in original collapsed optional evidence group.
  await page.locator('.ant-collapse-header').click();assert.equal(await page.locator('input[type=file]').count(),2)
  await page.locator('form input').fill('TRACK-123');await page.getByRole('button',{name:copy.markShipped,exact:true}).click()
  await expect(page.getByText(copy.statuses.in_transit,{exact:true}).first()).toBeVisible();assert.deepEqual(calls.commands,[{action:'mark_shipped',tracking_number:'TRACK-123'}])
  await page.screenshot({path:fileURLToPath(new URL('zh-in-transit-mobile.png',output)),fullPage:true})
  cases.push('Chinese shipping form, packaging-only controls, current timeline and mobile layout');await context.close()
 }
 {
  const {page,context}=await fixture({logged:false});await expect(page.getByTestId('login-page')).toBeVisible();assert.equal(await page.getByTestId('buyback-request-details').count(),0);cases.push('anonymous account route uses existing login');await context.close()
 }
 // Compare the untouched original body/CSS under the same React/Ant runtime.
 for(const locale of ['en','zh'])for(const width of [390,1440])for(const state of ['list','awaiting_photos','final_offer']){
  const pair=[]
  for(const compare of ['current','reference']){
   const f=await fixture({locale,width,state:state==='list'?'awaiting_photos':state,list:state==='list',compare})
   await expect(f.page.getByTestId('buyback-compare')).toHaveAttribute('data-ready','true')
   if(state==='list')await expect(f.page.locator('article')).toHaveCount(20)
   else await expect(f.page.getByTestId('buyback-request-details')).toBeVisible()
   await f.page.evaluate(()=>document.fonts.ready)
   pair.push(f)
  }
  const targets=pair.map(f=>f.page.getByTestId('buyback-compare'))
  assert.equal(await targets[0].innerText(),await targets[1].innerText())
  const images=[]
  for(const [i,target]of targets.entries()){
   const image=await target.screenshot({animations:'disabled'});images.push(image)
   await writeFile(new URL(`${locale}-${width}-${state}-${i?'source':'current'}.png`,output),image)
  }
  assert.ok(images[0].equals(images[1]),`${locale}/${width}/${state} differs from original body`)
  cases.push(`${locale}/${width}/${state}: original body identical pixels in shared Ant5 runtime`)
  for(const f of pair)await f.context.close()
 }
 assert.deepEqual(errors,[])
 const report={cases,hydration_errors:0,scope:'Real Astro/React original account list/detail and customer client with intercepted API; 12 original-body pixel comparisons in shared Ant5; not real Gin joint or Next/Ant6 full-page acceptance.'}
 await writeFile(new URL('report.json',output),JSON.stringify(report,null,2)+'\n');console.log('BUYBACK_REQUESTS_BROWSER '+JSON.stringify(report))
}finally{if(browser)await browser.close();if(server)await server.stop()}
