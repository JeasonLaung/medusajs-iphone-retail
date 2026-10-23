import type { AuthenticatedMedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import type { SeoCommandBody } from "../../../seo-validators"
import { seoCommandWorkflow } from "../../../../workflows/seo-command"
export async function POST(req: AuthenticatedMedusaRequest<SeoCommandBody>, res: MedusaResponse) {
  const { result } = await seoCommandWorkflow(req.scope).run({ input: req.validatedBody })
  res.json({ result })
}
