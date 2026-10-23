import { summarizeBuybackRequests } from "../lib/buyback-summary"

const request = (status: string, phones = 1, final_amount: number | null = null, currency_code = "eur") => ({ status, items: Array.from({ length: phones }, () => ({})), final_amount, currency_code })

describe("customer buyback summary", () => {
  it("counts phones rather than requests and only completed payments as received", () => {
    expect(summarizeBuybackRequests([
      request("paid", 2, 240), request("accepted", 3, 300), request("received", 1),
      request("return_requested", 2), request("returning", 1), request("returned", 4), request("canceled", 5),
    ])).toEqual({ recycled: 2, processing: 7, received: [{ currency_code: "EUR", amount: 240 }] })
  })
  it("includes more than one page and keeps currencies separate with rounded totals", () => {
    expect(summarizeBuybackRequests([
      ...Array.from({ length: 25 }, () => request("paid", 1, 0.1)), request("paid", 1, 50, "usd"),
    ])).toEqual({ recycled: 26, processing: 0, received: [{ currency_code: "EUR", amount: 2.5 }, { currency_code: "USD", amount: 50 }] })
  })
  it("returns empty totals for a customer with no requests", () => {
    expect(summarizeBuybackRequests([])).toEqual({ recycled: 0, processing: 0, received: [] })
  })
})
