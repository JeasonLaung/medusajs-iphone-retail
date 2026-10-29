import test from 'node:test'
import assert from 'node:assert/strict'
import qs from 'qs'
import {nativeQuery,algoliaResult,attributes,validateNativeResults,createSearchClient,type NativeResult} from '../src/components/search/client.ts'
import {createStoreSearchRouting} from '../src/components/search/routing.ts'

const fixture=(key='0'):NativeResult=>({key,query:'',currency_code:'eur',count:1,page:1,limit:50,hits:[{id:'phone',title:'iPhone 13',handle:'iphone-13',thumbnail:'/phone.webp',values:Object.fromEntries(attributes.map(a=>[a,a==='category'?['iPhone','iPhone 13']:a==='storage_options'?['128 GB']:[]])),colors:[{name:'Blue',hex:'#0000ff'}],model_storage_options:['128 GB','256 GB'],stocked_storage_options:['128 GB'],stocked_color_options:['Blue'],price:{min:'499.125',max:'600',original:'700',on_sale:true}}],facets:Object.fromEntries(attributes.map(a=>[a,[]])),catalog_facets:Object.fromEntries(attributes.map(a=>[a,[]])),price_distribution:['499.125']})

test('native search adapter keeps region currency, OR groups and decimal bounds explicit',()=>{
 const q=nativeQuery({indexName:'product/sort/min_price_eur:asc',params:{query:'i13pm',page:2,hitsPerPage:50,facetFilters:['category:iPhone',['category:iPhone 13','category:iPhone 14'],['storage_options:128 GB','storage_options:256 GB']],numericFilters:['min_price_eur>=100.01','min_price_eur<=999']}},'main','eur')
 assert.deepEqual(q,{key:'main',query:'i13pm',filters:{category:['iPhone 13','iPhone 14'],storage_options:['128 GB','256 GB']},sort:'price:asc',page:3,limit:50,min_price:'100.01',max_price:'999'})
 for(const request of [{indexName:'foreign'}, {indexName:'product/sort/min_price_usd:asc'}, {indexName:'product',params:{facetFilters:['status:draft']}},{indexName:'product',params:{numericFilters:['min_price_usd>=100']}},{indexName:'product',params:{page:-1}}])assert.throws(()=>nativeQuery(request,'x','eur'))
})

test('native result projection retains exact card prices and source stock labels',()=>{
 const native=fixture(),query=nativeQuery({indexName:'product',params:{hitsPerPage:50}},'0','eur')
 assert.equal(validateNativeResults({results:[native]},[query],'eur')[0],native)
 const result=algoliaResult(native,{indexName:'product',params:{hitsPerPage:50}})
 assert.equal(result.hits[0].__gin_price?.min,'499.125');assert.deepEqual(result.hits[0].stocked_storage_options,['128 GB']);assert.equal((result.hits[0] as unknown as Record<string,unknown>).min_price_eur,499.125)
 assert.deepEqual(result.__gin_distribution,[499.125]);assert.equal(result.facets_stats.min_price_eur?.min,499.125)
 for(const edit of [{currency_code:'usd'},{key:'other'},{page:2},{count:-1},{price_distribution:['NaN']}])assert.throws(()=>validateNativeResults({results:[{...native,...edit}]},[query],'eur'))
})

test('batch adapter chunks native requests, omits credentials and never falls back on dependency failure',async()=>{
 let calls=0
 const client=createSearchClient('region','eur','https://gin.example',async(input,init)=>{
  calls++;assert.equal(String(input),'https://gin.example/api/store/search');assert.equal(init?.method,'POST');assert.equal(init?.credentials,'omit');assert.equal(init?.redirect,'error')
  assert.equal(new Headers(init?.headers).get('cookie'),null)
  const body=JSON.parse(String(init?.body));assert.equal(body.region_id,'region');assert.ok(body.queries.length<=8)
  return Response.json({results:body.queries.map((q:{key:string})=>fixture(q.key))})
 })
 const result=await client.search(Array.from({length:9},()=>({indexName:'product',params:{hitsPerPage:50}})))
 assert.equal(calls,2);assert.equal(result.results.length,9)
 const failed=createSearchClient('region','eur','https://gin.example',async()=>Response.json({error:'PRIVATE'}, {status:503}))
 await assert.rejects(failed.search([{indexName:'product',params:{hitsPerPage:50}}]),/Store data unavailable/)
})

test('original search URL mapping preserves escapes, nested links, sort, range and page',()=>{
 const routing=createStoreSearchRouting('product'),state={product:{query:'i13pm',page:2,sortBy:'product/sort/min_price_eur:asc',refinementList:{color_families:['Blue, Gray','100%'],storage_options:['128 GB','256 GB']},range:{min_price_eur:'100:500'}}}
 const route=routing.stateMapping.stateToRoute(state)
 const url=routing.router.createURL({qsModule:qs,routeState:route,location:new URL('https://example.test/dk-en/store#keep') as unknown as Location} as Parameters<typeof routing.router.createURL>[0])
 const parsed=routing.router.parseURL({qsModule:qs,location:new URL(url) as unknown as Location} as Parameters<typeof routing.router.parseURL>[0])
 assert.deepEqual(routing.stateMapping.routeToState(parsed),state)
 assert.equal(new URL(url).hash,'#keep')
 const legacy=routing.stateMapping.routeToState(qs.parse('product[query]=iphone&product[page]=2&product[refinementList][storage_options][0]=128%20GB'))
 assert.equal(legacy.product.page,2);assert.deepEqual(legacy.product.refinementList?.storage_options,['128 GB'])
})
