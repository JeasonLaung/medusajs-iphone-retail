import test from 'node:test'
import assert from 'node:assert/strict'
import {loadFooterData} from '../src/lib/footer-data.ts'

const category={id:'iphone',name:'iPhone',handle:'iphone',category_children:[{id:'series',name:'iPhone 13 & Pro',handle:'iphone/13',parent_category_id:'iphone'}]}
const collection={id:'collection',title:'Deals',handle:'deals'}
function transport(change?:(body:Record<string,unknown>,path:string)=>void):typeof fetch{
 return async(url,options)=>{
  assert.equal(options?.credentials,'omit');assert.equal(options?.cache,'no-store');assert.deepEqual(options?.headers,{Accept:'application/json'})
  const path=new URL(String(url)).pathname,isCategory=path.endsWith('product-categories')
  const body:Record<string,unknown>=isCategory?{product_categories:[structuredClone(category)],count:1,limit:2,offset:0}:{collections:[structuredClone(collection)],count:1,limit:6,offset:0}
  change?.(body,path);return Response.json(body)
 }
}
test('footer public DTO projects only direct iPhone children and visible collection links',async()=>{
 assert.deepEqual(await loadFooterData('https://gin.example',transport()),{iphoneCategory:{id:'iphone',name:'iPhone',handle:'iphone',category_children:[{id:'series',name:'iPhone 13 & Pro',handle:'iphone/13'}]},collections:[collection]})
})
test('footer missing root and empty collections are valid, dependency failures remain errors',async()=>{
 assert.deepEqual(await loadFooterData('https://gin.example',transport((body,path)=>{body[path.endsWith('product-categories')?'product_categories':'collections']=[];body.count=0})),{iphoneCategory:null,collections:[]})
 for(const status of [404,503])await assert.rejects(loadFooterData('https://gin.example',async()=>new Response('',{status})))
 await assert.rejects(loadFooterData(undefined,transport()))
})
test('footer rejects mismatched counts, foreign children, duplicate IDs and unsafe collection links',async()=>{
 for(const change of [
  (b:Record<string,unknown>)=>{b.count=3},
  (b:Record<string,unknown>,p:string)=>{if(p.endsWith('product-categories'))b.product_categories=[{...category,handle:'foreign'}]},
  (b:Record<string,unknown>,p:string)=>{if(p.endsWith('product-categories'))b.product_categories=[{...category,category_children:[{...category.category_children[0],parent_category_id:'foreign'}]}]},
  (b:Record<string,unknown>,p:string)=>{if(p.endsWith('collections'))b.collections=[{...collection,handle:'bad?query'}]},
  (b:Record<string,unknown>,p:string)=>{if(p.endsWith('collections')){b.collections=[collection,collection];b.count=2}},
 ])await assert.rejects(loadFooterData('https://gin.example',transport(change)))
})
