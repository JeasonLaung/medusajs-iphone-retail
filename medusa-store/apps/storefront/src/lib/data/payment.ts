"use server"

import { sdk } from "@lib/config"
import { getAuthHeaders, getCacheOptions } from "./cookies"
import { HttpTypes } from "@medusajs/types"
import { isMollie, isPaypal } from "@lib/constants"

export const listCartPaymentMethods = async (regionId: string) => {
  const headers = {
    ...(await getAuthHeaders()),
  }

  const next = {
    ...(await getCacheOptions("payment_providers")),
  }

  return sdk.client
    .fetch<HttpTypes.StorePaymentProviderListResponse>(
      `/store/payment-providers`,
      {
        method: "GET",
        query: { region_id: regionId },
        headers,
        next,
        cache: "no-store",
      }
    )
    .then(({ payment_providers }) =>
      payment_providers.sort((a, b) => {
        return a.id > b.id ? 1 : -1
      })
    )
    .catch(() => {
      return null
    })
}

export async function getCheckoutPaymentOptions(cart: HttpTypes.StoreCart) {
  const methods = (await listCartPaymentMethods(cart.region_id!)) ?? []
  type PayPalConfig = {
    client_id?: string
    currency_supported?: boolean
    paypal_enabled?: boolean
    card_enabled?: boolean
    client_token?: string
  }
  let paypalConfig: PayPalConfig | null = null
  if (methods.some((method) => isPaypal(method.id))) {
    paypalConfig = await sdk.client
      .fetch<PayPalConfig>("/store/paypal/config", {
        query: { cart_id: cart.id },
        headers: await getAuthHeaders(),
        cache: "no-store",
      })
      .catch(() => null)
  }
  const available = methods.filter((method) => {
    if (!isPaypal(method.id)) return true
    if (!paypalConfig?.client_id || paypalConfig.currency_supported === false)
      return false
    return method.id === "pp_paypal_paypal"
      ? paypalConfig.paypal_enabled !== false
      : paypalConfig.card_enabled !== false && !!paypalConfig.client_token
  })
  const unavailable = [] as { id: string; title: string }[]
  if (!available.some((method) => isPaypal(method.id))) {
    unavailable.push({ id: "pp_paypal_paypal", title: "PayPal" })
  }
  if (!available.some((method) => isMollie(method.id))) {
    unavailable.push({
      id: "pp_mollie-hosted-checkout_mollie",
      title: "Mollie",
    })
  }
  return { available, unavailable }
}
