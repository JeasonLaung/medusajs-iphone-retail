import { emitEventStep } from "@medusajs/medusa/core-flows"
import { createAfterSalesWorkflow } from "../workflows/create-after-sales"
import { updateAfterSalesWorkflow } from "../workflows/update-after-sales"

const mockStepResults: Record<string, any> = {}
jest.mock("@medusajs/framework/workflows-sdk", () => ({
  createStep: jest.fn((name: string) => () => mockStepResults[name]),
  createWorkflow: jest.fn((_name, compose) => compose),
  transform: (data: any, map: (value: any) => unknown) => map(data),
  StepResponse: class { constructor(public output: unknown) {} },
  WorkflowResponse: class { constructor(public output: unknown) {} },
}))
jest.mock("@medusajs/medusa/core-flows", () => ({
  acquireLockStep: jest.fn(() => ({ config: jest.fn() })),
  releaseLockStep: jest.fn(() => ({ config: jest.fn() })),
  emitEventStep: jest.fn(),
}))

const request = { id: "asr_test", order_id: "order_test", type: "repair", status: "pending", items: [], reason: "Test" }
const create = createAfterSalesWorkflow as unknown as (input: any) => { output: unknown }
const update = updateAfterSalesWorkflow as unknown as (input: any) => { output: unknown }
beforeEach(() => jest.clearAllMocks())

it.each(["return", "exchange", "repair"])("queues a grouped notification for a newly created %s request", type => {
  mockStepResults["create-request"] = { ...request, type }
  const result = create({ order_id: request.order_id })
  expect(emitEventStep).toHaveBeenCalledWith({ eventName: "after_sales.created", data: { id: request.id, order_id: request.order_id, type } })
  expect(result.output).toEqual({ ...request, type })
})

it.each(["processing", "completed", "rejected"])("queues the %s status event and preserves the API response shape", status => {
  const updated = { ...request, status }
  mockStepResults["update-request"] = { request: updated, previous_status: "pending" }
  const result = update({ order_id: request.order_id })
  expect(emitEventStep).toHaveBeenCalledWith({ eventName: `after_sales.${status}`, data: [{ id: request.id, order_id: request.order_id, type: request.type }] })
  expect(result.output).toEqual(updated)
})

it("does not queue a second notification when processing is saved again", () => {
  mockStepResults["update-request"] = { request: { ...request, status: "processing" }, previous_status: "processing" }
  update({ order_id: request.order_id })
  expect(emitEventStep).toHaveBeenCalledWith({ eventName: "after_sales.processing", data: [] })
})
