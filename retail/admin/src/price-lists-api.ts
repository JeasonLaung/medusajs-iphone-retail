import {APIError,request,csrfCookie,stockQuantity,validPriceQuantityBounds} from './api'
export type PriceListSummary={id:string;title:string;status:'active'|'draft';type:'sale'|'override';starts_at:string|null;ends_at:string|null;rules_count:number|null}
export type PriceListDetail=PriceListSummary&{description:string;price_count:number;enabled:boolean;revision:string;rules:{id:string;attribute:string;value:string|null}[]}
export type PriceListCommand={operation_id:string;expected_revision:string;title:string;description:string;status:'active'|'draft';type:'sale'|'override';starts_at:string|null;ends_at:string|null}
const stamp=(v:unknown)=>v===null||typeof v==='string'&&v.length<=40&&!Number.isNaN(Date.parse(v))
const rev=(v:unknown)=>typeof v==='string'&&/^[a-f0-9]{64}$/.test(v)
const summary=(d:PriceListSummary)=>d&&typeof d.id==='string'&&typeof d.title==='string'&&['active','draft'].includes(d.status)&&['sale','override'].includes(d.type)&&stamp(d.starts_at)&&stamp(d.ends_at)&&(d.rules_count===null||Number.isSafeInteger(d.rules_count)&&d.rules_count>=0)
export async function listPriceLists(limit:number,offset:number,signal?:AbortSignal){
 const d=await request<{price_lists:PriceListSummary[];count:number;limit:number;offset:number}>(`price-lists?limit=${limit}&offset=${offset}`,{signal},'/api/admin/')
 if(!d||d.limit!==limit||d.offset!==offset||!Number.isSafeInteger(d.count)||d.count<0||!Array.isArray(d.price_lists)||d.price_lists.length>limit||!d.price_lists.every(summary)||new Set(d.price_lists.map(p=>p.id)).size!==d.price_lists.length)throw new APIError(0,'request.invalid_response');return d
}
export async function readPriceList(id:string,signal?:AbortSignal):Promise<PriceListDetail>{
 const d=await request<PriceListDetail>('price-lists/'+encodeURIComponent(id),{signal},'/api/admin/')
 if(!summary(d)||d.id!==id||typeof d.description!=='string'||!Number.isSafeInteger(d.price_count)||d.price_count<0||typeof d.enabled!=='boolean'||!rev(d.revision)||!Array.isArray(d.rules)||d.rules.length>10000||!d.rules.every(r=>r&&typeof r.id==='string'&&typeof r.attribute==='string'&&(r.value===null||typeof r.value==='string')))throw new APIError(0,'request.invalid_response');return d
}
export async function editPriceList(id:string,body:PriceListCommand){
 const csrf=csrfCookie();if(!csrf)throw new APIError(403,'auth.invalid_csrf')
 const d=await request<{revision:string;replayed:boolean}>('price-lists/'+encodeURIComponent(id),{method:'POST',body,csrf},'/api/admin/')
 if(!d||!rev(d.revision)||typeof d.replayed!=='boolean')throw new APIError(0,'request.invalid_response');return d
}

export type PriceListRemoveCommand={operation_id:string;expected_revision:string}
export async function removePriceList(id:string,body:PriceListRemoveCommand){
 const csrf=csrfCookie();if(!csrf)throw new APIError(403,'auth.invalid_csrf')
 const d=await request<{revision:string;replayed:boolean}>('price-lists/'+encodeURIComponent(id)+'/remove',{method:'POST',body,csrf},'/api/admin/')
 if(!d||!rev(d.revision)||typeof d.replayed!=='boolean')throw new APIError(0,'request.invalid_response');return d
}

