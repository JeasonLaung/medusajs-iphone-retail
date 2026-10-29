import type {SearchClient} from 'instantsearch.js'
import {publicJSON} from '../../lib/gin-public.ts'
import {amountParts} from '../../lib/money.ts'

export const PRODUCT_INDEX_NAME='product'
export const attributes=['category','model','storage_options','battery_options','display_size','color_families','rear_cameras','condition_options','release_year','labels','option_values'] as const
export function indexedCurrency(currency:string){if(!/^[a-z]{3}$/.test(currency))throw new Error('Invalid search currency');return currency}
export function priceAttribute(prefix:string,currency:string){return `${prefix}_${indexedCurrency(currency)}`}
type Params={query?:string;page?:number;hitsPerPage?:number;facetFilters?:(string|string[])[];numericFilters?:(string|string[])[];facets?:string[]}
type Request={indexName:string;params?:Params}
type Query={key:string;query:string;filters:Record<string,string[]>;sort:string;page:number;limit:number;min_price?:string;max_price?:string}
type Price={min:string;max:string;original:string|null;on_sale:boolean}
type Facet={value:string;count:number}
export type NativeResult={key:string;query:string;currency_code:string;hits:{id:string;title:string;handle:string;thumbnail:string|null;values:Record<string,string[]>;colors:{name:string;hex:string}[];model_storage_options:string[];stocked_storage_options:string[];stocked_color_options:string[];price:Price|null}[];count:number;page:number;limit:number;facets:Record<string,Facet[]>;catalog_facets:Record<string,Facet[]>;price_distribution:string[]}
const unavailable=()=>new Error('Search unavailable')
export function displayNumber(value:string){amountParts(value);const n=Number(value);if(!Number.isFinite(n)||n>Number.MAX_SAFE_INTEGER)throw unavailable();return n}
export function nativeQuery(request:Request,key:string,currency:string):Query{
 const p=request.params??{},prefix='product/sort/',price=priceAttribute('min_price',currency)
 const sorts:Record<string,string>={[`${price}:asc`]:'price:asc',[`${price}:desc`]:'price:desc','created_at:desc':'created_at:desc','title:asc':'title:asc','title:desc':'title:desc'}
 const sort=request.indexName==='product'?'relevance':request.indexName.startsWith(prefix)?sorts[request.indexName.slice(prefix.length)]:undefined
 if(!sort||p.page!==undefined&&(!Number.isInteger(p.page)||p.page<0)||p.hitsPerPage!==undefined&&(!Number.isInteger(p.hitsPerPage)||p.hitsPerPage<0||p.hitsPerPage>100))throw unavailable()
 const q:Query={key,query:p.query??'',filters:{},sort,page:(p.page??0)+1,limit:p.hitsPerPage===0?1:p.hitsPerPage??20}
 for(const group of p.facetFilters??[]){
  const values=typeof group==='string'?[group]:group;let attribute='';const selected:string[]=[]
  for(const filter of values){const at=filter.indexOf(':');const a=filter.slice(0,at),value=filter.slice(at+1);if(at<1||!attributes.includes(a as typeof attributes[number])||attribute&&attribute!==a||!value)throw unavailable();attribute=a;selected.push(value)}
  // The permanent iPhone scope is enforced by Gin, separately from series OR.
  if(attribute==='category'&&selected.length===1&&selected[0]==='iPhone')continue
  if(q.filters[attribute])throw unavailable();q.filters[attribute]=selected
 }
 for(const group of p.numericFilters??[]){if(typeof group!=='string')throw unavailable();const m=/^([^<>=]+)(>=|<=|=)([0-9]+(?:\.[0-9]+)?)$/.exec(group);if(!m||m[1]!==price)throw unavailable();displayNumber(m[3]);if(m[2]==='>='||m[2]==='=')q.min_price=m[3];if(m[2]==='<='||m[2]==='=')q.max_price=m[3]}
 return q
}
export function validateNativeResults(value:unknown,queries:Query[],currency:string):NativeResult[]{
 if(!value||typeof value!=='object'||!('results' in value)||!Array.isArray(value.results)||value.results.length!==queries.length)throw unavailable()
 return value.results.map((unknownRow,i)=>{
  const r=unknownRow as NativeResult,q=queries[i]
  if(!r||r.key!==q.key||r.query!==q.query||r.currency_code!==currency||r.page!==q.page||r.limit!==q.limit||!Number.isInteger(r.count)||r.count<0||r.count>1000||!Array.isArray(r.hits)||r.hits.length>q.limit||!Array.isArray(r.price_distribution)||r.price_distribution.length>1000)throw unavailable()
  const strings=(v:unknown):v is string[]=>Array.isArray(v)&&v.length<=1000&&v.every(s=>typeof s==='string'&&s.length<=65536)
  const facets=(v:unknown)=>{if(!v||typeof v!=='object')throw unavailable();for(const a of attributes){const rows=(v as Record<string,unknown>)[a];if(!Array.isArray(rows)||rows.length>10000||rows.some(f=>!f||typeof f.value!=='string'||!Number.isInteger(f.count)||f.count<0))throw unavailable()}}
  facets(r.facets);facets(r.catalog_facets);r.price_distribution.forEach(displayNumber)
  for(const h of r.hits){
   if(!h||typeof h.id!=='string'||typeof h.title!=='string'||typeof h.handle!=='string'||h.thumbnail!==null&&typeof h.thumbnail!=='string'||!h.values||!Array.isArray(h.colors)||!strings(h.model_storage_options)||!strings(h.stocked_storage_options)||!strings(h.stocked_color_options))throw unavailable()
   for(const a of attributes)if(!strings(h.values[a]))throw unavailable()
   for(const c of h.colors)if(!c||typeof c.name!=='string'||!/^#[0-9a-fA-F]{6}$/.test(c.hex))throw unavailable()
   if(h.price){displayNumber(h.price.min);displayNumber(h.price.max);if(h.price.original!==null)displayNumber(h.price.original);if(typeof h.price.on_sale!=='boolean')throw unavailable()}
  }
  return r
 })
}
export function algoliaResult(r:NativeResult,request:Request){
 const p=request.params??{},currency=r.currency_code,price=priceAttribute('min_price',currency)
 const amounts=r.price_distribution.map(displayNumber),sum=amounts.reduce((a,b)=>a+b,0)
 const facets:Record<string,Record<string,number>>=Object.fromEntries(attributes.map(a=>[a,Object.fromEntries(r.facets[a].map(f=>[f.value,f.count]))]))
 // algoliasearch-helper attaches numeric stats while visiting facets keys.
 // Include the numeric facet even though the API intentionally returns raw
 // decimal distribution data instead of decimal strings as object keys.
 facets[price]={}
 const hits=r.hits.map(h=>({objectID:h.id,id:h.id,title:h.title,handle:h.handle,thumbnail:h.thumbnail,...Object.fromEntries(attributes.map(a=>[a,h.values[a]])),color_options:h.colors.map(c=>c.name),color_hexes:h.colors.map(c=>c.hex),model_storage_options:h.model_storage_options,stocked_storage_options:h.stocked_storage_options,stocked_color_options:h.stocked_color_options,__gin_price:h.price,...h.price?{[price]:displayNumber(h.price.min),[priceAttribute('max_price',currency)]:displayNumber(h.price.max),[priceAttribute('original_price',currency)]:h.price.original===null?null:displayNumber(h.price.original),[priceAttribute('on_sale',currency)]:h.price.on_sale}:{}}))
 return {hits:p.hitsPerPage===0?[]:hits,nbHits:r.count,page:(p.page??0),nbPages:p.hitsPerPage===0?0:Math.ceil(r.count/(p.hitsPerPage??20)),hitsPerPage:p.hitsPerPage??20,processingTimeMS:0,query:p.query??'',params:'',index:request.indexName,facets,facets_stats:amounts.length?{[price]:{min:amounts[0],max:amounts[amounts.length-1],avg:sum/amounts.length,sum}}:{},exhaustiveNbHits:true,exhaustiveFacetsCount:true,renderingContent:{},__gin_distribution:amounts}
}
export function createSearchClient(regionID:string,currency:string,origin?:string,fetcher:typeof fetch=fetch,observe?:(results:NativeResult[])=>void):SearchClient{
 indexedCurrency(currency)
 return {async search(input:unknown){
  const requests=(Array.isArray(input)?input:(input as {requests:Request[]}).requests) as Request[]
  if(!Array.isArray(requests)||requests.length>32)throw unavailable()
  const results:ReturnType<typeof algoliaResult>[]=[]
  for(let offset=0;offset<requests.length;offset+=8){
   const batch=requests.slice(offset,offset+8),queries=batch.map((r,i)=>nativeQuery(r,String(offset+i),currency)),body={region_id:regionID,queries}
   let response:unknown
   response=await publicJSON(origin??globalThis.location?.origin,'/api/store/search',8*1024*1024,fetcher,false,body)
   const native=validateNativeResults(response,queries,currency);observe?.(native);results.push(...native.map((r,i)=>algoliaResult(r,batch[i])))
  }
  return {results}
 }} as unknown as SearchClient
}
