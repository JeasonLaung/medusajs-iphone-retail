// Existing @medusajs/icons 2.21.2 paths, MIT; license retained in ../products/ICONS-LICENSE.txt.
import type {SVGProps} from 'react'
function Icon({path,color='currentColor',...props}:SVGProps<SVGSVGElement>&{path:string}){return <svg xmlns="http://www.w3.org/2000/svg" width={15} height={15} viewBox="0 0 15 15" fill="none" {...props}><path stroke={color} strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d={path}/></svg>}
export const ChevronDown=(props:SVGProps<SVGSVGElement>)=><Icon path="M13.056 5.278 7.5 10.833 1.944 5.278" {...props}/>
export const XMark=(props:SVGProps<SVGSVGElement>)=><Icon path="m11.25 3.75-7.5 7.5M3.75 3.75l7.5 7.5" {...props}/>
