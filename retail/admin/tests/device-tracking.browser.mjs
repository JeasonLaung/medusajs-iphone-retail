import assert from 'node:assert/strict'
import {createServer} from 'vite'
import {chromium,expect} from '@playwright/test'
let server,browser
const origin='http://127.0.0.1:19384',csrf='a'.repeat(64),errors=[],commands=[]
const product={id:'tracking-product',revision:'a'.repeat(64),title:'Device fixture',handle:'tracking-product',status:'draft',thumbnail:null,collection_id:null,created_at:'2026-10-28T00:00:00Z',updated_at:'2026-10-28T00:00:00Z',variant_count:1,subtitle:null,description:null,options:[],images:[]}
const variant={id:'tracking-variant',product_id:product.id,title:'Device SKU',sku:'DEVICE-SKU',thumbnail:null,variant_rank:0,manage_inventory:true,allow_backorder:false,configuration_complete:true,options:[],images:[],unassigned_option_value_ids:[]}
let tracking={product_id:product.id,variant_id:variant.id,revision:'a'.repeat(64),enabled:false,eligible:true,reason:'',inventory_item_id:'stock-one'}
let lost=true,readonly=false,conflict=false,unavailable=false,malformed=false
try{
 server=await createServer({server:{port:19384,strictPort:true,host:'127.0.0.1'},logLevel:'error'});await server.listen()
 browser=await chromium.launch({headless:true,executablePath:process.env.ADMIN_TEST_BROWSER||'C:/Program Files/Google/Chrome/Application/chrome.exe'})
 const context=await browser.newContext({reducedMotion:'reduce'});await context.addCookies([{name:'retail_admin_csrf',value:csrf,url:origin}]);await context.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','en'))
 await context.route('**/api/admin/**',async route=>{
  const request=route.request(),path=new URL(request.url()).pathname;let body,status=200
  if(path==='/api/admin/auth/me')body={user:{id:'operator',name:'Operator',email:'operator@example.invalid',role:'admin',enabled:true}}
  else if(path==='/api/admin/auth/permissions')body={revision:'1',scope:'employee',actions:['product.read','inventory.read','device.read',...(readonly?[]:['device.enroll'])]}
  else if(path.endsWith('/device-tracking')){
   if(request.method()==='POST'){
    assert.equal(request.headers()['x-csrf-token'],csrf);commands.push(request.postDataJSON())
    if(conflict){status=409;body={error:{code:'device.conflict'}}}
    else {tracking={...tracking,enabled:true,eligible:false,reason:'already_enabled',revision:'b'.repeat(64)};if(lost){lost=false;await route.abort('failed');return};body={tracking,replayed:true}}
   }else if(unavailable){status=503;body={error:{code:'device.unavailable'}}}
   else body={...tracking,...(malformed?{variant_id:'other-variant'}:{})}
  }else if(path.endsWith('/variants'))body={variants:[variant],count:1,limit:20,offset:0}
  else if(path==='/api/admin/products/tracking-product')body={product}
  else throw Error('Unexpected tracking request '+path)
  await route.fulfill({status,contentType:'application/json',body:JSON.stringify(body)})
 })
 const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message))
 const open=async()=>{await page.goto(origin+'/products/tracking-product');await page.getByRole('button',{name:'Device tracking',exact:true}).click();return page.getByRole('dialog')}
 let drawer=await open();await drawer.getByRole('button',{name:'Enable individual device tracking',exact:true}).click();await expect(drawer.getByText('The result is unconfirmed. Retry to recover the original operation.',{exact:true})).toBeVisible();await expect(drawer.getByRole('button',{name:'Reload',exact:true})).toBeDisabled();await drawer.getByRole('button',{name:'Retry this operation',exact:true}).click();await expect(drawer.getByText('Enabled',{exact:true})).toBeVisible();assert.equal(commands.length,2);assert.deepEqual(commands[0],commands[1]);assert.match(commands[0].operation_id,/^[0-9a-f-]{36}$/)
 await page.reload();await page.getByRole('button',{name:'Device tracking',exact:true}).click();await expect(page.getByRole('dialog').getByText('Enabled',{exact:true})).toBeVisible()
 tracking={...tracking,enabled:false,eligible:true,reason:''};readonly=true;drawer=await open();await expect(drawer.getByText('Disabled',{exact:true})).toBeVisible();await expect(drawer.getByRole('button',{name:'Enable individual device tracking',exact:true})).toHaveCount(0)
 readonly=false;tracking={...tracking,eligible:false,reason:'shared_inventory_item'};drawer=await open();await expect(drawer.getByText('Another SKU uses this inventory item. Individual tracking cannot be enabled.',{exact:true})).toBeVisible();await expect(drawer.getByRole('button',{name:'Enable individual device tracking',exact:true})).toHaveCount(0)
 tracking={...tracking,eligible:true,reason:''};conflict=true;drawer=await open();await drawer.getByRole('button',{name:'Enable individual device tracking',exact:true}).click();await expect(drawer.getByText('Permissions or SKU configuration changed. Reload before continuing.',{exact:true})).toBeVisible();await expect(drawer.getByRole('button',{name:'Enable individual device tracking',exact:true})).toHaveCount(0);conflict=false;await drawer.getByRole('button',{name:'Reload',exact:true}).click();await expect(drawer.getByRole('button',{name:'Enable individual device tracking',exact:true})).toBeVisible()
 unavailable=true;drawer=await open();await expect(drawer.getByRole('button',{name:'Reconnect',exact:true})).toBeVisible();unavailable=false;await drawer.getByRole('button',{name:'Reconnect',exact:true}).click();await expect(drawer.getByRole('button',{name:'Enable individual device tracking',exact:true})).toBeVisible()
 malformed=true;drawer=await open();await expect(drawer.getByRole('button',{name:'Reconnect',exact:true})).toBeVisible();await expect(drawer.getByRole('button',{name:'Enable individual device tracking',exact:true})).toHaveCount(0);malformed=false
 await page.setViewportSize({width:390,height:844});drawer=await open();await expect(drawer.getByRole('button',{name:'Enable individual device tracking',exact:true})).toBeVisible();assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
 await context.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','zh-CN'));await page.goto(origin+'/products/tracking-product');await page.getByRole('button',{name:'设备追踪',exact:true}).click();await expect(page.getByRole('dialog').getByRole('button',{name:'开启单台设备追踪',exact:true})).toBeVisible()
 assert.deepEqual(errors,[])
 console.log('DEVICE_TRACKING_ADMIN_BROWSER '+JSON.stringify({real_react_antd:true,synthetic_api:true,real_gin_joint:false,original_product_page:true,lost_response_same_request_retry:true,read_only_controls:true,shared_item_explained:true,conflict_reload:true,unavailable_retry:true,wrong_variant_rejected:true,language_en_zh:true,mobile_no_page_overflow:true,browser_errors:0}))
}finally{await browser?.close();await server?.close()}
