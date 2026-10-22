const handlers: { save?: Function; rollback?: Function } = {}
jest.mock("@medusajs/framework/workflows-sdk", () => ({
  createStep: (_name, save, rollback) => { handlers.save = save; handlers.rollback = rollback; return () => ({}) },
  createWorkflow: () => ({}),
  StepResponse: class { constructor(public result, public snapshot) {} },
}))
import { CustomerServiceBody } from "../../api/admin/customer-service/validators"
import { GET } from "../../api/store/customer-service/route"
require("../../workflows/customer-service")

const contact = {
  id: "sales-1", name: "Sales", department: "pre_sales", whatsapp: "+8613800000000", email: "sales@example.test",
  timezone: "Asia/Shanghai", days: [1, 2, 3, 4, 5], start: "09:00", end: "18:00", availability: "schedule", enabled: true,
}
function containerFor(contacts = [contact]) {
  const store = { id: "store-1", metadata: { unrelated: "keep", customer_service_contacts: contacts } }
  const stores = { listStores: jest.fn().mockResolvedValue([store]), retrieveStore: jest.fn().mockResolvedValue(store), updateStores: jest.fn().mockResolvedValue(store) }
  return { stores, container: { resolve: () => stores } }
}
test("accepts multiple pre-sales and after-sales contacts with a single channel", () => {
  expect(CustomerServiceBody.parse({ contacts: [contact, { ...contact, id: "after-1", department: "after_sales", whatsapp: "" }] }).contacts).toHaveLength(2)
})
test.each([
  { whatsapp: "https://evil.test" }, { email: "mailto:bad" }, { timezone: "invalid/timezone" },
  { days: [] }, { start: "18:00", end: "09:00" }, { start: "25:00" }, { whatsapp: "", email: "" },
])("rejects invalid contact settings: %j", patch => {
  expect(CustomerServiceBody.safeParse({ contacts: [{ ...contact, ...patch }] }).success).toBe(false)
})
test("rejects duplicate IDs", () => {
  expect(CustomerServiceBody.safeParse({ contacts: [contact, contact] }).success).toBe(false)
})
test("saves and restores contacts while preserving other store metadata", async () => {
  const { stores, container } = containerFor()
  const result = await handlers.save!({ contacts: [] }, { container })
  expect(stores.updateStores).toHaveBeenCalledWith("store-1", { metadata: { unrelated: "keep", customer_service_contacts: [] } })
  await handlers.rollback!(result.snapshot, { container })
  expect(stores.updateStores).toHaveBeenLastCalledWith("store-1", { metadata: { unrelated: "keep", customer_service_contacts: [contact] } })
})
test("public endpoint publishes enabled contacts without store metadata", async () => {
  const { container } = containerFor([contact, { ...contact, id: "hidden", enabled: false }])
  const res = { json: jest.fn(), setHeader: jest.fn() }
  await GET({ scope: container } as any, res as any)
  const body = res.json.mock.calls[0][0]
  expect(body.contacts).toHaveLength(1)
  expect(body.contacts[0].enabled).toBeUndefined()
  expect(body.store).toBeUndefined()
  expect(res.setHeader).toHaveBeenCalledWith("Cache-Control", "no-store")
})
