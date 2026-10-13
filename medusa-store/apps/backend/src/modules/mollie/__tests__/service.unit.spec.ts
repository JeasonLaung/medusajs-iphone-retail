import upstream from "@variablevic/mollie-payments-medusa/providers/mollie"
import {
  createMollieProvider,
  mollieWebhookResult,
  validateMollieReturnUrl,
  type MollieProviderConstructor,
} from "../service"

const returnUrl = "https://shop.example/api/mollie-return?cart_id=cart_test"
const payment = {
  id: "tr_test",
  status: "paid",
  amount: { value: "329.00", currency: "EUR" },
  metadata: { session_id: "payses_test" },
}

function setup() {
  const Base = (
    upstream as { services: MollieProviderConstructor[] }
  ).services.find((service) => service.identifier === "mollie-hosted-checkout")!
  const Service = createMollieProvider(Base)
  const service = new Service(
    { logger: console },
    {
      apiKey: `test_${"x".repeat(30)}`,
      redirectUrl: "https://shop.example/api/mollie-return",
      medusaUrl: "https://api.example",
    },
  )
  const client = {
    payments: {
      get: jest.fn().mockResolvedValue(payment),
      create: jest.fn().mockResolvedValue({ ...payment, status: "open" }),
      cancel: jest.fn().mockResolvedValue({ ...payment, status: "canceled" }),
    },
    paymentRefunds: { create: jest.fn().mockResolvedValue({ id: "re_test" }) },
  }
  service.client_ = client
  return { service, client }
}

describe("Mollie checkout", () => {
  it("uses the server session ID and exact amount when creating a payment", async () => {
    const { service, client } = setup()
    const result = await service.initiatePayment({
      amount: 329,
      currency_code: "eur",
      context: { idempotency_key: "payses_test" },
      data: { cart_id: "cart_test", redirect_url: returnUrl },
    })
    expect(client.payments.create).toHaveBeenCalledWith(
      expect.objectContaining({
        amount: { value: "329.00", currency: "EUR" },
        redirectUrl: returnUrl,
        webhookUrl:
          "https://api.example/hooks/payment/mollie-hosted-checkout_mollie",
        metadata: { session_id: "payses_test", cart_id: "cart_test" },
        idempotencyKey: "payses_test",
      }),
    )
    expect(result.data?.session_id).toBe("payses_test")
  })

  it("rejects redirects outside the storefront or to another cart", () => {
    expect(() =>
      validateMollieReturnUrl(
        "https://evil.example/api/mollie-return?cart_id=cart_test",
        returnUrl,
        "cart_test",
      ),
    ).toThrow()
    expect(() =>
      validateMollieReturnUrl(returnUrl, returnUrl, "cart_other"),
    ).toThrow()
  })

  it("parses webhook amounts without overwriting the Medusa session reference", () => {
    const result = mollieWebhookResult(payment)
    expect(result.action).toBe("captured")
    expect(result.data?.session_id).toBe("payses_test")
    expect(Number(result.data?.amount.toString())).toBe(329)
  })

  it("ignores an open payment and maps cancellations and failures", () => {
    expect(mollieWebhookResult({ ...payment, status: "open" }).action).toBe(
      "not_supported",
    )
    expect(mollieWebhookResult({ ...payment, status: "canceled" }).action).toBe(
      "canceled",
    )
    expect(mollieWebhookResult({ ...payment, status: "failed" }).action).toBe(
      "failed",
    )
  })

  it("re-fetches webhook payments instead of trusting caller-supplied payment data", async () => {
    const { service, client } = setup()
    const result = await service.getWebhookActionAndData({
      data: { id: "tr_test", status: "failed", amount: "1.00" },
      rawData: "",
      headers: {},
    })
    expect(client.payments.get).toHaveBeenCalledWith("tr_test")
    expect(result.action).toBe("captured")
    expect(Number(result.data?.amount.toString())).toBe(329)
  })

  it("refunds only the requested amount and preserves the original payment ID", async () => {
    const { service, client } = setup()
    const result = await service.refundPayment({
      amount: 10.5,
      data: { id: "tr_test" },
    })
    expect(client.paymentRefunds.create).toHaveBeenCalledWith({
      paymentId: "tr_test",
      amount: { value: "10.50", currency: "EUR" },
    })
    expect(result.data?.id).toBe("tr_test")
  })

  it("never replaces or cancels a paid payment when the cart total changes", async () => {
    const { service, client } = setup()
    await expect(
      service.updatePayment({
        amount: 330,
        currency_code: "eur",
        data: { id: "tr_test" },
      }),
    ).rejects.toThrow("already been made")
    await expect(
      service.cancelPayment({ data: { id: "tr_test" } }),
    ).rejects.toThrow("already been made")
    expect(client.payments.create).not.toHaveBeenCalled()
    expect(client.payments.cancel).not.toHaveBeenCalled()
  })

  it("replaces an unpaid payment at the new total without changing the session ID", async () => {
    const { service, client } = setup()
    client.payments.get.mockResolvedValue({ ...payment, status: "open" })
    await service.updatePayment({
      amount: 330,
      currency_code: "eur",
      data: {
        id: "tr_test",
        session_id: "payses_test",
        cart_id: "cart_test",
        redirect_url: returnUrl,
      },
    })
    expect(client.payments.cancel).toHaveBeenCalledWith("tr_test")
    expect(client.payments.create).toHaveBeenCalledWith(
      expect.objectContaining({
        amount: { value: "330.00", currency: "EUR" },
        metadata: { session_id: "payses_test", cart_id: "cart_test" },
        idempotencyKey: expect.any(String),
      }),
    )
    expect(client.payments.create.mock.calls[0][0].idempotencyKey).not.toBe(
      "payses_test",
    )
  })
})
