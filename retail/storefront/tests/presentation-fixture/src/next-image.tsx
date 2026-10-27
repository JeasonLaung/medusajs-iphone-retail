// Test reference bridge for next/image's fill geometry, not its optimizer.
export default function NextImage({src,alt,sizes,priority}: {src:string;alt:string;sizes:string;priority?:boolean;fill?:boolean}) {
 return <img src={src} alt={alt} sizes={sizes} loading={priority?'eager':'lazy'} decoding="async" style={{position:'absolute',height:'100%',width:'100%',left:0,top:0,right:0,bottom:0,color:'transparent'}} />
}
