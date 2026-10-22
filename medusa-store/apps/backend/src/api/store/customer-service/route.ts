import type { MedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { customerServiceSettings } from "../../../lib/customer-service"

export async function GET(req: MedusaRequest, res: MedusaResponse) {
  const { contacts } = await customerServiceSettings(req.scope)
  res.setHeader("Cache-Control", "no-store")
  res.json({ contacts: contacts.filter(contact => contact.enabled).map(({ enabled, ...contact }) => contact) })
}
