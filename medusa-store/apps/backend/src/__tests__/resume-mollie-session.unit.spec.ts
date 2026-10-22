import { mollieHostedProvider, resumeMollieSession } from "../workflows/resume-mollie-session"
import type { IPaymentModuleService } from "@medusajs/framework/types"

function setup(statuses: string[]) {
  const sessions = statuses.map((status, index) => ({
    id: `payses_${index}`, provider_id: mollieHostedProvider, status,
    amount: 329, currency_code: "eur", data: { id: `tr_${index}` },
  }))
  const service = {
    retrievePaymentCollection: jest.fn().mockResolvedValue({ amount: 329, currency_code: "eur", payment_sessions: sessions }),
    updatePaymentSession: jest.fn().mockImplementation(async ({ id }) => sessions.find((session) => session.id === id)),
    deletePaymentSession: jest.fn().mockResolvedValue(undefined),
  }
  return { service, payment: service as unknown as IPaymentModuleService }
}

it("recovers the paid session and removes duplicate unpaid sessions before completion", async () => {
  const { service, payment } = setup(["pending", "authorized", "pending"])
  await expect(resumeMollieSession(payment, "pay_col_test")).resolves.toBe("payses_1")
  expect(service.updatePaymentSession).toHaveBeenCalledTimes(3)
  expect(service.deletePaymentSession.mock.calls).toEqual([["payses_0"], ["payses_2"]])
})

it("reuses an unpaid session at the current total", async () => {
  const { service, payment } = setup(["pending"])
  await expect(resumeMollieSession(payment, "pay_col_test")).resolves.toBe("payses_0")
  expect(service.deletePaymentSession).not.toHaveBeenCalled()
})

it("allows a fresh attempt when every old session has failed", async () => {
  const { payment } = setup(["canceled", "error"])
  await expect(resumeMollieSession(payment, "pay_col_test")).resolves.toBeNull()
})

it("preserves all records if more than one payment succeeded", async () => {
  const { service, payment } = setup(["authorized", "authorized"])
  await expect(resumeMollieSession(payment, "pay_col_test")).rejects.toThrow("MOLLIE_MULTIPLE_PAID_SESSIONS")
  expect(service.deletePaymentSession).not.toHaveBeenCalled()
})

it("does not discard payment records when the paid amount differs from the cart", async () => {
  const { service, payment } = setup(["authorized"])
  service.retrievePaymentCollection.mockResolvedValueOnce({
    amount: 330, currency_code: "eur", payment_sessions: [{
      id: "payses_0", provider_id: mollieHostedProvider, amount: 329, currency_code: "eur", data: {},
    }],
  })
  await expect(resumeMollieSession(payment, "pay_col_test")).rejects.toThrow("MOLLIE_PAID_AMOUNT_MISMATCH")
  expect(service.deletePaymentSession).not.toHaveBeenCalled()
})
