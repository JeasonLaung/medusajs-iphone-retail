import type { MedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { mobileIntakeAccess } from "./access"
import { submitMobileIntakeWorkflow } from "../../workflows/mobile-intake"
import type { MobileIntakeBodyType } from "./validators"

export async function GET(req: MedusaRequest, res: MedusaResponse) {
  const { session, source } = await mobileIntakeAccess(req.scope, String(req.headers["x-intake-token"] || ""))
  res.setHeader("Cache-Control", "no-store")
  res.json({ category: session.category, source: { id: source.id, name: source.name }, expires_at: session.expires_at })
}

export async function POST(req: MedusaRequest<MobileIntakeBodyType>, res: MedusaResponse) {
  const { result } = await submitMobileIntakeWorkflow(req.scope).run({
    input: { token: String(req.headers["x-intake-token"] || ""), device: req.validatedBody },
  })
  res.setHeader("Cache-Control", "no-store")
  res.json({ result })
}
