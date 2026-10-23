import type { MedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { BUYBACK_MODULE } from "../../../../modules/buyback"
import type BuybackModuleService from "../../../../modules/buyback/service"
import { assessmentPrice, normalizeCapacity, type PricingRules } from "../../../../lib/buyback-pricing"
import type { z } from "@medusajs/framework/zod"
import type { BuybackEstimateBody } from "../../../buyback-pricing-validators"

export async function POST(req: MedusaRequest<z.infer<typeof BuybackEstimateBody>>, res: MedusaResponse) {
  const { product_id, capacity, assessment } = req.validatedBody
  const service = req.scope.resolve<BuybackModuleService>(BUYBACK_MODULE)
  const [price] = await service.listBuybackPrices({ product_id, capacity: normalizeCapacity(capacity) })
  res.json({ ...assessmentPrice(price?.pricing_rules as PricingRules | null, assessment, price ? Number(price.amount) : null), currency_code: "eur" })
}
