import assert from 'node:assert/strict'
import {createServer} from 'vite'
import {chromium,expect} from '@playwright/test'
let server,browser
const origin='http://127.0.0.1:19385',errors=[]
try{
 server=await createServer({server:{port:19385,strictPort:true,host:'127.0.0.1'},logLevel:'error'});await server.listen()
 browser=await chromium.launch({headless:true,executablePath:process.env.ADMIN_TEST_BROWSER||'C:/Program Files/Google/Chrome/Application/chrome.exe'})
 for(const lang of ['en','zh-CN'])for(const width of [390,1440]){
  const context=await browser.newContext({viewport:{width,height:1000},reducedMotion:'reduce'}),page=await context.newPage()
  await context.addInitScript(lang=>localStorage.setItem('eurorefurb.admin.language',lang),lang)
  page.on('pageerror',e=>errors.push(e.message))
  let denied=false,outage=false,malformed=false,empty=false
  const parcel={id:'scoped-parcel',order_id:'shared-order',reference:'9007199254740993123',location_id:'warehouse-a',created_at:'2026-10-30T00:00:00Z',packed_at:'2026-10-30T01:00:00Z',shipped_at:'2026-10-30T02:00:00Z',delivered_at:null}
  await context.route('**/api/admin/**',async route=>{
   const path=new URL(route.request().url()).pathname;let body,status=200
   assert.equal(route.request().method(),'GET')
   if(path==='/api/admin/auth/me')body={user:{id:'fixture',name:'Warehouse',email:'warehouse@example.invalid',role:'admin',enabled:true}}
   else if(path==='/api/admin/auth/permissions')body={revision:'1',scope:'warehouse',actions:[]}
   else if(path==='/api/admin/warehouse/access')body={revision:'1',actions:['order.fulfillment.read'],locations:empty?[]:[{id:'warehouse-a',name:'Warehouse A'}]}
   else if(path==='/api/admin/warehouse/locations/warehouse-a/fulfillments')body={location_id:'warehouse-a',offset:0,limit:50,has_more:false,fulfillments:[{...parcel,location_id:malformed?'warehouse-b':'warehouse-a'}]}
   else if(path==='/api/admin/warehouse/locations/warehouse-a/orders/shared-order/fulfillments/scoped-parcel'){
    if(denied){status=403;body={error:{code:'authorization.denied'}}}else if(outage){status=503;body={error:{code:'device.unavailable'}}}else body={...parcel,items:[{id:'item',item_id:'line',title:'Phone',sku:'SCOPED-SKU',quantity:'1.000000000000000001'}],devices:[{id:'device',item_id:'line',serial_number:'WAREHOUSE-SN',status:'shipped'}],labels:[{id:'label',tracking_number:'TRACK-SCOPED',tracking_url:'',label_url:''}]}
   }else throw Error('Unexpected scoped API '+path)
   await route.fulfill({status,contentType:'application/json',body:JSON.stringify(body)})
  })
  await page.goto(origin+'/warehouse-fulfillments')
  const panel=page.getByTestId('warehouse-fulfillments'),open=()=>panel.getByRole('button',{name:'scoped-parcel',exact:true}).click()
  await open();const drawer=page.locator('.ant-drawer-content')
  await expect(drawer.getByText('WAREHOUSE-SN',{exact:true})).toBeVisible();await expect(drawer.getByText('TRACK-SCOPED',{exact:true})).toBeVisible();await expect(drawer.getByText('1.000000000000000001',{exact:true})).toBeVisible()
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true)
  await page.getByRole('button',{name:'Close',exact:true}).click()
  outage=true;await open();await expect(drawer.getByRole('alert')).toBeVisible();outage=false;await drawer.getByRole('button',{name:lang==='en'?'Reconnect':'重新连接',exact:true}).click();await expect(drawer.getByText('WAREHOUSE-SN',{exact:true})).toBeVisible()
  await page.getByRole('button',{name:'Close',exact:true}).click();denied=true;await open();await expect(drawer.getByText(lang==='en'?'You do not have permission to access this feature':'没有访问此功能的权限',{exact:true})).toBeVisible()
  await page.getByRole('button',{name:'Close',exact:true}).click();malformed=true;await page.reload();await expect(page.getByRole('alert')).toBeVisible();await expect(page.getByRole('button',{name:'scoped-parcel',exact:true})).toHaveCount(0)
  malformed=false;empty=true;await page.reload();await expect(panel.getByRole('alert')).toBeVisible()
  await context.close()
 }
 assert.deepEqual(errors,[])
 console.log('WAREHOUSE_FULFILLMENTS_BROWSER '+JSON.stringify({formal_admin:true,synthetic_api:true,real_gin_browser_joint:false,languages:['en','zh-CN'],widths:[390,1440],scoped_detail_and_tracking:true,exact_quantity:true,dependency_retry:true,permission_revocation_visible:true,foreign_location_response_rejected:true,empty_scope:true,browser_errors:0}))
}finally{await browser?.close();await server?.close()}
