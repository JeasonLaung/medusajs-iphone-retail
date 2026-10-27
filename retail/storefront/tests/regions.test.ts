import test from 'node:test'
import assert from 'node:assert/strict'
import {readFile} from 'node:fs/promises'
import {loadRegions,projectRegions} from '../src/lib/regions.ts'
import {routeCountry,routeLanguage,storefrontSegment,localeFromSegment} from '../src/lib/storefront-route.ts'

const fixture=()=>({count:1,regions:[{id:'region-fixture',name:'Region',currency_code:'eur',automatic_taxes:true,countries:[{iso_2:'dk',iso_3:'DNK',num_code:'008',name:'DENMARK',display_name:'Denmark',region_id:'region-fixture'}]}]})
test('country/language helpers retain the exact original implementation and behavior',async()=>{
 const old=await readFile(new URL('../../../medusa-store/apps/storefront/src/lib/util/storefront-route.ts',import.meta.url),'utf8')
 const current=await readFile(new URL('../src/lib/storefront-route.ts',import.meta.url),'utf8')
 assert.equal(current.replaceAll('\r\n','\n'),old.replaceAll('\r\n','\n'))
 for(const country of ['dk','DK-en','us-zh','de-zh-CN',''])for(const lang of ['zh-CN','ZH','en',null,undefined]){
  assert.equal(routeCountry(country),country.toLowerCase().split('-')[0])
  assert.equal(routeLanguage(lang),lang?.toLowerCase().startsWith('zh')?'zh':'en')
  assert.equal(storefrontSegment(country,lang),`${routeCountry(country)}-${routeLanguage(lang)}`)
 }
 assert.equal(localeFromSegment('dk-zh'),'zh-CN');assert.equal(localeFromSegment('dk-en'),'en');assert.equal(localeFromSegment('dk'),'en')
})
test('region projection keeps exact routing fields, strips private additions and does not mutate input',()=>{
 const input=fixture(),before=JSON.stringify(input)
 const output=projectRegions({...input,private:'do-not-copy',regions:input.regions.map(r=>({...r,metadata:{secret:true},countries:r.countries.map(c=>({...c,metadata:{secret:true}}))}))})
 assert.deepEqual(output,input.regions);assert.equal(output[0].countries[0].num_code,'008');assert.equal(JSON.stringify(input),before)
 assert.deepEqual(projectRegions({count:0,regions:[]}),[])
 for(const edit of [(v:ReturnType<typeof fixture>)=>{v.count=2},(v:ReturnType<typeof fixture>)=>{v.regions[0].currency_code='EUR'},(v:ReturnType<typeof fixture>)=>{v.regions[0].countries[0].region_id='foreign'},(v:ReturnType<typeof fixture>)=>{v.regions[0].countries[0].iso_2='DK'},(v:ReturnType<typeof fixture>)=>{v.regions.push(v.regions[0]);v.count++},(v:ReturnType<typeof fixture>)=>{v.regions[0].countries.push(v.regions[0].countries[0])}]){const value=fixture();edit(value);assert.throws(()=>projectRegions(value),/Store regions unavailable/)}
 assert.throws(()=>projectRegions({count:1,regions:[{...fixture().regions[0],automatic_taxes:null}]}))
 assert.throws(()=>projectRegions({count:1,regions:[{...fixture().regions[0],countries:[{...fixture().regions[0].countries[0],num_code:8}]}]}))
 assert.throws(()=>projectRegions({count:101,regions:Array.from({length:101},(_,i)=>({...fixture().regions[0],id:`r-${i}`,countries:[]}))}))
})
test('Gin transport fixes its path, omits cookies, disallows redirects and validates before exposing data',async()=>{
 let calls=0
 const upstream:typeof fetch=async(input,init)=>{
  calls++;assert.equal(String(input),'https://gin.example/api/store/regions')
  assert.equal(init?.credentials,'omit');assert.equal(init?.cache,'no-store');assert.equal(init?.redirect,'error')
  assert.deepEqual(init?.headers,{Accept:'application/json'});assert.ok(init?.signal)
  return Response.json(fixture())
 }
 assert.deepEqual(await loadRegions('https://gin.example',upstream),fixture().regions);assert.equal(calls,1)
 for(const origin of [undefined,'','file:///tmp/data','https://user:password@gin.example','https://gin.example/path','https://gin.example/?q=1','https://gin.example/#part'])await assert.rejects(loadRegions(origin,upstream),/Store regions unavailable/)
 assert.equal(calls,1)
 for(const response of [Response.json(fixture(),{status:503}),new Response('<h1>secret</h1>',{headers:{'Content-Type':'text/html'}}),Response.json({regions:[]}),new Response(new Uint8Array(2*1024*1024+1),{headers:{'Content-Type':'application/json'}}),new Response(new Uint8Array([0xff]),{headers:{'Content-Type':'application/json'}})]){
  await assert.rejects(loadRegions('https://gin.example',async()=>response),error=>error instanceof Error&&error.message==='Store regions unavailable')
 }
 await assert.rejects(loadRegions('https://gin.example',async()=>{throw new Error('private upstream address')}),error=>error instanceof Error&&!String(error).includes('private'))
})
