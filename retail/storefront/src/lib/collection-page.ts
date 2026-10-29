import {publicJSON} from './gin-public.ts'
import {loadProducts,type Product} from './products.ts'
import type {Region} from './regions.ts'
import {compareAmounts} from './money.ts'

export type CollectionSort='created_at'|'price_asc'|'price_desc'
export type CollectionQuery={page:number;sort:CollectionSort;optionValues:string[]}
export type CollectionPage={collection:{id:string;title:string;handle:string};products:Product[];count:number;totalPages:number;query:CollectionQuery}
const invalid=()=>new Error('Collection unavailable')
function text(value:unknown,max:number){if(typeof value!=='string'||!value||Array.from(value).length>max||/[\x00-\x1f\x7f]/.test(value))throw invalid();return value}
function object(value:unknown):Record<string,unknown>{if(!value||typeof value!=='object'||Array.isArray(value))throw invalid();return value as Record<string,unknown>}
export function parseCollectionQuery(params:URLSearchParams):CollectionQuery{
 const pages=params.getAll('page'),sorts=params.getAll('sortBy')
 if(pages.length>1||sorts.length>1)throw invalid()
 const raw=pages[0]??'1',sort=sorts[0]??'created_at'
 if(!/^[1-9][0-9]{0,5}$/.test(raw)||!['created_at','price_asc','price_desc'].includes(sort))throw invalid()
 const selected=params.getAll('optionValueIds'),values=selected.length===1?selected[0]!.split(','):selected
 if(values.length>100)throw invalid()
 return {page:Number(raw),sort:sort as CollectionSort,optionValues:[...new Set(values.filter(Boolean).map(v=>text(v,191)))]}
}
// Preserve the original 100-product working set, 12-product pages and minimum
// variant price ordering. Exact decimals replace Number arithmetic. A missing
// variant price retains the source's zero sorting key, never a displayed price.
export function collectionProducts(products:Product[],sort:CollectionSort):Product[]{
 const minimum=(p:Product):string|null=>{
  let result:string|null=null
  for(const v of p.variants){const price=v.calculated_price?.calculated_amount??'0';if(result===null||compareAmounts(price,result)<0)result=price}
  return result
 }
 if(sort==='created_at')return [...products]
 const keys=new Map(products.map(p=>[p.id,minimum(p)]))
 return [...products].sort((a,b)=>{
  const x=keys.get(a.id)!,y=keys.get(b.id)!,delta=x===null?(y===null?0:1):y===null?-1:compareAmounts(x,y)
  return sort==='price_asc'?delta:-delta
 })
}
export function collectionNavigation(current:string,change:{page:number}|{sort:CollectionSort}):string{
 const url=new URL(current),params=url.searchParams
 if('page'in change)params.set('page',String(change.page))
 else{params.set('sortBy',change.sort);params.delete('page')}
 return url.pathname+(params.size?'?'+params.toString():'')
}
export async function loadCollectionPage(origin:string|undefined,handle:string,region:Region,query:CollectionQuery,fetcher:typeof fetch=fetch):Promise<CollectionPage|null>{
 text(handle,512)
 const params=new URLSearchParams({handle,limit:'2'}),body=object(await publicJSON(origin,`/api/store/collections?${params}`,1024*1024,fetcher))
 const rows=body.collections
 if(!Number.isSafeInteger(body.count)||Number(body.count)<0||Number(body.count)>1||body.limit!==2||body.offset!==0||!Array.isArray(rows)||rows.length!==body.count)throw invalid()
 if(!rows.length)return null
 const row=object(rows[0]),collection={id:text(row.id,191),title:text(row.title,4096),handle:text(row.handle,512)}
 if(collection.handle!==handle)throw invalid()
 const listed=await loadProducts(origin,{collection_id:collection.id,region_id:region.id,limit:100,...(query.sort==='created_at'?{order:'-created_at' as const}:{}),...(query.optionValues.length?{option_value_id:query.optionValues}:{})},fetcher)
 for(const p of listed.products){
  if(p.collection_id!==collection.id||p.variants.some(v=>v.calculated_price&&v.calculated_price.currency_code!==region.currency_code))throw invalid()
 }
 const sorted=collectionProducts(listed.products,query.sort),offset=(query.page-1)*12
 return {collection,products:sorted.slice(offset,offset+12),count:sorted.length,totalPages:Math.ceil(sorted.length/12),query}
}
