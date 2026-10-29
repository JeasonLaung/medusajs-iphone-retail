import assert from 'node:assert/strict'
import {createServer} from 'vite'
import {chromium,expect} from '@playwright/test'
let server,browser
const origin='http://127.0.0.1:19386',errors=[],commands=[],photoCommands=[]
const product={id:'intake-product',revision:'a'.repeat(64),title:'Intake fixture',handle:'intake-product',status:'draft',thumbnail:null,collection_id:null,created_at:'2026-10-28T00:00:00Z',updated_at:'2026-10-28T00:00:00Z',variant_count:1,subtitle:null,description:null,options:[],images:[]}
const variant={id:'intake-variant',product_id:product.id,title:'Intake SKU',sku:'INTAKE-SKU',thumbnail:null,variant_rank:0,manage_inventory:true,allow_backorder:false,configuration_complete:true,options:[],images:[],unassigned_option_value_ids:[]}
let photoLost=true,lost=true,conflict=false,empty=false,unavailable=false,readonly=false
try{
 server=await createServer({server:{port:19386,strictPort:true,host:'127.0.0.1'},logLevel:'error'});await server.listen()
 browser=await chromium.launch({headless:true,executablePath:process.env.ADMIN_TEST_BROWSER||'C:/Program Files/Google/Chrome/Application/chrome.exe'})
 const context=await browser.newContext({reducedMotion:'reduce'});await context.addCookies([{name:'retail_admin_csrf',value:'a'.repeat(64),url:origin}]);await context.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','en'))
 await context.route('**/api/store/device-photos/**',route=>route.fulfill({contentType:'image/png',body:Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jB9kAAAAASUVORK5CYII=','base64')}))
 await context.route('**/api/admin/**',async route=>{
  const request=route.request(),path=new URL(request.url()).pathname;let body,status=200
  if(path==='/api/admin/auth/me')body={user:{id:'intake-operator',name:'Operator',email:'intake@example.invalid',role:'admin',enabled:true}}
  else if(path==='/api/admin/auth/permissions')body={revision:'1',scope:'employee',actions:['product.read','inventory.read','warehouse.read','device.read',...(readonly?[]:['device.intake'])]}
  else if(path==='/api/admin/device-photos'){
   const input=request.postDataJSON();photoCommands.push(input);assert.equal(request.headers()['x-csrf-token'],'a'.repeat(64));assert.equal(input.mime_type,'image/jpeg');assert.equal(Buffer.from(input.content,'base64').subarray(0,3).toString('hex'),'ffd8ff')
   if(photoLost){photoLost=false;await route.abort('failed');return}
   body={photo:{id:'dphoto_'+input.operation_id,url:'/api/store/device-photos/dphoto_'+input.operation_id},replayed:true}
  }
  else if(path.endsWith('/device-tracking'))body={product_id:product.id,variant_id:variant.id,revision:'a'.repeat(64),enabled:true,eligible:false,reason:'already_enabled',inventory_item_id:'intake-stock'}
  else if(path.endsWith('/device-intake')){
   if(request.method()==='POST'){
    const input=request.postDataJSON();commands.push(input);assert.equal(request.headers()['x-csrf-token'],'a'.repeat(64))
    if(conflict){status=409;body={error:{code:'device.conflict'}}}
    else{if(lost){lost=false;await route.abort('failed');return};body={devices:input.devices.map((d,i)=>({...d,id:'device-'+i,variant_id:variant.id,location_id:'intake-location',serial_number:d.serial_number.toUpperCase(),grade:d.grade??'B',battery_health:d.battery_health??85,state:d.category==='sale'?'available':'inspection'})),replayed:true}}
   }else if(unavailable){status=503;body={error:{code:'device.unavailable'}}}
   else body={recent_devices:[],recent_limit:50,revision:'b'.repeat(64),requires_radio:true,options:[{title:'Condition',value:'B'},{title:'Battery',value:'85%'}],sources:empty?[]:[{id:'intake-source',name:'Intake source',type:'self',location_id:'intake-location',location_name:'Intake warehouse'}]}
  }else if(path.endsWith('/variants'))body={variants:[variant],count:1,limit:20,offset:0}
  else if(path==='/api/admin/products/intake-product')body={product}
  else throw Error('Unexpected intake request '+path)
  await route.fulfill({status,contentType:'application/json',body:JSON.stringify(body)})
 })
 const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message));const open=async()=>{await page.goto(origin+'/products/intake-product');await page.getByRole('button',{name:'Device tracking',exact:true}).click();await page.getByRole('dialog').getByRole('button',{name:'Device intake',exact:true}).click();return page.getByRole('dialog').last()}
 const fill=async dialog=>{await dialog.getByLabel('Own warehouse',{exact:true}).press('ArrowDown');await page.getByText('Intake source (Intake warehouse)',{exact:true}).click();await dialog.getByLabel('SN',{exact:true}).fill('test-sn');await dialog.getByLabel('IMEI',{exact:true}).fill('000000000000091')}
 let dialog=await open();await fill(dialog);
 await dialog.locator('input[type=file]').first().setInputFiles({name:'phone.png',mimeType:'image/png',buffer:Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jB9kAAAAASUVORK5CYII=','base64')})
 await expect(dialog.getByText('Photo upload failed. Retry, or cancel to discard this unconfirmed photo.',{exact:true})).toBeVisible();await expect(dialog.getByRole('button',{name:'Submit intake batch',exact:true})).toBeDisabled();await expect(dialog.getByRole('button',{name:'Reload',exact:true})).toBeDisabled()
 await dialog.getByRole('button',{name:'Retry this save',exact:true}).click();await expect(dialog.locator('.ant-upload-list-item')).toHaveCount(1);assert.equal(photoCommands.length,2);assert.deepEqual(photoCommands[0],photoCommands[1]);
 await dialog.getByRole('button',{name:'Submit intake batch',exact:true}).click();await expect(dialog.getByText('The result is unconfirmed. Retry the original save.',{exact:true})).toBeVisible();await expect(dialog.getByLabel('SN',{exact:true})).toBeDisabled();await expect(dialog.getByRole('button',{name:'Reload',exact:true})).toBeDisabled();await dialog.getByRole('button',{name:'Retry this save',exact:true}).click();await expect(dialog.getByRole('cell',{name:'TEST-SN',exact:true})).toBeVisible();assert.equal(commands.length,2);assert.deepEqual(commands[0],commands[1]);assert.equal(commands[0].devices[0].imei,'000000000000091');assert.equal(commands[0].devices[0].grade,null);assert.equal(commands[0].devices[0].photos.length,1)
 dialog=await open();
 for(let i=0;i<8;i++){await dialog.locator('input[type=file]').first().setInputFiles({name:`limit-${i}.png`,mimeType:'image/png',buffer:Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jB9kAAAAASUVORK5CYII=','base64')});await expect(dialog.locator('.ant-upload-list-item')).toHaveCount(i+1)}
 await expect(dialog.getByRole('button',{name:'Add photo',exact:true})).toBeDisabled();await dialog.locator('.ant-upload-list-item').first().getByTitle('Remove file').click();await expect(dialog.locator('.ant-upload-list-item')).toHaveCount(7);await expect(dialog.getByRole('button',{name:'Add photo',exact:true})).toBeEnabled()
 conflict=true;dialog=await open();await fill(dialog);await dialog.getByRole('button',{name:'Submit intake batch',exact:true}).click();await expect(dialog.getByText('An identity conflicts, or configuration or permissions changed. Check the batch and reload.',{exact:true})).toBeVisible();await expect(dialog.getByRole('button',{name:'Submit intake batch',exact:true})).toHaveCount(0);conflict=false;await dialog.getByRole('button',{name:'Reload',exact:true}).click();await expect(dialog.getByRole('button',{name:'Submit intake batch',exact:true})).toBeVisible()
 empty=true;dialog=await open();await expect(dialog.getByText('Create an own-warehouse device source from Warehouses first.',{exact:true})).toBeVisible();await expect(dialog.getByRole('button',{name:'Submit intake batch',exact:true})).toBeDisabled();empty=false
 unavailable=true;dialog=await open();await expect(dialog.getByRole('button',{name:'Reconnect',exact:true})).toBeVisible();unavailable=false;await dialog.getByRole('button',{name:'Reconnect',exact:true}).click();await expect(dialog.getByRole('button',{name:'Submit intake batch',exact:true})).toBeVisible()
 readonly=true;await page.goto(origin+'/products/intake-product');await page.getByRole('button',{name:'Device tracking',exact:true}).click();await expect(page.getByRole('dialog').getByText('Enabled',{exact:true})).toBeVisible();await expect(page.getByRole('button',{name:'Device intake',exact:true})).toHaveCount(0);assert.deepEqual(errors,[])
 console.log('DEVICE_INTAKE_ADMIN_BROWSER '+JSON.stringify({real_react_antd:true,synthetic_api:true,real_gin_joint:false,lost_response_same_request_retry:true,uncertain_form_locked:true,leading_zero_preserved:true,sku_defaults_left_null:true,conflict_reload:true,empty_source_disabled:true,unavailable_retry:true,read_only_controls:true,browser_errors:0,photo_upload_integrated:true,photo_lost_response_same_request_retry:true,photo_pending_blocks_intake:true,eight_photo_limit_and_removal:true}))
}finally{await browser?.close();await server?.close()}
