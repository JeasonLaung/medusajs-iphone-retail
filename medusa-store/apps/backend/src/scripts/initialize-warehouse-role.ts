import type { ExecArgs } from "@medusajs/framework/types"
import { ensureWarehouseRole } from "../lib/warehouse-role"

// Repair the shared warehouse role without assigning roles to existing users.
export default async function initializeWarehouseRole({ container }: ExecArgs) {
  const role = await ensureWarehouseRole(container)
  console.log(`Warehouse role initialized: ${role.id}; existing user permissions preserved`)
}
