import {localeFromSegment,routeLanguage} from './storefront-route.ts'
import type {PreferenceTarget} from './preferences.ts'

// This new public preference is independent of all old login/cart cookies.
export const localePreferenceCookie='_retail_locale'
export const localePreferenceAge=60*60*24*365
const valid=(v:string)=>v.length<=35&&/^[A-Za-z]{2,8}(?:-[A-Za-z0-9]{1,8})*$/.test(v)&&!/[\r\n]/.test(v)

// Like the original middleware, the route wins when its language differs.
// A same-language regional code (en-GB, zh-Hans, da, etc.) remains a preference;
// routeLanguage intentionally maps all non-Chinese presentation to English.
export function routePreferenceCookie(segment:string,current:string|undefined):string|undefined {
 if(!/^[a-z]{2}-(en|zh)$/.test(segment))return undefined
 const route=localeFromSegment(segment)
 return current!==undefined&&!valid(current)||routeLanguage(current)!==routeLanguage(route)?route:undefined
}

export async function savePreference(target:PreferenceTarget,options:{apply:(target:PreferenceTarget)=>Promise<unknown>;writeLocale:(locale:string)=>void;navigate:(location:string)=>void}):Promise<void>{
 if(!valid(target.locale)||!/^\/[a-z]{2}-(en|zh)(?:\/|$)/.test(target.location)||/[\\\x00-\x1f\x7f]/.test(target.location))throw Error('Preferences unavailable')
 await options.apply(target)
 options.writeLocale(target.locale)
 options.navigate(target.location)
}

export function writeBrowserLocale(locale:string){
 if(!valid(locale))throw Error('Preferences unavailable')
 document.cookie=`${localePreferenceCookie}=${encodeURIComponent(locale)}; Path=/; Max-Age=${localePreferenceAge}; SameSite=Strict${location.protocol==='https:'?'; Secure':''}`
 const values=document.cookie.split(';').map(v=>v.trim()).filter(v=>v.startsWith(localePreferenceCookie+'='))
 if(values.length!==1||values[0]!==localePreferenceCookie+'='+encodeURIComponent(locale))throw Error('Preferences unavailable')
}
