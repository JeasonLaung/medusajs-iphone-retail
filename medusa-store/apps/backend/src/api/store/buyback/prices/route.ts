import type { MedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import { BUYBACK_MODULE } from "../../../../modules/buyback"
import type BuybackModuleService from "../../../../modules/buyback/service"

export async function GET(req: MedusaRequest, res: MedusaResponse) {
  const service = req.scope.resolve<BuybackModuleService>(BUYBACK_MODULE)
  const productId = typeof req.query.product_id === "string" ? req.query.product_id.slice(0, 100) : ""
  const prices = productId ? await service.listBuybackPrices({ product_id: productId }, { take: 100 }) : []
  res.json({ prices: prices.map(({ product_id, capacity, amount, currency_code }) => ({ product_id, capacity, amount, currency_code })) })
}
