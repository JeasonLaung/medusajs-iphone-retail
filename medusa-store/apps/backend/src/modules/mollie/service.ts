import {
  AbstractPaymentProvider,
  BigNumber,
  MedusaError,
  PaymentActions,
  PaymentSessionStatus,
} from "@medusajs/framework/utils"
import type {
  InitiatePaymentInput,
  InitiatePaymentOutput,
  RefundPaymentInput,
  RefundPaymentOutput,
  UpdatePaymentInput,
  UpdatePaymentOutput,
  ProviderWebhookPayload,
  WebhookActionResult,
  CancelPaymentInput,
  CancelPaymentOutput,
} from "@medusajs/framework/types"
import { randomUUID } from "node:crypto"
import createMollieClient from "@mollie/api-client"
import { MOLLIE_SETTINGS_MODULE } from "../mollie-settings"
import type MollieSettingsModuleService from "../mollie-settings/service"
import { decryptMollieSecret } from "../mollie-settings/security"

type MolliePayment = Record<string, unknown> & {
  id: string
  status: string
  isCancelable?: boolean | null
  amount: { value: string; currency: string }
  metadata?: {
    session_id?: string
    idempotency_key?: string
    cart_id?: string
  }
}

type MollieProviderInstance = Omit<
  AbstractPaymentProvider<Record<string, unknown>>,
  | "initiatePayment"
  | "updatePayment"
  | "refundPayment"
  | "cancelPayment"
  | "getWebhookActionAndData"
> & {
  options_: { redirectUrl: string; medusaUrl: string; description?: string }
  debug_: boolean
  client_: {
    payments: {
      get(id: string): Promise<MolliePayment>
      create(input: Record<string, unknown>): Promise<MolliePayment>
      cancel(id: string): Promise<MolliePayment>
    }
    paymentRefunds: {
      create(input: Record<string, unknown>): Promise<Record<string, unknown>>
    }
  }
  normalizePaymentCreateParams(): Record<string, unknown>
}

export type MollieProviderConstructor = {
  new (
    container: Record<string, unknown>,
    options: Record<string, unknown>,
  ): MollieProviderInstance
  identifier: string
}

const invalid = (message: string) =>
  new MedusaError(MedusaError.Types.INVALID_DATA, message)

export function validateMollieReturnUrl(
  value: unknown,
  configuredUrl: string,
  cartId: unknown,
) {
  const configured = new URL(configuredUrl)
  const url = new URL(typeof value === "string" ? value : configuredUrl)
  if (
    !["http:", "https:"].includes(url.protocol) ||
    url.username ||
    url.password ||
    url.origin !== configured.origin ||
    url.pathname !== "/api/mollie-return" ||
    typeof cartId !== "string" ||
    !/^cart_[\w]+$/.test(cartId) ||
    url.searchParams.get("cart_id") !== cartId
  )
    throw invalid("A valid storefront payment return URL is required")
  return url.href
}

export function mollieWebhookResult(
  payment: MolliePayment,
): WebhookActionResult {
  const sessionId =
    payment.metadata?.session_id || payment.metadata?.idempotency_key
  if (!sessionId || !/^payses_[\w]+$/.test(sessionId))
    throw invalid("Missing Mollie payment session reference")
  const amount = Number(payment.amount?.value)
  if (!Number.isFinite(amount) || amount < 0)
    throw invalid("Invalid Mollie payment amount")
  const actions: Record<string, WebhookActionResult["action"]> = {
    paid: PaymentActions.SUCCESSFUL,
    authorized: PaymentActions.AUTHORIZED,
    canceled: PaymentActions.CANCELED,
    expired: PaymentActions.FAILED,
    failed: PaymentActions.FAILED,
    pending: PaymentActions.PENDING,
    open: PaymentActions.NOT_SUPPORTED,
  }
  return {
    action: actions[payment.status] || PaymentActions.NOT_SUPPORTED,
    data: {
      session_id: sessionId,
      amount: new BigNumber(payment.amount.value),
    },
  }
}

