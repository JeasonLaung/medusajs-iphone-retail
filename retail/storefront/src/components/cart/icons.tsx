// Original @medusajs/icons 2.21.2 paths, MIT; see ../products/ICONS-LICENSE.txt.
import type {SVGProps} from 'react'
const Icon=({path,color='currentColor',...props}:SVGProps<SVGSVGElement>&{path:string})=><svg xmlns="http://www.w3.org/2000/svg" width={15} height={15} viewBox="0 0 15 15" fill="none" {...props}><path stroke={color} strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d={path}/></svg>
export const Minus=(props:SVGProps<SVGSVGElement>)=><Icon path="M2.5 7.5h10" {...props}/>
export const Plus=(props:SVGProps<SVGSVGElement>)=><Icon path="M7.5 2.5v10M2.5 7.5h10" {...props}/>
