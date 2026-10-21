import type { MedusaResponse } from "@medusajs/framework/http"
import { ContainerRegistrationKeys, remoteQueryObjectFromString } from "@medusajs/framework/utils"
import { GET as listInventoryItems } from "@medusajs/medusa/api/admin/inventory-items/route"

export const GET = async (req: Parameters<typeof listInventoryItems>[0], res: MedusaResponse) => {
  const order = req.query.order
  if (typeof order !== "string" || !/^-?(reserved_quantity|stocked_quantity)$/.test(order)) {
    return listInventoryItems(req, res)
  }

  const field = order.replace(/^-/, "") as "reserved_quantity" | "stocked_quantity"
  const direction = order.startsWith("-") ? -1 : 1
  const remoteQuery = req.scope.resolve(ContainerRegistrationKeys.REMOTE_QUERY)
  const { skip = 0, take = 20 } = req.queryConfig.pagination
  // SUM() is null for items with no levels, although the API displays them as 0.
  // Read only IDs and totals before paging so these items sort as displayed.
  const totals: { id: string; reserved_quantity?: number; stocked_quantity?: number }[] = []
  let count = 0
  do {
    const result = await remoteQuery(remoteQueryObjectFromString({
      entryPoint: "inventory_items",
      variables: {
        filters: req.filterableFields,
        skip: totals.length,
        take: 500,
        order: { id: "ASC" },
      },
      fields: ["id", "reserved_quantity", "stocked_quantity"],
    }))
    count = result.metadata.count
    totals.push(...result.rows)
    if (!result.rows.length) break
  } while (totals.length < count)

  totals.sort((a, b) => direction * (Number(a[field] ?? 0) - Number(b[field] ?? 0)) || a.id.localeCompare(b.id))
  const ids = totals.slice(skip, skip + take).map((item) => item.id)
  let inventoryItems: { id: string }[] = []
  if (ids.length) {
    const query = req.scope.resolve(ContainerRegistrationKeys.QUERY)
    const { data } = await query.graph({
      entity: "inventory_item",
      filters: { ...req.filterableFields, id: ids },
      pagination: { skip: 0, take: ids.length },
      fields: [...new Set(["id", ...req.queryConfig.fields])],
    })
    const byId = new Map(data.map((item) => [item.id, item]))
    inventoryItems = ids.map((id) => byId.get(id)).filter(Boolean) as { id: string }[]
  }

  res.json({ inventory_items: inventoryItems, count, offset: skip, limit: take })
}
