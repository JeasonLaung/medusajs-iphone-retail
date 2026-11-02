import {loadSEOConfig} from './seo-config.ts'

export type SEOArtifact='robots'|'llms'|'bing'|'indexnow'
const response=(body:string|null,status=200,type='text/plain')=>new Response(body,{status,headers:{'Content-Type':`${type}; charset=utf-8`,'Cache-Control':'no-store'}})

// These are publication files only; serving them never submits to a provider.
// Keep the original route bodies and explicit verification switches.
export async function seoArtifact(kind:SEOArtifact,origin:string|undefined,key?:string,fetcher:typeof fetch=fetch):Promise<Response>{
 try{
  const {settings:v}=await loadSEOConfig(origin,'/',fetcher),site=v['seo.site_url']
  if(kind==='robots')return response(v['seo.robots_txt']||`User-agent: *\nAllow: /\nDisallow: /*/account\nDisallow: /*/checkout\nDisallow: /*/cart\nDisallow: /*/order\nDisallow: /api/\nSitemap: ${site}/sitemap.xml\n`)
  if(kind==='llms'){
   const home=v['seo.home_path']||'/dk-en'
   return response(v['seo.llms_txt']||`# ${v['seo.site_name']||'EuroRefurb'}\n\n${v['seo.default_description']||'Refurbished iPhones. Product specifications, prices and availability are provided on each product page.'}\n\n## Pages\n- [Store](${site}${home}/store)\n- [About](${site}${home}/about)\n- [Help](${site}${home}/help)\n- [Sitemap](${site}/sitemap.xml)\n`)
  }
  if(kind==='bing')return v['bing.verification_enabled']==='true'&&v['bing.site_auth_xml']?response(v['bing.site_auth_xml'],200,'application/xml'):response(null,404)
  return key&&/^[A-Za-z0-9-]{8,128}$/.test(key)&&v['seo.indexnow_enabled']==='true'&&key===v['seo.indexnow_key']?response(key):response(null,404)
 }catch{return response('SEO publication unavailable',503)}
}
