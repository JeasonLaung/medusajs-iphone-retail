import test from 'node:test'
import assert from 'node:assert/strict'
import {loadCartVariants} from '../src/lib/cart-variants.ts'
import type {CartView} from '../src/lib/cart-client.ts'

function view(handles:(string|null)[]=['phone','phone',null]):CartView{
 return {amounts:{cart_id:'cart',version:'8',currency_code:'eur',totals:{total:'200.000000000000000000002'},shipping_methods:[],pricing_recalculated:false,checkout:false,lines:handles.map((_,i)=>({id:`line-${i}`,variant_id:`v-${i}`,quantity:'2',unit_price:'100.000000000000000000001',is_tax_inclusive:false,totals:{subtotal:'200.000000000000000000002'}}))},items:handles.map((handle,i)=>({id:`line-${i}`,title:'Saved name',product_id:handle===null?null:'p-'+handle,product_title:null,thumbnail:null,variant_title:null,product_handle:handle,option_values:{}}))}
}
function product(handle='phone',ids=['v-0','v-1']){
 return {id:'p-'+handle,title:'Current name',handle,subtitle:null,description:null,thumbnail:null,collection_id:null,display:{colors:[],model_specs:null,launch_prices:[],source:null},options:[],images:[],variants:ids.map(id=>({id,title:'Current configuration',sku:null,thumbnail:null,variant_rank:0,options:[],images:[],configuration_complete:true,calculated_price:{calculated_amount:'1',original_amount:null,currency_code:'eur',is_calculated_price_tax_inclusive:false,is_original_price_tax_inclusive:false,is_calculated_price_price_list:false,is_original_price_price_list:false,calculated_price:{price_list_type:null}},manage_inventory:true,allow_backorder:false,inventory_quantity:'4',availability:{state:'available',can_fulfill:true,max_fulfillable_quantity:'4'},display:{slug:null,demo_price_basis:null,apple_launch_price:null}}))}
}
function response(handle:string,ids?:string[]){return Response.json({product:product(handle,ids),capabilities:{pricing:true,availability:true,checkout:false}})}

test('cart observations deduplicate saved product IDs, join exact variant IDs and never replace saved prices or labels',async()=>{
 const saved=view();saved.items[0].product_handle='renamed-old-handle';const before=structuredClone(saved),requests:URL[]=[]
 const variants=await loadCartVariants(saved,'https://gin.example','region','eur',async(url,options)=>{
  requests.push(new URL(String(url)));assert.equal(options?.credentials,'omit');assert.equal(options?.cache,'no-store');assert.deepEqual(options?.headers,{Accept:'application/json'})
  return response('phone',['v-0','v-1','other'])
 })
 assert.equal(requests.length,1);assert.equal(requests[0].pathname,'/api/store/products/p-phone');assert.equal(requests[0].searchParams.get('region_id'),'region')
 assert.deepEqual(Object.keys(variants).sort(),['v-0','v-1']);assert.equal(variants['v-0'].availability.max_fulfillable_quantity,'4');assert.deepEqual(saved,before)
})

test('missing, ambiguous, malformed, foreign and failed observations preserve an unknown stock fallback',async()=>{
 const fetchers:(typeof fetch)[]=[async()=>new Response('',{status:404}),async()=>new Response('',{status:503}),async()=>{throw Error('offline')},async()=>response('foreign'),async()=>response('phone',['foreign-id']),async()=>{
  const p=product();p.variants[0].calculated_price.currency_code='usd';return Response.json({product:p,capabilities:{pricing:true,availability:true,checkout:false}})
 }]
 for(const fetcher of fetchers)assert.equal(Object.keys(await loadCartVariants(view(['phone']),'https://gin.example','region','eur',fetcher)).length,0)
 const saved=view(['phone']);saved.items[0].product_id=null
 let calls=0;assert.equal(Object.keys(await loadCartVariants(saved,'https://gin.example','region','eur',async()=>{calls++;throw Error()})).length,0);assert.equal(calls,0)
})

test('observations use four bounded workers, keep successful products and stop starting work after cancellation',async()=>{
 const saved=view(Array.from({length:9},(_,i)=>'phone-'+i));let active=0,max=0,calls=0
 const variants=await loadCartVariants(saved,'https://gin.example','region','eur',async url=>{
  const handle=new URL(String(url)).pathname.split('/').at(-1)!.slice(2);calls++;active++;max=Math.max(max,active)
  await new Promise(resolve=>setTimeout(resolve,5));active--
  return handle==='phone-2'?new Response('',{status:503}):response(handle,['v-'+handle.slice(6)])
 })
 assert.equal(max,4);assert.equal(calls,9);assert.equal(Object.keys(variants).length,8)
 const abort=new AbortController();calls=0
 await loadCartVariants(saved,'https://gin.example','region','eur',async()=>{calls++;abort.abort();throw Error('cancel')},abort.signal)
 assert.equal(calls,1)
})

test('invalid cart observation contexts fail before any public request',async()=>{
 let calls=0;const fetcher:typeof fetch=async()=>{calls++;throw Error()}
 await assert.rejects(loadCartVariants(view(),'https://gin.example','region','usd',fetcher))
 const bad=view();bad.items[1]=bad.items[0]
 await assert.rejects(loadCartVariants(bad,'https://gin.example','region','eur',fetcher))
 const large=view(Array(1001).fill('phone'))
 await assert.rejects(loadCartVariants(large,'https://gin.example','region','eur',fetcher));assert.equal(calls,0)
})
