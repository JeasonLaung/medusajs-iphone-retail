import test from 'node:test'
import assert from 'node:assert/strict'
import {loadProduct,loadProducts,optionPriceDifference,productPrice,variantPrice,appleLaunchPrice,type Product,type Quote,type ProductQuery} from '../src/lib/products.ts'
import {amountParts,compareAmounts,differenceAmounts,formatAmount,percentageDecrease} from '../src/lib/money.ts'

function price(amount = '49.99', original: string | null = '60', kind: 'sale' | 'override' | null = 'sale'): Quote {
 return {calculated_amount:amount,original_amount:original,currency_code:'eur',is_calculated_price_tax_inclusive:true,is_original_price_tax_inclusive:false,is_calculated_price_price_list:kind !== null,is_original_price_price_list:false,calculated_price:{price_list_type:kind}}
}
function fixture(): Product {
 return {display:{colors:[],model_specs:null,launch_prices:[],source:null},id:'product-1',title:'Phone',handle:'phone',subtitle:null,description:'Original description',thumbnail:'/images/phone.webp',collection_id:null,options:[{id:'storage',title:'Storage',values:[{id:'128',value:'128 GB',rank:0}]}],images:[{id:'image-1',url:'/images/phone.webp',rank:0}],variants:[{display:{slug:null,demo_price_basis:null,apple_launch_price:null},id:'variant-1',title:'128 GB',sku:'0000123456789012345',thumbnail:null,variant_rank:0,options:[{id:'128',option_id:'storage',value:'128 GB'}],images:[{id:'image-1',url:'/images/phone.webp',rank:0}],configuration_complete:true,calculated_price:price(),manage_inventory:true,allow_backorder:false,inventory_quantity:'9007199254740993',availability:{state:'available',can_fulfill:true,max_fulfillable_quantity:'9007199254740993'}}]}
}
function response(p = fixture(), pricing = true) { return {products:[p],count:1,offset:0,limit:12,capabilities:{pricing,availability:true,checkout:false}} }
async function project(p: Product) { return (await loadProducts('https://gin.example',{region_id:'region'},async()=>Response.json(response(p)))).products[0] }

