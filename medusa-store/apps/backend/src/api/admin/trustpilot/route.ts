import type {
  AuthenticatedMedusaRequest,
  MedusaResponse,
} from "@medusajs/framework/http"
import {
  trustpilotSettings,
  type TrustpilotSettings,
} from "../../../lib/trustpilot"
import { saveTrustpilotWorkflow } from "../../../workflows/save-trustpilot"

export async function GET(
  req: AuthenticatedMedusaRequest,
  res: MedusaResponse,
) {
  const { settings } = await trustpilotSettings(req.scope)
  res.setHeader("Cache-Control", "no-store")
  res.json({ settings })
}

export async function POST(
  req: AuthenticatedMedusaRequest<TrustpilotSettings>,
  res: MedusaResponse,
) {
  const { result } = await saveTrustpilotWorkflow(req.scope).run({
    input: req.validatedBody,
  })
  res.json(result)
}
