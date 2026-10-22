import { createStep } from "@medusajs/framework/workflows-sdk"
import { ContainerRegistrationKeys } from "@medusajs/framework/utils"
import { DEVICES_MODULE } from "../modules/devices"
import { AFTER_SALES_MODULE } from "../modules/after-sales"
import "../workflows/create-after-sales"

jest.mock("@medusajs/framework/workflows-sdk", () => ({
  createStep: jest.fn(() => ({})), createWorkflow: jest.fn(() => ({})),
  StepResponse: class { constructor(public output: unknown) {} }, WorkflowResponse: jest.fn(),
}))
jest.mock("@medusajs/medusa/core-flows", () => ({}))

const create = (createStep as jest.Mock).mock.calls.find(([name]) => name === "create-request")[1]
const input = { order_id: "o1", customer_id: "c1", type: "return", reason: "Please return this order", items: [{ item_id: "i1", quantity: 1 }] }
function setup({ quantity = 1, shipped = 0, canceled = false, open = false, customer = "c1", status = "pending" } = {}) {
  const service = {
    listAfterSalesRequests: jest.fn().mockResolvedValue(open ? [{ id: "existing" }] : []),
    createAfterSalesRequests: jest.fn(async (body) => ({ ...body, id: "r1", status: "pending" })),
  }
  const query = { graph: jest.fn().mockResolvedValue({ data: [{
    id: "o1", customer_id: customer, status,
    items: [{ id: "i1", variant_id: "v1", title: "Phone", detail: { quantity } }],
    fulfillments: shipped ? [{ shipped_at: new Date(), canceled_at: canceled ? new Date() : null, items: [{ line_item_id: "i1", quantity: shipped }] }] : [],
  }] }) }
  const devices = {
    listDeviceVariants: jest.fn().mockResolvedValue([{ variant_id: "v1" }]),
    listDeviceAssignments: jest.fn().mockResolvedValue(shipped && !canceled ? [{ device_id: "d1", item_id: "i1" }] : []),
  }
  return { service, container: { resolve: (key: string) => ({ [ContainerRegistrationKeys.QUERY]: query, [DEVICES_MODULE]: devices, [AFTER_SALES_MODULE]: service })[key] } }
}

it("accepts an unshipped managed item return without device IDs", async () => {
  const { container, service } = setup()
  await create(input, { container })
  expect(service.createAfterSalesRequests).toHaveBeenCalledWith(expect.objectContaining({ type: "return", items: { lines: [{ item_id: "i1", quantity: 1, title: "Phone" }] } }))
})

it.each(["repair", "exchange"])("still requires shipped devices for %s", async (type) => {
  const { container, service } = setup()
  await expect(create({ ...input, type }, { container })).rejects.toThrow("Select the shipped devices")
  expect(service.createAfterSalesRequests).not.toHaveBeenCalled()
})

it("does not bypass device selection for a shipped item", async () => {
  await expect(create(input, { container: setup({ shipped: 1 }).container })).rejects.toThrow("Select the shipped devices")
})

it("continues accepting a shipped return with its assigned device", async () => {
  await create({ ...input, items: [{ item_id: "i1", quantity: 1, device_ids: ["d1"] }] }, { container: setup({ shipped: 1 }).container })
})

it("limits a return without devices to the unshipped quantity", async () => {
  const { container } = setup({ quantity: 2, shipped: 1 })
  await create(input, { container })
  await expect(create({ ...input, items: [{ item_id: "i1", quantity: 2 }] }, { container })).rejects.toThrow("Select the shipped devices")
})

it("ignores canceled shipments when counting unshipped quantity", async () => {
  await create(input, { container: setup({ shipped: 1, canceled: true }).container })
})

it.each([{ customer: "another" }, { status: "canceled" }, { open: true }])("preserves ownership, order status and duplicate request checks: %j", async (options) => {
  const { container, service } = setup(options)
  await expect(create(input, { container })).rejects.toThrow()
  expect(service.createAfterSalesRequests).not.toHaveBeenCalled()
})

it("rejects a quantity greater than the purchased quantity", async () => {
  await expect(create({ ...input, items: [{ item_id: "i1", quantity: 2 }] }, { container: setup().container })).rejects.toThrow("Choose valid order items")
})
