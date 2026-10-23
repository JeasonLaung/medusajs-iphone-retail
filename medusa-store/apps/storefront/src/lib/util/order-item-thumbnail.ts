import type { HttpTypes } from "@medusajs/types"
import iphoneThumbnails from "./iphone-thumbnails.json"

export function getOrderItemThumbnail(item: Partial<Pick<
  HttpTypes.StoreOrderLineItem,
  "thumbnail" | "variant" | "variant_title" | "product_handle"
>>) {
  if (item.variant?.thumbnail) return item.variant.thumbnail

  // Older orders retain the product's main image, even for another color.
  // Use only known model/color pairs when recovering their selected image.
  const model = item.product_handle?.replace(/^swappie-/, "")
  const color = (item.variant_title ?? item.variant?.title ?? "")
    .split("/").pop()?.trim().toLowerCase()
    .replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "")
    .replace(/^product-red$/, "red")
  const local = (iphoneThumbnails as Record<string, string>)[`${model}/${color}`]

  return local || item.thumbnail
}
