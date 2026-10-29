import {publicJSON} from './gin-public.ts'
import {loadRegions} from './regions.ts'
import {storeSeriesUrl} from './store-series-url.ts'

const unavailable=()=>new Error('Category redirect unavailable')
function object(value:unknown):Record<string,unknown>{
 if(!value||typeof value!=='object'||Array.isArray(value))throw unavailable()
 return value as Record<string,unknown>
}
function text(value:unknown,max:number):string{
 if(typeof value!=='string'||!value||Array.from(value).length>max||/[\x00-\x1f\x7f]/.test(value))throw unavailable()
 return value
}

// The original category page permanently redirects to store. Only a direct
// child of the public iPhone category adds its name as the series refinement;
// unknown categories also redirect to store, rather than becoming new pages.
export async function loadCategoryRedirect(origin:string|undefined,segment:string,handle:string,fetcher:typeof fetch=fetch):Promise<string|null>{
 if(!/^[a-z]{2}-(en|zh)$/.test(segment)||!handle||Array.from(handle).length>512||/[\x00-\x1f\x7f]/.test(handle))return null
 const regions=await loadRegions(origin,fetcher)
 if(!regions.some(r=>r.countries.some(c=>c.iso_2===segment.slice(0,2))))return null
 const query=new URLSearchParams({handle,limit:'2'})
 const body=object(await publicJSON(origin,`/api/store/product-categories?${query}`,2*1024*1024,fetcher))
 const rows=body.product_categories
 if(!Number.isSafeInteger(body.count)||Number(body.count)<0||Number(body.count)>1||body.limit!==2||body.offset!==0||!Array.isArray(rows)||rows.length!==body.count)throw unavailable()
 let series:string|undefined
 if(rows.length){
  const row=object(rows[0]),id=text(row.id,191),name=text(row.name,4096)
  if(text(row.handle,512)!==handle)throw unavailable()
  if(row.parent_category===null){
   if(row.parent_category_id!==null)throw unavailable()
  }else{
   const parent=object(row.parent_category),parentID=text(parent.id,191)
   if(parentID===id||row.parent_category_id!==parentID)throw unavailable()
   if(text(parent.handle,512)==='iphone')series=name
  }
 }
 return `/${segment}${storeSeriesUrl(series)}`
}
