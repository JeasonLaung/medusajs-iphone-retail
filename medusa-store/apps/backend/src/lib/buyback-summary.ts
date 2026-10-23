import { roundPrice } from "./buyback-pricing"

type SummaryRequest = { status: string; items: unknown; currency_code: string; final_amount: unknown }

export function summarizeBuybackRequests(requests: SummaryRequest[]) {
  let recycled = 0
  let processing = 0
  const received = new Map<string, number>()
  for (const request of requests) {
    const phones = Array.isArray(request.items) ? request.items.length : 0
    if (request.status === "paid") {
      recycled += phones
      const currency = request.currency_code.toUpperCase()
      received.set(currency, roundPrice((received.get(currency) || 0) + Number(request.final_amount ?? 0)))
    } else if (!["returned", "canceled"].includes(request.status)) {
      processing += phones
    }
  }
  return { recycled, processing, received: [...received].sort(([a], [b]) => a.localeCompare(b)).map(([currency_code, amount]) => ({ currency_code, amount })) }
}
