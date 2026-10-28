import type {AnchorHTMLAttributes} from 'react'
export default function LocalizedLink({segment,href,...props}:Omit<AnchorHTMLAttributes<HTMLAnchorElement>,'href'> & {segment:string;href:string}) {
 if (!/^[a-z]{2}(?:-(?:en|zh))?$/.test(segment) || !href.startsWith('/') || href.startsWith('//') || /[\\\x00-\x1f\x7f]/.test(href)) throw new Error('Invalid local link')
 return <a {...props} href={`/${segment}${href}`}/>
}
