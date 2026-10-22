import "server-only"
import { sdk } from "@lib/config"
import type { ServiceContact } from "@modules/customer-service/contact"

export async function getServiceContacts() {
  try {
    const data = await sdk.client.fetch<{ contacts: ServiceContact[] }>("/store/customer-service", { cache: "no-store" })
    return { contacts: data.contacts, failed: false }
  } catch {
    return { contacts: [] as ServiceContact[], failed: true }
  }
}
