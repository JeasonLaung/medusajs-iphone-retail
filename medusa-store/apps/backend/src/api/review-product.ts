import type { MedusaContainer } from "@medusajs/framework/types"
import { ContainerRegistrationKeys, MedusaError } from "@medusajs/framework/utils"

export async function requireReviewProduct(container: MedusaContainer, productId: string, salesChannelIds: string[]) {
  const query = container.resolve(ContainerRegistrationKeys.QUERY)
  const { data: products } = await query.graph({
    entity: "product",
    fields: ["id", "sales_channels.id"],
    filters: { id: productId, status: "published" },
  })
  const product = products[0]
  if (!product || !product.sales_channels?.some((channel) => channel && salesChannelIds.includes(channel.id))) {
    throw new MedusaError(MedusaError.Types.NOT_FOUND, "review.product_unavailable")
  }
}
