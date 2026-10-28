// Native equivalent of the original Next Image fill layout. Image URLs remain
// the explicit public Gin URLs; there is no Medusa/Next image proxy dependency.
import type {ImgHTMLAttributes} from 'react'
export default function ProductImage({src,alt,sizes,priority = false,fill = false,...props}: Omit<ImgHTMLAttributes<HTMLImageElement>,'src'|'alt'> & {src: string; alt: string; sizes: string; priority?: boolean; fill?: boolean}) {
 return <img {...props} src={src} alt={alt} sizes={sizes} loading={priority ? 'eager' : 'lazy'} fetchPriority={priority ? 'high' : undefined} decoding="async" style={fill ? {position:'absolute',height:'100%',width:'100%',inset:0,color:'transparent'} : {color:'transparent'}} />
}
