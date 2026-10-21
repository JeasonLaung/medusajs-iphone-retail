import assert from "node:assert/strict"
import type { ExecArgs, ICartModuleService } from "@medusajs/framework/types"
import { ContainerRegistrationKeys, Modules } from "@medusajs/framework/utils"
import { addToCartWorkflow, createCartWorkflow } from "@medusajs/medusa/core-flows"
import { loadDeviceStock } from "../lib/device-stock"

export default async function check({ container }: ExecArgs) {
  const snapshot = await loadDeviceStock(container)
  const variants = new Map(snapshot.managed.map(row => [row.inventory_item_id, row.variant_id]))
  const surplus = snapshot.levels.filter(level => Number(level.stocked_quantity) > (snapshot.caps.get(`${variants.get(level.inventory_item_id)}:${level.location_id}`) ?? 0))
  assert.equal(surplus.length, 0, "Unsupported native stock must be removed")
  const query = container.resolve(ContainerRegistrationKeys.QUERY)
  const { data: orders } = await query.graph({ entity: "order", filters: { id: "order_01M5EMDP5968NC6TAVRP5V6KFA" }, fields: ["id", "status", "region_id", "sales_channel_id", "items.variant_id"] })
  const order = orders[0]
  assert(order && order.status !== "canceled", "Original order must be preserved")
  const carts: ICartModuleService = container.resolve(Modules.CART)
  let cartId: string | undefined
  try {
    const { result: cart } = await createCartWorkflow(container).run({ input: { region_id: order.region_id!, sales_channel_id: order.sales_channel_id! } })
    cartId = cart.id
    const result = await addToCartWorkflow(container).run({ input: { cart_id: cart.id, items: [{ variant_id: order.items![0]!.variant_id!, quantity: 1 }] }, throwOnError: false })
    assert(result.errors.some(error => /DEVICE_STOCK_UNAVAILABLE/.test(error.error.message)), "Live add-to-cart workflow must reject phantom stock through the new hook")
    const persisted = await carts.retrieveCart(cart.id, { relations: ["items"] })
    assert.equal(persisted.items?.length ?? 0, 0, "Rejected addition must not persist a line item")
    console.log("VERIFIED: no unsupported native stock; live add-to-cart hook rejects missing device/supply stock; rejected cart is empty; original order is preserved")
  } finally {
    if (cartId) await carts.deleteCarts(cartId)
  }
}