test('currency display preserves major units, locale conventions and decimals beyond safe Number precision',()=>{
 assert.equal(formatAmount('9007199254740993.005','eur'),'€9,007,199,254,740,993.01')
 assert.equal(formatAmount('1.005','eur'),'€1.01')
 assert.equal(formatAmount('49.99','eur'),'€49.99')
 assert.equal(formatAmount('0','eur'),'€0.00')
 assert.equal(formatAmount('1.5','jpy'),'¥2')
 assert.equal(formatAmount('1.2345','kwd'),'KWD\u00a01.235')
 for (const locale of ['en-US','en-IE','zh-CN','de-DE']) for (const amount of ['0','49.99','100.25','9999']) assert.equal(formatAmount(amount,'eur',locale),new Intl.NumberFormat(locale,{style:'currency',currency:'eur'}).format(Number(amount)))
 for (const invalid of [49.99,NaN,Infinity,'NaN','Infinity','1e3','+1','-1','01',' 1','1/2','1.','1,234','9'.repeat(193),'0.'+'0'.repeat(129)]) assert.throws(()=>amountParts(invalid))
 assert.throws(()=>formatAmount('1','EUR'))
})
test('lowest-price comparison, signed option deltas and percent differences remain exact and immutable',()=>{
 assert.equal(compareAmounts('9007199254740993.000000000000000001','9007199254740993'),1)
 assert.equal(compareAmounts('1.00','1'),0)
 assert.deepEqual(differenceAmounts('9007199254740993.000000000000000001','9007199254740993'),{sign:1,amount:'0.000000000000000001'})
 assert.deepEqual(differenceAmounts('0','100.250'),{sign:-1,amount:'100.25'})
 assert.deepEqual(differenceAmounts('1.00','1'),{sign:0,amount:'0'})
 assert.equal(percentageDecrease('100','49.5'),'51'); assert.equal(percentageDecrease('100','100.5'),'-1')
 assert.equal(percentageDecrease('0','0'),null);assert.equal(percentageDecrease('100','100.0001'),'0')
 assert.equal(percentageDecrease('9007199254740993','0'),'100')
})
test('product projection preserves strings, relation membership and availability while dropping private additions',async()=>{
 const p = fixture(), input = JSON.stringify(p)
 const output = await loadProducts('https://gin.example',{region_id:'region'},async()=>Response.json({...response(p),secret:'hidden',products:[{...p,metadata:{private:'hidden'},variants:p.variants.map(v=>({...v,metadata:{private:'hidden'},reserved_quantity:'123'}))}]}))
 assert.deepEqual(output.products,[p]);assert.equal(output.products[0].variants[0].sku,'0000123456789012345');assert.equal(JSON.stringify(p),input)
 const incomplete = fixture();incomplete.options[0].values.push({id:'256',value:'256 GB',rank:1});incomplete.variants[0].options.push({id:'256',option_id:'storage',value:'256 GB'})
 Object.assign(incomplete.variants[0],{configuration_complete:false,calculated_price:null,inventory_quantity:null,availability:{state:'incomplete',can_fulfill:false,max_fulfillable_quantity:null}})
 assert.deepEqual(await project(incomplete),incomplete)
 const unavailable = fixture();Object.assign(unavailable.variants[0],{inventory_quantity:null,availability:{state:'unavailable',can_fulfill:false,max_fulfillable_quantity:null}});assert.deepEqual(await project(unavailable),unavailable)
 const untracked = fixture();Object.assign(untracked.variants[0],{manage_inventory:false,inventory_quantity:null,availability:{state:'untracked',can_fulfill:true,max_fulfillable_quantity:null}});assert.deepEqual(await project(untracked),untracked)
 const backorder = fixture();Object.assign(backorder.variants[0],{allow_backorder:true,inventory_quantity:'0',availability:{state:'backorder',can_fulfill:true,max_fulfillable_quantity:'0'}});assert.deepEqual(await project(backorder),backorder)
})
test('malformed prices, foreign relations, contradictory stock and oversized response relations fail as a whole',async()=>{
 const edits: ((p: Product)=>void)[] = [
  p=>{p.variants[0].options[0].id='foreign'},p=>{p.variants[0].options[0].value='foreign'},p=>{p.variants[0].images[0].url='/foreign'},
  p=>{p.variants[0].options=[]},p=>{p.variants.push(p.variants[0])},p=>{p.variants[0].calculated_price!.calculated_amount=49.99 as unknown as string},
  p=>{p.variants[0].calculated_price!.original_amount='NaN'},p=>{p.variants[0].calculated_price!.currency_code='EUR'},p=>{p.variants[0].calculated_price!.is_calculated_price_price_list=false},
  p=>{p.variants[0].configuration_complete=false},p=>{p.variants[0].inventory_quantity='01'},p=>{p.variants[0].availability.max_fulfillable_quantity='9007199254740994'},
  p=>{p.variants[0].availability.can_fulfill=false},p=>{p.variants[0].availability.state='untracked'},p=>{p.variants[0].availability.state='backorder'},
  p=>{p.variants[0].availability.max_fulfillable_quantity=null},p=>{p.images=Array.from({length:1001},(_,i)=>({id:`im-${i}`,url:'/image',rank:i}))},
  p=>{p.options=Array.from({length:1000},(_,i)=>({id:`o-${i}`,title:'Option',values:Array.from({length:11},(_,j)=>({id:`v-${i}-${j}`,value:'x',rank:0}))}))},
 ]
 for (const edit of edits) { const p = fixture();edit(p);await assert.rejects(project(p),error=>error instanceof Error && error.message==='Store products unavailable') }
 const noPrice = fixture();noPrice.variants[0].calculated_price=null
 const invalidBodies = [{...response(),count:2},{...response(),offset:1},{...response(),limit:13},{...response(),capabilities:{pricing:true,availability:true,checkout:true}},{...response(),products:[fixture(),fixture()]},{...response(),count:0}]
 for (const body of invalidBodies) await assert.rejects(loadProducts('https://gin.example',{region_id:'region'},async()=>Response.json(body)))
 await assert.rejects(loadProducts('https://gin.example',{},async()=>Response.json(response(fixture(),false))))
 assert.deepEqual((await loadProducts('https://gin.example',{},async()=>Response.json(response(noPrice,false)))).products,[noPrice])
})
test('Gin product queries use fixed paths, encode filters and never forward credentials or arbitrary overrides',async()=>{
 let calls = 0
 const upstream: typeof fetch = async (input,init)=>{
  calls++;const url = new URL(String(input));assert.equal(url.origin,'https://gin.example');assert.equal(url.pathname,'/api/store/products')
  assert.equal(url.searchParams.get('q'),'Phone & 100%');assert.equal(url.searchParams.get('region_id'),'region');assert.equal(url.searchParams.get('quantity'),'2')
  assert.equal(init?.credentials,'omit');assert.equal(init?.cache,'no-store');assert.equal(init?.redirect,'error');assert.deepEqual(init?.headers,{Accept:'application/json'});assert.ok(init?.signal)
  return Response.json(response())
 }
 await loadProducts('https://gin.example',{q:'Phone & 100%',region_id:'region',quantity:'2'},upstream)
 assert.equal(calls,1)
 for (const input of [{quantity:'1'},{quantity:'0',region_id:'r'},{quantity:'1e3',region_id:'r'},{quantity:'1000000001',region_id:'r'},{region_id:''},{q:'\n'},{limit:0},{limit:101},{offset:100001},{offset:1.5},{currency_code:'usd'},{sales_channel_id:'foreign'},{fields:'*'},{limit:null}]) await assert.rejects(loadProducts('https://gin.example',input as ProductQuery,upstream))
 assert.equal(calls,1)
 const empty = await loadProducts('https://gin.example',{offset:100,limit:5},async()=>Response.json({products:[],count:4,limit:5,offset:100,capabilities:{pricing:false,availability:true,checkout:false}}));assert.deepEqual(empty.products,[])
})
test('unknown detail is null; upstream failure, malformed UTF-8, redirects and byte overflow never become empty products',async()=>{
 const p = fixture()
 assert.deepEqual(await loadProduct('https://gin.example','product-1',{region_id:'region'},async(input)=>{ assert.equal(String(input),'https://gin.example/api/store/products/product-1?region_id=region');return Response.json({product:p,capabilities:response().capabilities}) }),p)
 assert.equal(await loadProduct('https://gin.example','missing',{},async()=>Response.json({private:'hidden'},{status:404})),null)
 await assert.rejects(loadProduct('https://gin.example','product-1',{},async()=>Response.json(null)))
 await assert.rejects(loadProduct('https://gin.example','foreign',{region_id:'region'},async()=>Response.json({product:p,capabilities:response().capabilities})))
 await assert.rejects(loadProduct('https://gin.example','p',{q:'forbidden'} as ProductQuery,async()=>Response.json({})))
 for (const origin of [undefined,'','https://user:secret@gin.example','file:///tmp/data','https://gin.example/path','https://gin.example/?q=1','https://gin.example/#part']) await assert.rejects(loadProducts(origin,{},async()=>Response.json({})))
 for (const reply of [Response.json({private:'hidden'},{status:404}),Response.json({},{status:503}),Response.json({},{status:302}),new Response('private',{headers:{'Content-Type':'text/html'}}),new Response(new Uint8Array([0xff]),{headers:{'Content-Type':'application/json'}}),new Response(new Uint8Array(8*1024*1024+1),{headers:{'Content-Type':'application/json'}})]) await assert.rejects(loadProducts('https://gin.example',{},async()=>reply),error=>error instanceof Error && error.message==='Store products unavailable')
 await assert.rejects(loadProducts('https://gin.example',{},async()=>{throw new Error('private address')}),error=>error instanceof Error && !String(error).includes('private'))
})
test('product price adapter preserves cheapest/selected semantics and exact option differences without selling incomplete variants',()=>{
 const p = fixture(), v = p.variants[0]
 v.calculated_price=price('9007199254740993.000000000000000001',null,null)
 const cheaper = structuredClone(v);cheaper.id='variant-2';cheaper.sku='0002';cheaper.calculated_price=price('9007199254740993',null,null);p.variants.push(cheaper)
 const before = JSON.stringify(p), view = productPrice(p,'0000123456789012345','en-IE')
 assert.equal(view.cheapestPrice?.calculated_amount,'9007199254740993');assert.equal(view.variantPrice?.calculated_amount,'9007199254740993.000000000000000001')
 assert.deepEqual(optionPriceDifference(v,cheaper),{sign:1,amount:'0.000000000000000001'});assert.equal(JSON.stringify(p),before)
 assert.equal(productPrice(p,'unknown').variantPrice,null);v.configuration_complete=false;assert.equal(variantPrice(v),null);assert.equal(optionPriceDifference(v,cheaper),null)
 cheaper.calculated_price=price('0',null,null);assert.equal(variantPrice(cheaper)?.percentage_diff,null);assert.equal(variantPrice(cheaper)?.calculated_price,'€0.00')
 cheaper.calculated_price=null;assert.equal(productPrice(p).cheapestPrice,null)
 const mixed = fixture();mixed.variants.push({...structuredClone(mixed.variants[0]),id:'usd',calculated_price:{...price(),currency_code:'usd'}});assert.throws(()=>productPrice(mixed))
})

