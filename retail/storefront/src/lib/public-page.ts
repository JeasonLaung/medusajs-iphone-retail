import {loadRegions} from './regions.ts'
import {loadLocales} from './locales.ts'
import {loadFooterData} from './footer-data.ts'

// Resolve country before loading any optional content. Unknown routes remain
// 404; failed dependencies remain errors for the page's 503 boundary.
export async function loadPublicPage(origin:string|undefined,segment:string,fetcher:typeof fetch=fetch){
 if(!/^[a-z]{2}-(en|zh)$/.test(segment))return null
 const regions=await loadRegions(origin,fetcher)
 if(!regions.some(region=>region.countries.some(country=>country.iso_2===segment.slice(0,2))))return null
 const [locales,footer]=await Promise.all([loadLocales(origin,fetcher),loadFooterData(origin,fetcher)])
 return {regions,locales,footer}
}