// Keep the upstream method identifiers and SDK, and repair the integration
// points that must carry Medusa's session ID, monetary value and cart return URL.
export function createMollieProvider(BaseProvider: MollieProviderConstructor) {
  return class MolliePaymentProvider extends BaseProvider {
    static identifier = BaseProvider.identifier
    private container_: Record<string, unknown>

    constructor(
      container: Record<string, unknown>,
      options: Record<string, unknown>,
    ) {
      super(container, options)
      this.container_ = container
      this.debug_ = false
    }

    private async refreshMollieConfig() {
      const settings = this.container_[MOLLIE_SETTINGS_MODULE] as MollieSettingsModuleService | undefined
      if (!settings) return
      const [configs] = await settings.listAndCountMollieConfigs({}, { take: 1 })
      const config = configs[0]
      if (!config?.enabled || !config.api_key) return
      const apiKey = decryptMollieSecret(config.api_key)
      if (!apiKey) return
      this.client_ = createMollieClient({ apiKey }) as unknown as MollieProviderInstance["client_"]
      this.options_.redirectUrl = config.redirect_url || this.options_.redirectUrl
    }

    async initiatePayment(
      input: InitiatePaymentInput,
    ): Promise<InitiatePaymentOutput> {
      await this.refreshMollieConfig()
      const sessionId = input.data?.session_id || input.context?.idempotency_key
      if (typeof sessionId !== "string")
        throw invalid("Missing payment session reference")
      const redirectUrl = validateMollieReturnUrl(
        input.data?.redirect_url,
        this.options_.redirectUrl,
        input.data?.cart_id,
      )
      const params = this.normalizePaymentCreateParams()
      const webhookUrl = new URL(
        `/hooks/payment/${BaseProvider.identifier}_mollie`,
        this.options_.medusaUrl,
      ).href
      const payment = await this.client_.payments.create({
        ...params,
        webhookUrl,
        amount: {
          value: Number(input.amount.toString()).toFixed(2),
          currency: input.currency_code.toUpperCase(),
        },
        description: this.options_.description || "iPhone retail order",
        redirectUrl,
        metadata: { session_id: sessionId, cart_id: input.data?.cart_id },
        idempotencyKey:
          typeof input.data?.mollie_attempt === "string"
            ? input.data.mollie_attempt
            : sessionId,
      })
      return {
        id: payment.id,
        data: {
          ...payment,
          cart_id: input.data?.cart_id,
          redirect_url: redirectUrl,
          session_id: sessionId,
        },
      }
    }

    async updatePayment(
      input: UpdatePaymentInput,
    ): Promise<UpdatePaymentOutput> {
      await this.refreshMollieConfig()
      const id = input.data?.id
      if (typeof id !== "string") throw invalid("Missing Mollie payment ID")
      const payment = await this.client_.payments.get(id)
      const sameAmount =
        Number(payment.amount.value) === Number(input.amount.toString()) &&
        payment.amount.currency.toLowerCase() ===
          input.currency_code.toLowerCase()
      if (!sameAmount) {
        if (["paid", "authorized"].includes(payment.status))
          throw invalid(
            "This payment has already been made. Finish your order before changing its amount.",
          )
        // Mollie cannot change an existing payment amount. Replace only an
        // unpaid payment, so the hosted checkout always displays the cart total.
        await this.cancelPayment({ data: input.data })
        return this.initiatePayment({
          ...input,
          data: { ...input.data, mollie_attempt: randomUUID() },
        })
      }
      const statuses: Record<string, PaymentSessionStatus> = {
        open: PaymentSessionStatus.PENDING,
        pending: PaymentSessionStatus.PENDING,
        paid: PaymentSessionStatus.AUTHORIZED,
        authorized: PaymentSessionStatus.AUTHORIZED,
        canceled: PaymentSessionStatus.CANCELED,
        expired: PaymentSessionStatus.ERROR,
        failed: PaymentSessionStatus.ERROR,
      }
      return {
        data: { ...input.data, ...payment },
        status: statuses[payment.status] ?? PaymentSessionStatus.ERROR,
      }
    }

    async refundPayment(
      input: RefundPaymentInput,
    ): Promise<RefundPaymentOutput> {
      await this.refreshMollieConfig()
      const id = input.data?.id
      if (typeof id !== "string") throw invalid("Missing Mollie payment ID")
      const payment = await this.client_.payments.get(id)
      const refund = await this.client_.paymentRefunds.create({
        paymentId: id,
        amount: {
          value: new BigNumber(input.amount).numeric.toFixed(2),
          currency: payment.amount.currency,
        },
      })
      // Keep the original payment ID for subsequent partial refunds.
      return { data: { ...input.data, last_refund: refund } }
    }

    async cancelPayment(
      input: CancelPaymentInput,
    ): Promise<CancelPaymentOutput> {
      await this.refreshMollieConfig()
      const id = input.data?.id
      if (typeof id !== "string") throw invalid("Missing Mollie payment ID")
      const payment = await this.client_.payments.get(id)
      if (["paid", "authorized"].includes(payment.status))
        throw invalid(
          "This payment has already been made. Finish your order before changing payment methods.",
        )
      if (["canceled", "expired", "failed"].includes(payment.status))
        return { data: input.data }
      // Hosted checkout payments can be open without supporting cancellation.
      // Their local session may be discarded, but never hide a processing payment.
      if (payment.status === "open" && payment.isCancelable === false)
        return { data: { ...input.data, ...payment } }
      const canceled = await this.client_.payments.cancel(id)
      return { data: { ...input.data, ...canceled } }
    }

    async getWebhookActionAndData(
      payload: ProviderWebhookPayload["payload"],
    ): Promise<WebhookActionResult> {
      await this.refreshMollieConfig()
      const id = payload.data?.id
      if (typeof id !== "string" || !/^tr_[\w]+$/.test(id))
        throw invalid("Invalid Mollie payment ID")
      // Mollie posts only an ID. Fetch the authoritative payment instead of
      // accepting status or amount supplied by the webhook caller.
      return mollieWebhookResult(await this.client_.payments.get(id))
    }

  }
}
