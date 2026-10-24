import { defineWidgetConfig } from "@medusajs/admin-sdk"
import type { AdminOrder, DetailWidgetProps } from "@medusajs/framework/types"
import { Button, Container, Text } from "@medusajs/ui"
import { useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import DhlOfficialTracking from "../components/dhl-official-tracking"
import ShipmentTracking from "../components/shipment-tracking"
import { sdk } from "../lib/sdk"

const OrderShipping = ({ data: order }: DetailWidgetProps<AdminOrder>) => {
  const { i18n } = useTranslation()
  const zh = i18n.language.startsWith("zh")
  const query = useQuery({
    queryKey: ["orders", order.id, "shipping"],
    queryFn: () => sdk.admin.order.retrieve(order.id, { fields: "id,metadata,*fulfillments,*fulfillments.labels" }),
  })
  if (query.isPending) return <Container className="px-6 py-4"><Text role="status">{zh ? "正在加载物流…" : "Loading shipping…"}</Text></Container>
  if (query.isError) return <Container className="space-y-3 px-6 py-4"><Text role="alert">{zh ? "物流信息加载失败" : "Unable to load shipping"}</Text><Button size="small" variant="secondary" onClick={() => void query.refetch()}>{zh ? "重试" : "Retry"}</Button></Container>
  return <><ShipmentTracking shipments={query.data.order.fulfillments || []} carrierDemo={query.data.order.metadata?.shipping_demo === "dhl-official"} />{query.data.order.metadata?.shipping_demo === "dhl-official" && <DhlOfficialTracking />}</>
}

export const config = defineWidgetConfig({ zone: "order.details.after" })

export default OrderShipping
