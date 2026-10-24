"use client"

import ShipmentTracking from "@modules/order/components/shipment-tracking"
import { useAccountCopy } from "@modules/account/use-account-copy"
import demo from "../../../../../../shared/shipping-demo.json"

export default function ShippingDemo() {
  const { locale } = useAccountCopy()
  return <div className="content-container py-8">
    <h1 className="text-2xl font-semibold">{locale.startsWith("zh") ? "顾客订单物流演示" : "Customer order shipping demo"}</h1>
    <ShipmentTracking shipments={[demo]} demonstration />
  </div>
}
