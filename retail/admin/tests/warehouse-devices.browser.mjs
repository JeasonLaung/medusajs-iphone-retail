import assert from 'node:assert/strict'
import {createServer} from 'vite'
import {chromium,expect} from '@playwright/test'
let server,browser
const origin='http://127.0.0.1:19387',errors=[],commands=[]
let lost=true,readonly=false,malformed=false,conflict=false,empty=false,unavailable=false
const row={id:'device-one',serial_number:'WAREHOUSE-ONE',imei:'000000000000001',imei2:null,eid:null,variant_id:'variant-one',variant_title:'Test SKU',source_id:'source-one',location_id:'warehouse-a',category:'sale',state:'pending',grade:'B',battery_health:85,photos:[],created_at:'2026-10-30T00:00:00Z',bin:'Shelf A',revision:'a'.repeat(64),allowed_actions:['make_available','retire','delete']}
try{
 server=await createServer({server:{port:19387,strictPort:true,host:'127.0.0.1'},logLevel:'error'});await server.listen()
 browser=await chromium.launch({headless:true,executablePath:process.env.ADMIN_TEST_BROWSER||'C:/Program Files/Google/Chrome/Application/chrome.exe'})
 const context=await browser.newContext({reducedMotion:'reduce'});await context.addCookies([{name:'retail_admin_csrf',value:'a'.repeat(64),url:origin}]);await context.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','en'))
 await context.route('**/api/admin/**',async route=>{
  const req=route.request(),url=new URL(req.url()),path=url.pathname;let body,status=200
  if(path==='/api/admin/auth/me')body={user:{id:'warehouse-operator',name:'Worker',email:'worker@example.invalid',role:'admin',enabled:true}}
  else if(path==='/api/admin/auth/permissions')body={revision:'1',scope:'warehouse',actions:[]}
  else if(path==='/api/admin/warehouse/access'){
   if(unavailable){status=503;body={error:{code:'authorization.unavailable'}}}
   else body={revision:'1',actions:['warehouse.intake.read',...(readonly?[]:['warehouse.intake.write'])],locations:empty?[]:[{id:'warehouse-a',name:'Warehouse A'}]}
  }else if(path==='/api/admin/warehouse/locations/warehouse-a/devices/sale'){
   const offset=Number(url.searchParams.get('offset'));body={location_id:'warehouse-a',category:'sale',offset,limit:50,has_more:offset===0,devices:[{...row,...(malformed?{location_id:'other'}:{}),allowed_actions:readonly?[]:row.allowed_actions}]}
  }else if(path==='/api/admin/warehouse/locations/warehouse-a/devices/sale/device-one'){
   assert.equal(req.headers()['x-csrf-token'],'a'.repeat(64));const input=req.postDataJSON();commands.push(input)
   if(conflict){status=409;body={error:{code:'device.conflict'}}}
   else if(lost){lost=false;await route.abort('failed');return}
   else body={id:row.id,state:'retired',deleted:false,replayed:true}
  }else throw Error('Unexpected device request '+path)
  await route.fulfill({status,contentType:'application/json',body:JSON.stringify(body)})
 })
 const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message));await page.goto(origin+'/warehouse-devices');await expect(page.getByRole('cell',{name:row.serial_number,exact:true})).toBeVisible()
 await expect(page.getByRole('radio',{name:'Returned',exact:true})).toHaveCount(0)
 await page.getByRole('button',{name:'Next page',exact:true}).click();await expect(page.getByRole('button',{name:'Next page',exact:true})).toBeDisabled();await page.getByRole('button',{name:'Previous page',exact:true}).click()
 await page.getByRole('button',{name:'Retire',exact:true}).click();let dialog=page.getByRole('dialog');await dialog.getByRole('button',{name:'Save',exact:true}).click();await expect(dialog.getByRole('button',{name:'Retry this save',exact:true})).toBeVisible();await expect(dialog.getByRole('button',{name:'Save',exact:true})).toHaveCount(0);await dialog.getByRole('button',{name:'Retry this save',exact:true}).click();await expect(dialog).not.toBeVisible();assert.deepEqual(commands[0],commands[1]);assert.equal(commands.length,2)
 conflict=true;await page.getByRole('button',{name:'Retire',exact:true}).click();await dialog.getByRole('button',{name:'Save',exact:true}).click();await expect(dialog.getByRole('alert')).toBeVisible();await expect(dialog.getByRole('button',{name:'Retry this save',exact:true})).toHaveCount(0);await dialog.getByRole('button',{name:'Cancel',exact:true}).click();conflict=false
 readonly=true;await page.reload();await expect(page.getByRole('cell',{name:row.serial_number,exact:true})).toBeVisible();await expect(page.getByRole('button',{name:'Retire',exact:true})).toHaveCount(0)
 malformed=true;await page.reload();await expect(page.getByRole('alert')).toBeVisible();await expect(page.getByRole('cell',{name:row.serial_number,exact:true})).toHaveCount(0);malformed=false
 empty=true;await page.reload();await expect(page.getByText('No selected warehouses means no warehouse access.',{exact:true})).toBeVisible();empty=false
 unavailable=true;await page.reload();await expect(page.getByRole('button',{name:'Reconnect',exact:true})).toBeVisible();unavailable=false;await page.getByRole('button',{name:'Reconnect',exact:true}).click();await expect(page.getByRole('cell',{name:row.serial_number,exact:true})).toBeVisible()
 assert.deepEqual(errors,[]);console.log('WAREHOUSE_DEVICES_ADMIN_BROWSER '+JSON.stringify({synthetic_api:true,real_gin_joint:false,lost_response_same_request_retry:true,conflict_requires_reload:true,read_only_controls:true,category_controls:true,cross_location_response_rejected:true,pagination:true,empty_scope:true,load_failure_retry:true,browser_errors:0}))
}finally{await browser?.close();await server?.close()}
