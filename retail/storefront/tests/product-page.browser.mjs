import assert from 'node:assert/strict'
import {createServer} from 'node:http'
import {createServer as portProbe} from 'node:net'
import {once} from 'node:events'
import {spawn} from 'node:child_process'
import {createRequire} from 'node:module'
import {fileURLToPath} from 'node:url'
import {product as sourceProduct} from './selection-fixture/src/product.ts'

// Exercise the built, formal route (not a separate preview/test Astro root).
// This server owns synthetic public API data and never accesses live accounts.
const require=createRequire(new URL('../../admin/package.json',import.meta.url))
const {chromium,expect}=require('@playwright/test')
const product=structuredClone(sourceProduct)
product.description='Plain description <script>window.injected=true</script>'
product.images.forEach(image=>image.url='/iphone-thumbnails/iphone-14/blue.webp')
for(const variant of product.variants)variant.images.forEach(image=>image.url='/iphone-thumbnails/iphone-14/blue.webp')
product.images.push(...product.variants.flatMap(variant=>variant.images))
// The shared pure-selection fixture also exercises beyond-Number amounts;
// use normal handset prices for this real page's mobile layout checks.
product.variants.forEach((variant,index)=>variant.calculated_price.calculated_amount=String(499+index*100))
const region={id:'region',name:'Europe',currency_code:'eur',automatic_taxes:false,countries:[{iso_2:'dk',iso_3:'dnk',num_code:'208',name:'Denmark',display_name:'Denmark',region_id:'region'}]}
const upstreamRequests=[]
let dependencyFailed=false,child,browser
const api=createServer((request,response)=>{
 upstreamRequests.push({url:request.url,headers:request.headers})
 if(dependencyFailed){response.writeHead(503,{'content-type':'application/json'});response.end('{"error":"PRIVATE_UPSTREAM_FAILURE"}');return}
 const url=new URL(request.url,'http://fixture.invalid')
 let body
 switch(url.pathname){
  case '/api/store/regions':body={regions:[region],count:1};break
  case '/api/store/locales':body={locales:[],count:0};break
  case '/api/store/product-categories':body={product_categories:[],count:0,limit:2,offset:0};break
  case '/api/store/collections':body={collections:[],count:0,limit:6,offset:0};break
  case '/api/store/products':{
   const found=url.searchParams.get('handle')===product.handle
   body={products:found?[product]:[],count:found?1:0,limit:2,offset:0,capabilities:{pricing:true,availability:true,checkout:false}};break
  }
  default:response.writeHead(404);response.end();return
 }
 response.writeHead(200,{'content-type':'application/json'});response.end(JSON.stringify(body))
})
try{
 api.listen(0,'127.0.0.1');await once(api,'listening')
 const probe=portProbe();probe.listen(0,'127.0.0.1');await once(probe,'listening')
 const port=probe.address().port;await new Promise((resolve,reject)=>probe.close(error=>error?reject(error):resolve()))
 const origin=`http://127.0.0.1:${port}`
 child=spawn(process.execPath,['dist/server/entry.mjs'],{env:{...process.env,HOST:'127.0.0.1',PORT:String(port),GIN_API_ORIGIN:`http://127.0.0.1:${api.address().port}`,ASTRO_TELEMETRY_DISABLED:'1'},stdio:['ignore','ignore','pipe'],windowsHide:true})
 let output='';child.stderr.on('data',chunk=>{output=(output+chunk).slice(-4000)})
 child.on('error',()=>{})
 let started=false
 for(let i=0;i<100;i++){
  assert.equal(child.exitCode,null,'Owned Astro exited before readiness')
  try{if((await fetch(origin+'/health')).status===200){started=true;break}}catch{}
  await new Promise(resolve=>setTimeout(resolve,100))
 }
 assert.ok(started,'Owned Astro did not become ready')
 const path='/dk-en/products/'+product.handle
 const reply=await fetch(origin+path,{headers:{Cookie:'retail_admin_session=PRIVATE_COOKIE',Authorization:'Bearer PRIVATE_AUTH'}})
 assert.equal(reply.status,200);assert.equal(reply.headers.get('cache-control'),'no-store');assert.equal(reply.headers.get('set-cookie'),null)
 const html=await reply.text()
 assert.ok(html.includes('data-testid="product-container"'));assert.ok(html.includes('All iPhones'))
 assert.ok(!html.includes('<script>window.injected=true</script>'))
 assert.ok(!html.includes('PRIVATE_COOKIE'));assert.ok(!html.includes('PRIVATE_AUTH'))
 assert.ok(upstreamRequests.every(request=>!request.headers.cookie&&!request.headers.authorization))
 assert.equal((await fetch(origin+'/dk-en/products/missing')).status,404)
 assert.equal((await fetch(origin+'/zz-en/products/'+product.handle)).status,404)
 assert.equal((await fetch(origin+'/dk-fr/products/'+product.handle)).status,404)
 browser=await chromium.launch({headless:true,...(process.env.ADMIN_TEST_BROWSER?{executablePath:process.env.ADMIN_TEST_BROWSER}:{})})
 const page=await browser.newPage(),errors=[]
 page.on('pageerror',error=>errors.push(error.message))
 // No third-party font requests are needed to verify navigation/hydration.
 await page.route('https://**/*',route=>route.abort())
 await page.goto(origin+path+'?campaign=a&campaign=b&v_id=v128#product-info')
 await expect(page.getByRole('heading',{name:product.title,exact:true})).toBeVisible()
 await page.getByRole('radio',{name:/256/}).check()
 await expect(page).toHaveURL(/iphone-13-256gb-blue-S85\?campaign=a&campaign=b#product-info$/)
 await expect(page.locator('link[rel="canonical"]')).toHaveAttribute('href',origin+'/dk-en/products/iphone-13-256gb-blue-S85')
 await expect(page.getByRole('button',{name:'Add to cart',exact:true}).first()).toBeDisabled()
 await page.reload()
 await expect(page.getByRole('radio',{name:/256/})).toBeChecked()
 assert.deepEqual(errors,[])
 await page.screenshot({path:fileURLToPath(new URL('../../../logs/gin-migration/formal-product-desktop.png',import.meta.url)),fullPage:true})
 await page.setViewportSize({width:390,height:844})
 await page.goto(origin+'/dk-zh/products/'+product.handle)
 await expect(page.getByRole('navigation',{name:'面包屑导航'})).toBeVisible()
 await expect(page.locator('html')).toHaveAttribute('lang','zh-CN')
 assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),'Mobile page overflows')
 await page.screenshot({path:fileURLToPath(new URL('../../../logs/gin-migration/formal-product-mobile.png',import.meta.url)),fullPage:true})
 dependencyFailed=true
 const failure=await fetch(origin+path)
 assert.equal(failure.status,503);assert.equal(failure.headers.get('cache-control'),'no-store')
 assert.ok(!(await failure.text()).includes('PRIVATE_UPSTREAM_FAILURE'))
 console.log('ASTRO_FORMAL_PRODUCT_PAGE '+JSON.stringify({formal_built_route:true,public_gin_contract:true,ssr_header_footer:true,private_headers_omitted:true,english_chinese:true,selection_hydration:true,slug_reload:true,query_hash_preserved:true,disabled_unconnected_commerce:true,mobile_no_overflow:true,missing_404_dependency_503:true}))
}finally{
 if(browser)await browser.close()
 if(child&&child.exitCode===null){const ended=once(child,'exit');child.kill();await ended}
 await new Promise(resolve=>api.close(resolve))
}
