import type {
  MedusaRequest,
  MedusaResponse,
  MedusaNextFunction,
} from "@medusajs/framework/http"
import {
  ContainerRegistrationKeys,
  MedusaError,
  remoteQueryObjectFromString,
} from "@medusajs/framework/utils"

const manualProvider = "pp_system_default"
const isProduction = () => process.env.NODE_ENV === "production"
const rejectManualPayment = () => {
  throw new MedusaError(
    MedusaError.Types.NOT_ALLOWED,
    "Manual payment is disabled in production. Choose an online payment method.",
  )
}

export function guardManualPaymentSelection(
  req: MedusaRequest,
  _res: MedusaResponse,
  next: MedusaNextFunction,
) {
  if (isProduction() && (req.body as { provider_id?: string })?.provider_id === manualProvider)
    rejectManualPayment()
  next()
}

export function guardMarkAsPaid(
  _req: MedusaRequest,
  _res: MedusaResponse,
  next: MedusaNextFunction,
) {
  // Medusa's mark-as-paid workflow creates and captures a system payment.
  if (isProduction()) rejectManualPayment()
  next()
}

export async function guardManualCartCompletion(
  req: MedusaRequest,
  _res: MedusaResponse,
  next: MedusaNextFunction,
) {
  if (!isProduction()) return next()
  const query = req.scope.resolve(ContainerRegistrationKeys.QUERY)
  const { data: carts } = await query.graph({
    entity: "cart",
    fields: ["id", "payment_collection.payment_sessions.provider_id", "payment_collection.payment_sessions.status"],
    filters: { id: req.params.id },
  })
  const activeStatuses = ["pending", "requires_more", "authorized", "captured", "pending_authorization"]
  if (carts[0]?.payment_collection?.payment_sessions?.some(
    (session) => session?.provider_id === manualProvider && activeStatuses.includes(session.status),
  )) rejectManualPayment()
  next()
}

export async function guardManualPaymentAuthorization(
  req: MedusaRequest,
  _res: MedusaResponse,
  next: MedusaNextFunction,
) {
  if (!isProduction()) return next()
  const query = req.scope.resolve(ContainerRegistrationKeys.QUERY)
  const { data } = await query.graph({
    entity: "payment_session",
    fields: ["provider_id"],
    filters: { id: (req.body as { payment_session_id: string }).payment_session_id },
  })
  if (data[0]?.provider_id === manualProvider) rejectManualPayment()
  next()
}

export async function guardManualPaymentCapture(
  req: MedusaRequest,
  _res: MedusaResponse,
  next: MedusaNextFunction,
) {
  if (!isProduction()) return next()
  const query = req.scope.resolve(ContainerRegistrationKeys.QUERY)
  const { data } = await query.graph({
    entity: "payment",
    fields: ["provider_id"],
    filters: { id: req.params.id },
  })
  if (data[0]?.provider_id === manualProvider) rejectManualPayment()
  next()
}

export async function listProductionPaymentProviders(
  req: MedusaRequest,
  res: MedusaResponse,
  next: MedusaNextFunction,
) {
  if (!isProduction()) return next()
  const isStore = req.path.startsWith("/store/")
  if (isStore && !req.filterableFields.region_id)
    throw new MedusaError(MedusaError.Types.NOT_ALLOWED, "You must provide the region_id to list payment providers")
  const remoteQuery = req.scope.resolve(ContainerRegistrationKeys.REMOTE_QUERY)
  const query = remoteQueryObjectFromString({
    entryPoint: isStore ? "region_payment_provider" : "payment_provider",
    variables: {
      filters: isStore
        ? { region_id: req.filterableFields.region_id, payment_provider_id: { $ne: manualProvider } }
        : { $and: [req.filterableFields, { id: { $ne: manualProvider } }] },
      ...req.queryConfig.pagination,
    },
    fields: isStore
      ? req.queryConfig.fields.map((field) => `payment_provider.${field}`)
      : req.queryConfig.fields,
  })
  const { rows, metadata } = await remoteQuery(query)
  res.json({
    payment_providers: isStore ? rows.map((row) => row.payment_provider) : rows,
    count: metadata.count,
    offset: metadata.skip,
    limit: metadata.take,
  })
}
