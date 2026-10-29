import assert from 'node:assert/strict'
import {createServer} from 'vite'
import {chromium,expect} from '@playwright/test'
import {mkdir,writeFile} from 'node:fs/promises'
import {fileURLToPath} from 'node:url'
let server,browser
const id='00000000-0000-4000-8000-000000000001',expired='00000000-0000-4000-8000-000000000002',csrf='b'.repeat(64),errors=[]
const fixture=(id,status,retryable)=>({id,order_id:'order_'+id,status,attempts:3,created_at:'2026-10-30T00:00:00Z',first_attempt_at:'2026-10-30T00:00:00Z',next_attempt_at:null,sent_at:null,failed_at:'2026-10-30T00:00:00Z',error_code:'network_error',retryable})
try{
 server=await createServer({server:{port:19374,strictPort:true,host:'127.0.0.1'},logLevel:'error'});await server.listen()
 browser=await chromium.launch({headless:true,executablePath:process.env.ADMIN_TEST_BROWSER||'C:/Program Files/Google/Chrome/Application/chrome.exe'})
 for(const lang of ['en','zh-CN'])for(const width of [390,1440]){
  const context=await browser.newContext({viewport:{width,height:1000},reducedMotion:'reduce'}),page=await context.newPage(),posts=[];let rows=[fixture(id,'failed',true),fixture(expired,'failed',false)],lost=true,write=true,forbidden=false
  page.on('pageerror',e=>errors.push(e.message));await context.addCookies([{name:'retail_admin_csrf',value:csrf,url:'http://127.0.0.1:19374'}]);await context.addInitScript(lang=>localStorage.setItem('eurorefurb.admin.language',lang),lang)
  await context.route('**/api/admin/**',async route=>{
   const req=route.request(),path=new URL(req.url()).pathname;let body,status=200
   if(path==='/api/admin/auth/me')body={user:{id:'fixture',name:'Fixture',email:'admin@example.invalid',role:'admin',enabled:true}}
   else if(path==='/api/admin/auth/permissions')body={revision:'1',scope:'employee',actions:write?['mail.read','mail.write']:['mail.read']}
   else if(path==='/api/admin/notification-mail/accounts')body={revision:'1',registration_account_id:null,accounts:[]}
   else if(path==='/api/admin/notification-mail/order-jobs')body={jobs:rows,count:rows.length,limit:50,offset:0}
   else if(path===`/api/admin/notification-mail/order-jobs/${id}/retry`){
    assert.equal(req.method(),'POST');assert.equal(req.headers()['x-csrf-token'],csrf);const input=req.postDataJSON();assert.deepEqual(Object.keys(input).sort(),['expected_attempts','operation_id']);posts.push(input)
    if(forbidden){status=403;body={error:{code:'authorization.denied'}}}
    else{if(posts.length>1)assert.deepEqual(posts[0],input);rows=[{...rows[0],status:'pending',attempts:0,retryable:false},rows[1]];body={id,replayed:posts.length>1};if(lost){lost=false;await route.abort('failed');return}}
   }else throw Error('Unexpected API '+path)
   await route.fulfill({status,contentType:'application/json',body:JSON.stringify(body)})
  })
  await page.goto('http://127.0.0.1:19374/mailboxes');const panel=page.getByTestId('order-mail-jobs');await expect(panel.getByRole('heading',{name:lang==='en'?'Order email jobs':'订单邮件任务'})).toBeVisible()
  const retry=panel.getByRole('button',{name:lang==='en'?'Queue again':'重新排队',exact:true});await expect(retry).toHaveCount(2);await expect(retry.nth(1)).toBeDisabled()
  await retry.first().click();await page.getByRole('button',{name:lang==='en'?'OK':/确\s*定/,exact:true}).click()
  await expect(panel.getByText(lang==='en'?'The retry result is not confirmed. Retry this same operation.':'重试结果尚未确认，请使用同一操作重试。')).toBeVisible()
  await retry.first().click();await expect.poll(()=>posts.length).toBe(2);assert.deepEqual(posts[0],posts[1]);await expect(retry.first()).toBeDisabled()
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
  const output=new URL('../../../logs/gin-migration/order-mail-admin/',import.meta.url);await mkdir(output,{recursive:true});await page.screenshot({path:fileURLToPath(new URL(`${lang}-${width}.png`,output)),fullPage:true})
  rows=[fixture(id,'failed',true),fixture(expired,'failed',false)];forbidden=true;await panel.getByRole('button',{name:lang==='en'?'Reload':'重新加载',exact:true}).click();await expect(retry.first()).toBeEnabled();await retry.first().click();await page.getByRole('button',{name:lang==='en'?'OK':/确\s*定/,exact:true}).click();await expect(retry.first()).toBeDisabled()
  write=false;await page.reload();await expect(panel.getByRole('heading')).toBeVisible();await expect(panel.getByRole('button',{name:lang==='en'?'Queue again':'重新排队',exact:true})).toHaveCount(0)
  await context.close()
 }
 assert.deepEqual(errors,[])
 const report={formal_react_admin:true,synthetic_api:true,real_gin_mysql:false,ant_components:true,private_task_projection:true,uncertain_same_operation_retry:true,expired_disabled:true,permission_failure_visible:true,read_only_no_retry:true,languages:['en','zh-CN'],widths:[390,1440],page_overflow:false,browser_errors:0}
 await writeFile(new URL('../../../logs/gin-migration/order-mail-admin/report.json',import.meta.url),JSON.stringify(report,null,2));console.log('ORDER_MAIL_ADMIN_BROWSER '+JSON.stringify(report))
}finally{await browser?.close();await server?.close()}
