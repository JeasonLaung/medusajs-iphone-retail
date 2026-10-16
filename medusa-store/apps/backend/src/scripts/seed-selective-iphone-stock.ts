import type { ExecArgs } from "@medusajs/framework/types"
import { ContainerRegistrationKeys } from "@medusajs/framework/utils"
import { createInventoryLevelsWorkflow, updateProductVariantsWorkflow } from "@medusajs/medusa/core-flows"
import { readFileSync } from "node:fs"
import { resolve } from "node:path"
import { DEVICES_MODULE } from "../modules/devices"
import DevicesModuleService from "../modules/devices/service"

const SEED_VERSION = "selective-iphone-v1"
type CatalogueItem = {
  slug: string
  price_eur: number
  source_image_url: string
  available_storages_gb: number[]
  available_colors: string[]
}
const colourSlug = (colour: string) => colour.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "")

// Explicit demo seed: three configurations per model, with 3/2/1 units.
// Never restock previously seeded SKUs or overwrite merchant/device inventory.
export default async function seedSelectiveIphoneStock({ container }: ExecArgs) {
  const { products: catalogue } = JSON.parse(readFileSync(resolve(process.cwd(), "../../../imports/swappie-iphone-catalog.json"), "utf8")) as { products: CatalogueItem[] }
  const query = container.resolve(ContainerRegistrationKeys.QUERY)
  const managed = await container.resolve<DevicesModuleService>(DEVICES_MODULE).listDeviceVariants({}, { take: 10000 })
  const { data: stores } = await query.graph({ entity: "store", fields: ["default_sales_channel_id"] })
  const channelId = stores[0]?.default_sales_channel_id
  const { data: locations } = await query.graph({ entity: "stock_location", fields: ["id", "name", "sales_channels.id"] })
  const location = locations.find((location) => location.sales_channels?.some((channel) => channel?.id === channelId))
  if (!location) throw new Error("No stock location is linked to the default sales channel")

  const plans = catalogue.flatMap((item) => {
    const mainColour = item.available_colors.find((colour) => item.source_image_url.includes(`-${colourSlug(colour)}.png`)) ?? item.available_colors[0]
    const colours = [mainColour, ...item.available_colors.filter((colour) => colour !== mainColour)]
    const capacities = [...item.available_storages_gb].sort((a, b) => a - b)
    return [
      { storage: capacities[0], colour: colours[0], condition: "S", battery: 85, quantity: 3 },
      { storage: capacities[0], colour: colours[1] ?? colours[0], condition: "S", battery: 100, quantity: 2 },
      { storage: capacities[1] ?? capacities[0], colour: colours[2] ?? colours[0], condition: "A", battery: 85, quantity: 1 },
    ].map((choice) => ({
      ...choice,
      sku: `${item.slug}-${choice.storage}gb-${colourSlug(choice.colour)}-${choice.condition}${choice.battery}`,
      price: item.price_eur,
      model: item.slug,
    }))
  })
  const { data: variants } = await query.graph({
    entity: "product_variant",
    filters: { sku: plans.map((plan) => plan.sku) },
    fields: ["id", "sku", "metadata", "price_set.prices.currency_code", "price_set.prices.amount", "inventory_items.inventory_item_id", "inventory_items.inventory.location_levels.id", "inventory_items.inventory.location_levels.location_id", "inventory_items.inventory.location_levels.stocked_quantity"],
    pagination: { take: plans.length },
  })
  if (variants.length !== plans.length) throw new Error("Some planned configuration SKUs are missing; run complete-iphone-variants first")
  // Validate the entire selection before making changes.
  for (const variant of variants) {
    if (variant.inventory_items?.length !== 1) throw new Error(`Expected one inventory item for ${variant.sku}`)
  }
  let created = 0
  let skipped = 0
  for (const plan of plans) {
    const variant = variants.find((variant) => variant.sku === plan.sku)!
    if (variant.metadata?.demo_stock_seed === SEED_VERSION || managed.some((managed) => managed.variant_id === variant.id)) {
      skipped++
      continue
    }
    const inventory = variant.inventory_items![0]!
    const level = inventory.inventory?.location_levels?.find((level) => level?.location_id === location.id)
    const hasPrice = variant.price_set?.prices?.some((price) => price?.currency_code === "eur" && price.amount != null)
    if (!hasPrice) {
      await updateProductVariantsWorkflow(container).run({ input: { product_variants: [{
        id: variant.id,
        prices: [{ currency_code: "eur", amount: plan.price }],
        metadata: { ...variant.metadata, price_pending: false, demo_price_basis: "model-starting-price", demo_data: true },
      }] } })
    }
    if (!level) {
      await createInventoryLevelsWorkflow(container).run({ input: { inventory_levels: [{
        inventory_item_id: inventory.inventory_item_id!, location_id: location.id, stocked_quantity: plan.quantity,
      }] } })
      created++
    }
    await updateProductVariantsWorkflow(container).run({ input: { product_variants: [{
      id: variant.id,
      metadata: { ...variant.metadata, ...(!hasPrice ? { price_pending: false, demo_price_basis: "model-starting-price", demo_data: true } : {}), demo_stock_seed: SEED_VERSION },
    }] } })
    console.log(`${plan.sku}: ${level ? "existing inventory retained" : `${plan.quantity} demo units added`}`)
  }
  console.log(`Selective demo stock: ${catalogue.length} models, ${plans.length} selected SKUs, ${created} stock levels created, ${skipped} already seeded or device-managed SKUs skipped`)
}
