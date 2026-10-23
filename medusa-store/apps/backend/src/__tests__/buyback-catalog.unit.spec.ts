import { createStep } from "@medusajs/framework/workflows-sdk"
import { BUYBACK_MODULE } from "../modules/buyback"
import { buybackCatalog } from "../lib/buyback-catalog"
import { referenceRules } from "../lib/buyback-catalog"
import "../workflows/buyback-catalog"

jest.mock("@medusajs/framework/workflows-sdk", () => ({ createStep: jest.fn(() => ({})), createWorkflow: jest.fn(() => ({})), StepResponse: class { constructor(public output: unknown, public compensation: unknown) {} }, WorkflowResponse: jest.fn() }))
jest.mock("@medusajs/medusa/core-flows", () => ({}))
jest.mock("../lib/buyback-catalog", () => ({ ...jest.requireActual("../lib/buyback-catalog"), buybackCatalog: jest.fn() }))
jest.mock("../api/device-access", () => ({ deviceAccess: jest.fn().mockResolvedValue({ role: "admin" }) }))
jest.mock("../lib/warehouse-access", () => ({ warehouseAccess: jest.fn().mockResolvedValue({ warehouse: false }) }))

const update = (createStep as jest.Mock).mock.calls.find(([name]) => name === "update-buyback-catalog")[1]
const rules = referenceRules("iPhone 14", "256GB")!
function setup(existing = true) {
  const entry = { key: "p:256GB", product_id: "p", model: "iPhone 14", capacity: "256GB", amount: existing ? 294 : null, rules: existing ? rules : null, reference: rules }
  ;(buybackCatalog as jest.Mock).mockResolvedValue({ entries: [entry] })
  const service = { listBuybackPrices: jest.fn().mockResolvedValue(existing ? [{ id: "price1", amount: 294, pricing_rules: rules }] : []), updateBuybackPrices: jest.fn().mockResolvedValue({ id: "price1" }), createBuybackPrices: jest.fn().mockResolvedValue({ id: "new1" }), deleteBuybackPrices: jest.fn().mockResolvedValue(undefined) }
  const container = { resolve: (key: string) => key === BUYBACK_MODULE ? service : null }
  const run = (command: unknown) => update({ actor_id: "admin1", command }, { container })
  return { service, run }
}
describe("buyback catalog changes", () => {
  it("scales the entire quote curve and offsets only bases and floor", async () => {
    const { service, run } = setup()
    expect((await run({ action: "adjust", keys: ["p:256GB"], percentage: -10, adjustment: 5 })).output).toEqual({ updated: 1, skipped: 0 })
    const data = service.updateBuybackPrices.mock.calls[0][0]
    expect(data.amount).toBe(269.6)
    expect(data.pricing_rules.fault_costs.BROKEN_SCREEN).toBe(94.5)
    expect(data.pricing_rules.damage_fee).toBe(12.58)
    expect(data.pricing_rules.floor).toBe(27.5)
    expect(rules.bases.LIKE_NEW).toBe(294)
  })
  it("preserves existing references in initialize-only-missing mode", async () => {
    const { service, run } = setup()
    expect((await run({ action: "reference", keys: ["p:256GB"], only_missing: true, percentage: 0, adjustment: 0 })).output).toEqual({ updated: 0, skipped: 1 })
    expect(service.updateBuybackPrices).not.toHaveBeenCalled()
  })
  it("saves new rules and provides rollback data without changing applications", async () => {
    const { service, run } = setup(false)
    const result = await run({ action: "save", product_id: "p", capacity: "256GB", rules })
    expect(service.createBuybackPrices).toHaveBeenCalledWith({ product_id: "p", capacity: "256GB", amount: 294, pricing_rules: rules })
    expect(result.compensation).toEqual({ previous: [], created: ["new1"] })
  })
  it("rejects unknown configurations and restores previous rules if a write fails", async () => {
    const { service, run } = setup()
    await expect(run({ action: "adjust", keys: ["other:128GB"], percentage: 0, adjustment: 0 })).rejects.toThrow("BUYBACK_INVALID_MODEL")
    service.updateBuybackPrices.mockRejectedValueOnce(new Error("Write failed"))
    await expect(run({ action: "save", product_id: "p", capacity: "256GB", rules })).rejects.toThrow("Write failed")
    expect(service.updateBuybackPrices).toHaveBeenLastCalledWith({ id: "price1", amount: 294, pricing_rules: rules })
  })
})
