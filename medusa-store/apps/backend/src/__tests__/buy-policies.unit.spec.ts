import { createStep } from "@medusajs/framework/workflows-sdk"
import { BuyPoliciesBody, defaultBuyPolicies, buyPolicySettings } from "../lib/buy-policies"
import "../workflows/save-buy-policies"

jest.mock("@medusajs/framework/workflows-sdk", () => ({
  createStep: jest.fn(() => ({})), createWorkflow: jest.fn(() => ({})),
  StepResponse: class { constructor(public output: unknown, public compensation: unknown) {} }, WorkflowResponse: jest.fn(),
}))

const [, save, rollback] = (createStep as jest.Mock).mock.calls.find(([name]) => name === "save-buy-policies")
const policies = { ...defaultBuyPolicies, warranty_months: 12, return_days: 30 }

it("rejects negative, fractional and excessive periods and overlong notes", () => {
  for (const patch of [
    { warranty_months: -1 }, { warranty_months: 1.5 }, { warranty_months: 121 },
    { return_days: -1 }, { return_days: 0.5 }, { return_days: 366 },
    { warranty_note: { en: "x".repeat(201), zh: "" } },
  ]) expect(BuyPoliciesBody.safeParse({ ...policies, ...patch }).success).toBe(false)
  expect(BuyPoliciesBody.safeParse({ ...policies, warranty_months: 0, return_days: 0 }).success).toBe(true)
})

it("provides initial defaults without writing store data", async () => {
  const service = { listStores: jest.fn().mockResolvedValue([{ id: "store", metadata: {} }]) }
  const result = await buyPolicySettings({ resolve: () => service } as never)
  expect(result.policies).toEqual(defaultBuyPolicies)
})

it("saves and rolls back policies while preserving unrelated and later metadata", async () => {
  let store = { id: "store", metadata: { customer_service_contacts: ["keep"], buy_policies: defaultBuyPolicies } as Record<string, unknown> }
  const service = {
    listStores: jest.fn(async () => [store]),
    retrieveStore: jest.fn(async () => store),
    updateStores: jest.fn(async (_id: string, patch: { metadata: Record<string, unknown> }) => { store = { ...store, ...patch }; return store }),
  }
  const container = { resolve: () => service }
  const saved = await save(policies, { container })
  expect(store.metadata.buy_policies).toEqual(policies)
  expect(store.metadata.customer_service_contacts).toEqual(["keep"])
  expect(saved.output).toEqual({ policies })
  expect((await buyPolicySettings(container as never)).policies).toEqual(policies)
  await save(policies, { container })
  expect(store.metadata.buy_policies).toEqual(policies)
  store.metadata.other_setting = "new"
  await rollback(saved.compensation, { container })
  expect(store.metadata.buy_policies).toEqual(defaultBuyPolicies)
  expect(store.metadata.other_setting).toBe("new")
})
