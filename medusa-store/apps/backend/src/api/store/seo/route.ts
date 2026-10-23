import type { MedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { normalizePath, seoService } from "../../../lib/seo/core"
import { publicConfig } from "../../../lib/seo/publication"
export async function GET(req: MedusaRequest, res: MedusaResponse) {
  res.setHeader("Cache-Control", "no-store")
  return res.json(await publicConfig(seoService(req.scope), normalizePath(String(req.query.path || "/"))))
}
