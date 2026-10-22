import type { AccountKey } from "@modules/account/i18n"

export function getAfterSalesErrorKey(error: unknown): AccountKey {
  const { status, message } = (error || {}) as { status?: number; message?: string }
  if (status === 401) return "afterSalesSessionExpired"
  if (status === 404) return "afterSalesOrderNotLinked"
  if (status === 400 || status === 409) {
    switch (message) {
      case "Select the shipped devices for this order item":
        return "afterSalesShippedDevicesRequired"
      case "Choose valid order items and quantities":
        return "afterSalesInvalidItems"
      case "This order item has no registered devices":
        return "afterSalesDevicesChanged"
      case "This order cannot receive an after-sales request":
        return "afterSalesUnavailable"
      case "An after-sales request is already open for this order":
        return "openRequest"
    }
  }
  return "submitRequestError"
}
