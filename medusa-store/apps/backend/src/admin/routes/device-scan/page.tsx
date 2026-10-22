import { defineRouteConfig } from "@medusajs/admin-sdk"
import { useWarehouseAccess } from "../../lib/use-warehouse-menu"
import { Navigate, useSearchParams } from "react-router-dom"

const DeviceDirectoryPage = () => {
  const [params] = useSearchParams()
  const { data } = useWarehouseAccess()
  if (!data) return null
  const destination = data.warehouse ? (data.permissions.includes("warehouse_intake:read") ? "intake" : "returned") : params.get("order_id") ? "orders" : "lookup"
  const search = params.toString()
  return <Navigate to={`/device-scan/${destination}${search ? `?${search}` : ""}`} replace />
}

export default DeviceDirectoryPage

export const config = defineRouteConfig({ label: "deviceScan.title", translationNs: "translation", rank: 5 })
