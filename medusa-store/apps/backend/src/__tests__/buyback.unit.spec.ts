import { createStep } from "@medusajs/framework/workflows-sdk"
import { Modules } from "@medusajs/framework/utils"
import { BUYBACK_MODULE } from "../modules/buyback"
import BuybackModuleService from "../modules/buyback/service"
import { BuybackAdminCommand, BuybackCustomerCommand, CreateBuyback } from "../api/buyback-validators"
import { assertPhotos, customerBuyback } from "../lib/buyback"
import { referenceRules } from "../lib/buyback-catalog"
import "../workflows/buyback"
import "../workflows/expire-buyback-photos"

jest.mock("@medusajs/framework/workflows-sdk", () => ({
  createStep: jest.fn(() => ({})), createWorkflow: jest.fn(() => ({})), transform: jest.fn(),
  StepResponse: class { constructor(public output: unknown) {} }, WorkflowResponse: jest.fn(),
}))
jest.mock("@medusajs/medusa/core-flows", () => ({}))
jest.mock("../api/device-access", () => ({ deviceAccess: jest.fn().mockResolvedValue({ role: "admin" }) }))
jest.mock("../lib/warehouse-access", () => ({ warehouseAccess: jest.fn().mockResolvedValue({ warehouse: false }) }))

const update = (createStep as jest.Mock).mock.calls.find(([name]) => name === "update-buyback-request")[1]
const create = (createStep as jest.Mock).mock.calls.find(([name]) => name === "create-buyback-request")[1]
const expire = (createStep as jest.Mock).mock.calls.find(([name]) => name === "expire-buyback-photos")[1]
const assessment = { battery: "90", checks: [true, true, true, true, true], faults: ["none"], glass: ["none"], screen: "none", frame: "none", back: "none" }
const item = { id: "item1", product_id: "prod1", model_name: "iPhone 13", capacity: "128GB", assessment, device_id: "dev1", final_amount: null, reference_amount: 180, serial_number: "SERIAL1", imei: "123456789012345" }
const photo = (type: string, status = "approved", itemId = "item1", at = "2026-10-01") => ({ id: `${itemId}-${type}-${at}`, request_id: "req1", item_id: itemId, photo_type: type, review_status: status, uploaded_at: at, storage_key: "private-key" })
function setup(status = "received", extra: Record<string, unknown> = {}, photos = [photo("front"), photo("back")]) {
  let request = { id: "req1", customer_id: "customer1", status, items: [{ ...item }], history: [], final_amount: null, reference_amount: 180, ...extra }
  const service = {
    retrieveBuybackRequest: jest.fn().mockImplementation(() => Promise.resolve(request)),
    listBuybackRequests: jest.fn().mockImplementation((filter) => Promise.resolve(filter.customer_id && filter.customer_id !== request.customer_id ? [] : [request])),
    listBuybackPhotos: jest.fn().mockResolvedValue(photos),
    updateBuybackRequests: jest.fn().mockImplementation((data) => { request = { ...request, ...data }; return Promise.resolve(request) }),
    updateBuybackPhotos: jest.fn().mockResolvedValue({}),
  }
  const files = { deleteFiles: jest.fn().mockResolvedValue(undefined) }
  const container = { resolve: (key: string) => ({ [BUYBACK_MODULE]: service, [Modules.FILE]: files })[key] }
  const admin = (command: unknown) => update({ request_id: "req1", actor_id: "admin1", command }, { container })
  const customer = (command: unknown) => update({ request_id: "req1", customer_id: "customer1", command }, { container })
  return { service, files, container, admin, customer }
}
describe("buyback inspection, customer decision and photo controls", () => {
  it.each(["customer", "platform"] as const)("records who canceled the request: %s", async (actor) => {
    const flow = setup("awaiting_photos")
    const { output } = actor === "customer"
      ? await flow.customer({ action: "cancel" })
      : await flow.admin({ action: "cancel", note: "Canceled by platform" })
    expect(output.history).toEqual([expect.objectContaining({ action: "cancel", actor, status: "canceled" })])
  })

  it("recalculates and snapshots the questionnaire estimate when creating a request", async () => {
    const service = {
      listBuybackRequests: jest.fn().mockResolvedValue([]),
      listBuybackPrices: jest.fn().mockResolvedValue([{ amount: 294, pricing_rules: referenceRules("iPhone 14", "256GB") }]),
      createBuybackRequests: jest.fn().mockImplementation((data) => Promise.resolve({ id: "req-new", ...data })),
      listBuybackPhotos: jest.fn().mockResolvedValue([]),
    }
    const query = { graph: jest.fn().mockResolvedValueOnce({ data: [{ id: "customer1", email: "test@example.invalid" }] }).mockResolvedValueOnce({ data: [{ id: "p14", title: "iPhone 14", options: [{ title: "Capacity", values: [{ value: "256GB" }] }] }] }) }
    const container = { resolve: (key: string) => key === BUYBACK_MODULE ? service : query }
    const result = await create({ customer_id: "customer1", submission_key: "key", items: [{ model: "iphone-14", capacity: "256GB", assessment: { ...assessment, battery: "79", faults: ["pixels"] } }] }, { container })
    expect(result.output.reference_amount).toBe(152.02)
    expect(result.output.items[0].reference_amount).toBe(152.02)
    expect(service.createBuybackRequests.mock.calls[0][0]).not.toHaveProperty("final_amount")
  })
  it("uses photo methods generated by the real module service", () => {
    for (const method of ["listBuybackPhotos", "createBuybackPhotos", "updateBuybackPhotos", "deleteBuybackPhotos"] as const) {
      expect(typeof BuybackModuleService.prototype[method]).toBe("function")
    }
    for (const method of Object.keys(setup().service)) {
      expect(typeof BuybackModuleService.prototype[method]).toBe("function")
    }
  })
  it("returns an existing submission with photos when the customer retries", async () => {
    const flow = setup("awaiting_photos", { submission_key: "existing-key" })
    const { output } = await create({ customer_id: "customer1", submission_key: "existing-key" }, { container: flow.container })
    expect(flow.service.listBuybackRequests).toHaveBeenCalledWith({ customer_id: "customer1", submission_key: "existing-key" })
    expect(output).toMatchObject({ id: "req1", status: "awaiting_photos", photos: expect.arrayContaining([expect.objectContaining({ photo_type: "front" }), expect.objectContaining({ photo_type: "back" })]) })
    expect(output).not.toHaveProperty("customer_id")
    expect(output).not.toHaveProperty("submission_key")
    expect(output.photos[0]).not.toHaveProperty("storage_key")
  })
  it("rejects client supplied prices, final status and customer identity", () => {
    const body = { submission_key: "8b9b870f-65ba-4a76-9edb-0bf543646382", contact_name: "Test Customer", phone: "123456789", return_address: { name: "Test", address_1: "Example 1", city: "Paris", postal_code: "75001", country_code: "fr" }, items: [{ model: "iphone-13", capacity: "128GB", assessment }], reference_acknowledged: true, privacy_acknowledged: true }
    expect(CreateBuyback.safeParse(body).success).toBe(true)
    for (const extra of [{ reference_amount: 999 }, { final_amount: 999 }, { customer_id: "other" }, { status: "paid" }]) expect(CreateBuyback.safeParse({ ...body, ...extra }).success).toBe(false)
    expect(BuybackCustomerCommand.safeParse({ action: "mark_paid", payment_reference: "test" }).success).toBe(false)
    expect(BuybackAdminCommand.safeParse({ action: "final_offer", items: [{ item_id: "item1", amount: 1.001, notes: "test" }] }).success).toBe(false)
  })
  it("requires physical receipt and registered devices before a final offer", async () => {
    const command = { action: "final_offer", items: [{ item_id: "item1", amount: 145.51, notes: "Screen scratches" }] }
    await expect(setup("in_transit").admin(command)).rejects.toThrow("BUYBACK_LINK_DEVICES_FIRST")
    await expect(setup("received", { items: [{ ...item, device_id: null }] }).admin(command)).rejects.toThrow("BUYBACK_LINK_DEVICES_FIRST")
    const { output } = await setup().admin(command)
    expect(output).toMatchObject({ status: "final_offer", final_amount: 145.51, reference_amount: 180 })
  })
  it("requires customer acceptance of the current final offer before payment", async () => {
    await expect(setup("final_offer", { final_amount: 145.51 }).admin({ action: "mark_paid", payment_reference: "BANK123" })).rejects.toThrow("BUYBACK_ACCEPT_FIRST")
    const flow = setup("final_offer", { final_amount: 145.51 })
    await expect(flow.customer({ action: "accept_offer", final_amount: 180 })).rejects.toThrow("BUYBACK_OFFER_CHANGED")
    expect((await flow.customer({ action: "accept_offer", final_amount: 145.51 })).output.status).toBe("accepted")
    expect((await flow.admin({ action: "mark_paid", payment_reference: "BANK123" })).output).toMatchObject({ status: "paid", payment_reference: "BANK123" })
    await expect(flow.customer({ action: "decline_offer" })).rejects.toThrow("BUYBACK_INVALID_STATE")
  })
  it("returns declined phones to the saved original address without payment", async () => {
    const address = { name: "Test", address_1: "Original 1", country_code: "fr" }
    const flow = setup("final_offer", { final_amount: 145.51, return_address: address })
    expect((await flow.customer({ action: "decline_offer" })).output.status).toBe("return_requested")
    await expect(flow.admin({ action: "mark_paid", payment_reference: "BANK123" })).rejects.toThrow()
    expect((await flow.admin({ action: "mark_returning", return_tracking: "RETURN123" })).output.status).toBe("returning")
    expect((await flow.admin({ action: "mark_returned" })).output).toMatchObject({ status: "returned", return_address: address, return_tracking: "RETURN123" })
  })
  it("does not use packaging or another phone's photos as appearance evidence", () => {
    expect(() => assertPhotos([item] as any, [photo("packaging"), photo("sealed"), photo("front", "approved", "item2"), photo("back")] as any)).toThrow("BUYBACK_PHOTOS_REQUIRED")
    expect(() => assertPhotos([{ ...item, assessment: { ...assessment, frame: "damaged" } }] as any, [photo("front"), photo("back")] as any)).toThrow()
    expect(() => assertPhotos([item] as any, [photo("front"), photo("back")] as any, true)).not.toThrow()
  })
  it("requires approved current photos, including replacements, before shipment", async () => {
    const photos = [photo("front"), photo("back"), photo("front", "retake", "item1", "2026-10-02")]
    await expect(setup("photo_review", {}, photos).admin({ action: "approve_shipping", shipping_instructions: "Prepaid label supplied" })).rejects.toThrow("BUYBACK_PHOTOS_REQUIRED")
    expect((await setup("photo_review").admin({ action: "approve_shipping", shipping_instructions: "Prepaid label supplied" })).output.status).toBe("ready_to_ship")
  })
  it("does not expose another customer's application", async () => {
    await expect(customerBuyback(setup().container as any, "req1", "customer2")).rejects.toThrow("BUYBACK_NOT_FOUND")
  })
  it("deletes private photo objects only for applications past retention", async () => {
    const flow = setup()
    flow.service.listBuybackRequests.mockResolvedValueOnce([{ id: "req1" }]).mockResolvedValueOnce([])
    const { output } = await expire({}, { container: flow.container })
    expect(output.expired).toBe(2)
    expect(flow.service.listBuybackRequests.mock.calls[0][0].closed_at.$lt).toBeInstanceOf(Date)
    expect(flow.files.deleteFiles).toHaveBeenCalledTimes(2)
    expect(flow.service.updateBuybackPhotos).toHaveBeenCalledWith(expect.objectContaining({ storage_key: "", review_status: "expired" }))
  })
})
