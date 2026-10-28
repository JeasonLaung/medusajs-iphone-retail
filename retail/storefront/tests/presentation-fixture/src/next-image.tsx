// Test reference bridge for next/image's fill geometry, not its optimizer.
import type {ImgHTMLAttributes} from 'react'
export default function NextImage({src,alt,sizes,priority,fill,...props}:Omit<ImgHTMLAttributes<HTMLImageElement>,'src'|'alt'> & {src:string;alt:string;sizes:string;priority?:boolean;fill?:boolean}) {
 return <img {...props} src={src} alt={alt} sizes={sizes} loading={priority?'eager':'lazy'} decoding="async" style={fill?{position:'absolute',height:'100%',width:'100%',left:0,top:0,right:0,bottom:0,color:'transparent'}:{color:'transparent'}} />
}
