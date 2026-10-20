import type { AuthenticatedMedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { createMobileIntakeWorkflow } from "../../../../workflows/mobile-intake"
import type { CreateMobileIntakeBody } from "../../../mobile-intake/validators"

export async function POST(req: AuthenticatedMedusaRequest<CreateMobileIntakeBody>, res: MedusaResponse) {
  const { result } = await createMobileIntakeWorkflow(req.scope).run({
    input: { actor_id: req.auth_context.actor_id, source_id: req.validatedBody.source_id, category: req.validatedBody.category },
  })
  res.setHeader("Cache-Control", "no-store")
  res.json(result)
}
