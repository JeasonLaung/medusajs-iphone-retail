import type { AuthenticatedMedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import type { CustomerServiceBody } from "./validators"
import { customerServiceSettings } from "../../../lib/customer-service"
import { saveCustomerServiceWorkflow } from "../../../workflows/customer-service"

export async function GET(req: AuthenticatedMedusaRequest, res: MedusaResponse) {
  const { contacts } = await customerServiceSettings(req.scope)
  res.json({ contacts })
}

export async function POST(req: AuthenticatedMedusaRequest<CustomerServiceBody>, res: MedusaResponse) {
  const { result } = await saveCustomerServiceWorkflow(req.scope).run({ input: req.validatedBody })
  res.json(result)
}
