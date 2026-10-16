import type { MedusaRequest, MedusaResponse } from "@medusajs/framework/http"
import {
  guardManualPaymentSelection,
  guardMarkAsPaid,
  guardManualCartCompletion,
  guardManualPaymentAuthorization,
  guardManualPaymentCapture,
  listProductionPaymentProviders,
} from "../api/payment-guards"

describe("production manual payment guards", () => {
  const previousEnvironment = process.env.NODE_ENV
  const manual = "pp_system_default"
  const next = jest.fn()
  const graph = jest.fn()
  const remoteQuery = jest.fn()
  const json = jest.fn()
  const response = { json } as unknown as MedusaResponse
  const request = (body = {}, path = "/store/payment-providers") => ({
    body,
    path,
    params: { id: "cart_test" },
    filterableFields: { region_id: "reg_test" },
    queryConfig: { fields: ["id"], pagination: { skip: 0, take: 20 } },
    scope: { resolve: (key: string) => key === "query" ? { graph } : remoteQuery },
  }) as unknown as MedusaRequest

  beforeEach(() => {
    process.env.NODE_ENV = "production"
    jest.clearAllMocks()
  })
  afterAll(() => {
    if (previousEnvironment === undefined) delete process.env.NODE_ENV
    else process.env.NODE_ENV = previousEnvironment
  })

  it("rejects direct manual session creation, but permits online providers", () => {
    expect(() => guardManualPaymentSelection(request({ provider_id: manual }), response, next)).toThrow("Manual payment is disabled")
    expect(next).not.toHaveBeenCalled()
    for (const provider_id of ["pp_paypal_paypal", "pp_mollie_mollie"])
      guardManualPaymentSelection(request({ provider_id }), response, next)
    expect(next).toHaveBeenCalledTimes(2)
  })

  it.each(["pending", "requires_more", "authorized", "captured", "pending_authorization"])("rejects an old %s manual cart session", async (status) => {
    graph.mockResolvedValue({ data: [{ payment_collection: { payment_sessions: [{ provider_id: manual, status }] } }] })
    await expect(guardManualCartCompletion(request(), response, next)).rejects.toThrow("Manual payment is disabled")
    expect(next).not.toHaveBeenCalled()
  })

  it("allows online checkout when old manual sessions are canceled", async () => {
    graph.mockResolvedValue({ data: [{ payment_collection: { payment_sessions: [
      { provider_id: manual, status: "canceled" },
      { provider_id: "pp_paypal_paypal", status: "authorized" },
    ] } }] })
    await guardManualCartCompletion(request(), response, next)
    expect(next).toHaveBeenCalledTimes(1)
  })

  it("allows zero-total checkout without payment sessions", async () => {
    graph.mockResolvedValue({ data: [{}] })
    await guardManualCartCompletion(request(), response, next)
    expect(next).toHaveBeenCalledTimes(1)
  })

  it("blocks marking a collection paid because that creates a system payment", () => {
    expect(() => guardMarkAsPaid(request(), response, next)).toThrow("Manual payment is disabled")
    expect(next).not.toHaveBeenCalled()
  })

  it.each([guardManualPaymentAuthorization, guardManualPaymentCapture])("blocks existing manual authorization/capture but permits online payments", async (guard) => {
    graph.mockResolvedValue({ data: [{ provider_id: manual }] })
    await expect(guard(request({ payment_session_id: "payses_test" }), response, next)).rejects.toThrow("Manual payment is disabled")
    expect(next).not.toHaveBeenCalled()
    graph.mockResolvedValue({ data: [{ provider_id: "pp_paypal_paypal" }] })
    await guard(request({ payment_session_id: "payses_test" }), response, next)
    expect(next).toHaveBeenCalledTimes(1)
  })

  it.each(["/store/payment-providers", "/admin/payments/payment-providers"])("excludes the system provider in the database query for %s", async (path) => {
    const online = { id: "pp_paypal_paypal" }
    remoteQuery.mockResolvedValue({ rows: path.startsWith("/store/") ? [{ payment_provider: online }] : [online], metadata: { count: 1, skip: 0, take: 20 } })
    await listProductionPaymentProviders(request({}, path), response, next)
    const query = JSON.stringify(remoteQuery.mock.calls[0][0])
    expect(query).toContain(manual)
    expect(query).toContain("$ne")
    expect(json).toHaveBeenCalledWith({ payment_providers: [online], count: 1, offset: 0, limit: 20 })
    expect(next).not.toHaveBeenCalled()
  })

  it("keeps development testing available without querying payment records", async () => {
    process.env.NODE_ENV = "development"
    const req = request({ provider_id: manual })
    guardManualPaymentSelection(req, response, next)
    guardMarkAsPaid(req, response, next)
    await guardManualCartCompletion(req, response, next)
    await guardManualPaymentAuthorization(req, response, next)
    await guardManualPaymentCapture(req, response, next)
    await listProductionPaymentProviders(req, response, next)
    expect(next).toHaveBeenCalledTimes(6)
    expect(graph).not.toHaveBeenCalled()
    expect(remoteQuery).not.toHaveBeenCalled()
  })
})
