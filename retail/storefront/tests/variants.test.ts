import test from 'node:test'
import assert from 'node:assert/strict'
import type {Product,Variant} from '../src/lib/products.ts'
import {availableOptionVariants,completeVariants,galleryImages,initialVariant,preferredOptionVariant,productHandleFromSlug,purchaseOptions,selectionMetadata,selectionURL,variantForOptions,variantFromLocation,variantInStock,variantOptions,variantSlug} from '../src/lib/product-variants.ts'
import {loadProductPage} from '../src/lib/product-page.ts'

// Only a reviewed pure reference module is loaded, not a Medusa/Next runtime.
// The computed URL keeps its erased framework type imports out of Astro check.
const legacy = await import(new URL('../../../medusa-store/apps/storefront/src/lib/util/product-variants.ts',import.meta.url).href)
function fixture(): Product {
 const p: Product = {id:'phone',title:'iPhone 13',handle:'swappie-iphone-13',subtitle:null,description:'Original',thumbnail:'/phone.webp',collection_id:null,options:[],images:[{id:'blue-image',url:'/blue.webp',rank:0},{id:'red-image',url:'/red.webp',rank:1}],variants:[],display:{colors:[],model_specs:null,launch_prices:[],source:null}}
 const definitions = [['Color',['Blue','Red']],['Packaging',['Box','Bulk']],['Battery',['85','100']],['Storage',['128 GB','256 GB']],['Condition',['S','A']]] as const
 p.options = definitions.map(([title,values])=>({id:title.toLowerCase(),title,values:values.map((value,rank)=>({id:`${title}-${rank}`,value,rank}))}))
 for (let i=0;i<32;i++) {
  if (i%7===0) continue // Missing combinations are intentional.
  const choices = p.options.map((o,index)=>({option_id:o.id,...o.values[(i>>index)&1]}))
  const values = Object.fromEntries(choices.map(o=>[o.option_id,o.value]))
  const untracked = i%9===0, backorder = i%11===0, stock = i%5===0 ? '0' : '5'
  p.variants.push({id:`v-${i}`,title:choices.map(o=>o.value).join(' · '),sku:`000${i}`,thumbnail:null,variant_rank:i,options:choices,images:[p.images[values.color==='Blue'?0:1]],configuration_complete:true,calculated_price:null,manage_inventory:!untracked,allow_backorder:backorder,inventory_quantity:untracked?null:stock,availability:{state:untracked?'untracked':stock==='5'?'available':backorder?'backorder':'out_of_stock',can_fulfill:untracked||stock==='5'||backorder,max_fulfillable_quantity:untracked?null:stock},display:{slug:`iphone-13-${values.storage.replace(' ','').toLowerCase()}-${values.color.toLowerCase()}-${values.packaging.toLowerCase()}-${values.condition}${values.battery}`,demo_price_basis:null,apple_launch_price:null}})
 }
 const incomplete = structuredClone(p.variants[0]);incomplete.id='incomplete';incomplete.options=incomplete.options.slice(1);incomplete.configuration_complete=false;incomplete.inventory_quantity=null;incomplete.availability={state:'incomplete',can_fulfill:false,max_fulfillable_quantity:null};incomplete.display.slug=null;p.variants.push(incomplete)
 return p
}
function oldProduct(p: Product) {
 return {...p,variants:p.variants.map(v=>({...v,inventory_quantity:v.inventory_quantity===null?undefined:Number(v.inventory_quantity),metadata:{slug:v.display.slug}}))}
}
const ids = (values: {id:string}[]) => values.map(v=>v.id)
function regions() { return {count:1,regions:[{id:'region',name:'Europe',currency_code:'eur',automatic_taxes:true,countries:[{iso_2:'dk',iso_3:'DNK',num_code:'208',name:'DENMARK',display_name:'Denmark',region_id:'region'}]}]} }
function upstream(p: Product, inspect?: (url: URL)=>void): typeof fetch {
 return async (input,init)=>{
  const url = new URL(String(input));inspect?.(url)
  assert.equal(url.origin,'https://gin.example');assert.equal(init?.credentials,'omit');assert.equal(init?.redirect,'error')
  if (url.pathname==='/api/store/regions') return Response.json(regions())
  assert.equal(url.pathname,'/api/store/products');assert.equal(url.searchParams.get('handle'),'swappie-iphone-13');assert.equal(url.searchParams.get('region_id'),'region');assert.equal(url.searchParams.get('limit'),'2')
  return Response.json({count:1,products:[p],limit:2,offset:0,capabilities:{pricing:true,availability:true,checkout:false}})
 }
}

