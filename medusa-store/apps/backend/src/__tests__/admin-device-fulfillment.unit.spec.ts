import fs from "node:fs"
import path from "node:path"
import { transformDeviceFulfillment } from "../lib/admin-device-fulfillment"
import { DeviceCommand } from "../api/admin/devices/validators"

const dashboard = path.dirname(require.resolve("@medusajs/dashboard/package.json"))
const dist = path.join(dashboard, "dist")

describe("native fulfillment integration", () => {
  it.each(["order-create-fulfillment-", "order-detail-"])("extends the installed %s module with valid JavaScript", (prefix) => {
    const filename = fs.readdirSync(dist).find((name) => name.startsWith(prefix) && name.endsWith(".mjs"))!
    const id = path.join(dist, filename)
    const transformed = transformDeviceFulfillment(fs.readFileSync(id, "utf8"), id)!
    expect(transformed).toContain(prefix === "order-detail-" ? "itemId: f_item.line_item_id" : "await deviceFulfillment.create(payload)")
    const swc = require("@swc/core")
    expect(() => swc.parseSync(transformed, { syntax: "ecmascript" })).not.toThrow()
  })
  it("fails explicitly when an installed dashboard integration point changes", () => {
    expect(() => transformDeviceFulfillment("changed", `${dashboard}/dist/order-create-fulfillment-test.mjs`)).toThrow("integration changed")
  })
  it("allows fulfillment with device identities supplied later", () => {
    expect(DeviceCommand.parse({ action: "create_fulfillment", order_id: "o1", allocations: [], fulfillment: { location_id: "l1", shipping_option_id: "so1", no_notification: true, items: [{ id: "i1", quantity: 1 }] } })).toMatchObject({ allocations: [] })
  })
  it("rejects duplicate fulfillment rows and fractional quantities", () => {
    const body = { action: "create_fulfillment", order_id: "o1", allocations: [], fulfillment: { location_id: "l1", shipping_option_id: "so1", no_notification: true, items: [{ id: "i1", quantity: 1 }, { id: "i1", quantity: 1 }] } }
    expect(DeviceCommand.safeParse(body).success).toBe(false)
    expect(DeviceCommand.safeParse({ ...body, fulfillment: { ...body.fulfillment, items: [{ id: "i1", quantity: 0.5 }] } }).success).toBe(false)
  })
})
