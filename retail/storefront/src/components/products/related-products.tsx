// Markup and horizontal rail inherited from the source related-products component.
import type {Product} from '../../lib/products'
import {productPrice} from '../../lib/products'
import ProductImage from './product-image'
import {getProductCopy} from './i18n'
import RelatedProductsRail from './related-products-rail'

export default function RelatedProducts({products,segment}: {products:Product[];segment:string}) {
 const copy=getProductCopy(segment)
 if(!products.length)return null
 return <div className="mt-20 product-page-constraint" data-testid="related-products-container">
  <div className="flex flex-col items-center text-center mb-6">
   <span className="text-base-regular text-ui-fg-subtle mb-2">{copy.related}</span>
   <p className="text-2xl-regular text-ui-fg-base max-w-lg">{copy.relatedText}</p>
  </div>
  <RelatedProductsRail label={copy.related} previous={copy.previousProducts} next={copy.nextProducts}>
   {products.map(product=>{
    const {cheapestPrice}=productPrice(product,undefined,segment.endsWith('-zh')?'zh-CN':'en-IE')
    return <li key={product.id}><a href={`/${segment}/products/${encodeURIComponent(product.handle)}`} className="group block">
     <div className="relative aspect-square overflow-hidden rounded-xl bg-ui-bg-subtle">
      {product.thumbnail&&<ProductImage src={product.thumbnail} alt={product.title} fill sizes="(max-width: 1023px) 144px, 176px" className="object-contain p-4 transition-transform motion-reduce:transition-none group-hover:scale-105 motion-reduce:group-hover:scale-100"/>}
     </div>
     <div className="mt-3 flex flex-col gap-1 text-sm"><span>{product.title}</span>{cheapestPrice&&<span className="text-ui-fg-subtle" data-testid="related-product-price">{cheapestPrice.calculated_price}</span>}</div>
    </a></li>
   })}
  </RelatedProductsRail>
 </div>
}