test('option ordering, complete candidates and all generated configuration changes agree with the original pure helper',()=>{
 const p=fixture(), old=oldProduct(p), before=JSON.stringify(p)
 assert.deepEqual(ids(purchaseOptions(p)),ids(legacy.purchaseOptions(old)))
 assert.deepEqual(ids(completeVariants(p)),ids(legacy.completeVariants(old)))
 const selections = [{},...p.variants.map(variantOptions),{storage:'128 GB'},{storage:'256 GB',condition:'A'},{unrelated:'value'}]
 let compared=0
 for (const selected of selections) for (const option of p.options) for (const value of option.values) {
  assert.deepEqual(ids(availableOptionVariants(p,selected,option.id,value.value)),ids(legacy.availableOptionVariants(old,selected,option.id,value.value)))
  assert.equal(preferredOptionVariant(p,selected,option.id,value.value)?.id,legacy.preferredOptionVariant(old,selected,option.id,value.value)?.id);compared++
 }
 assert.ok(compared>=300)
 for (const v of p.variants) assert.deepEqual(variantOptions(v),legacy.variantOptions(old.variants.find(o=>o.id===v.id)))
 assert.deepEqual(availableOptionVariants(p,{},'unknown','anything'),[])
 assert.equal(JSON.stringify(p),before)
})

test('initial location, exact selected options and gallery fallback preserve original request priorities',()=>{
 const p=fixture(), variants=completeVariants(p), a=variants[0], b=variants[1], old=oldProduct(p)
 const path=`/dk-en/products/${variantSlug(a)}`
 assert.equal(variantFromLocation(variants,path,b.id,b.id)?.id,a.id)
 assert.equal(variantFromLocation(variants,path.toUpperCase(),b.id)?.id,a.id)
 assert.equal(variantFromLocation(variants,'/dk-en/products/model',b.id,a.id)?.id,b.id)
 assert.equal(variantFromLocation(variants,'/dk-en/products/model','unknown',a.id)?.id,a.id)
 assert.equal(initialVariant(p,path,b.id)?.id,a.id)
 assert.equal(initialVariant(p,'/dk-en/products/model','unknown')?.id,variants.find(variantInStock)?.id)
 const sold=variants.find(v=>!variantInStock(v))!;assert.equal(initialVariant(p,'/dk-en/products/model',sold.id)?.id,sold.id)
 assert.equal(variantForOptions(p,variantOptions(b))?.id,b.id)
 assert.equal(variantForOptions(p,{...variantOptions(b),extra:'value'}),undefined)
 assert.deepEqual(galleryImages(p,path,b.id),a.images);assert.deepEqual(galleryImages(p,'/dk-en/products/model',null),p.images)
 assert.equal(legacy.variantFromLocation(old.variants,path,b.id,b.id)?.id,a.id)
 for (const value of ['iphone-13-128gb-blue-S85','IPHONE-16-PRO-MAX-1TB-BLACK-A100','swappie-iphone-13','plain-model','iphone-unknown']) assert.equal(productHandleFromSlug(value),legacy.productHandleFromSlug(value))
})

test('stock selection respects incomplete configurations, migration holds and common-warehouse limits',()=>{
 const p=fixture(), v=structuredClone(completeVariants(p)[0])
 v.inventory_quantity='9007199254740993';v.availability={state:'unavailable',can_fulfill:false,max_fulfillable_quantity:'0'}
 assert.equal(variantInStock(v),false)
 v.allow_backorder=true;assert.equal(variantInStock(v),false)
 v.availability={state:'backorder',can_fulfill:true,max_fulfillable_quantity:'0'};assert.equal(variantInStock(v),true)
 v.configuration_complete=false;assert.equal(variantInStock(v),false)
 const allUnavailable=fixture();for(const candidate of allUnavailable.variants){candidate.availability={state:'unavailable',can_fulfill:false,max_fulfillable_quantity:null};candidate.inventory_quantity=null}
 assert.equal(initialVariant(allUnavailable,'/dk-en/products/model',null)?.id,completeVariants(allUnavailable)[0].id)
 allUnavailable.variants.forEach(v=>{v.configuration_complete=false});assert.equal(initialVariant(allUnavailable,'/dk-en/products/model',null),undefined)
 assert.equal(variantInStock(undefined),false)
})

