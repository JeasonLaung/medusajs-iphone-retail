import assert from 'node:assert/strict'
import {createServer} from 'vite'
import {chromium,expect} from '@playwright/test'
let server,browser
const origin='http://127.0.0.1:19385',commands=[],errors=[]
let sources=[],lost=true,readonly=false,unavailable=false,malformed=false
try{
 server=await createServer({server:{port:19385,strictPort:true,host:'127.0.0.1'},logLevel:'error'});await server.listen()
 browser=await chromium.launch({headless:true,executablePath:process.env.ADMIN_TEST_BROWSER||'C:/Program Files/Google/Chrome/Application/chrome.exe'})
 const context=await browser.newContext({reducedMotion:'reduce'});await context.addCookies([{name:'retail_admin_csrf',value:'a'.repeat(64),url:origin}]);await context.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','en'))
 await context.route('**/api/admin/**',async route=>{
  const request=route.request(),path=new URL(request.url()).pathname;let body,status=200
  if(path==='/api/admin/auth/me')body={user:{id:'source-operator',name:'Operator',email:'source@example.invalid',role:'admin',enabled:true}}
  else if(path==='/api/admin/auth/permissions')body={revision:'1',scope:'employee',actions:['warehouse.read','device.read',...(readonly?[]:['warehouse.write','device.source.write'])]}
  else if(path==='/api/admin/warehouses')body={revision:'1',locations:sources.map(s=>({id:s.location_id,name:s.location_name,address:null}))}
  else if(path==='/api/admin/device-sources'){
   if(request.method()==='POST'){
    const input=request.postDataJSON();commands.push(input);assert.equal(request.headers()['x-csrf-token'],'a'.repeat(64))
    if(lost){lost=false;sources=[{id:'source-one',name:input.name,type:input.type,location_id:'source-location',location_name:input.name}];await route.abort('failed');return}
    body={source:sources[0],revision:'2',replayed:true}
   }else if(unavailable){status=503;body={error:{code:'device.unavailable'}}}
   else body={revision:'2',sources:malformed?[...sources,...sources]:sources}
  }else throw Error('Unexpected source request '+path)
  await route.fulfill({status,contentType:'application/json',body:JSON.stringify(body)})
 })
 const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message));const open=async()=>{await page.goto(origin+'/warehouses');await page.getByRole('button',{name:'Device sources',exact:true}).click();return page.getByRole('dialog')}
 let drawer=await open();await drawer.getByRole('button',{name:'New device source',exact:true}).click();await drawer.getByLabel('Name',{exact:true}).fill('Retry source');await drawer.getByRole('button',{name:'Save',exact:true}).click();await expect(drawer.getByText('The result is unconfirmed. Retry the original save to recover it.',{exact:true})).toBeVisible();await expect(drawer.getByLabel('Name',{exact:true})).toBeDisabled();await expect(drawer.getByRole('button',{name:'Reload',exact:true})).toBeDisabled();await drawer.getByRole('button',{name:'Retry this save',exact:true}).click();await expect(page.getByRole('dialog').getByRole('cell',{name:'Retry source',exact:true})).toBeVisible();assert.equal(commands.length,2);assert.deepEqual(commands[0],commands[1])
 readonly=true;drawer=await open();await expect(drawer.getByRole('cell',{name:'Retry source',exact:true})).toBeVisible();await expect(drawer.getByRole('button',{name:'New device source',exact:true})).toHaveCount(0)
 unavailable=true;drawer=await open();await expect(drawer.getByRole('button',{name:'Reconnect',exact:true})).toBeVisible();unavailable=false;await drawer.getByRole('button',{name:'Reconnect',exact:true}).click();await expect(drawer.getByRole('cell',{name:'Retry source',exact:true})).toBeVisible()
 malformed=true;drawer=await open();await expect(drawer.getByRole('button',{name:'Reconnect',exact:true})).toBeVisible();await expect(drawer.getByRole('cell',{name:'Retry source',exact:true})).toHaveCount(0);assert.deepEqual(errors,[])
 console.log('DEVICE_SOURCES_ADMIN_BROWSER '+JSON.stringify({real_react_antd:true,synthetic_api:true,real_gin_joint:false,lost_response_same_request_retry:true,uncertain_form_locked:true,read_only_controls:true,unavailable_retry:true,duplicate_projection_rejected:true,browser_errors:0}))
}finally{await browser?.close();await server?.close()}
