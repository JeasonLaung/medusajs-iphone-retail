import { AuthenticatedMedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { AFTER_SALES_MODULE } from "../../../../../modules/after-sales"
import AfterSalesModuleService from "../../../../../modules/after-sales/service"
import { serializeAfterSales } from "../../../../../modules/after-sales/serialize"
import { updateAfterSalesWorkflow } from "../../../../../workflows/update-after-sales"
import type { UpdateAfterSalesBody } from "./validators"

export async function GET(req: AuthenticatedMedusaRequest, res: MedusaResponse) {
  const service: AfterSalesModuleService = req.scope.resolve(AFTER_SALES_MODULE)
  const requests = await service.listAfterSalesRequests({ order_id: req.params.id }, { order: { created_at: "DESC" } })
  res.json({ requests: requests.map(serializeAfterSales) })
}

export async function POST(req: AuthenticatedMedusaRequest<UpdateAfterSalesBody>, res: MedusaResponse) {
  const { result } = await updateAfterSalesWorkflow(req.scope).run({ input: { ...req.validatedBody, order_id: req.params.id } })
  res.json({ request: result })
}
