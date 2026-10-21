import { createStep } from "@medusajs/framework/workflows-sdk"
import { loadDeviceStock } from "../lib/device-stock"
import "../workflows/reconcile-device-stock"

jest.mock("@medusajs/framework/workflows-sdk", () => ({
  createStep: jest.fn(() => ({})), createWorkflow: jest.fn(() => ({})),
  StepResponse: class { constructor(public output: unknown) {} }, WorkflowResponse: jest.fn(),
}))
jest.mock("@medusajs/medusa/core-flows", () => ({}))
jest.mock("../lib/device-stock", () => ({ loadDeviceStock: jest.fn() }))

const prepare = (createStep as jest.Mock).mock.calls.find(([name]) => name === "prepare-device-stock-reconciliation")[1]
const level = { id: "level", inventory_item_id: "inv1", location_id: "loc1", stocked_quantity: 1, reserved_quantity: 1 }

async function plan(stocked: number, cap: number) {
  ;(loadDeviceStock as jest.Mock).mockResolvedValue({
    managed: [{ variant_id: "v1", inventory_item_id: "inv1" }],
    levels: [{ ...level, stocked_quantity: stocked }],
    caps: new Map([["v1:loc1", cap]]),
  })
  return (await prepare({}, { container: {} })).output.updates
}

test("never replaces manually managed stock with device counts", async () => {
  expect(await plan(1, 0)).toEqual([])
  expect(await plan(100, 0)).toEqual([])
})

test("keeps backed stock and does not recreate consumed supplier units", async () => {
  expect(await plan(2, 2)).toEqual([])
  expect(await plan(1, 3)).toEqual([])
})

test("a corrected stock level is unchanged on retry", async () => {
  expect(await plan(0, 0)).toEqual([])
})
