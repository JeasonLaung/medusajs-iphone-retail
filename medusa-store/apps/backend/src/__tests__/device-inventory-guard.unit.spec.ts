import { guardDeviceInventory } from "../api/device-guards"
import type { MedusaRequest, MedusaResponse } from "@medusajs/framework/http"

it("allows native stock changes without consulting device enrolment", async () => {
  const next = jest.fn()
  const resolve = jest.fn(() => { throw new Error("Device records must not restrict quantity stock") })
  await guardDeviceInventory({ scope: { resolve } } as unknown as MedusaRequest, {} as MedusaResponse, next)
  expect(next).toHaveBeenCalledTimes(1)
  expect(resolve).not.toHaveBeenCalled()
})
