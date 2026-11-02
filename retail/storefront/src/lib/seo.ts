// Default metadata behavior from medusa-store/src/lib/data/seo.ts and
// modules/common/components/seo-data. Configured SEO publication is separate.
export type SeoDefaults={title:string;description?:string;robots?:string;canonicalPath?:string;openGraph?:{title:string;description:string;images:string[]}}
export const privateSEOPath=(path:string)=>/\/(account|checkout|cart|order|payment-result|verify-account)(\/|$)/.test(path)||/\/sell\/(iphone|requests)(\/|$)/.test(path)
export const jsonLD=(value:unknown)=>jsonLDText(JSON.stringify(value))
// Escape script delimiters without parsing or rounding raw JSON number tokens.
export const jsonLDText=(raw:string)=>raw.replace(/</g,'\\u003c').replace(/>/g,'\\u003e').replace(/&/g,'\\u0026')
export function defaultSEO(url:URL,defaults:SeoDefaults,status=200){
 const path=url.pathname.replace(/\/$/,'')||'/',origin=url.origin
 const country=path.split('/')[1]?.match(/^([a-z]{2})-(en|zh)$/)?.[1]
 const canonical=new URL(defaults.canonicalPath||path,origin).href
 const languages=country?['en','zh-CN','x-default'].map(lang=>({lang,href:origin+path.replace(/^\/[a-z]{2}-(en|zh)/,`/${country}-${lang==='zh-CN'?'zh':'en'}`)})):[]
 return {...defaults,canonical,languages,
  robots:privateSEOPath(path)?'noindex, nofollow':defaults.robots||(status>=400?'noindex':undefined),
  openGraph:{title:defaults.openGraph?.title??defaults.title,description:defaults.openGraph?.description??defaults.description,url:canonical,images:(defaults.openGraph?.images??[]).map(image=>new URL(image,origin).href)},
  structured:[{'@context':'https://schema.org','@type':'Organization',name:'EuroRefurb',url:origin},{'@context':'https://schema.org','@type':'WebSite',name:'EuroRefurb',url:origin}],
 }
}