test('reviewed display preserves colors, specification strings, slug and launch-price precedence without private metadata',async()=>{
 const p=fixture();p.title='iPhone 13'
 p.display={colors:[{name:'Blue',hex:'#0000FF'}],model_specs:{release_year:'2021',display_size_inches:'6.1',tech_specs_url:'https://support.apple.com/specs',apple_model_numbers:['A0123']},launch_prices:[{storage:'128 GB',amount:'9007199254740993.005',currency_code:'eur'}],source:'swappie-html-snapshot'}
 p.variants[0].display={slug:'iphone-13-128gb-blue-S85',demo_price_basis:'model-starting-price',apple_launch_price:{amount:'869.125000000000000001',currency_code:'eur'}}
 assert.deepEqual(await project(p),p)
 assert.equal(appleLaunchPrice(p,p.variants[0],'eur'),'869.125000000000000001')
 p.variants[0].display.apple_launch_price=null;assert.equal(appleLaunchPrice(p,p.variants[0],'eur'),'9007199254740993.005')
 p.variants[0].display.apple_launch_price={amount:'1000',currency_code:'usd'}
 assert.equal(appleLaunchPrice(p,p.variants[0],'eur'),'869');assert.equal(appleLaunchPrice(p,p.variants[0],'usd'),'1000')
 p.variants[0].display.apple_launch_price=null;p.display.launch_prices=[]
 assert.equal(appleLaunchPrice(p,p.variants[0],'eur'),'869');assert.equal(appleLaunchPrice(p,p.variants[0],'usd'),null);assert.equal(appleLaunchPrice(p,undefined,'eur'),null)
 const raw=JSON.parse(JSON.stringify(p));raw.display.private='secret';raw.display.model_specs.private='secret';raw.variants[0].display.private='secret'
 assert.deepEqual(await project(raw),p)
 for(const edit of [(p:Product)=>{p.display.colors[0].hex='url(secret)'},(p:Product)=>{p.display.model_specs!.release_year=2021 as unknown as string},(p:Product)=>{p.display.model_specs!.tech_specs_url='https://apple.com.evil.example/specs'},(p:Product)=>{p.variants[0].display.slug='iphone-13-S85?secret'},(p:Product)=>{p.display.launch_prices=[{storage:'128 GB',amount:'0',currency_code:'eur'}]},(p:Product)=>{p.display.colors=Array.from({length:65},()=>({name:'Blue',hex:'#0000FF'}))}]){const value=structuredClone(p);edit(value);await assert.rejects(project(value))}
})
