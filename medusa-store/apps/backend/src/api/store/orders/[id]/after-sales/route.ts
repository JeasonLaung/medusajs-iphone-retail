import { AuthenticatedMedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { ContainerRegistrationKeys, MedusaError } from "@medusajs/framework/utils"
import { AFTER_SALES_MODULE } from "../../../../../modules/after-sales"
import AfterSalesModuleService from "../../../../../modules/after-sales/service"
import { serializeAfterSales } from "../../../../../modules/after-sales/serialize"
import { createAfterSalesWorkflow } from "../../../../../workflows/create-after-sales"
import type { CreateAfterSalesBody } from "./validators"

export async function GET(req: AuthenticatedMedusaRequest, res: MedusaResponse) {
  const query = req.scope.resolve(ContainerRegistrationKeys.QUERY)
  const { data: orders } = await query.graph({ entity: "order", fields: ["id", "customer_id"], filters: { id: req.params.id } })
  if (!orders[0] || orders[0].customer_id !== req.auth_context.actor_id) {
    throw new MedusaError(MedusaError.Types.NOT_FOUND, "Order not found")
  }
  const service: AfterSalesModuleService = req.scope.resolve(AFTER_SALES_MODULE)
  const requests = await service.listAfterSalesRequests({ order_id: req.params.id, customer_id: req.auth_context.actor_id }, { order: { created_at: "DESC" } })
  res.json({ requests: requests.map(serializeAfterSales) })
}

export async function POST(req: AuthenticatedMedusaRequest<CreateAfterSalesBody>, res: MedusaResponse) {
  const { result } = await createAfterSalesWorkflow(req.scope).run({
    input: { ...req.validatedBody, order_id: req.params.id, customer_id: req.auth_context.actor_id },
  })
  res.status(201).json({ request: result })
}
