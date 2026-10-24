import { MedusaError } from "@medusajs/framework/utils"

export type DhlOfficialDemo = {
  requested_tracking_number: string
  fetched_at: string
  source: string
  shipment: { id: string; service: string; status?: DhlDemoEvent; events: DhlDemoEvent[] }
}
type DhlDemoEvent = {
  timestamp: string
  statusCode?: string
  status?: string
  description?: string
  location?: { address?: { addressLocality?: string; countryCode?: string } }
}
let cached: DhlOfficialDemo | undefined
let pending: Promise<DhlOfficialDemo> | undefined

// Fixed official sample only. Never use demo-key for real customer shipments.
export async function getDhlOfficialDemo(): Promise<DhlOfficialDemo> {
  if (process.env.NODE_ENV === "production") throw new MedusaError(MedusaError.Types.NOT_FOUND, "Demo is available in development only")
  if (cached && Date.now() - Date.parse(cached.fetched_at) < 30_000) return cached
  if (pending) return pending
  pending = (async () => {
    const response = await fetch("https://api-eu.dhl.com/track/shipments?trackingNumber=7777777770&service=express", {
      headers: { "DHL-API-Key": "demo-key", Accept: "application/json" },
      signal: AbortSignal.timeout(15_000), redirect: "error",
    })
    if (!response.ok) throw new MedusaError(MedusaError.Types.UNEXPECTED_STATE, `DHL returned HTTP ${response.status}`)
    const body = await response.json() as { shipments?: DhlOfficialDemo["shipment"][] }
    const shipment = body.shipments?.[0]
    if (!shipment?.id || !Array.isArray(shipment.events)) throw new MedusaError(MedusaError.Types.UNEXPECTED_STATE, "DHL returned an unexpected response")
    cached = { requested_tracking_number: "7777777770", fetched_at: new Date().toISOString(), source: "https://developer.dhl.com/tracking", shipment }
    return cached
  })()
  try { return await pending } finally { pending = undefined }
}
