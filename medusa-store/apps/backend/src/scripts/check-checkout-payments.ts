import type { ExecArgs } from "@medusajs/framework/types"
import { ContainerRegistrationKeys, MedusaError } from "@medusajs/framework/utils"

// Read-only local smoke check. No merchant API call or order completion.
export default async function checkCheckoutPayments({ container }: ExecArgs) {
  const query = container.resolve(ContainerRegistrationKeys.QUERY)
  const { data: carts } = await query.graph({
    entity: "cart",
    fields: ["id", "completed_at", "items.id", "shipping_address.country_code"],
    filters: { completed_at: null },
    pagination: { take: 20, order: { updated_at: "DESC" } },
  })
  const cart = carts.find((item) => !item.completed_at && item.items?.length)
  if (!cart)
    throw new MedusaError(MedusaError.Types.NOT_FOUND,
      "No open cart with items is available for the checkout smoke check",
    )
  const country = cart.shipping_address?.country_code || "dk"
  const baseUrl = process.env.STOREFRONT_URL || "http://127.0.0.1:8000"
  const response = await fetch(`${baseUrl}/${country}/checkout?step=payment`, {
    headers: { cookie: `_medusa_cart_id=${cart.id}` },
  })
  const html = await response.text()
  const checks = {
    httpOk: response.status === 200,
    paypalVisible: html.includes("PayPal"),
    mollieVisible: html.includes("Mollie"),
    noApplicationError: !html.includes("Application error:"),
  }
  container.resolve(ContainerRegistrationKeys.LOGGER).info(
    JSON.stringify({
      path: new URL(response.url).pathname,
      title: html.match(/<title>(.*?)<\/title>/)?.[1],
      checkoutHeading: html.includes("Shipping Address"),
      length: html.length,
    }),
  )
  container
    .resolve(ContainerRegistrationKeys.LOGGER)
    .info(JSON.stringify(checks))
  if (Object.values(checks).some((passed) => !passed))
    throw new MedusaError(MedusaError.Types.INVALID_DATA, "Checkout payment smoke check failed")
}
