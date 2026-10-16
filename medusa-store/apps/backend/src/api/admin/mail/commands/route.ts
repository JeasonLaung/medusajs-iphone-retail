import type {
  AuthenticatedMedusaRequest,
  MedusaResponse,
} from "@medusajs/framework/http"
import { mailCommandWorkflow } from "../../../../workflows/mail-command"
import type { MailCommandBody } from "../validators"

export async function POST(
  req: AuthenticatedMedusaRequest<MailCommandBody>,
  res: MedusaResponse
) {
  const { result } = await mailCommandWorkflow(req.scope).run({
    input: { ...req.validatedBody, actor_id: req.auth_context.actor_id },
  })
  res.json(result)
}
