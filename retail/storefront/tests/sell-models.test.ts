import test from 'node:test'
import assert from 'node:assert/strict'
import {readFile} from 'node:fs/promises'
import {createRequire} from 'node:module'
import vm from 'node:vm'
import {loadSellModels,sellModels} from '../src/lib/sell-models.ts'
import type {Product} from '../src/lib/products.ts'
const require=createRequire(import.meta.url),ts=require('typescript')
function product(id:string,title:string,handle='swappie-'+id):Product{
 return {id,title,handle,subtitle:null,description:null,thumbnail:null,collection_id:null,options:[{id:'opt-'+id,title:'Storage',values:['256 GB','1 TB','128GB','256GB','2048 gb','not storage'].map((value,i)=>({id:id+'-'+i,value,rank:i}))}],images:[],variants:[],display:{colors:[],model_specs:null,launch_prices:[],source:null}}
}
const products=[...['iPhone 17 Pro Max','iPhone 17 Pro','iPhone Air','iPhone 17','iPhone 16 Plus','iPhone 16e','iPhone 13 mini','iPhone SE','iPhone SE (2022)',' iPhone 13 ','iphone 12','iPad','iPhones 15'].map((s,i)=>product('p'+i,s)),product('later','iPhone 13'),product('without-handle','iPhone 10','')]
async function original(rows:Product[]){
 const source=await readFile(new URL('../../../medusa-store/apps/storefront/src/lib/data/sell-models.ts',import.meta.url),'utf8')
 const compiled=ts.transpileModule(source,{compilerOptions:{target:ts.ScriptTarget.ES2022,module:ts.ModuleKind.CommonJS}}).outputText
 const exports:Record<string,Function>={}
 vm.runInNewContext(compiled,{exports,require:(name:string)=>{
  if(name==='server-only')return {}
  if(name==='@lib/config')return {sdk:{store:{product:{list:async({offset,limit}:{offset:number;limit:number})=>({products:rows.slice(offset,offset+limit),count:rows.length})}}}}
  throw Error('Unexpected original sell-models dependency')
 }})
 return JSON.parse(JSON.stringify(await exports.getSellModels()))
}
test('sell catalogue preserves actual original model filtering, duplicate titles, ranks and capacities',async()=>{
 assert.deepEqual(sellModels(products),await original(products))
 const result=sellModels(products)
 assert.deepEqual(result.slice(0,4).map(m=>m.name),['iPhone 17 Pro Max','iPhone 17 Pro','iPhone Air','iPhone 17'])
 assert.equal(result.find(m=>m.name==='iPhone 13')?.id,'later')
 assert.deepEqual(result[0].capacities,['128GB','256GB','1TB','2048GB'])
 const chinese=product('cn','iPhone 18');chinese.options[0].title='容量';chinese.options[0].values[0].value='\ufeff512\u00a0GB'
 assert.deepEqual(sellModels([chinese]),await original([chinese]))
})
test('sell catalogue loads every Gin page in catalogue order without customer data',async()=>{
 const calls:string[]=[]
 const fetcher:typeof fetch=async(input,init)=>{
  const u=new URL(String(input));calls.push(u.search)
  assert.equal(u.origin,'https://gin.example');assert.equal(u.pathname,'/api/store/products')
  assert.equal(init?.credentials,'omit');assert.equal(init?.redirect,'error');assert.equal(init?.cache,'no-store')
  assert.deepEqual(init?.headers,{Accept:'application/json'})
  assert.equal(u.searchParams.has('order'),false)
  const offset=Number(u.searchParams.get('offset')),limit=Number(u.searchParams.get('limit'))
  return Response.json({products:products.slice(offset,offset+limit),count:products.length,offset,limit,capabilities:{pricing:false,availability:true,checkout:false}})
 }
 assert.deepEqual(await loadSellModels('https://gin.example',fetcher),await original(products))
 assert.equal(calls.length,2)
})
test('sell catalogue does not return partial model choices on bad or inconsistent pagination',async()=>{
 for(const mode of ['duplicate','empty','count','oversized','upstream']){
  await assert.rejects(()=>loadSellModels('https://gin.example',async(input)=>{
   if(mode==='upstream')return Response.json({}, {status:503})
   const u=new URL(String(input)),offset=Number(u.searchParams.get('offset')),limit=12
   const rows=offset===0?products.slice(0,12):mode==='empty'?[]:mode==='duplicate'?[products[0]]:products.slice(12)
   const count=mode==='oversized'?100001:offset>0&&mode==='count'?16:products.length
   return Response.json({products:rows,count,offset,limit,capabilities:{pricing:false,availability:true,checkout:false}})
  }))
 }
 assert.deepEqual(await loadSellModels('https://gin.example',async()=>Response.json({products:[],count:0,offset:0,limit:12,capabilities:{pricing:false,availability:true,checkout:false}})),[])
})
