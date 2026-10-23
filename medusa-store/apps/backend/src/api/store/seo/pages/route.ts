import type { MedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { allRows, seoService } from "../../../../lib/seo/core"
export async function GET(req: MedusaRequest, res: MedusaResponse) {
  const pages = await allRows(seoService(req.scope), "page-metas")
  res.setHeader("Cache-Control", "no-store")
  return res.json({ pages: pages.map(({ path, robots, canonical, hreflang, updated_at }) => ({ path, robots, canonical, hreflang, updated_at })) })
}
