import {createContext,useContext,type ReactNode,type AnchorHTMLAttributes} from 'react'
import LocalizedLink from '../products/localized-link'
const Route=createContext('dk-en')
export function SearchRoute({segment,children}:{segment:string;children:ReactNode}){return <Route.Provider value={segment}>{children}</Route.Provider>}
export function useSearchRoute(){return {countryCode:useContext(Route)}}
export function SearchLink(props:Omit<AnchorHTMLAttributes<HTMLAnchorElement>,'href'>&{href:string}){return <LocalizedLink segment={useContext(Route)} {...props}/>}
