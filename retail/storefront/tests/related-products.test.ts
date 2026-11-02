import test from 'node:test'
import assert from 'node:assert/strict'
import {loadRelatedProducts} from '../src/lib/related-products.ts'
import {product} from './selection-fixture/src/product.ts'

test('recommendations retain the source 50-candidate window and default ID order before excluding and slicing',async()=>{
 const candidates=Array.from({length:50},(_,i)=>{
  const p=structuredClone(product);p.id=i===0?product.id:'related-'+i;p.handle='related-'+i;p.title=i<40?'Accessory':'iPhone '+i
  p.variants=[];return p
 })
 const rows=await loadRelatedProducts('https://gin.example',product,'region',async(input,init)=>{
  const url=new URL(String(input));assert.equal(url.searchParams.get('related_to'),product.id);assert.equal(url.searchParams.get('region_id'),'region');assert.equal(url.searchParams.get('limit'),'50');assert.equal(url.searchParams.has('order'),false)
  assert.equal(new Headers(init?.headers).has('authorization'),false)
  return Response.json({products:candidates,count:80,limit:50,offset:0,capabilities:{pricing:true,availability:true,checkout:false}})
 })
 assert.equal(rows.length,10);assert.equal(rows[0].id,'related-40');assert.equal(rows.at(-1)?.id,'related-49')
 const accessory={...product,title:'Accessory'}
 const unrestricted=await loadRelatedProducts('https://gin.example',accessory,'region',async()=>Response.json({products:candidates,count:80,limit:50,offset:0,capabilities:{pricing:true,availability:true,checkout:false}}))
 assert.equal(unrestricted.length,12);assert.equal(unrestricted[0].id,'related-1')
})
