import type { MedusaContainer } from "@medusajs/framework/types"

// Compatibility for older diagnostic scripts. Medusa core validates stock and
// reservations; serial records must never cap quantities at checkout.
export async function validateDeviceCartStock(
  _container: MedusaContainer,
  _salesChannelId: string | null | undefined,
  _items: { variant_id?: string | null; quantity: number }[],
) {}
