import {useParams} from './navigation'
import type {AnchorHTMLAttributes} from 'react'
export default function Link({href,...props}:AnchorHTMLAttributes<HTMLAnchorElement>){const {countryCode}=useParams();return <a {...props} href={`/${countryCode}${href}`}/>}
