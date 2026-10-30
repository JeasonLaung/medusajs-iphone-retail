import type {AnchorHTMLAttributes} from 'react'
import Link from '../products/localized-link'
import {useSellNavigation} from './navigation'
export default function LocalizedLink(props:Omit<AnchorHTMLAttributes<HTMLAnchorElement>,'href'>&{href:string}){return <Link {...props} segment={useSellNavigation().countryCode}/>}
