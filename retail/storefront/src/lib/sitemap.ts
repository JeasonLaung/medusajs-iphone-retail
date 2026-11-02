import {publicJSON} from './gin-public.ts'
import {loadSEOConfig,seoConfig,type SEOConfig} from './seo-config.ts'
import {loadRegions} from './regions.ts'
import {getHomeCopy} from '../components/journal/i18n.ts'

export type DiscoveryEntry={handle:string;updated_at:string;slugs:string[]}
export type DiscoveryPage={path:string;robots:string;canonical:string;hreflang:Record<string,string>|null;updated_at:string}
const invalid=()=>new Error('Sitemap unavailable')
const object=(v:unknown):Record<string,unknown>=>{if(!v||typeof v!=='object'||Array.isArray(v))throw invalid();return v as Record<string,unknown>}
const text=(v:unknown)=>{if(typeof v!=='string')throw invalid();return v}
const timestamp=(v:unknown)=>{const s=text(v);if(!Number.isFinite(Date.parse(s)))throw invalid();return s}
const xml=(text:string)=>text.replace(/[<>&"']/g,c=>({'<':'&lt;','>':'&gt;','&':'&amp;','"':'&quot;',"'":'&apos;'}[c]!))

export async function loadDiscoveryCatalog(origin:string|undefined,kind:'products'|'collections',fetcher:typeof fetch=fetch):Promise<DiscoveryEntry[]>{
 const entries:DiscoveryEntry[]=[];let count:number|undefined
 for(let offset=0;;){
  const raw=object(await publicJSON(origin,`/api/store/seo/catalog?kind=${kind}&limit=100&offset=${offset}`,32*1024*1024,fetcher))
  if(!Number.isSafeInteger(raw.count)||Number(raw.count)<0||raw.limit!==100||raw.offset!==offset||!Array.isArray(raw.entries)||raw.entries.length>100)throw invalid()
  if(count!==undefined&&raw.count!==count)throw invalid()
  count=Number(raw.count)
  if(raw.entries.length!==Math.min(100,count-offset))throw invalid()
  for(const value of raw.entries){
   const r=object(value),handle=text(r.handle)
   if(Array.from(handle).length>512||/[\x00-\x1f\x7f]/.test(handle)||!Array.isArray(r.slugs)||r.slugs.length>10000)throw invalid()
   const slugs=r.slugs.map(v=>{const s=text(v);if(!/^iphone-[a-z0-9-]+-[SA](?:85|100)$/.test(s))throw invalid();return s})
   if(kind==='collections'&&slugs.length)throw invalid()
   entries.push({handle,updated_at:timestamp(r.updated_at),slugs})
  }
  offset+=raw.entries.length
  if(offset>=count)return entries
 }
}

export function discoveryPages(raw:unknown,settings:SEOConfig['settings']):DiscoveryPage[]{
 const rows=object(raw).pages;if(!Array.isArray(rows))throw invalid()
 const seen=new Set<string>()
 return rows.map(value=>{
  const p=object(value),path=text(p.path)
  if(!path.startsWith('/')||path.startsWith('//')||/[?#\\\x00-\x1f]/.test(path)||seen.has(path))throw invalid()
  seen.add(path)
  const page=seoConfig({settings,page:{...p,title:'',description:'',og_image:'',structured_data:null}},path).page!
  return {path,robots:page.robots,canonical:page.canonical,hreflang:page.hreflang,updated_at:timestamp(p.updated_at)}
 })
}

// Preserve the source's paths, de-duplication, override filtering, timestamps,
// alternates and 1000-document partitioning. No provider submission occurs.
export function sitemapResponse(url:URL,settings:SEOConfig['settings'],countries:string[],products:DiscoveryEntry[],collections:DiscoveryEntry[],pages:DiscoveryPage[]):Response{
 const origin=settings['seo.site_url'],overrides=new Map(pages.map(p=>[p.path,p]))
 const entries=new Map<string,{lastmod?:string;country:string;rest:string}>()
 for(const country of new Set(countries))for(const language of ['en','zh']){
  const prefix=`/${country}-${language}`
  for(const rest of ['','/store','/about','/help','/reviews','/blog','/sell',...getHomeCopy(language==='zh'?'zh-CN':'en').blogPosts.map(p=>`/blog/${p.slug}`)])entries.set(prefix+rest,{country,rest})
  for(const p of products){
   if(!p.handle)continue
   for(const slug of [p.handle,...p.slugs]){const rest=`/products/${encodeURIComponent(slug)}`;entries.set(prefix+rest,{lastmod:p.updated_at,country,rest})}
  }
  for(const c of collections)if(c.handle){const rest=`/collections/${encodeURIComponent(c.handle)}`;entries.set(prefix+rest,{lastmod:c.updated_at,country,rest})}
 }
 const documents=Array.from(entries).map(([path,entry])=>{
  const page=overrides.get(path)
  if(page?.robots?.split(',').some(r=>r.trim()==='noindex')||(page?.canonical&&page.canonical.replace(/\/$/,'')!==origin+path))return ''
  const lastmod=[entry.lastmod,page?.updated_at].filter(Boolean).sort().at(-1)
  const alternates=page?.hreflang&&Object.keys(page.hreflang).length?Object.entries(page.hreflang):['en','zh-CN','x-default'].map(lang=>[lang,`${origin}/${entry.country}-${lang==='zh-CN'?'zh':'en'}${entry.rest}`])
  return `<url><loc>${xml(origin+path)}</loc>${lastmod?`<lastmod>${xml(new Date(lastmod).toISOString())}</lastmod>`:''}${alternates.map(([lang,href])=>`<xhtml:link rel="alternate" hreflang="${xml(lang)}" href="${xml(href)}"/>`).join('')}</url>`
 }).filter(Boolean)
 const size=1000,part=url.searchParams.get('part')
 let content:string
 if(part===null&&documents.length>size){
  content=`<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">${Array.from({length:Math.ceil(documents.length/size)},(_,i)=>`<sitemap><loc>${xml(`${origin}/sitemap.xml?part=${i+1}`)}</loc></sitemap>`).join('')}</sitemapindex>`
 }else{
  const index=part===null?1:Number(part)
  if(!Number.isInteger(index)||index<1||(index-1)*size>=documents.length)return new Response(null,{status:404,headers:{'Cache-Control':'no-store'}})
  content=`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" xmlns:xhtml="http://www.w3.org/1999/xhtml">${documents.slice((index-1)*size,index*size).join('')}</urlset>`
 }
 return new Response(`<?xml version="1.0" encoding="UTF-8"?>${content}`,{headers:{'Content-Type':'application/xml; charset=utf-8','Cache-Control':'no-store'}})
}

export async function loadSitemap(url:URL,origin:string|undefined,fetcher:typeof fetch=fetch):Promise<Response>{
 try{
  const [config,regions,rawPages,products,collections]=await Promise.all([loadSEOConfig(origin,'/',fetcher),loadRegions(origin,fetcher),publicJSON(origin,'/api/store/seo/pages',32*1024*1024,fetcher),loadDiscoveryCatalog(origin,'products',fetcher),loadDiscoveryCatalog(origin,'collections',fetcher)])
  return sitemapResponse(url,config.settings,regions.flatMap(r=>r.countries.map(c=>c.iso_2)),products,collections,discoveryPages(rawPages,config.settings))
 }catch{return new Response('Sitemap unavailable',{status:503,headers:{'Content-Type':'text/plain; charset=utf-8','Cache-Control':'no-store'}})}
}
