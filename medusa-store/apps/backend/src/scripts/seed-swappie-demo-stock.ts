import type { ExecArgs } from "@medusajs/framework/types"
import { ContainerRegistrationKeys, MedusaError } from "@medusajs/framework/utils"
import {
  createInventoryLevelsWorkflow,
  updateInventoryLevelsWorkflow,
} from "@medusajs/medusa/core-flows"
import { readFileSync } from "node:fs"
import { resolve } from "node:path"

const DEMO_QUANTITY = 100

type Manifest = {
  products: { sku: string }[]
}

export default async function seedSwappieDemoStock({ container }: ExecArgs) {
  const manifestPath = resolve(
    process.cwd(),
    "../../../imports/swappie-iphone-catalog.json"
  )
  const manifest = JSON.parse(readFileSync(manifestPath, "utf8")) as Manifest
  const expectedSkus = new Set(manifest.products.map((item) => item.sku))
  const query = container.resolve(ContainerRegistrationKeys.QUERY)

  const { data: stores } = await query.graph({
    entity: "store",
    fields: ["default_sales_channel_id"],
  })
  const channelId = stores[0]?.default_sales_channel_id
  const { data: locations } = await query.graph({
    entity: "stock_location",
    fields: ["id", "name", "sales_channels.id"],
  })
  const location = locations.find((item) =>
    item.sales_channels?.some((channel) => channel?.id === channelId)
  )
  if (!location) {
    throw new MedusaError(
      MedusaError.Types.INVALID_DATA,
      "No stock location is linked to the default sales channel"
    )
  }

  const { data: variants } = await query.graph({
    entity: "product_variant",
    fields: [
      "sku",
      "product.handle",
      "inventory_items.inventory_item_id",
      "inventory_items.inventory.location_levels.id",
      "inventory_items.inventory.location_levels.location_id",
      "inventory_items.inventory.location_levels.stocked_quantity",
    ],
  })
  const targets = variants.filter(
    (variant) =>
      variant.sku &&
      expectedSkus.has(variant.sku) &&
      variant.product?.handle?.startsWith("swappie-")
  )
  if (targets.length !== expectedSkus.size) {
    throw new MedusaError(
      MedusaError.Types.INVALID_DATA,
      `Expected ${expectedSkus.size} Swappie variants, found ${targets.length}`
    )
  }

  const creates: { inventory_item_id: string; location_id: string; stocked_quantity: number }[] = []
  const updates: { id: string; inventory_item_id: string; location_id: string; stocked_quantity: number }[] = []
  let unchanged = 0

  for (const variant of targets) {
    const inventory = variant.inventory_items?.[0]
    const inventoryItemId = inventory?.inventory_item_id
    if (!inventoryItemId || variant.inventory_items?.length !== 1) {
      throw new MedusaError(
        MedusaError.Types.INVALID_DATA,
        `Expected one inventory item for SKU ${variant.sku}`
      )
    }
    const level = inventory.inventory?.location_levels?.find(
      (item) => item?.location_id === location.id
    )
    if (!level) {
      creates.push({
        inventory_item_id: inventoryItemId,
        location_id: location.id,
        stocked_quantity: DEMO_QUANTITY,
      })
    } else if (Number(level.stocked_quantity) !== DEMO_QUANTITY) {
      updates.push({
        id: level.id,
        inventory_item_id: inventoryItemId,
        location_id: location.id,
        stocked_quantity: DEMO_QUANTITY,
      })
    } else {
      unchanged += 1
    }
  }

  if (creates.length) {
    await createInventoryLevelsWorkflow(container).run({
      input: { inventory_levels: creates },
    })
  }
  if (updates.length) {
    await updateInventoryLevelsWorkflow(container).run({
      input: { updates },
    })
  }

  console.log(
    `Demo stock at ${location.name}: ${creates.length} created, ` +
      `${updates.length} reset, ${unchanged} unchanged; ` +
      `${DEMO_QUANTITY} units per SKU (test data)`
  )
}
