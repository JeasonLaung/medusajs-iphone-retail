import {publicImageURL} from '../../lib/public-image-url.ts'
// Preserve the original fill layout and serve migrated catalog assets locally.
import type {ImgHTMLAttributes} from 'react'
export default function ProductImage({src,alt,sizes,priority = false,fill = false,...props}: Omit<ImgHTMLAttributes<HTMLImageElement>,'src'|'alt'> & {src: string; alt: string; sizes: string; priority?: boolean; fill?: boolean}) {
 return <img {...props} src={publicImageURL(src)} alt={alt} sizes={sizes} loading={priority ? 'eager' : 'lazy'} fetchPriority={priority ? 'high' : undefined} decoding="async" style={fill ? {position:'absolute',height:'100%',width:'100%',inset:0,color:'transparent'} : {color:'transparent'}} />
}
