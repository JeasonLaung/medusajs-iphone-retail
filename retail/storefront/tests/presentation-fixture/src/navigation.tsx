import {createContext,useContext,useSyncExternalStore} from 'react'
export const Navigation=createContext({segment:'dk-en',location:'/dk-en/products/swappie-iphone-13'})
export const useParams=()=>({countryCode:useContext(Navigation).segment})
const subscribe=(changed:()=>void)=>{window.addEventListener('popstate',changed);window.addEventListener('eurorefurb:product-location',changed);return()=>{window.removeEventListener('popstate',changed);window.removeEventListener('eurorefurb:product-location',changed)}}
const address=()=>window.location.pathname+window.location.search+window.location.hash
function useLocation(){const server=useContext(Navigation).location;return useSyncExternalStore(subscribe,address,()=>server)}
export const usePathname=()=>new URL(useLocation(),'https://test.invalid').pathname
export const useSearchParams=()=>new URL(useLocation(),'https://test.invalid').searchParams
export const useRouter=()=>({push:(path:string)=>window.dispatchEvent(new CustomEvent('fixture-navigation',{detail:path}))})
