import {loadRegions,type Region} from './regions.ts'
import {loadProducts,type Product, type Variant, type Image} from './products.ts'
import {routeCountry} from './storefront-route.ts'
import {galleryImages,initialVariant,productHandleFromSlug,variantSlug} from './product-variants.ts'

export type ProductPageData = {
 regions: Region[]; region: Region; product: Product; requestedVariantID?: string; initialVariant?: Variant; images: Image[]
 metadata: {title: string; description: string; canonicalPath: string; openGraph: {title: string; description: string; images: string[]}}
}
const unavailable = () => new Error('Product page data unavailable')

// Request-scoped data composition for the unchanged Astro page. No
// process-global country map or fallback currency; failures do not become 404.
export async function loadProductPage(origin: string | undefined, segment: string, handle: string, search = new URLSearchParams(), fetcher: typeof fetch = fetch): Promise<ProductPageData | null> {
 try {
  if (!/^[a-z]{2}(?:-(?:en|zh))?$/i.test(segment) || !handle || Array.from(handle).length > 512 || /[\x00-\x1f\x7f]/.test(handle)) return null
  const regions = await loadRegions(origin,fetcher), country = routeCountry(segment)
  const region = regions.find(r=>r.countries.some(c=>c.iso_2 === country))
  if (!region) return null
  const modelHandle = productHandleFromSlug(handle)
  const page = await loadProducts(origin,{handle:modelHandle,region_id:region.id,limit:2},fetcher)
  if (page.count === 0) return null
  if (page.count !== 1 || page.products.length !== 1 || page.products[0].handle !== modelHandle) throw unavailable()
  const product = page.products[0]
  for (const v of product.variants) if (v.calculated_price && v.calculated_price.currency_code !== region.currency_code) throw unavailable()
  let slugVariant: Variant | undefined
  if (modelHandle !== handle) {
   const matches = product.variants.filter(v=>variantSlug(v)?.toLowerCase() === handle.toLowerCase())
   if (matches.length > 1) throw unavailable()
   slugVariant = matches[0]
   if (!slugVariant?.configuration_complete) return null
  }
  const requestedVariantID = slugVariant?.id ?? search.get('v_id') ?? undefined
  const pathname = `/${segment.toLowerCase()}/products/${encodeURIComponent(handle)}`
  const initial = initialVariant(product,pathname,search.get('v_id'),requestedVariantID)
  return {regions,region,product,requestedVariantID,initialVariant:initial,images:galleryImages(product,pathname,search.get('v_id'),requestedVariantID),metadata:{
   title:`${product.title}${slugVariant ? ` ${slugVariant.title}` : ''}`,description:product.title,
   canonicalPath:`/${segment.toLowerCase()}/products/${encodeURIComponent(variantSlug(slugVariant) ?? product.handle)}`,
   openGraph:{title:`${product.title} | EuroRefurb`,description:product.title,images:product.thumbnail ? [product.thumbnail] : []},
  }}
 } catch { throw unavailable() }
}
