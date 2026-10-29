import test from 'node:test'
import assert from 'node:assert/strict'
import {loadCollectionPage,collectionProducts,parseCollectionQuery,collectionNavigation} from '../src/lib/collection-page.ts'
import {readFileSync} from 'node:fs'
import ts from 'typescript'
import type {Product,Quote} from '../src/lib/products.ts'
const region={id:'region',name:'Europe',currency_code:'eur',automatic_taxes:false,countries:[]}
const price=(amount:string):Quote=>({calculated_amount:amount,original_amount:null,currency_code:'eur',is_calculated_price_tax_inclusive:false,is_original_price_tax_inclusive:false,is_calculated_price_price_list:false,is_original_price_price_list:false,calculated_price:{price_list_type:null}})
function product(id:string,amount:string|null):Product{return {id,title:id,handle:id,subtitle:null,description:null,thumbnail:null,collection_id:'col',options:[],images:[],display:{colors:[],model_specs:null,launch_prices:[],source:null},variants:[{id:'v-'+id,title:id,sku:null,thumbnail:null,variant_rank:0,options:[],images:[],configuration_complete:true,calculated_price:amount===null?null:price(amount),manage_inventory:false,allow_backorder:false,inventory_quantity:null,availability:{state:'untracked',can_fulfill:true,max_fulfillable_quantity:null},display:{slug:null,demo_price_basis:null,apple_launch_price:null}}]}}
function fixture(products:Product[],observe?:(url:URL)=>void,collection:unknown={id:'col',title:'Collection',handle:'phones'}):typeof fetch{
 return async(url,options)=>{
  const u=new URL(String(url));observe?.(u)
  assert.equal(u.origin,'https://gin.example');assert.equal(options?.credentials,'omit');assert.equal(options?.redirect,'error');assert.deepEqual(options?.headers,{Accept:'application/json'})
  if(u.pathname==='/api/store/collections')return Response.json({collections:collection===null?[]:[collection],count:collection===null?0:1,limit:2,offset:0})
  assert.equal(u.pathname,'/api/store/products')
  return Response.json({products,count:products.length,limit:100,offset:0,capabilities:{pricing:true,availability:true,checkout:false}})
 }
}
test('collection query keeps original comma/repeated option URLs, sort and twelve-item page semantics',()=>{
 assert.deepEqual(parseCollectionQuery(new URLSearchParams()),{page:1,sort:'created_at',optionValues:[]})
 assert.deepEqual(parseCollectionQuery(new URLSearchParams('page=2&sortBy=price_asc&optionValueIds=a,b,a')),{page:2,sort:'price_asc',optionValues:['a','b']})
 assert.deepEqual(parseCollectionQuery(new URLSearchParams('optionValueIds=a&optionValueIds=b&optionValueIds=a')).optionValues,['a','b'])
 for(const q of ['page=0','page=-1','page=01','page=1.5','page=NaN','page=1000000','page=1&page=2','sortBy=wrong','sortBy=price_asc&sortBy=price_desc','optionValueIds=%00','optionValueIds='+Array(101).fill('a').join(',')])assert.throws(()=>parseCollectionQuery(new URLSearchParams(q)))
})
test('collection sorts exact decimal minimum variant prices without mutation and retains source missing-price ordering',()=>{
 const low=product('low','9007199254740993.000000000000000001'),high=product('high','9007199254740993.000000000000000002'),missing=product('missing',null),empty={...product('empty','1'),variants:[]}
 const rows=[high,low,missing,empty],before=JSON.stringify(rows)
 assert.deepEqual(collectionProducts(rows,'price_asc').map(p=>p.id),['missing','low','high','empty'])
 assert.deepEqual(collectionProducts(rows,'price_desc').map(p=>p.id),['empty','high','low','missing'])
 assert.deepEqual(collectionProducts(rows,'created_at').map(p=>p.id),rows.map(p=>p.id));assert.equal(JSON.stringify(rows),before)
 const many=product('many','10');many.variants.push({...many.variants[0]!,id:'other',calculated_price:price('1')})
 assert.deepEqual(collectionProducts([low,many],'price_asc').map(p=>p.id),['many','low'])
})
test('public collection lookup feeds scoped latest hundred products, option selectors and then original pagination',async()=>{
 const rows=Array.from({length:25},(_,i)=>product('p-'+i,String(25-i))),calls:URL[]=[]
 const data=await loadCollectionPage('https://gin.example','phones',region,{page:2,sort:'price_asc',optionValues:['storage','colour']},fixture(rows,u=>calls.push(u)))
 assert.equal(data?.count,25);assert.equal(data?.totalPages,3);assert.equal(data?.products.length,12);assert.equal(data?.products[0]?.id,'p-12')
 assert.equal(calls[0]?.searchParams.get('handle'),'phones')
 assert.equal(calls[1]?.searchParams.get('collection_id'),'col');assert.equal(calls[1]?.searchParams.get('region_id'),'region');assert.equal(calls[1]?.searchParams.has('order'),false);assert.deepEqual(calls[1]?.searchParams.getAll('option_value_id'),['storage','colour'])
 const latest=await loadCollectionPage('https://gin.example','phones',region,{page:1,sort:'created_at',optionValues:[]},fixture(rows,u=>{if(u.pathname.endsWith('products'))assert.equal(u.searchParams.get('order'),'-created_at')}));assert.equal(latest?.products[0]?.id,'p-0')
 const empty=await loadCollectionPage('https://gin.example','phones',region,{page:99,sort:'created_at',optionValues:[]},fixture(rows));assert.deepEqual(empty?.products,[])
 assert.equal(await loadCollectionPage('https://gin.example','phones',region,{page:1,sort:'created_at',optionValues:[]},fixture([],undefined,null)),null)
})
test('wrong collection, currency, ambiguous lookup and unavailable APIs do not become empty success',async()=>{
 const query={page:1,sort:'created_at' as const,optionValues:[]}
 for(const p of [{...product('wrong','1'),collection_id:'foreign'},(()=>{const p=product('wrong','1');p.variants[0]!.calculated_price!.currency_code='usd';return p})()])await assert.rejects(loadCollectionPage('https://gin.example','phones',region,query,fixture([p])))
 await assert.rejects(loadCollectionPage('https://gin.example','phones',region,query,fixture([],undefined,{id:'col',title:'Title',handle:'foreign'})))
 await assert.rejects(loadCollectionPage('https://gin.example','phones',region,query,async()=>Response.json({collections:[{id:'col',title:'Title',handle:'phones'}],count:2,limit:2,offset:0})))
 await assert.rejects(loadCollectionPage('https://gin.example','phones',region,query,async()=>new Response('',{status:503})))
})
test('sort resets page and pagination retains repeated option filters and other query values',()=>{
 const url='https://shop.example/dk-en/collections/phones?page=3&optionValueIds=a&optionValueIds=b&keep=yes#old'
 assert.equal(collectionNavigation(url,{sort:'price_desc'}),'/dk-en/collections/phones?optionValueIds=a&optionValueIds=b&keep=yes&sortBy=price_desc')
 assert.equal(collectionNavigation(url,{page:2}),'/dk-en/collections/phones?page=2&optionValueIds=a&optionValueIds=b&keep=yes')
})

test('price sorting agrees with the existing storefront helper on ordinary source product values',()=>{
 const source=readFileSync(new URL('../../../medusa-store/apps/storefront/src/lib/util/sort-products.ts',import.meta.url),'utf8')
 const compiled=ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.CommonJS}}).outputText
 const exports:Record<string,any>={};new Function('exports',compiled)(exports)
 const amounts=[['a','12'],['b','0'],['c','10.25'],['d','10.25'],['e',null]] as const
 const rows=amounts.map(([id,amount])=>product(id,amount)),original=rows.map(p=>({...p,variants:p.variants.map(v=>({...v,calculated_price:v.calculated_price?{...v.calculated_price,calculated_amount:Number(v.calculated_price.calculated_amount)}:null}))}))
 for(const sort of ['price_asc','price_desc'] as const)assert.deepEqual(collectionProducts(rows,sort).map(p=>p.id),exports.sortProducts(structuredClone(original),sort).map((p:{id:string})=>p.id))
})
