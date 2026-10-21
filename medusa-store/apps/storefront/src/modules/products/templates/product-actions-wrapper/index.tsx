import { listProducts } from "@lib/data/products"
import { HttpTypes } from "@medusajs/types"
import ProductActions from "@modules/products/components/product-actions"
import { retrieveCart } from "@lib/data/cart"
import { cartVariantQuantities } from "@lib/util/cart-inventory"

/**
 * Fetches real time pricing for a product and renders the product actions component.
 */
export default async function ProductActionsWrapper({
  id,
  region,
  initialVariantId,
}: {
  id: string
  region: HttpTypes.StoreRegion
  initialVariantId?: string
}) {
  const product = await listProducts({
    queryParams: { id: [id] },
    regionId: region.id,
  }).then(({ response }) => response.products[0])

  if (!product) {
    return null
  }

  const cart = await retrieveCart()
  return <ProductActions product={product} region={region} initialVariantId={initialVariantId} cartQuantities={cartVariantQuantities(cart?.items ?? [])} />
}
