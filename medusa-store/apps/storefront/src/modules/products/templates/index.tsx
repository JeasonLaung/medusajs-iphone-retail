import React, { Suspense } from "react"
import ImageGallery from "@modules/products/components/image-gallery"
import ProductActions from "@modules/products/components/product-actions"
import ProductDetails from "@modules/products/components/product-detail"
import RelatedProducts from "@modules/products/components/related-products"
import Breadcrumbs from "@modules/common/components/breadcrumbs"
import SkeletonRelatedProducts from "@modules/skeletons/templates/skeleton-related-products"
import { notFound } from "next/navigation"
import { HttpTypes } from "@medusajs/types"
import { getProductCopy } from "../i18n"
import styles from "../components/product-detail/styles.module.css"

type ProductTemplateProps = {
  product: HttpTypes.StoreProduct
  region: HttpTypes.StoreRegion
  countryCode: string
  images: HttpTypes.StoreProductImage[]
  selectedVariantId?: string
}

const ProductTemplate: React.FC<ProductTemplateProps> = ({ product, region, countryCode, images, selectedVariantId }) => {
  if (!product?.id) return notFound()
  const copy = getProductCopy(countryCode)
  return <div className={styles.page}>
    <Breadcrumbs locale={countryCode} items={[
      { label: copy.back, href: "/store" },
      { label: product.title ?? "" },
    ]} />
    <div className={styles.hero} data-testid="product-container">
      <div className={styles.visual}><ImageGallery images={product.images ?? images} variants={product.variants ?? []} initialVariantId={selectedVariantId} title={product.title ?? ""} />{product.metadata?.source === "swappie-html-snapshot" && <p className={styles.muted + " mt-4 text-center"}>{copy.imageNote}</p>}</div>
      <section aria-labelledby="product-title" id="product-info">
        <ProductActions product={product} region={region} initialVariantId={selectedVariantId} />
      </section>
    </div>
    <ProductDetails product={product} locale={countryCode} />
    <div className="mt-20" data-testid="related-products-container"><Suspense fallback={<SkeletonRelatedProducts />}><RelatedProducts product={product} countryCode={countryCode} /></Suspense></div>
  </div>
}
export default ProductTemplate
