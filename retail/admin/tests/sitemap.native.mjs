import assert from 'node:assert/strict'
import {chromium} from '@playwright/test'
import {mkdir,writeFile} from 'node:fs/promises'
import {createHash} from 'node:crypto'
import {fileURLToPath} from 'node:url'
const output=fileURLToPath(new URL('../../../logs/gin-migration/sitemap-'+Date.now()+'/',import.meta.url))
await mkdir(output,{recursive:true,mode:0o700})
const browser=await chromium.launch({channel:'chrome',headless:true}),errors=[],observations=[]
const normalize=value=>value.replace(/http:\/\/(?:localhost|127\.0\.0\.1):(?:8000|9300)(?=[/?"\s]|$)/g,'ORIGIN')
const sha=value=>createHash('sha256').update(value).digest('hex')
try{
 const context=await browser.newContext(),page=await context.newPage();page.on('pageerror',e=>errors.push(e.message))
 const parse=async body=>page.evaluate(body=>{
  const doc=new DOMParser().parseFromString(body,'application/xml')
  return {error:!!doc.querySelector('parsererror'),root:doc.documentElement.localName,children:[...doc.querySelectorAll('sitemap > loc')].map(n=>n.textContent),urls:[...doc.querySelectorAll('url')].map(n=>({loc:n.querySelector('loc')?.textContent,lastmod:n.querySelector('lastmod')?.textContent??null,languages:[...n.getElementsByTagNameNS('http://www.w3.org/1999/xhtml','link')].map(a=>({lang:a.getAttribute('hreflang'),href:a.getAttribute('href')}))}))}
 },body)
 const request=async(port,path,status=200)=>{
  const r=await context.request.get(`http://127.0.0.1:${port}${path}`,{timeout:60000}),body=await r.text()
  assert.equal(r.status(),status,`${port}${path}`)
  if(port===9300)assert.equal(r.headers()['cache-control'],'no-store')
  if(status!==200)return null
  assert.equal(r.headers()['content-type'],'application/xml; charset=utf-8')
  const doc=await parse(body);assert.equal(doc.error,false,`${port}${path}`)
  observations.push({port,path,status:r.status(),bytes:Buffer.byteLength(body),sha256:sha(body),root:doc.root,urls:doc.urls.length,children:doc.children.length})
  return doc
 }
 const all={}
 for(const port of [8000,9300]){
  const index=await request(port,'/sitemap.xml'),urls=[]
  if(index.root==='sitemapindex'){
   assert.ok(index.children.length>1)
   for(const child of index.children){
    const u=new URL(child);assert.ok(['localhost','127.0.0.1'].includes(u.hostname));assert.equal(u.port,String(port));assert.equal(u.pathname,'/sitemap.xml')
    const part=await request(port,u.pathname+u.search);assert.equal(part.root,'urlset');assert.ok(part.urls.length>0&&part.urls.length<=1000);urls.push(...part.urls)
   }
  }else{assert.equal(index.root,'urlset');urls.push(...index.urls)}
  const projected=urls.map(row=>JSON.parse(normalize(JSON.stringify(row)))).sort((a,b)=>a.loc.localeCompare(b.loc))
  assert.equal(new Set(projected.map(row=>row.loc)).size,projected.length)
  all[port]={parts:index.children.length||1,urls:projected}
 }
 assert.equal(all[9300].parts,all[8000].parts);assert.equal(all[9300].urls.length,all[8000].urls.length)
 for(let i=0;i<all[8000].urls.length;i++)assert.deepEqual(all[9300].urls[i],all[8000].urls[i],`source document ${i}`)
 for(const part of ['0','-1','invalid',String(all[9300].parts+1)])await request(9300,'/sitemap.xml?part='+part,404)
 for(const path of ['/sitemap.xml','/sitemap.xml?part=1']){
  const r=await page.goto('http://127.0.0.1:9300'+path,{waitUntil:'domcontentloaded'});assert.equal(r.status(),200)
  assert.equal(await page.evaluate(()=>document.contentType),'application/xml')
 }
 assert.deepEqual(errors,[])
 const raw=JSON.stringify({all,observations,errors},null,2)+'\n'
 await writeFile(output+'observations.json',raw,{mode:0o600})
 await writeFile(output+'report.json',JSON.stringify({source_parts:all[8000].parts,astro_parts:all[9300].parts,source_urls:all[8000].urls.length,astro_urls:all[9300].urls.length,loc_lastmod_languages_equal:true,ordering_compared:false,invalid_parts_checked:4,browser_xml_routes:2,observations_sha256:sha(raw),requests:observations,errors,api_mocking:false,business_writes:false,real_provider_called:false,configured_overrides_browser_verified:false,full_seo_accepted:false,goal_complete:false},null,2)+'\n')
 console.log(JSON.stringify({output,parts:all[9300].parts,urls:all[9300].urls.length,passed:true}))
 await context.close()
}finally{await browser.close()}
