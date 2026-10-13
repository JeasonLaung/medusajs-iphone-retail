import type { ExecArgs } from "@medusajs/framework/types"
import {
  ContainerRegistrationKeys,
  MedusaError,
  Modules,
} from "@medusajs/framework/utils"
import { updateRegionsWorkflow } from "@medusajs/medusa/core-flows"

export default async function enableCheckoutPayments({
  container,
  args,
}: ExecArgs) {
  const regionId = args[0]
  if (!regionId?.startsWith("reg_"))
    throw new MedusaError(
      MedusaError.Types.INVALID_DATA,
      "Pass the region ID to enable checkout payments for that region",
    )
  const query = container.resolve(ContainerRegistrationKeys.QUERY)
  const payment = container.resolve(Modules.PAYMENT)
  const { data: regions } = await query.graph({
    entity: "region",
    fields: ["id", "payment_providers.id"],
    filters: { id: regionId },
  })
  const region = regions[0]
  if (!region)
    throw new MedusaError(MedusaError.Types.NOT_FOUND, "Region not found")
  const providers = await payment.listPaymentProviders(
    { is_enabled: true },
    { take: 100 },
  )
  const added = providers
    .filter(
      (provider) =>
        provider.id === "pp_paypal_paypal" ||
        provider.id === "pp_paypal_card_paypal_card" ||
        provider.id.startsWith("pp_mollie-"),
    )
    .map((provider) => provider.id)
  const ids = [
    ...new Set([
      ...(region.payment_providers ?? []).flatMap((provider) =>
        provider ? [provider.id] : [],
      ),
      ...added,
    ]),
  ]
  await updateRegionsWorkflow(container).run({
    input: { selector: { id: regionId }, update: { payment_providers: ids } },
  })
  container
    .resolve(ContainerRegistrationKeys.LOGGER)
    .info(
      `Checkout payment providers enabled for ${regionId}: ${added.join(", ")}`,
    )
}
