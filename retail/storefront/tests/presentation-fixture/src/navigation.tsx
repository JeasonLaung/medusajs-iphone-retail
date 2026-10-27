import {createContext,useContext} from 'react'
export const Navigation=createContext({segment:'dk-en',location:'/dk-en/products/swappie-iphone-13'})
export const useParams=()=>({countryCode:useContext(Navigation).segment})
export const usePathname=()=>new URL(useContext(Navigation).location,'https://test.invalid').pathname
export const useSearchParams=()=>new URL(useContext(Navigation).location,'https://test.invalid').searchParams
