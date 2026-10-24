import type { MedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { getDhlOfficialDemo } from "../../../../lib/dhl-official-demo"

export async function GET(_req: MedusaRequest, res: MedusaResponse) {
  res.setHeader("Cache-Control", "no-store")
  try { res.json(await getDhlOfficialDemo()) } catch (error) {
    if (process.env.NODE_ENV === "production") { res.status(404).json({ message: "Not found" }); return }
    res.status(502).json({ message: "DHL_DEMO_UNAVAILABLE" })
  }
}
