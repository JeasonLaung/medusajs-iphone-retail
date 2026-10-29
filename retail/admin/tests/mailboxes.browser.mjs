import assert from 'node:assert/strict'
import {createServer} from 'vite'
import {chromium,expect} from '@playwright/test'
import {mkdir,writeFile} from 'node:fs/promises'

let server,browser,page
const id='mail_8e7a27c6-59b6-4865-97f3-6a769c459d6d',csrf='b'.repeat(64),saves=[],errors=[]
let revision=1,accounts=[],uncertain=true,forbidden=false
try{
 server=await createServer({server:{port:19373,strictPort:true,host:'127.0.0.1'},logLevel:'error'})
 await server.listen()
 browser=await chromium.launch({headless:true,executablePath:process.env.ADMIN_TEST_BROWSER})
 const context=await browser.newContext({reducedMotion:'reduce'})
 const origin='http://127.0.0.1:19373'
 await context.addCookies([{name:'retail_admin_csrf',value:csrf,url:origin}])
 await context.route('**/api/admin/**',async route=>{
  const request=route.request(),path=new URL(request.url()).pathname
  let body,status=200
  if(path==='/api/admin/auth/me')body={user:{id:'new-fixture',name:'Fixture',email:'admin@example.invalid',role:'admin',enabled:true}}
  else if(path==='/api/admin/auth/permissions')body={revision:'1',scope:'employee',actions:['mail.read','mail.write']}
  else if(path==='/api/admin/notification-mail/accounts'){
   if(request.method()==='GET')body={revision:String(revision),registration_account_id:accounts.find(a=>a.registration)?.id??null,accounts}
   else{
    assert.equal(request.headers()['x-csrf-token'],csrf)
    const input=request.postDataJSON();saves.push(input)
    if(uncertain){uncertain=false;await route.abort('failed');return}
    if(forbidden){status=403;body={error:{code:'authorization.denied'}}}
    else{
     revision++;accounts=[{id,name:input.name,email:input.email,reply_to:input.reply_to,enabled:input.enabled,has_api_key:true,has_webhook_secret:true,registration:input.registration}]
     body={id,revision:String(revision),replayed:false}
    }
   }
  }else throw new Error('Unexpected fixture request '+path)
  await route.fulfill({status,contentType:'application/json',body:JSON.stringify(body)})
 })
 page=await context.newPage();page.on('pageerror',e=>errors.push(e.message))
 await page.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','en'))
 await page.goto(origin+'/mailboxes')
 await expect(page.getByText('No registration verification mailbox selected').first()).toBeVisible()
 await page.getByRole('button',{name:'Add mailbox',exact:true}).click()
 await page.getByLabel('Sender name',{exact:true}).fill('Notification sender')
 await page.getByLabel('Sender email',{exact:true}).fill('sender@example.invalid')
 await page.getByLabel('Reply-to email',{exact:true}).fill('reply@example.invalid')
 await page.getByLabel('Resend API Key',{exact:true}).fill('re_browser_artificial')
 await page.getByLabel('Webhook Secret',{exact:true}).fill('webhook-artificial')
 await page.getByLabel('Use for registration verification',{exact:true}).check()
 await page.getByRole('button',{name:'Save',exact:true}).click()
 await expect(page.getByText('The save result is not yet confirmed.',{exact:false})).toBeVisible()
 await expect(page.getByLabel('Sender name',{exact:true})).toBeDisabled()
 await page.getByRole('button',{name:'Retry this save',exact:true}).click()
 await expect(page.getByRole('dialog')).toHaveCount(0)
 assert.deepEqual(saves[0],saves[1])
 await expect(page.getByRole('cell',{name:'sender@example.invalid',exact:true})).toBeVisible()
 await page.reload()
 await expect(page.getByRole('cell',{name:'sender@example.invalid',exact:true})).toBeVisible()
 await page.getByRole('button',{name:'Edit mailbox',exact:true}).click()
 await expect(page.getByLabel('Resend API Key',{exact:true})).toHaveValue('')
 await expect(page.getByLabel('Webhook Secret',{exact:true})).toHaveValue('')
 await page.getByLabel('Sender name',{exact:true}).fill('Renamed sender')
 await page.getByRole('button',{name:'Save',exact:true}).click()
 await expect(page.getByRole('cell',{name:'Renamed sender',exact:true})).toBeVisible()
 assert.equal(saves.at(-1).api_key,'');assert.equal(saves.at(-1).webhook_secret,'')
 await page.getByRole('button',{name:'Edit mailbox',exact:true}).click()
 await page.getByLabel('Enable sending',{exact:true}).uncheck()
 await expect(page.getByLabel('Use for registration verification',{exact:true})).not.toBeChecked()
 await expect(page.getByLabel('Use for registration verification',{exact:true})).toBeDisabled()
 await page.getByRole('button',{name:'Cancel',exact:true}).click()
 await expect(page.getByRole('dialog')).toHaveCount(0)
 forbidden=true
 await page.getByRole('button',{name:'Edit mailbox',exact:true}).click()
 await page.getByRole('button',{name:'Save',exact:true}).click()
 await expect(page.getByRole('button',{name:'Save',exact:true})).toBeDisabled()
 await page.getByRole('button',{name:'Cancel',exact:true}).click()
 const output=new URL('../../../logs/gin-migration/mailboxes/',import.meta.url);await mkdir(output,{recursive:true})
 await page.setViewportSize({width:390,height:844})
 await page.reload();await expect(page.getByRole('cell',{name:'Renamed sender',exact:true})).toBeVisible()
 assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth))
 await page.screenshot({path:new URL('en-mobile.png',output).pathname.replace(/^\/(?:([A-Za-z]):)/,'$1:')})
 await context.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','zh-CN'))
 const translated=await context.newPage();translated.on('pageerror',e=>errors.push(e.message))
 await translated.setViewportSize({width:1440,height:900});await translated.goto(origin+'/mailboxes')
 await expect(translated.getByRole('heading',{name:'通知邮箱',exact:true})).toBeVisible()
 await expect(translated.getByRole('cell',{name:'Renamed sender',exact:true})).toBeVisible()
 await translated.screenshot({path:new URL('zh-desktop.png',output).pathname.replace(/^\/(?:([A-Za-z]):)/,'$1:')})
 assert.deepEqual(errors,[])
 const report={scope:'Real React Admin with synthetic mailbox API; not Gin SQL browser joint acceptance',new_edit_refresh:true,secrets_blank:true,uncertain_retry_same_operation:true,disable_clears_registration:true,permission_failure_visible:true,english_and_chinese:true,mobile_no_page_overflow:true,hydration_errors:errors.length}
 await writeFile(new URL('report.json',output),JSON.stringify(report,null,2))
 console.log('MAILBOX_ADMIN_BROWSER '+JSON.stringify(report))
}catch(error){if(page)console.log('VISIBLE_BUTTONS',await page.getByRole('button').allTextContents());throw error}finally{await browser?.close();await server?.close()}
