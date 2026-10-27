// Existing @medusajs/icons 2.21.2 SVG paths, MIT; see ICONS-LICENSE.txt.
// Kept locally so migrated components do not require a Medusa runtime package.
import type {SVGProps} from 'react'
function Icon({path,color = 'currentColor',...props}: SVGProps<SVGSVGElement> & {path: string}) {
 return <svg xmlns="http://www.w3.org/2000/svg" width={15} height={15} viewBox="0 0 15 15" fill="none" {...props}><path stroke={color} strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d={path} /></svg>
}
export const ChevronLeft = (props: SVGProps<SVGSVGElement>) => <Icon path="M9.25 3.25 5 7.5l4.25 4.25" {...props} />
export const ChevronRight = (props: SVGProps<SVGSVGElement>) => <Icon path="M5.75 11.75 10 7.5 5.75 3.25" {...props} />
export const ArrowUpRightMini = (props: SVGProps<SVGSVGElement>) => <Icon path="m12.167 2.833-9.334 9.334M6.824 2.833h5.343v5.342" {...props} />
