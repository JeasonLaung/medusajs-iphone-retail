import assert from 'node:assert/strict'
import {createServer} from 'vite'
import {chromium,expect} from '@playwright/test'
let server,browser
const origin='http://127.0.0.1:19381',id='usr_825c8ed0-4c90-4ac8-81a2-262359847013',csrf='c'.repeat(64),saves=[],errors=[]
let revision=1,users=[],lost=true,readonly=false,conflict=false,unavailable=false
const locations=[{id:'warehouse-a',name:'Warehouse A'},{id:'warehouse-b',name:'Warehouse B'}]
try{
 server=await createServer({server:{port:19381,strictPort:true,host:'127.0.0.1'},logLevel:'error'});await server.listen()
 browser=await chromium.launch({headless:true,executablePath:process.env.ADMIN_TEST_BROWSER||'C:/Program Files/Google/Chrome/Application/chrome.exe'})
 const context=await browser.newContext({reducedMotion:'reduce'})
 await context.addCookies([{name:'retail_admin_csrf',value:csrf,url:origin}])
 await context.route('**/api/admin/**',async route=>{
  const request=route.request(),path=new URL(request.url()).pathname;let body,status=200
  if(path==='/api/admin/auth/me')body={user:{id:'new-fixture',name:'Fixture',email:'admin@example.invalid',role:'admin',enabled:true}}
  else if(path==='/api/admin/auth/permissions')body={revision:'1',scope:'employee',actions:readonly?['warehouse.read','authorization.read']:['warehouse.read','warehouse.write','authorization.read','authorization.write','inventory.read','inventory.stock.write','order.fulfillment.read']}
  else if(path==='/api/admin/warehouse-users'){
   if(unavailable){status=503;body={error:{code:'warehouse_users.unavailable'}}}
   else if(request.method()==='GET')body={revision:String(revision),users,locations}
   else{
    assert.equal(request.headers()['x-csrf-token'],csrf)
    const input=request.postDataJSON();saves.push(input)
    if(conflict){status=409;body={error:{code:'warehouse_users.conflict'}}}
    else{
     const replay=saves.length===2
     if(!replay){revision++;users=[{id,name:input.name,email:input.email,enabled:input.enabled,mode:input.mode,locations:input.locations,actions:input.actions}]}
     if(lost){lost=false;await route.abort('failed');return}
     body={id,revision:String(revision),replayed:replay}
    }
   }
  }else throw Error('Unexpected warehouse-user fixture request '+path)
  await route.fulfill({status,contentType:'application/json',body:JSON.stringify(body)})
 })
 await context.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','en'))
 const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message))
 await page.goto(origin+'/warehouse-users');await expect(page.getByText('No warehouse users',{exact:true})).toBeVisible()
 await page.getByRole('button',{name:'New warehouse user',exact:true}).click()
 const dialog=page.getByRole('dialog')
 await dialog.getByLabel('Name',{exact:true}).fill('Warehouse operator')
 await dialog.getByLabel('Email',{exact:true}).fill('warehouse@example.invalid')
 await dialog.getByLabel('Password',{exact:true}).fill('synthetic-only-password')
 await dialog.getByText('Warehouse A (warehouse-a)',{exact:true}).click();await dialog.locator('.ant-transfer-operation button').first().click()
 await dialog.getByRole('checkbox',{name:'View inventory',exact:true}).click();await expect(dialog.getByRole('checkbox',{name:'View inventory',exact:true})).toBeChecked()
 await dialog.getByRole('button',{name:'Save',exact:true}).click()
 await expect(dialog.getByRole('button',{name:'Retry this save',exact:true})).toBeVisible()
 await expect(dialog.getByLabel('Password',{exact:true})).toBeDisabled()
 await dialog.getByRole('button',{name:'Retry this save',exact:true}).click()
 await expect(page.getByRole('button',{name:'Warehouse operator',exact:true})).toBeVisible()
 assert.equal(saves.length,2);assert.deepEqual(saves[0],saves[1]);assert.deepEqual(saves[0].locations,['warehouse-a']);assert.equal(saves[0].actions.length,1)
 await page.getByRole('button',{name:'Warehouse operator',exact:true}).click()
 await expect(dialog.getByLabel('Email',{exact:true})).toBeDisabled();await expect(dialog.getByLabel('Password',{exact:true})).toHaveValue('')
 await dialog.getByLabel('Enabled',{exact:true}).click()
 await dialog.getByRole('radio',{name:'All active warehouses',exact:true}).click();await expect(dialog.getByRole('radio',{name:'All active warehouses',exact:true})).toBeChecked()
 await dialog.getByRole('button',{name:'Save',exact:true}).click();await expect(dialog).not.toBeVisible()
 assert.equal(saves[2].enabled,false);assert.equal(saves[2].mode,'all');assert.deepEqual(saves[2].locations,[]);assert.equal(saves[2].password,'')
 await page.reload();await expect(page.getByText('All active warehouses',{exact:true})).toBeVisible()
 conflict=true;await page.getByRole('button',{name:'Warehouse operator',exact:true}).click();await dialog.getByLabel('Name',{exact:true}).fill('Conflict');await dialog.getByRole('button',{name:'Save',exact:true}).click()
 await expect(dialog.getByText('Permissions or account changed, or the email already exists. Close and reload.',{exact:true})).toBeVisible();await expect(dialog.getByRole('button',{name:'Retry this save',exact:true})).toHaveCount(0);await dialog.getByRole('button',{name:'Cancel',exact:true}).click();conflict=false
 readonly=true;await page.reload();await expect(page.getByRole('button',{name:'New warehouse user',exact:true})).toHaveCount(0);await expect(page.getByRole('button',{name:'Warehouse operator',exact:true})).toHaveCount(0)
 await page.setViewportSize({width:390,height:844});await page.reload();await expect(page.getByText('Warehouse operator',{exact:true})).toBeVisible();assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
 await context.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','zh-CN'));await page.reload();await expect(page.getByRole('heading',{name:'仓库用户',exact:true})).toBeVisible()
 unavailable=true;await page.reload();await expect(page.getByRole('button',{name:'重新连接',exact:true})).toBeVisible();unavailable=false;await page.getByRole('button',{name:'重新连接',exact:true}).click();await expect(page.getByText('Warehouse operator',{exact:true})).toBeVisible()
 assert.deepEqual(errors,[])
 console.log('WAREHOUSE_USERS_ADMIN_BROWSER '+JSON.stringify({real_react_antd:true,synthetic_api:true,real_gin_joint:false,create_edit_reload:true,lost_response_identical_retry:true,password_not_prefilled:true,empty_password_preserves:true,all_clears_selection:true,disable_account:true,conflict_visible:true,read_only_controls:true,language_en_zh:true,mobile_no_page_overflow:true,load_failure_retry:true,browser_errors:0}))
}catch(error){const p=browser?.contexts()[0]?.pages()[0];if(p)await p.screenshot({path:'../../logs/gin-migration/warehouse-users-browser-failure.png',fullPage:true});throw error}finally{await browser?.close();await server?.close()}
