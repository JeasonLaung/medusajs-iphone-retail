import type { MedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { deleteMobileIntakeWorkflow } from "../../../../workflows/mobile-intake"

export async function DELETE(req: MedusaRequest, res: MedusaResponse) {
  const { result } = await deleteMobileIntakeWorkflow(req.scope).run({
    input: { token: String(req.headers["x-intake-token"] || ""), device_id: req.params.id },
  })
  res.setHeader("Cache-Control", "no-store")
  res.json({ result })
}
