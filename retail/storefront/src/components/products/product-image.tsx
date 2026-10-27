// Native equivalent of the original Next Image fill layout. Image URLs remain
// the explicit public Gin URLs; there is no Medusa/Next image proxy dependency.
export default function ProductImage({src,alt,sizes,priority = false}: {src: string; alt: string; sizes: string; priority?: boolean; fill?: boolean}) {
 return <img src={src} alt={alt} sizes={sizes} loading={priority ? 'eager' : 'lazy'} fetchPriority={priority ? 'high' : undefined} decoding="async" style={{position:'absolute',height:'100%',width:'100%',inset:0,color:'transparent'}} />
}
