import { createStep, createWorkflow } from "@medusajs/framework/workflows-sdk"
import { ContainerRegistrationKeys } from "@medusajs/framework/utils"
import { updateFulfillmentStep } from "@medusajs/medusa/core-flows"
import { deviceAccess } from "../api/device-access"
import { warehouseAccess } from "../lib/warehouse-access"
import { DEVICES_MODULE } from "../modules/devices"
import { UpdateShipmentTracking } from "../api/admin/orders/[id]/fulfillments/[fulfillment_id]/tracking/validators"
import "../workflows/update-shipment-tracking"

jest.mock("@medusajs/framework/workflows-sdk", () => ({
  createStep: jest.fn(() => jest.fn()), createWorkflow: jest.fn(() => ({})),
  transform: jest.fn((input, fn) => fn(input)),
  StepResponse: class { constructor(public output: unknown, public compensationInput: unknown) {} }, WorkflowResponse: jest.fn(),
}))
jest.mock("@medusajs/medusa/core-flows", () => ({ acquireLockStep: jest.fn(), releaseLockStep: jest.fn(), updateFulfillmentStep: jest.fn() }))
jest.mock("../api/device-access", () => ({ deviceAccess: jest.fn(), supplierDenied: () => new Error("supplier denied") }))
jest.mock("../lib/warehouse-access", () => ({ warehouseAccess: jest.fn(), warehouseDenied: () => new Error("warehouse denied") }))

const context = (createStep as jest.Mock).mock.calls.find(([name]) => name === "shipment-tracking-context")[1]
const sync = (createStep as jest.Mock).mock.calls.find(([name]) => name === "sync-shipment-device-tracking")
const input = { order_id: "o1", fulfillment_id: "f1", actor_id: "u1", labels: [{ tracking_number: "NEW123", tracking_url: "" }] }
const label = { id: "l1", tracking_number: "OLD123", tracking_url: "https://carrier.test/old", label_url: "https://carrier.test/label.pdf" }
function setup(fulfillment: Record<string, unknown> = {}) {
  const graph = jest.fn().mockResolvedValue({ data: [{ id: "o1", fulfillments: [{ id: "f1", shipped_at: "2026-10-20", canceled_at: null, labels: [], ...fulfillment }] }] })
  const devices = { listDeviceAssignments: jest.fn().mockResolvedValue([{ id: "a1", tracking_number: "OLD123", status: "shipped" }]), updateDeviceAssignments: jest.fn() }
  const container = { resolve: (key: string) => ({ [ContainerRegistrationKeys.QUERY]: { graph }, [DEVICES_MODULE]: devices })[key] }
  return { container, devices, graph }
}
beforeEach(() => {
  ;(deviceAccess as jest.Mock).mockResolvedValue({ role: "admin" })
  ;(warehouseAccess as jest.Mock).mockResolvedValue({ warehouse: false })
})
it("prepares only labels for a shipped parcel without tracking", async () => {
  expect((await context(input, { container: setup().container })).output).toEqual({ id: "f1", labels: [{ tracking_number: "NEW123", tracking_url: "#", label_url: "#" }] })
})
it("preserves label IDs, files and other entries on delivered parcels", async () => {
  const second = { ...label, id: "l2", tracking_number: "SECOND" }
  const labels = [{ id: "l1", tracking_number: "NEW123", tracking_url: "" }, { id: "l2", tracking_number: "SECOND", tracking_url: second.tracking_url }]
  const { output } = await context({ ...input, labels }, { container: setup({ labels: [label, second], delivered_at: "2026-10-21" }).container })
  expect(output).toEqual({ id: "f1", labels: [{ ...label, tracking_number: "NEW123", tracking_url: "#" }, second] })
})
it.each([{ shipped_at: null }, { canceled_at: "2026-10-21" }, { id: "foreign" }])("rejects an unavailable or foreign parcel %j", async (fulfillment) => {
  await expect(context(input, { container: setup(fulfillment).container })).rejects.toThrow("SHIPMENT_TRACKING_NOT_SHIPPED")
})
it.each([{ labels: [{ id: "foreign", tracking_number: "X" }] }, { labels: input.labels }, { labels: [{ id: "l1", tracking_number: "X" }, { id: "l1", tracking_number: "Y" }] }])("rejects stale, foreign or duplicate label IDs", async ({ labels }) => {
  await expect(context({ ...input, labels }, { container: setup({ labels: [label] }).container })).rejects.toThrow("SHIPMENT_TRACKING_STALE_LABELS")
})
it.each(["supplier", "warehouse"])("rejects a restricted %s operator before reading orders", async (role) => {
  if (role === "supplier") (deviceAccess as jest.Mock).mockResolvedValue({ role })
  else (warehouseAccess as jest.Mock).mockResolvedValue({ warehouse: true })
  const { container, graph } = setup()
  await expect(context(input, { container })).rejects.toThrow("denied")
  expect(graph).not.toHaveBeenCalled()
})
it("updates only device tracking numbers and compensates previous values", async () => {
  const { container, devices } = setup()
  const result = await sync[1](input, { container })
  expect(devices.listDeviceAssignments).toHaveBeenCalledWith({ order_id: "o1", fulfillment_id: "f1" })
  expect(devices.updateDeviceAssignments).toHaveBeenCalledWith([{ id: "a1", tracking_number: "NEW123" }])
  await sync[2](result.compensationInput, { container })
  expect(devices.updateDeviceAssignments).toHaveBeenLastCalledWith([{ id: "a1", tracking_number: "OLD123" }])
})
it("composes a label-only native update followed by device synchronization", () => {
  const steps = (createStep as jest.Mock).mock.results
  steps[0].value.mockReturnValue({ id: "f1", labels: input.labels })
  ;(updateFulfillmentStep as unknown as jest.Mock).mockReturnValue({ id: "f1" })
  ;(createWorkflow as jest.Mock).mock.calls[0][1](input)
  expect(updateFulfillmentStep).toHaveBeenCalledWith({ id: "f1", labels: input.labels })
  expect(steps[1].value).toHaveBeenCalledWith(input)
})
it("validates nonempty numbers and safe HTTP URLs", () => {
  expect(UpdateShipmentTracking.parse({ labels: [{ tracking_number: " ABC " }] }).labels[0].tracking_number).toBe("ABC")
  for (const url of ["javascript:alert(1)", "ftp://carrier.test", "invalid"]) expect(UpdateShipmentTracking.safeParse({ labels: [{ tracking_number: "ABC", tracking_url: url }] }).success).toBe(false)
  expect(UpdateShipmentTracking.safeParse({ labels: [{ tracking_number: "  " }] }).success).toBe(false)
  expect(UpdateShipmentTracking.safeParse({ labels: [] }).success).toBe(false)
})
