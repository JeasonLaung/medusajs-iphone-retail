import assert from 'node:assert/strict'
import {createServer} from 'node:http'
import {spawn} from 'node:child_process'
import {once} from 'node:events'
import {setTimeout as delay} from 'node:timers/promises'

const host='127.0.0.1',port=19372,shop=`http://${host}:${port}`
const region={id:'region',name:'Denmark',currency_code:'eur',automatic_taxes:false,countries:[{iso_2:'dk',iso_3:'dnk',num_code:'208',name:'Denmark',display_name:'Denmark',region_id:'region'}]}
const requests=[],failures=[],cases=[]
let app
const upstream=createServer((req,res)=>{
 try{
  assert.equal(req.headers.cookie,undefined);assert.equal(req.headers.authorization,undefined)
  const url=new URL(req.url,'http://fixture.invalid');requests.push(url.pathname+url.search)
  res.setHeader('Content-Type','application/json')
  if(url.pathname==='/api/store/regions'){res.end(JSON.stringify({regions:[region],count:1}));return}
  assert.equal(url.pathname,'/api/store/product-categories');assert.equal(url.searchParams.get('limit'),'2')
  const handle=url.searchParams.get('handle')
  if(handle==='failed'){res.writeHead(503);res.end('{}');return}
  const rows=handle==='unknown'?[]:[{id:'series',handle,name:handle==='iphone/中文'?'iPhone 中文 & Pro':'iPhone 13 & Pro',parent_category_id:handle==='iphone'?null:'iphone',parent_category:handle==='iphone'?null:{id:'iphone',handle:'iphone'}}]
  res.end(JSON.stringify({product_categories:rows,count:rows.length,limit:2,offset:0}))
 }catch(error){failures.push(error.message);res.writeHead(500);res.end('{}')}
})
try{
 const probe=createServer();probe.listen(port,host);await once(probe,'listening');await new Promise(resolve=>probe.close(resolve))
 upstream.listen(0,host);await once(upstream,'listening')
 app=spawn(process.execPath,['scripts/start.mjs'],{cwd:process.cwd(),env:{...process.env,HOST:host,PORT:String(port),GIN_API_ORIGIN:`http://${host}:${upstream.address().port}`,STOREFRONT_ORIGIN:shop},stdio:'ignore'})
 let ready=false
 for(let n=0;n<100;n++){
  if(app.exitCode!==null)throw new Error('Owned Astro runtime exited')
  try{const r=await fetch(shop+'/health');if([200,503].includes(r.status)){ready=true;break}}catch{}
  await delay(100)
 }
 assert.ok(ready)
 for(const [path,status,location]of [
  ['/dk-en/categories/iphone/13?unrelated=drop',308,'/dk-en/store?series=iPhone+13+%26+Pro'],
  ['/dk-zh/categories/iphone/%E4%B8%AD%E6%96%87',308,'/dk-zh/store?series=iPhone+%E4%B8%AD%E6%96%87+%26+Pro'],
  ['/dk-en/categories/iphone',308,'/dk-en/store'],
  ['/dk-en/categories/unknown',308,'/dk-en/store'],
  ['/dk-en/categories/failed',503,null],
  ['/us-en/categories/iphone/13',404,null],
  ['/dk-fr/categories/iphone/13',404,null],
 ]){
  const response=await fetch(shop+path,{redirect:'manual',headers:{Cookie:'retail_admin_session=PRIVATE; retail_customer_session=PRIVATE',Authorization:'PRIVATE'}})
  assert.equal(response.status,status,path);assert.equal(response.headers.get('location'),location,path)
  if(status!==308)assert.match(await response.text(),status===503?/temporarily unavailable/:/Page not found/)
  cases.push({path,status,location})
 }
 assert.deepEqual(failures,[])
 console.log('ASTRO_CATEGORY_REDIRECT_RUNTIME '+JSON.stringify({cases,upstream_requests:requests,private_credentials_filtered:true,built_formal_route:true,synthetic_api:true,real_gin_mysql:false}))
}finally{
 if(app&&app.exitCode===null){app.kill();await once(app,'exit')}
 await new Promise(resolve=>upstream.close(resolve))
}
