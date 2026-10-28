import {publicJSON} from './gin-public.ts'

export type FooterCategory={id:string;name:string;handle:string;category_children:{id:string;name:string;handle:string}[]}
export type FooterData={iphoneCategory:FooterCategory|null;collections:{id:string;title:string;handle:string}[]}
const unavailable=()=>new Error('Footer catalog unavailable')
const object=(v:unknown):Record<string,unknown>=>{if(!v||typeof v!=='object'||Array.isArray(v))throw unavailable();return v as Record<string,unknown>}
const text=(v:unknown):string=>{if(typeof v!=='string'||Array.from(v).length>4096||/[\x00-\x1f\x7f]/.test(v))throw unavailable();return v}
const identifier=(v:unknown)=>{const s=text(v);if(!s||s.length>191)throw unavailable();return s}
function rows(input:unknown,key:string,limit:number):unknown[]{
 const body=object(input),list=body[key]
 if(!Number.isSafeInteger(body.count)||Number(body.count)<0||body.limit!==limit||body.offset!==0||!Array.isArray(list)||list.length!==Math.min(limit,Number(body.count)))throw unavailable()
 return list
}
function unique(ids:string[]){if(new Set(ids).size!==ids.length)throw unavailable()}

// The original footer shows direct iPhone children and at most six collections.
// Errors remain errors; callers must not replace a failed dependency with an
// apparently successful empty catalog. No private headers cross this boundary.
export async function loadFooterData(origin:string|undefined,fetcher:typeof fetch=fetch):Promise<FooterData>{
 const [categories,collectionBody]=await Promise.all([
  publicJSON(origin,'/api/store/product-categories?handle=iphone&limit=2',2*1024*1024,fetcher),
  publicJSON(origin,'/api/store/collections?limit=6',1024*1024,fetcher)
 ])
 const roots=rows(categories,'product_categories',2)
 if(roots.length>1)throw unavailable()
 let iphoneCategory:FooterCategory|null=null
 if(roots.length){
  const root=object(roots[0]);if(root.handle!=='iphone'||!Array.isArray(root.category_children)||root.category_children.length>1000)throw unavailable()
  const rootID=identifier(root.id)
  const children=root.category_children.map(v=>{const child=object(v);if(child.parent_category_id!==rootID)throw unavailable();return {id:identifier(child.id),name:text(child.name),handle:text(child.handle)}})
  unique([rootID,...children.map(c=>c.id)])
  iphoneCategory={id:rootID,name:text(root.name),handle:'iphone',category_children:children}
 }
 const collections=rows(collectionBody,'collections',6).map(v=>{
  const row=object(v),handle=text(row.handle)
  if(!handle||handle.length>512||/[\\?#]/.test(handle))throw unavailable()
  return {id:identifier(row.id),title:text(row.title),handle}
 })
 unique(collections.map(c=>c.id));unique(collections.map(c=>c.handle))
 return {iphoneCategory,collections}
}
