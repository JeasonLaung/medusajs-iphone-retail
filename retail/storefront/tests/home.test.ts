import test from 'node:test'
import assert from 'node:assert/strict'
import {homeSeries,loadHome,loadHomeReviewPage} from '../src/lib/home.ts'
import type {FooterData} from '../src/lib/footer-data.ts'
import {product as original} from './selection-fixture/src/product.ts'

const footer:FooterData={collections:[],iphoneCategory:{id:'iphone',name:'iPhone',handle:'iphone',category_children:['13','Air','17','99','16'].map(f=>({id:`series-${f}`,name:`iPhone ${f} series`,handle:`iphone-${f.toLowerCase()}-series`}))}}
const product={id:'p',title:'iPhone',handle:'iphone'}
const review=(id:string,date='2026-10-29T10:00:00Z')=>({id,created_at:date,rating:5,title:'Good',content:'As described',author:'Customer',verified_purchase:true,private_email:'hidden'})
test('home preserves source family order and excludes unknown families and missing parent',()=>{
 assert.deepEqual(homeSeries(footer).map(s=>s.family),['17','Air','16','13'])
 assert.equal(homeSeries({...footer,iphoneCategory:null}).length,0)
 assert.equal(homeSeries(footer)[1].category.image,'/iphone-series/iphone-air/series.webp')
})
test('public review projection strips private fields, validates pages, and exposes failure separately from no reviews',async()=>{
 const response={reviews:[review('r')],offset:0,limit:5,count:1}
 const rows=await loadHomeReviewPage('https://gin.example',product,async(input,init)=>{
  const url=new URL(String(input));assert.equal(url.searchParams.get('product_id'),'p');assert.equal(init?.credentials,'omit');assert.equal(init?.redirect,'error');assert.equal(new Headers(init?.headers).get('Authorization'),null)
  return Response.json(response)
 })
 assert.equal(rows?.length,1);assert.ok(!JSON.stringify(rows).includes('hidden'))
 assert.deepEqual(await loadHomeReviewPage('https://gin.example',product,async()=>Response.json({...response,reviews:[],count:0})),[])
 for(const invalid of [{...response,count:2},{...response,reviews:[{...review('r'),rating:6}]},{...response,reviews:[{...review('r'),created_at:'invalid'}]}])assert.equal(await loadHomeReviewPage('https://gin.example',product,async()=>Response.json(invalid)),null)
 assert.equal(await loadHomeReviewPage('https://gin.example',product,async()=>new Response('',{status:404})),null)
})
test('home deduplicates products across series and selects the newest three public reviews',async()=>{
 const p=structuredClone(original);p.images.push(...p.variants.flatMap(v=>v.images));for(const v of p.variants)v.calculated_price=null
 let reviews=0
 const data=await loadHome('https://gin.example',footer,async(input)=>{
  const url=new URL(String(input))
  if(url.pathname==='/api/store/products')return Response.json({products:[p],count:1,offset:0,limit:100,capabilities:{pricing:false,availability:true,checkout:false}})
  reviews++;return Response.json({reviews:[review('old','2026-01-01T00:00:00Z'),review('new','2026-10-29T00:00:00Z'),review('third','2026-03-01T00:00:00Z'),review('second','2026-09-01T00:00:00Z')],count:4,limit:5,offset:0})
 })
 assert.equal(reviews,1);assert.deepEqual(data.reviews.reviews.map(r=>r.id),['new','second','third']);assert.equal(data.reviews.unavailable,false)
 const failed=await loadHome('https://gin.example',footer,async(input)=>new URL(String(input)).pathname==='/api/store/products'?Response.json({products:[p],count:1,offset:0,limit:100,capabilities:{pricing:false,availability:true,checkout:false}}):new Response('',{status:503}))
 assert.equal(failed.reviews.unavailable,true);assert.deepEqual(failed.reviews.reviews,[])
})
test('a failed category dependency cannot silently turn a populated home into an empty catalog',async()=>{
 await assert.rejects(loadHome('https://gin.example',footer,async()=>new Response('',{status:503})))
 const empty=await loadHome('https://gin.example',{...footer,iphoneCategory:null},async()=>{throw new Error('no reads expected')})
 assert.deepEqual(empty,{series:[],reviews:{reviews:[],unavailable:false}})
})
