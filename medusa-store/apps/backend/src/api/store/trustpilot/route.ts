import type { MedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { trustpilotSettings } from "../../../lib/trustpilot"

export async function GET(req: MedusaRequest, res: MedusaResponse) {
  const { settings } = await trustpilotSettings(req.scope)
  res.setHeader("Cache-Control", "no-store")
  res.json({ settings: settings.enabled ? settings : null })
}
