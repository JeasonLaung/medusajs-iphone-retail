import assert from 'node:assert/strict'
import {createServer} from 'vite'
import {chromium,expect} from '@playwright/test'
let server,browser
const origin='http://127.0.0.1:19380',id='sloc_825c8ed0-4c90-4ac8-81a2-262359847013',csrf='b'.repeat(64),saves=[],errors=[]
let revision=1,locations=[],lost=true,readonly=false,conflict=false,unavailable=false
try{
 server=await createServer({server:{port:19380,strictPort:true,host:'127.0.0.1'},logLevel:'error'});await server.listen()
 browser=await chromium.launch({headless:true,executablePath:process.env.ADMIN_TEST_BROWSER||'C:/Program Files/Google/Chrome/Application/chrome.exe'})
 const context=await browser.newContext({reducedMotion:'reduce'})
 await context.addCookies([{name:'retail_admin_csrf',value:csrf,url:origin}])
 await context.route('**/api/admin/**',async route=>{
  const request=route.request(),path=new URL(request.url()).pathname;let body,status=200
  if(path==='/api/admin/auth/me')body={user:{id:'new-fixture',name:'Fixture',email:'admin@example.invalid',role:'admin',enabled:true}}
  else if(path==='/api/admin/auth/permissions')body={revision:'1',scope:'employee',actions:readonly?['warehouse.read']:['warehouse.read','warehouse.write']}
  else if(path==='/api/admin/warehouses'){
   if(unavailable){status=503;body={error:{code:'warehouse.unavailable'}}}
   else if(request.method()==='GET')body={revision:String(revision),locations}
   else{
    assert.equal(request.headers()['x-csrf-token'],csrf)
    const input=request.postDataJSON();saves.push(input)
    if(conflict){status=409;body={error:{code:'warehouse.conflict'}}}
    else if(lost){lost=false;revision++;locations=[{id,name:input.name,address:input.address}];await route.abort('failed');return}
    else{if(saves.length>2){revision++;locations=[{id,name:input.name,address:input.address}]};body={id,revision:String(revision),replayed:saves.length===2}}
   }
  }else throw Error('Unexpected warehouse fixture request '+path)
  await route.fulfill({status,contentType:'application/json',body:JSON.stringify(body)})
 })
 await context.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','en'))
 const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message))
 await page.goto(origin+'/warehouses');await expect(page.getByText('No warehouses',{exact:true})).toBeVisible()
 await page.getByRole('button',{name:'New warehouse',exact:true}).click()
 const dialog=page.getByRole('dialog')
 await dialog.getByLabel('Name',{exact:true}).fill('Warehouse one')
 await dialog.getByLabel('Address line 1',{exact:true}).fill('Fixture street')
 await dialog.getByLabel('Country / region',{exact:true}).fill('DK')
 await dialog.getByLabel('Postal code',{exact:true}).fill('0012')
 await dialog.getByRole('button',{name:'Save',exact:true}).click()
 await expect(dialog.getByRole('button',{name:'Retry this save',exact:true})).toBeVisible()
 await expect(dialog.getByLabel('Name',{exact:true})).toBeDisabled()
 await dialog.getByRole('button',{name:'Retry this save',exact:true}).click()
 await expect(dialog).toHaveCount(0);assert.deepEqual(saves[0],saves[1]);assert.equal(saves[0].address.country_code,'dk')
 await expect(page.getByRole('button',{name:'Warehouse one',exact:true})).toBeVisible()
 await page.reload();await page.getByRole('button',{name:'Warehouse one',exact:true}).click()
 await expect(dialog.getByLabel('Address line 1',{exact:true})).toHaveValue('Fixture street')
 await expect(dialog.getByLabel('Postal code',{exact:true})).toHaveValue('0012')
 await dialog.getByLabel('Name',{exact:true}).fill('Warehouse updated');await dialog.getByRole('button',{name:'Save',exact:true}).click()
 await expect(dialog).toHaveCount(0);await expect(page.getByRole('button',{name:'Warehouse updated',exact:true})).toBeVisible()
 conflict=true;await page.getByRole('button',{name:'Warehouse updated',exact:true}).click();await dialog.getByRole('button',{name:'Save',exact:true}).click()
 await expect(dialog.getByText('Warehouses changed. Close and reload before editing.')).toBeVisible()
 await expect(dialog.getByRole('button',{name:'Retry this save',exact:true})).toHaveCount(0)
 await dialog.getByRole('button',{name:'Cancel',exact:true}).click();conflict=false
 readonly=true;await page.reload();await expect(page.getByRole('button',{name:'New warehouse',exact:true})).toHaveCount(0)
 await expect(page.getByRole('button',{name:'Warehouse updated',exact:true})).toHaveCount(0)
 await expect(page.getByRole('cell',{name:'Warehouse updated',exact:true})).toBeVisible()
 await page.setViewportSize({width:390,height:844});assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth))
 await context.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','zh-CN'))
 const translated=await context.newPage();translated.on('pageerror',e=>errors.push(e.message));await translated.goto(origin+'/warehouses')
 await expect(translated.getByRole('heading',{name:'仓库',exact:true})).toBeVisible()
 await expect(translated.getByRole('cell',{name:'Warehouse updated',exact:true})).toBeVisible()
 unavailable=true;await translated.reload();await expect(translated.getByRole('alert')).toBeVisible()
 unavailable=false;await translated.getByRole('button',{name:'重新连接',exact:true}).click();await expect(translated.getByRole('cell',{name:'Warehouse updated',exact:true})).toBeVisible()
 assert.deepEqual(errors,[])
 console.log('WAREHOUSE_ADMIN_BROWSER '+JSON.stringify({real_react_antd:true,synthetic_api:true,real_gin_joint:false,create_edit_refresh:true,lost_response_same_operation_retry:true,country_normalized:true,postal_leading_zeros_preserved:true,conflict_visible:true,read_only_controls:true,language_en_zh:true,mobile_no_page_overflow:true,load_failure_retry:true,browser_errors:0}))
}finally{await browser?.close();await server?.close()}
