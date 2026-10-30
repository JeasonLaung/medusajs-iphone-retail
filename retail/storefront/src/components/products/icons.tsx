// Existing @medusajs/icons 2.21.2 SVG paths, MIT; see ICONS-LICENSE.txt.
// Kept locally so migrated components do not require a Medusa runtime package.
import type {SVGProps} from 'react'
function Icon({path,color = 'currentColor',...props}: SVGProps<SVGSVGElement> & {path: string}) {
 return <svg xmlns="http://www.w3.org/2000/svg" width={15} height={15} viewBox="0 0 15 15" fill="none" {...props}><path stroke={color} strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d={path} /></svg>
}
export const ChevronLeft = (props: SVGProps<SVGSVGElement>) => <Icon path="M9.25 3.25 5 7.5l4.25 4.25" {...props} />
export const ChevronRight = (props: SVGProps<SVGSVGElement>) => <Icon path="M5.75 11.75 10 7.5 5.75 3.25" {...props} />
export const ArrowUpRightMini = (props: SVGProps<SVGSVGElement>) => <Icon path="m12.167 2.833-9.334 9.334M6.824 2.833h5.343v5.342" {...props} />
export const ArrowRight = (props: SVGProps<SVGSVGElement>) => <Icon path="M13.056 7.5H1.944M9.278 3.722 13.056 7.5l-3.778 3.778" {...props} />
export const ArrowLeft = (props: SVGProps<SVGSVGElement>) => <Icon path="M1.944 7.5h11.112M5.722 11.278 1.944 7.5l3.778-3.778" {...props} />

// Original @medusajs/icons InformationCircle geometry (MIT).
export const InformationCircle=(props:SVGProps<SVGSVGElement>)=><svg xmlns="http://www.w3.org/2000/svg" width={15} height={15} viewBox="0 0 15 15" fill="none" {...props}><g stroke="currentColor" strokeWidth={1.5}><path strokeLinecap="round" strokeLinejoin="round" d="m6.829 6.784.037-.018a.671.671 0 0 1 .95.763l-.633 2.537a.67.67 0 0 0 .951.763l.037-.018M7.5 4.1h.007v.007H7.5z"/><circle cx={7.5} cy={7.5} r={6.36}/></g></svg>
