import type {Region} from './regions.ts'
import {storefrontSegment} from './storefront-route.ts'

export type Locale={code:string;name:string}
export type PreferenceTarget={country:string;locale:string;regionID:string;currency:string;segment:string;currentPath:string;location:string}
const invalid=()=>new Error('Preferences unavailable')

// Build a navigation intent, never mutate a cart/cookie or claim a save here.
// A production caller must commit cart locale/region/repricing before navigating.
export function preferenceTarget(input:{regions:Region[];locales:Locale[];segment:string;pathname:string;search:string;hash:string;country:string;language:string}):PreferenceTarget{
 const {regions,segment,pathname,search,hash,country,language}=input
 if(!/^[a-z]{2}-(en|zh)$/.test(segment)||!pathname.startsWith('/'+segment+'/')&&pathname!=='/'+segment||/[\\\x00-\x1f\x7f]/.test(pathname+search+hash)||search!==''&&!search.startsWith('?')||hash!==''&&!hash.startsWith('#')||! /^[a-z]{2}$/.test(country))throw invalid()
 const matches=regions.filter(region=>region.countries.some(c=>c.iso_2===country))
 if(matches.length!==1||matches[0].countries.filter(c=>c.iso_2===country).length!==1)throw invalid()
 const region=matches[0]
 if(!region.id||!/^[a-z]{3}$/.test(region.currency_code))throw invalid()
 const locales=[...input.locales]
 for(const fallback of [{code:'en',name:'English'},{code:'zh-CN',name:'简体中文'}])if(!locales.some(l=>base(l.code)===base(fallback.code)))locales.push(fallback)
 if(!/^[A-Za-z]{2,8}(?:-[A-Za-z0-9]{1,8})*$/.test(language)||!locales.some(l=>l.code===language))throw invalid()
 const targetSegment=storefrontSegment(country,language)
 const currentPath=(pathname.slice(segment.length+1)||'/')+search+hash
 return {country,locale:language,regionID:region.id,currency:region.currency_code,segment:targetSegment,currentPath,location:'/'+targetSegment+currentPath}
}
function base(code:string){return code.toLowerCase().split(/[-_]/)[0]}
