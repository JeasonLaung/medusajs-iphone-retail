import {test} from 'node:test'
import assert from 'node:assert/strict'
import {seoArtifact} from '../src/lib/seo-artifacts.ts'
const origin='http://localhost:9100'
const settings={'seo.site_url':'https://shop.example'}
const publication=(values:Record<string,string>={}):typeof fetch=>async(input,init)=>{
 assert.equal(String(input),origin+'/api/store/seo?path=%2F');assert.equal(init?.credentials,'omit');assert.equal(init?.cache,'no-store')
 return Response.json({settings:{...settings,...values},page:null})
}
test('default discovery files retain source exclusions, canonical origin and home links',async()=>{
 const robots=await seoArtifact('robots',origin,undefined,publication())
 assert.equal(robots.status,200);assert.equal(robots.headers.get('cache-control'),'no-store');assert.equal(robots.headers.get('content-type'),'text/plain; charset=utf-8')
 assert.equal(await robots.text(),'User-agent: *\nAllow: /\nDisallow: /*/account\nDisallow: /*/checkout\nDisallow: /*/cart\nDisallow: /*/order\nDisallow: /api/\nSitemap: https://shop.example/sitemap.xml\n')
 const llms=await seoArtifact('llms',origin,undefined,publication({'seo.site_name':'翻新店','seo.home_path':'/dk-zh','seo.default_description':'机型与库存'}))
 assert.match(await llms.text(),/^# 翻新店\n\n机型与库存\n[\s\S]*https:\/\/shop.example\/dk-zh\/store/)
})
test('configured text remains verbatim and verification requires explicit matching switches',async()=>{
 for(const [kind,key]of [['robots','seo.robots_txt'],['llms','seo.llms_txt']]as const){
  const r=await seoArtifact(kind,origin,undefined,publication({[key]:'原始\r\n文本 & <xml>\n'}));assert.equal(await r.text(),'原始\r\n文本 & <xml>\n')
 }
 const values={'bing.site_auth_xml':'<?xml version="1.0"?><users><user>test</user></users>','seo.indexnow_key':'test-key-1234'}
 assert.equal((await seoArtifact('bing',origin,undefined,publication(values))).status,404)
 assert.equal((await seoArtifact('indexnow',origin,values['seo.indexnow_key'],publication(values))).status,404)
 const bing=await seoArtifact('bing',origin,undefined,publication({...values,'bing.verification_enabled':'true'}))
 assert.equal(bing.status,200);assert.equal(bing.headers.get('content-type'),'application/xml; charset=utf-8');assert.equal(await bing.text(),values['bing.site_auth_xml'])
 const enabled=publication({...values,'seo.indexnow_enabled':'true'})
 for(const key of ['wrong-key-123','short','../test-key-1234'])assert.equal((await seoArtifact('indexnow',origin,key,enabled)).status,404)
 assert.equal(await(await seoArtifact('indexnow',origin,values['seo.indexnow_key'],enabled)).text(),values['seo.indexnow_key'])
})
test('unavailable configuration is not published as a successful empty discovery file',async()=>{
 for(const kind of ['robots','llms','bing','indexnow']as const){
  const r=await seoArtifact(kind,origin,'test-key-1234',async()=>new Response('private diagnostic',{status:503}))
  assert.equal(r.status,503);assert.equal(r.headers.get('cache-control'),'no-store');assert.equal(await r.text(),'SEO publication unavailable')
 }
})
