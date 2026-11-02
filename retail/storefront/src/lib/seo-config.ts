import {publicJSON} from './gin-public.ts'
import {defaultSEO,privateSEOPath,type SeoDefaults} from './seo.ts'

export type SEOPage={path:string;title:string;description:string;canonical:string;og_image:string;hreflang:Record<string,string>|null;structured_data:Record<string,unknown>|unknown[]|null;structured_data_json?:string;robots:string}
export type SEOConfig={settings:Record<string,string>;page:SEOPage|null}
const keys=new Set(['seo.site_url','seo.site_name','seo.home_path','seo.default_title','seo.default_description','seo.robots_txt','seo.llms_txt','seo.indexnow_enabled','seo.indexnow_key','bing.verification_enabled','bing.site_auth_xml','analytics.enabled','ga4.measurement_id','seo.google_search_console.enabled','gsc.verification_code'])
const invalid=()=>new Error('SEO publication unavailable')
const object=(v:unknown):Record<string,unknown>=>{if(!v||typeof v!=='object'||Array.isArray(v))throw invalid();return v as Record<string,unknown>}
const text=(v:unknown)=>{if(typeof v!=='string')throw invalid();return v}
const httpURL=(v:string)=>{const u=new URL(v);if(!['http:','https:'].includes(u.protocol)||u.username||u.password)throw invalid();return u}
export function seoConfig(raw:unknown,path:string):SEOConfig{
 const r=object(raw),settings:Record<string,string>={}
 for(const [key,value]of Object.entries(object(r.settings))){if(!keys.has(key))throw invalid();settings[key]=text(value)}
 const origin=httpURL(settings['seo.site_url']);if(origin.pathname!=='/'||origin.search||origin.hash)throw invalid()
 if(r.page===null)return {settings,page:null}
 const p=object(r.page),page:SEOPage={path:text(p.path),title:text(p.title),description:text(p.description),canonical:text(p.canonical),og_image:text(p.og_image),robots:text(p.robots),hreflang:null,structured_data:null}
 if(page.path!==path)throw invalid()
 for(const url of [page.canonical,page.og_image])if(url)httpURL(url)
 if(p.hreflang!==null){page.hreflang={};for(const [lang,url]of Object.entries(object(p.hreflang))){if(!/^[a-z]{2}(?:-[A-Z]{2})?$|^x-default$/.test(lang))throw invalid();httpURL(text(url));page.hreflang[lang]=text(url)}}
 if(p.structured_data!==null){if(!p.structured_data||typeof p.structured_data!=='object')throw invalid();page.structured_data=p.structured_data as SEOPage['structured_data']}
 if(p.structured_data_json!==undefined){
  const raw=text(p.structured_data_json),parsed:unknown=JSON.parse(raw)
  if(JSON.stringify(parsed)!==JSON.stringify(page.structured_data))throw invalid()
  page.structured_data_json=raw
 }
 if(page.robots&&page.robots.split(',').some(v=>!['index','noindex','follow','nofollow','noarchive','nosnippet','noimageindex'].includes(v.trim())))throw invalid()
 return {settings,page}
}
export async function loadSEOConfig(origin:string|undefined,path:string,fetcher:typeof fetch=fetch){
 path=path.replace(/\/+$/,'')||'/'
 return seoConfig(await publicJSON(origin,`/api/store/seo?path=${encodeURIComponent(path)}`,32*1024*1024,fetcher),path)
}
// Port of withSEO/SEOData; the caller retains defaultSEO on read failure.
export function configuredSEO(url:URL,defaults:SeoDefaults,config:SEOConfig,status=200){
 const {settings:v,page:p}=config,origin=v['seo.site_url'],path=url.pathname.replace(/\/$/,'')||'/'
 const title=p?.title||defaults.title||v['seo.default_title']||v['seo.site_name']||'EuroRefurb'
 const description=p?.description||defaults.description||v['seo.default_description']||undefined
 const base=defaultSEO(new URL(url.pathname+url.search,origin),{...defaults,title,description},status)
 const canonical=p?.canonical||base.canonical,languages=p?.hreflang&&Object.keys(p.hreflang).length?Object.entries(p.hreflang).map(([lang,href])=>({lang,href})):base.languages
 const robots=p?.robots.split(',').map(r=>r.trim())
 const name=v['seo.site_name']||'EuroRefurb'
 const structured=p?.structured_data&&Object.keys(p.structured_data).length?p.structured_data:[{'@context':'https://schema.org','@type':'Organization',name,url:origin},{'@context':'https://schema.org','@type':'WebSite',name,url:origin}]
 return {...base,canonical,languages,structured,structuredJSON:p?.structured_data&&Object.keys(p.structured_data).length?p.structured_data_json:undefined,
  robots:privateSEOPath(path)?'noindex, nofollow':p?[robots?.includes('noindex')?'noindex':'index',robots?.includes('nofollow')?'nofollow':'follow',...['noarchive','nosnippet','noimageindex'].filter(r=>robots?.includes(r))].join(', '):base.robots,
  openGraph:{...base.openGraph,title:p?.title||defaults.openGraph?.title||title,description,url:canonical,images:p?.og_image?[p.og_image]:base.openGraph.images},
  googleVerification:v['seo.google_search_console.enabled']==='true'&&v['gsc.verification_code']?v['gsc.verification_code']:undefined,
  measurement:v['analytics.enabled']==='true'&&/^G-[A-Z0-9]+$/.test(v['ga4.measurement_id']||'')?v['ga4.measurement_id']:undefined,
 }
}
