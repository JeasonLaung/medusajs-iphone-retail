import test from 'node:test'
import assert from 'node:assert/strict'
import {loadCategoryRedirect} from '../src/lib/category-redirect.ts'

const region={id:'region',name:'Denmark',currency_code:'eur',automatic_taxes:true,countries:[{iso_2:'dk',iso_3:'dnk',num_code:'208',name:'Denmark',display_name:'Denmark',region_id:'region'}]}
const category={id:'series',handle:'iphone/13',name:'iPhone 13 & Pro',parent_category_id:'iphone',parent_category:{id:'iphone',handle:'iphone'}}
function transport(rows:unknown[]=[category],change?:(body:Record<string,unknown>)=>void):typeof fetch{
 return async(url,options)=>{
  const parsed=new URL(String(url))
  assert.equal(parsed.origin,'https://gin.example')
  assert.equal(options?.credentials,'omit');assert.equal(options?.redirect,'error');assert.deepEqual(options?.headers,{Accept:'application/json'})
  if(parsed.pathname==='/api/store/regions')return Response.json({regions:[region],count:1})
  assert.equal(parsed.pathname,'/api/store/product-categories')
  assert.deepEqual([...parsed.searchParams],[['handle','iphone/13'],['limit','2']])
  const body:Record<string,unknown>={product_categories:rows,count:rows.length,limit:2,offset:0}
  change?.(body);return Response.json(body)
 }
}
test('original nested category URL becomes a permanent store series destination with safe encoding',async()=>{
 assert.equal(await loadCategoryRedirect('https://gin.example','dk-en','iphone/13',transport()),'/dk-en/store?series=iPhone+13+%26+Pro')
 assert.equal(await loadCategoryRedirect('https://gin.example','dk-zh','iphone/13',transport([{...category,name:'iPhone 中文 & #?'}])),'/dk-zh/store?series=iPhone+%E4%B8%AD%E6%96%87+%26+%23%3F')
})
test('missing, root and non-direct iPhone categories retain the original unfiltered store redirect',async()=>{
 for(const rows of [[],[{...category,parent_category_id:null,parent_category:null}],[{...category,parent_category_id:'other',parent_category:{id:'other',handle:'other',parent_category:{id:'iphone',handle:'iphone'}}}]]){
  assert.equal(await loadCategoryRedirect('https://gin.example','dk-en','iphone/13',transport(rows)),'/dk-en/store')
 }
})
test('invalid country/language/handle has no redirect and does not perform a category lookup',async()=>{
 const never:typeof fetch=async()=>{throw new Error('Unexpected fetch')}
 for(const [segment,handle]of [['bad','iphone/13'],['dk-fr','iphone/13'],['dk-en',''],['dk-en','x'.repeat(513)],['dk-en','bad\nhandle']])assert.equal(await loadCategoryRedirect('https://gin.example',segment!,handle!,never),null)
 let requests=0
 const countriesOnly:typeof fetch=async()=>{requests++;return Response.json({regions:[region],count:1})}
 assert.equal(await loadCategoryRedirect('https://gin.example','us-en','iphone/13',countriesOnly),null);assert.equal(requests,1)
})
test('ambiguous or malformed public responses cannot silently remove a series filter',async()=>{
 for(const change of [
  (b:Record<string,unknown>)=>{b.count=2},
  (b:Record<string,unknown>)=>{b.product_categories=[category,category];b.count=2},
  (b:Record<string,unknown>)=>{b.product_categories=[{...category,handle:'foreign'}]},
  (b:Record<string,unknown>)=>{b.product_categories=[{...category,parent_category_id:'foreign'}]},
  (b:Record<string,unknown>)=>{b.product_categories=[{...category,parent_category:undefined}]},
  (b:Record<string,unknown>)=>{b.product_categories=[{...category,parent_category:{id:'series',handle:'iphone'},parent_category_id:'series'}]},
 ])await assert.rejects(loadCategoryRedirect('https://gin.example','dk-en','iphone/13',transport([category],change)))
})
test('category dependency failure stays an error instead of a successful fallback redirect',async()=>{
 for(const status of [404,503]){
  const fixture=transport()
  const broken:typeof fetch=async(url,options)=>new URL(String(url)).pathname.endsWith('regions')?fixture(url,options):new Response('',{status})
  await assert.rejects(loadCategoryRedirect('https://gin.example','dk-en','iphone/13',broken))
 }
})