test('selection targets retain other query values and hash and update slug/v_id exactly as the original action',()=>{
 const p=fixture(), v=p.variants[0], path='/dk-zh/products/swappie-iphone-13'
 const target=selectionURL('dk-zh',path,'?v_id=old&campaign=a&campaign=b&v_id=second&q=x%20y','#specs',v)
 assert.equal(target,`/dk-zh/products/${variantSlug(v)}?campaign=a&campaign=b&q=x+y#specs`)
 assert.deepEqual(selectionMetadata(p,v,`/dk-zh/products/${variantSlug(v)}`),{title:`iPhone 13 ${v.title} | EuroRefurb`,canonicalPath:`/dk-zh/products/${variantSlug(v)}`})
 const noSlug=structuredClone(v);noSlug.display.slug=null
 assert.equal(selectionURL('dk-zh',path,'?v_id=old&campaign=a&v_id=second','#images',noSlug),`${path}?v_id=${v.id}&campaign=a#images`)
 assert.equal(selectionMetadata(p,noSlug,path),null)
 for(const bad of ['//evil.example/path','relative','/path?query','/path\\bad']) assert.throws(()=>selectionURL('dk-en',bad,'','',v))
 assert.throws(()=>selectionURL('dk-fr',path,'','',v))
})

test('page composition resolves country, original model and slug without defaulting an unknown combination or swallowing dependency failure',async()=>{
 const p=fixture(), v=completeVariants(p)[0], requests: string[]=[]
 const page=await loadProductPage('https://gin.example','DK-ZH',variantSlug(v)!,new URLSearchParams({v_id:p.variants[1].id,currency_code:'usd'}),upstream(p,url=>requests.push(url.pathname)))
 assert.ok(page);assert.equal(page.region.id,'region');assert.equal(page.requestedVariantID,v.id);assert.equal(page.initialVariant?.id,v.id)
 assert.equal(page.metadata.title,`${p.title} ${v.title}`);assert.equal(page.metadata.canonicalPath,`/dk-zh/products/${variantSlug(v)}`)
 assert.deepEqual(page.metadata.openGraph,{title:'iPhone 13 | EuroRefurb',description:'iPhone 13',images:['/phone.webp']});assert.deepEqual(page.images,v.images)
 assert.deepEqual(requests,['/api/store/regions','/api/store/products'])
 const base=await loadProductPage('https://gin.example','dk-en',p.handle,new URLSearchParams('v_id=unknown'),upstream(p))
 assert.ok(base);assert.equal(base.metadata.title,p.title);assert.equal(base.metadata.canonicalPath,`/dk-en/products/${p.handle}`);assert.equal(base.initialVariant?.id,completeVariants(p).find(variantInStock)?.id)
 assert.deepEqual(base.images,p.images)
 assert.equal(await loadProductPage('https://gin.example','dk-en','iphone-13-128gb-missing-S85',undefined,upstream(p)),null)
 assert.equal(await loadProductPage('https://gin.example','us-en',p.handle,undefined,async()=>Response.json(regions())),null)
 assert.equal(await loadProductPage('https://gin.example','dk-en','missing',undefined,async(input)=>String(input).includes('/regions')?Response.json(regions()):Response.json({count:0,products:[],limit:2,offset:0,capabilities:{pricing:true,availability:true,checkout:false}})),null)
 assert.equal(await loadProductPage('https://gin.example','../bad',p.handle,undefined,async()=>{throw Error('must not fetch')}),null)
 await assert.rejects(loadProductPage('https://gin.example','dk-en',p.handle,undefined,async()=>Response.json({},{status:503})),/Product page data unavailable/)
 await assert.rejects(loadProductPage(undefined,'dk-en',p.handle),/Product page data unavailable/)
 const duplicate=fixture(), extra=structuredClone(v);extra.id='duplicate-slug';duplicate.variants.push(extra)
 await assert.rejects(loadProductPage('https://gin.example','dk-en',variantSlug(v)!,undefined,upstream(duplicate)),/Product page data unavailable/)
 const incomplete=fixture();incomplete.variants[0].configuration_complete=false;incomplete.variants[0].inventory_quantity=null;incomplete.variants[0].availability={state:'incomplete',can_fulfill:false,max_fulfillable_quantity:null}
 assert.equal(await loadProductPage('https://gin.example','dk-en',variantSlug(v)!,undefined,upstream(incomplete)),null)
 const foreign=fixture();foreign.variants[0].calculated_price={calculated_amount:'1',original_amount:null,currency_code:'usd',is_calculated_price_tax_inclusive:false,is_original_price_tax_inclusive:false,is_calculated_price_price_list:false,is_original_price_price_list:false,calculated_price:{price_list_type:null}}
 await assert.rejects(loadProductPage('https://gin.example','dk-en',p.handle,undefined,upstream(foreign)),/Product page data unavailable/)
})
