import { withSEO } from "@lib/data/seo"
import { Metadata } from "next"
import { notFound } from "next/navigation"
import { listProducts } from "@lib/data/products"
import { getRegion } from "@lib/data/regions"
import ProductTemplate from "@modules/products/templates"
import { HttpTypes } from "@medusajs/types"
import { productHandleFromSlug, variantSlug } from "@lib/util/product-variants"
import { cache } from "react"

// Share the same fresh read between metadata and page rendering for this request.
const getProduct = cache(async (countryCode: string, handle: string) =>
  listProducts({ countryCode, queryParams: { handle: productHandleFromSlug(handle) } })
    .then(({ response }) => response.products[0])
)

type Props = {
  params: Promise<{ countryCode: string; handle: string }>
  searchParams: Promise<{ v_id?: string }>
}

function getImagesForVariant(
  product: HttpTypes.StoreProduct,
  selectedVariantId?: string,
) {
  if (!selectedVariantId || !product.variants) {
    return product.images
  }

  const variant = product.variants!.find((v) => v.id === selectedVariantId)
  if (!variant || !variant.images?.length) {
    return product.images
  }

  const imageIdsMap = new Map(variant.images!.map((i) => [i.id, true]))
  return product.images?.filter((i) => imageIdsMap.has(i.id)) ?? null
}

async function defaultPageMetadata(props: Props): Promise<Metadata> {
  const params = await props.params
  const { handle } = params
  const region = await getRegion(params.countryCode)

  if (!region) {
    notFound()
  }

  const product = await getProduct(params.countryCode, handle)

  if (!product) {
    notFound()
  }

  const variant = productHandleFromSlug(handle) !== handle
    ? product.variants?.find((variant) => variantSlug(variant)?.toLowerCase() === handle.toLowerCase())
    : undefined
  if (productHandleFromSlug(handle) !== handle && !variant) notFound()

  return {
    title: `${product.title}${variant ? ` ${variant.title}` : ""}`,
    alternates: { canonical: `/${params.countryCode}/products/${variantSlug(variant) ?? product.handle}` },
    description: `${product.title}`,
    openGraph: {
      title: `${product.title} | EuroRefurb`,
      description: `${product.title}`,
      images: product.thumbnail ? [product.thumbnail] : [],
    },
  }
}

export default async function ProductPage(props: Props) {
  const params = await props.params
  const region = await getRegion(params.countryCode)
  const searchParams = await props.searchParams


  if (!region) {
    notFound()
  }

  const pricedProduct = await getProduct(params.countryCode, params.handle)

  if (!pricedProduct) {
    notFound()
  }

  const slugVariant = productHandleFromSlug(params.handle) !== params.handle
    ? pricedProduct.variants?.find((variant) => variantSlug(variant)?.toLowerCase() === params.handle.toLowerCase())
    : undefined
  if (productHandleFromSlug(params.handle) !== params.handle && !slugVariant) notFound()
  const selectedVariantId = slugVariant?.id ?? searchParams.v_id

  const images = getImagesForVariant(pricedProduct, selectedVariantId)

  return (
    <ProductTemplate
      product={pricedProduct}
      region={region}
      countryCode={params.countryCode}
      images={images ?? []}
      selectedVariantId={selectedVariantId}
    />
  )
}

export async function generateMetadata(...args: Parameters<typeof defaultPageMetadata>): Promise<Metadata> {
  return withSEO(await defaultPageMetadata(...args))
}
