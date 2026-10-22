import { listProducts } from "@lib/data/products"
import { getRegion } from "@lib/data/regions"
import { HttpTypes } from "@medusajs/types"
import Image from "next/image"
import LocalizedClientLink from "@modules/common/components/localized-client-link"
import { getProductPrice } from "@lib/util/get-product-price"
import { getProductCopy } from "../../i18n"
import RelatedProductsRail from "./rail"

type RelatedProductsProps = {
  product: HttpTypes.StoreProduct
  countryCode: string
}

export default async function RelatedProducts({
  product,
  countryCode,
}: RelatedProductsProps) {
  const copy = getProductCopy(countryCode)
  const region = await getRegion(countryCode)

  if (!region) {
    return null
  }

  // edit this function to define your related products logic
  const queryParams: HttpTypes.StoreProductListParams = {}
  if (region?.id) {
    queryParams.region_id = region.id
  }
  if (product.collection_id) {
    queryParams.collection_id = [product.collection_id]
  }
  if (product.tags) {
    queryParams.tag_id = product.tags
      .map((t) => t.id)
      .filter(Boolean) as string[]
  }
  queryParams.is_giftcard = false
  queryParams.limit = 50

  const products = await listProducts({
    queryParams,
    countryCode,
  }).then(({ response }) => {
    return response.products.filter(
      (responseProduct) => responseProduct.id !== product.id &&
        (!/^iPhone\b/i.test(product.title ?? "") || /^iPhone\b/i.test(responseProduct.title ?? ""))
    ).slice(0, 12)
  })

  if (!products.length) {
    return null
  }

  return (
    <div className="product-page-constraint">
      <div className="flex flex-col items-center text-center mb-6">
        <span className="text-base-regular text-gray-600 mb-2">
          {copy.related}
        </span>
        <p className="text-2xl-regular text-ui-fg-base max-w-lg">
          {copy.relatedText}
        </p>
      </div>

      <RelatedProductsRail label={copy.related} previous={copy.previousProducts} next={copy.nextProducts}>
        {products.map((product) => {
          const { cheapestPrice } = getProductPrice({ product })
          return <li key={product.id}>
            <LocalizedClientLink href={`/products/${product.handle}`} className="group block">
              <div className="relative aspect-square overflow-hidden rounded-xl bg-[#f5f5f7]">
                {product.thumbnail && <Image src={product.thumbnail} alt={product.title ?? ""} fill sizes="(max-width: 1023px) 144px, 176px" className="object-contain p-4 transition-transform motion-reduce:transition-none group-hover:scale-105 motion-reduce:group-hover:scale-100" />}
              </div>
              <div className="mt-3 flex flex-col gap-1 text-sm"><span>{product.title}</span>
                {cheapestPrice && <span className="text-[#6e6e73]">{new Intl.NumberFormat(countryCode.endsWith("-zh") ? "zh-CN" : "en-IE", { style: "currency", currency: cheapestPrice.currency_code }).format(cheapestPrice.calculated_price_number)}</span>}
              </div>
            </LocalizedClientLink>
          </li>
        })}
      </RelatedProductsRail>
    </div>
  )
}
