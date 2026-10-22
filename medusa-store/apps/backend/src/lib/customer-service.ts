import type { MedusaContainer } from "@medusajs/framework/types"
import { Modules } from "@medusajs/framework/utils"
import { CustomerServiceBody } from "../api/admin/customer-service/validators"

export async function customerServiceSettings(container: MedusaContainer) {
  const [store] = await container.resolve(Modules.STORE).listStores({}, { take: 1 })
  const parsed = CustomerServiceBody.safeParse({ contacts: store?.metadata?.customer_service_contacts ?? [] })
  return { store, contacts: parsed.success ? parsed.data.contacts : [] }
}
