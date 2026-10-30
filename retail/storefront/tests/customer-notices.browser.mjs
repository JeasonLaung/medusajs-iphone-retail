import assert from 'node:assert/strict'
import {createRequire} from 'node:module'
import {dev} from 'astro'
import {realpathSync} from 'node:fs'
import {fileURLToPath} from 'node:url'
import react from '@astrojs/react'
const require=createRequire(new URL('../../admin/package.json',import.meta.url)),{chromium,expect}=require('@playwright/test')
let server,browser
const rows=Array.from({length:23},(_,i)=>({id:'notice_00000000-0000-4000-8000-'+String(i+1).padStart(12,'0'),kind:'after_sales.processing',resource_id:'order-1',reference:'REF-'+i,action:'',read_at:null,created_at:'2026-10-30T00:00:00Z'}))
let fail=false,reads=0
try{
 server=await dev({root:new URL('./notices-fixture/',import.meta.url),configFile:false,output:'server',integrations:[react()],devToolbar:{enabled:false},server:{host:'127.0.0.1',port:0},logLevel:'error',vite:{resolve:{dedupe:['react','react-dom']},server:{fs:{allow:[fileURLToPath(new URL('../',import.meta.url)),realpathSync(fileURLToPath(new URL('../node_modules',import.meta.url)))]}}}})
 const origin='http://127.0.0.1:'+server.address.port
 browser=await chromium.launch({headless:true,executablePath:process.env.ADMIN_TEST_BROWSER||'C:/Program Files/Google/Chrome/Application/chrome.exe'})
 const context=await browser.newContext({reducedMotion:'reduce'});await context.addCookies([{name:'retail_customer_csrf',value:'b'.repeat(64),url:origin}])
 const page=await context.newPage(),errors=[];page.on('pageerror',e=>errors.push(e.message))
 await page.route('**/api/store/customer/notifications**',async route=>{
  const request=route.request()
  if(request.method()==='POST'){
   assert.equal(request.headers()['x-csrf-token'],'b'.repeat(64));reads++
   const body=request.postDataJSON();for(const row of rows)if(!body.id||row.id===body.id)row.read_at='2026-10-30T01:00:00Z'
   return route.fulfill({json:{success:true}})
  }
  if(fail)return route.fulfill({status:503,json:{error:{}}})
  const offset=Number(new URL(request.url()).searchParams.get('offset')||0)
  return route.fulfill({json:{notifications:rows.slice(offset,offset+20),count:23,unread:rows.filter(n=>!n.read_at).length,offset,limit:20}})
 })
 await page.goto(origin);await expect(page.getByTestId('notification-badge')).toHaveText('23');await page.getByTestId('notification-bell').click()
 const dialog=page.getByRole('dialog');await expect(dialog.getByRole('listitem')).toHaveCount(20)
 await dialog.getByRole('button',{name:'Mark as read',exact:true}).first().click();await expect(page.getByTestId('notification-badge')).toHaveText('22')
 await dialog.getByRole('button',{name:'Next',exact:true}).click();await expect(dialog.getByRole('listitem')).toHaveCount(3)
 await dialog.getByRole('button',{name:'Mark all as read',exact:true}).click();await expect(page.getByTestId('notification-badge')).toHaveCount(0);assert.equal(reads,2)
 fail=true;await page.evaluate(()=>window.dispatchEvent(new Event('focus')));await expect(dialog.getByRole('alert')).toBeVisible();await expect(dialog.getByRole('listitem')).toHaveCount(3)
 fail=false;await dialog.getByRole('button',{name:'Retry',exact:true}).click();await expect(dialog.getByRole('alert')).toHaveCount(0)
 await page.setViewportSize({width:390,height:844});assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
 await dialog.getByRole('button',{name:'Close notifications',exact:true}).click();const loaded=page.waitForResponse(r=>r.url().includes('/api/store/customer/notifications'));await page.goto(origin+'/?lang=zh');await loaded;await page.getByTestId('notification-bell').click();await expect(page.getByRole('heading',{name:'消息通知',exact:true})).toBeVisible()
 await page.getByRole('button',{name:'查看详情',exact:true}).first().click();await page.waitForURL(origin+'/dk-zh/account/orders/order-1');assert.equal(reads,3)
 assert.deepEqual(errors,[])
 console.log('CUSTOMER_NOTICES_BROWSER '+JSON.stringify({synthetic_api:true,original_dialog_structure:true,unread_badge:true,pagination_20:true,single_and_all_read:true,detail_navigation:true,error_retains_data:true,retry:true,chinese_english:true,mobile_390:true,browser_errors:0}))
}finally{await browser?.close();await server?.stop()}