export type InitialListPrice={product_id:string;variant_id:string;currency_code:string;amount:string;region_id:string;min_quantity:string;max_quantity:string}
export type PriceListCreateCommand=PriceListCommand&{product_ids:string[];customer_group_ids:string[];prices:InitialListPrice[]}
export type PriceListCreateContext={customer_groups:{id:string;name:string;members:number}[];enabled:boolean;revision:string;currencies:string[];regions:{id:string;name:string;currency_code:string}[];products:{id:string;title:string;variants:{id:string;title:string;sku:string|null;price_set_id:string|null;shared_variants:number}[]}[]}
export async function readPriceListCreate(products:string[],signal:AbortSignal,listID?:string):Promise<PriceListCreateContext>{
 const q=new URLSearchParams();products.forEach(id=>q.append('product_id',id))
 const d=await request<PriceListCreateContext>((listID?'price-lists/'+encodeURIComponent(listID)+'/price-addition':'price-list-creation')+(q.size?'?'+q:''),{signal},'/api/admin/')
 return validatePriceListCreateContext(d,products)
}
function validatePriceListCreateContext(d:PriceListCreateContext,products:string[]){
 if(!Array.isArray(d?.customer_groups)||d.customer_groups.length>1000||!d.customer_groups.every(g=>g&&typeof g.id==='string'&&typeof g.name==='string'))throw new APIError(0,'request.invalid_response')
 const codes=(v:unknown):v is string[]=>Array.isArray(v)&&v.length<=1000&&v.every(c=>typeof c==='string'&&/^[a-z]{3}$/.test(c))&&new Set(v).size===v.length
 if(!d||typeof d.enabled!=='boolean'||!rev(d.revision)||!codes(d.currencies)||!Array.isArray(d.regions)||d.regions.length>1000||!d.regions.every(r=>r&&typeof r.id==='string'&&typeof r.name==='string'&&codes([r.currency_code]))||new Set(d.regions.map(r=>r.id)).size!==d.regions.length||!Array.isArray(d.products)||d.products.length!==products.length||!d.products.every(p=>p&&products.includes(p.id)&&typeof p.title==='string'&&Array.isArray(p.variants)&&p.variants.length<=500&&p.variants.every(v=>v&&typeof v.id==='string'&&typeof v.title==='string'&&(v.sku===null||typeof v.sku==='string')&&(v.price_set_id===null||typeof v.price_set_id==='string')&&Number.isSafeInteger(v.shared_variants)&&v.shared_variants>=0))||new Set(d.products.map(p=>p.id)).size!==products.length)throw new APIError(0,'request.invalid_response')
 const variants=d.products.flatMap(p=>p.variants);if(variants.length>500||new Set(variants.map(v=>v.id)).size!==variants.length)throw new APIError(0,'request.invalid_response');return d
}
export async function createPriceList(body:PriceListCreateCommand){
 const csrf=csrfCookie();if(!csrf)throw new APIError(403,'auth.invalid_csrf')
 const d=await request<{id:string;revision:string;replayed:boolean}>('price-lists',{method:'POST',body,csrf},'/api/admin/')
 if(!d||typeof d.id!=='string'||!d.id.startsWith('plist_')||!rev(d.revision)||typeof d.replayed!=='boolean')throw new APIError(0,'request.invalid_response');return d
}

export type PriceListAddCommand=Pick<PriceListCreateCommand,'operation_id'|'expected_revision'|'product_ids'|'prices'>
export async function addPriceListPrices(id:string,body:PriceListAddCommand){
 const csrf=csrfCookie();if(!csrf)throw new APIError(403,'auth.invalid_csrf')
 const d=await request<{id:string;revision:string;replayed:boolean}>('price-lists/'+encodeURIComponent(id)+'/prices',{method:'POST',body,csrf},'/api/admin/')
 if(!d||d.id!==id||!rev(d.revision)||typeof d.replayed!=='boolean')throw new APIError(0,'request.invalid_response');return d
}

export type PriceListPriceRef={id:string;product_id:string;variant_id:string}
export type PriceListPriceUpdate=PriceListPriceRef&{amount:string;min_quantity:string;max_quantity:string}
export type PriceListBatchCommand={operation_id:string;expected_revision:string;product_ids:string[];create:InitialListPrice[];update:PriceListPriceUpdate[];delete:PriceListPriceRef[]}
export type PriceListBatchPrice=PriceListPriceUpdate&{price_set_id:string;currency_code:string;rules:{id:string;attribute:string;operator:string;value:string;priority:number}[]}
export type PriceListBatchContext=PriceListCreateContext&{prices:PriceListBatchPrice[]}
export async function readPriceListBatch(id:string,products:string[],signal:AbortSignal):Promise<PriceListBatchContext>{
 const q=new URLSearchParams();products.forEach(id=>q.append('product_id',id))
 const d=await request<PriceListBatchContext>('price-lists/'+encodeURIComponent(id)+'/price-batch'+(q.size?'?'+q:''),{signal},'/api/admin/')
 validatePriceListCreateContext(d,products)
 if(!Array.isArray(d.prices)||d.prices.length>500||new Set(d.prices.map(p=>p.id)).size!==d.prices.length||!d.prices.every(p=>p&&typeof p.id==='string'&&typeof p.price_set_id==='string'&&typeof p.currency_code==='string'&&/^[a-z]{3}$/.test(p.currency_code)&&typeof p.amount==='string'&&stockQuantity(p.amount)&&typeof p.min_quantity==='string'&&typeof p.max_quantity==='string'&&validPriceQuantityBounds(p.min_quantity,p.max_quantity)&&d.products.some(product=>product.id===p.product_id&&product.variants.some(v=>v.id===p.variant_id&&v.price_set_id===p.price_set_id))&&Array.isArray(p.rules)&&p.rules.length<=10000&&p.rules.every(r=>r&&typeof r.id==='string'&&typeof r.attribute==='string'&&typeof r.operator==='string'&&typeof r.value==='string'&&Number.isSafeInteger(r.priority))))throw new APIError(0,'request.invalid_response')
 return d
}
export async function batchPriceListPrices(id:string,body:PriceListBatchCommand){
 const csrf=csrfCookie();if(!csrf)throw new APIError(403,'auth.invalid_csrf')
 const d=await request<{id:string;revision:string;replayed:boolean}>('price-lists/'+encodeURIComponent(id)+'/price-batch',{method:'POST',body,csrf},'/api/admin/')
 if(!d||d.id!==id||!rev(d.revision)||typeof d.replayed!=='boolean')throw new APIError(0,'request.invalid_response');return d
}
