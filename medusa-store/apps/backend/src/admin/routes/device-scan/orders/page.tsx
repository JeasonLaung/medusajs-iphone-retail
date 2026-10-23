import { Navigate, useSearchParams } from "react-router-dom"

const DeviceOrdersPage = () => {
  const [params] = useSearchParams()
  const orderId = params.get("order_id")
  return <Navigate to={orderId ? `/orders/${encodeURIComponent(orderId)}` : "/orders"} replace />
}

export default DeviceOrdersPage
