import {test} from 'node:test'
import assert from 'node:assert/strict'
import {loadDiscoveryCatalog,discoveryPages,sitemapResponse,loadSitemap,type DiscoveryEntry,type DiscoveryPage} from '../src/lib/sitemap.ts'
import {getHomeCopy} from '../src/components/journal/i18n.ts'
const settings={'seo.site_url':'https://shop.example'},date='2026-02-03T04:05:06.123456Z'
const entry=(handle:string,slugs:string[]=[]):DiscoveryEntry=>({handle,updated_at:date,slugs})
const page=(path:string,extra:Partial<DiscoveryPage>={}):DiscoveryPage=>({path,robots:'index,follow',canonical:'',hreflang:null,updated_at:'2027-01-01T00:00:00Z',...extra})
const render=(products:DiscoveryEntry[],collections:DiscoveryEntry[]=[],pages:DiscoveryPage[]=[],part='')=>sitemapResponse(new URL('https://shop.example/sitemap.xml'+part),settings,['dk'],products,collections,pages)
test('source paths include model/SKU, active empty collections and translated blog paths with canonical alternates',async()=>{
 const r=render([entry('phone & 中文',['iphone-15-128gb-blue-S85','iphone-15-128gb-blue-S85'])],[entry('empty-collection')])
 assert.equal(r.status,200);assert.equal(r.headers.get('cache-control'),'no-store');assert.equal(r.headers.get('content-type'),'application/xml; charset=utf-8')
 const body=await r.text()
 for(const language of ['en','zh']){
  for(const suffix of ['', '/store','/about','/help','/reviews','/blog','/sell',...getHomeCopy(language).blogPosts.map(p=>'/blog/'+p.slug),'/products/phone%20%26%20%E4%B8%AD%E6%96%87','/products/iphone-15-128gb-blue-S85','/collections/empty-collection'])assert.equal(body.split(`<loc>https://shop.example/dk-${language}${suffix}</loc>`).length-1,1)
 }
 assert.match(body,/<lastmod>2026-02-03T04:05:06.123Z<\/lastmod>/)
 assert.match(body,/<xhtml:link rel="alternate" hreflang="zh-CN" href="https:\/\/shop.example\/dk-zh\/products\/iphone-15-128gb-blue-S85"\/>/)
 assert.equal(body.includes('/account'),false);assert.equal(body.includes('/checkout'),false)
})
test('page overrides remove noindex and foreign canonicals, preserve self canonical and escape language URLs',async()=>{
 const pages=[page('/dk-en/about',{robots:'noindex,follow'}),page('/dk-en/help',{canonical:'https://external.example/help'}),page('/dk-en/products/phone',{canonical:'https://shop.example/dk-en/products/phone/',hreflang:{en:'https://shop.example/phone?a=1&b=2'}})]
 const body=await render([entry('phone')],[],pages).text()
 assert.equal(body.includes('<loc>https://shop.example/dk-en/about</loc>'),false);assert.equal(body.includes('<loc>https://shop.example/dk-en/help</loc>'),false)
 assert.ok(body.includes('<loc>https://shop.example/dk-zh/about</loc>'))
 assert.ok(body.includes('<loc>https://shop.example/dk-en/products/phone</loc><lastmod>2027-01-01T00:00:00.000Z</lastmod><xhtml:link rel="alternate" hreflang="en" href="https://shop.example/phone?a=1&amp;b=2"/>'))
})
test('source partition boundaries emit an index, 1000 URLs, the remainder and 404 for invalid/out-of-range parts',async()=>{
 const products=Array.from({length:600},(_,i)=>entry('phone-'+i)),total=2*(7+getHomeCopy('en').blogPosts.length+600)
 const index=await render(products).text();assert.ok(index.includes('<sitemapindex'));assert.ok(index.includes('?part=2'));assert.equal(index.includes('?part=3'),false)
 const first=await render(products,[],[],'?part=1').text(),last=await render(products,[],[],'?part=2').text()
 assert.equal(first.match(/<url>/g)?.length,1000);assert.equal(last.match(/<url>/g)?.length,total-1000)
 assert.equal(new Set([...first.matchAll(/<loc>(.*?)<\/loc>/g),...last.matchAll(/<loc>(.*?)<\/loc>/g)].map(m=>m[1])).size,total)
 for(const part of ['0','-1','1.5','3','abc',''])assert.equal(render(products,[],[],'?part='+part).status,404)
 assert.equal(render(products,[],[],'?part=1e0').status,200)
 assert.equal(sitemapResponse(new URL('https://shop.example/sitemap.xml'),settings,[],[],[],[]).status,404)
})
test('catalog loader follows actual offset pages and rejects changing/incomplete results instead of publishing a partial map',async()=>{
 const calls:string[]=[],fetcher:typeof fetch=async(input,init)=>{
  const url=new URL(String(input));calls.push(url.pathname+url.search);assert.equal(init?.credentials,'omit');assert.equal(init?.redirect,'error')
  const offset=Number(url.searchParams.get('offset'));return Response.json({count:101,limit:100,offset,entries:Array.from({length:offset===0?100:1},(_,i)=>entry('phone-'+(offset+i)))})
 }
 assert.equal((await loadDiscoveryCatalog('http://localhost:9100','products',fetcher)).length,101);assert.equal(calls.length,2);assert.ok(calls[1].endsWith('offset=100'))
 for(const raw of [{count:1,limit:100,offset:0,entries:[]},{count:1,limit:100,offset:0,entries:[{...entry('phone'),updated_at:'invalid'}]},{count:1,limit:100,offset:0,entries:[entry('phone',['javascript:bad'])]}])await assert.rejects(loadDiscoveryCatalog('http://localhost:9100','products',async()=>Response.json(raw)))
 const response=await loadSitemap(new URL('https://shop.example/sitemap.xml'),'http://localhost:9100',async()=>new Response('private diagnostic',{status:503}))
 assert.equal(response.status,503);assert.equal(response.headers.get('cache-control'),'no-store');assert.equal(await response.text(),'Sitemap unavailable')
})
test('discovery overrides reject duplicate paths, malformed JSON fields and unsafe language links',()=>{
 assert.equal(discoveryPages({pages:[page('/dk-en/about')]},settings).length,1)
 for(const pages of [[page('/dk-en/about'),page('/dk-en/about')],[page('//other')],[page('/x',{hreflang:{en:'javascript:bad'}})],[page('/x',{updated_at:'invalid'})],[page('/x',{robots:'invalid'})]])assert.throws(()=>discoveryPages({pages},settings))
})
