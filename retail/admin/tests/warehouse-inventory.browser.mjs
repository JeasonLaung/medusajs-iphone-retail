import assert from 'node:assert/strict'
import {createServer} from 'vite'
import {chromium,expect} from '@playwright/test'
let server,browser
const origin='http://127.0.0.1:19382',csrf='d'.repeat(64),errors=[],saves=[]
let lost=true,readonly=false,empty=false,unavailable=false,conflict=false,malformed=false
const quantity='100.000000000000000001'
let level={id:'warehouse-level',inventory_item_id:'warehouse-item',sku:'WAREHOUSE-SKU',title:'Scoped stock',location_id:'warehouse-a',stocked_quantity:quantity,reserved_quantity:'0.000000000000000000',incoming_quantity:'1.250000000000000000',available_quantity:quantity,revision:'a'.repeat(64)}
try{
 server=await createServer({server:{port:19382,strictPort:true,host:'127.0.0.1'},logLevel:'error'});await server.listen()
 browser=await chromium.launch({headless:true,executablePath:process.env.ADMIN_TEST_BROWSER||'C:/Program Files/Google/Chrome/Application/chrome.exe'})
 const context=await browser.newContext({reducedMotion:'reduce'});await context.addCookies([{name:'retail_admin_csrf',value:csrf,url:origin}])
 await context.route('**/api/admin/**',async route=>{
  const request=route.request(),path=new URL(request.url()).pathname;let body,status=200
  if(path==='/api/admin/auth/me')body={user:{id:'warehouse-user',name:'Warehouse operator',email:'warehouse@example.invalid',role:'admin',enabled:true}}
  else if(path==='/api/admin/auth/permissions')body={revision:'1',scope:'warehouse',actions:[]}
  else if(path==='/api/admin/warehouse/access'){
   if(unavailable){status=503;body={error:{code:'authorization.unavailable'}}}
   else body={revision:'1',actions:readonly?['inventory.read']:['inventory.read','inventory.stock.write'],locations:empty?[]:[{id:'warehouse-a',name:'Warehouse A'}]}
  }else if(path==='/api/admin/warehouse/locations/warehouse-a/inventory'){
   const offset=Number(new URL(request.url()).searchParams.get('offset'))
   body={location_id:'warehouse-a',levels:offset?[{...level,id:'warehouse-level-last'}]:[{...level,...(malformed?{location_id:'warehouse-b'}:{})}],limit:50,offset,has_more:offset===0,writes_enabled:true}
  }else if(path==='/api/admin/warehouse/locations/warehouse-a/inventory/warehouse-level/stock'){
   assert.equal(request.headers()['x-csrf-token'],csrf);const input=request.postDataJSON();saves.push(input)
   if(conflict){status=409;body={error:{code:'inventory.edit_conflict'}}}
   else{
    if(lost){lost=false;level={...level,stocked_quantity:input.stocked_quantity,incoming_quantity:input.incoming_quantity,available_quantity:input.stocked_quantity,revision:'b'.repeat(64)};await route.abort('failed');return}
    body={revision:'b'.repeat(64),replayed:true}
   }
  }else throw Error('Unexpected scoped inventory request '+path)
  await route.fulfill({status,contentType:'application/json',body:JSON.stringify(body)})
 })
 await context.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','en'))
 const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message))
 await page.goto(origin+'/account');await expect(page.getByRole('menuitem',{name:'Warehouse inventory',exact:true})).toBeVisible()
 await expect(page.getByRole('menuitem',{name:'Products',exact:true})).toHaveCount(0)
 await page.getByRole('menuitem',{name:'Warehouse inventory',exact:true}).click();await expect(page.getByRole('cell',{name:'WAREHOUSE-SKU',exact:true})).toBeVisible()
 assert.ok((await page.textContent('body')).includes('100.000000000000000001'))
 await page.getByRole('button',{name:'Next page',exact:true}).click();await expect(page.getByRole('button',{name:'Next page',exact:true})).toBeDisabled();await page.getByRole('button',{name:'Previous page',exact:true}).click()
 await page.getByRole('button',{name:'Edit stock',exact:true}).click();const dialog=page.getByRole('dialog')
 await dialog.getByLabel('Stocked quantity',{exact:true}).fill('99.000000000000000001')
 await dialog.getByRole('button',{name:'Save',exact:true}).click();await expect(dialog.getByRole('button',{name:'Retry this save',exact:true})).toBeVisible()
 await expect(dialog.getByLabel('Stocked quantity',{exact:true})).toBeDisabled();await dialog.getByRole('button',{name:'Retry this save',exact:true}).click()
 await expect(dialog).not.toBeVisible();assert.equal(saves.length,2);assert.deepEqual(saves[0],saves[1]);assert.equal(saves[0].stocked_quantity,'99.000000000000000001')
 await page.reload();await expect(page.getByRole('cell',{name:'WAREHOUSE-SKU',exact:true})).toBeVisible();assert.ok((await page.textContent('body')).includes('99.000000000000000001'))
 conflict=true;await page.getByRole('button',{name:'Edit stock',exact:true}).click();await dialog.getByRole('button',{name:'Save',exact:true}).click();await expect(dialog.getByRole('button',{name:'Save',exact:true})).toBeDisabled();await dialog.getByRole('button',{name:'Cancel',exact:true}).click();conflict=false
 readonly=true;await page.reload();await expect(page.getByRole('cell',{name:'WAREHOUSE-SKU',exact:true})).toBeVisible();await expect(page.getByRole('button',{name:'Edit stock',exact:true})).toHaveCount(0)
 empty=true;await page.reload();await expect(page.getByText('No selected warehouses means no warehouse access.',{exact:true})).toBeVisible();await expect(page.getByRole('cell',{name:'WAREHOUSE-SKU',exact:true})).toHaveCount(0);empty=false
 malformed=true;await page.reload();await expect(page.getByRole('alert')).toBeVisible();await expect(page.getByRole('cell',{name:'WAREHOUSE-SKU',exact:true})).toHaveCount(0);malformed=false
 await page.setViewportSize({width:390,height:844});await page.reload();await expect(page.getByRole('cell',{name:'WAREHOUSE-SKU',exact:true})).toBeVisible();assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false)
 await context.addInitScript(()=>localStorage.setItem('eurorefurb.admin.language','zh-CN'));await page.reload();await expect(page.getByRole('heading',{name:'仓库库存',exact:true})).toBeVisible()
 unavailable=true;await page.reload();await expect(page.getByRole('button',{name:'重新连接',exact:true})).toBeVisible();unavailable=false;await page.getByRole('button',{name:'重新连接',exact:true}).click();await expect(page.getByRole('cell',{name:'WAREHOUSE-SKU',exact:true})).toBeVisible()
 assert.deepEqual(errors,[])
 console.log('WAREHOUSE_INVENTORY_ADMIN_BROWSER '+JSON.stringify({real_react_antd:true,synthetic_api:true,real_gin_joint:false,warehouse_menu_only:true,exact_decimal:true,pagination:true,edit_reload:true,lost_response_same_request_retry:true,conflict_visible:true,read_only_controls:true,empty_scope:true,cross_location_response_rejected:true,language_en_zh:true,mobile_no_page_overflow:true,load_failure_retry:true,browser_errors:0}))
}catch(error){const p=browser?.contexts()[0]?.pages()[0];if(p)await p.screenshot({path:'../../logs/gin-migration/warehouse-inventory-browser-failure.png',fullPage:true});throw error}finally{await browser?.close();await server?.close()}
