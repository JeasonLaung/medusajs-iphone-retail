import type { SubscriberArgs, SubscriberConfig } from "@medusajs/framework"
import { ContainerRegistrationKeys } from "@medusajs/framework/utils"
import { batchVariantImagesWorkflow, updateProductVariantsWorkflow } from "@medusajs/medusa/core-flows"
import { iphoneImageKey, variantImageKey } from "../lib/iphone-product-images"

// Keep later SKU creation and color edits aligned with the imported model images.
export default async function syncIphoneProductImages({ event, container }: SubscriberArgs<{ id: string }>) {
  const query = container.resolve(ContainerRegistrationKeys.QUERY)
  const { data: variants } = await query.graph({
    entity: "product_variant",
    filters: { id: event.data.id },
    fields: ["id", "thumbnail", "options.*", "options.option.title", "images.*", "product.handle", "product.title", "product.images.*"],
  })
  const variant = variants[0]
  if (!variant?.product) return
  const key = variantImageKey(variant.product, variant)
  if (!key) return
  const images = (variant.product.images ?? []).filter((image) => iphoneImageKey(image.url) === key).sort((a, b) => a.url.localeCompare(b.url))
  if (!images.length) return
  const current = variant.images ?? []
  const add = images.filter((image) => !current.some((existing) => existing.id === image.id)).map((image) => image.id)
  const remove = current.filter((image) => !images.some((expected) => expected.id === image.id)).map((image) => image.id)
  if (add.length || remove.length) {
    await batchVariantImagesWorkflow(container).run({ input: { variant_id: variant.id, add, remove } })
  }
  // The update emits another event; the equality guard makes that event a no-op.
  if (variant.thumbnail !== images[0].url) {
    await updateProductVariantsWorkflow(container).run({ input: { product_variants: [{ id: variant.id, thumbnail: images[0].url }] } })
  }
}

export const config: SubscriberConfig = { event: ["product-variant.created", "product-variant.updated"] }
