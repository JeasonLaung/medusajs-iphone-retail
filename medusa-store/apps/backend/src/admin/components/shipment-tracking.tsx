import { Container, Text, Badge } from "@medusajs/ui"
import { useTranslation } from "react-i18next"
import demo from "../../../../../shared/shipping-demo.json"

type Shipment = {
  id: string
  shipped_at?: string | Date | null
  delivered_at?: string | Date | null
  canceled_at?: string | Date | null
  labels?: { tracking_number: string; tracking_url: string }[]
}

export default function ShipmentTracking({ shipments, demonstration = false }: {
  shipments: Shipment[]
  demonstration?: boolean
}) {
  const { i18n } = useTranslation()
  const zh = i18n.language.startsWith("zh")
  const active = shipments.filter((shipment) => !shipment.canceled_at && shipment.shipped_at)
  return <Container className="space-y-4 px-6 py-4" data-testid="admin-shipment-tracking">
    <Text weight="plus">{zh ? "包裹物流" : "Shipment tracking"}</Text>
    {demonstration && <Text size="small" className="text-ui-fg-subtle">{zh ? "演示数据，不关联真实订单。使用 Ship24 官方测试单号；时间和轨迹节点为本地模拟，未调用物流 API。" : "Demo data, not linked to a real order. Uses an official Ship24 test number. Times and tracking events are locally simulated; no tracking API has been called."}</Text>}
    {!active.length && <Text size="small" className="text-ui-fg-subtle">{zh ? "暂无已发货包裹" : "No shipped packages yet"}</Text>}
    {active.map((shipment, index) => <div key={shipment.id} className="min-w-0 space-y-3 rounded-md bg-ui-bg-subtle p-4">
      <div className="flex flex-wrap items-center gap-2"><Text size="small" weight="plus">{zh ? `包裹 ${index + 1}` : `Package ${index + 1}`}</Text><Badge color={shipment.delivered_at ? "green" : "blue"}>{demonstration ? (zh ? "派送中（模拟）" : "Out for delivery (simulated)") : shipment.delivered_at ? (zh ? "已送达" : "Delivered") : (zh ? "已发货" : "Shipped")}</Badge></div>
      {shipment.labels?.length ? shipment.labels.map((label, labelIndex) => {
        const safeUrl = /^https?:\/\//i.test(label.tracking_url) ? label.tracking_url : null
        return <div key={labelIndex} className="min-w-0 space-y-1">
          <Text size="small" className="break-all">{zh ? "物流单号：" : "Tracking number: "}{label.tracking_number || (zh ? "暂未提供" : "Not provided")}</Text>
          {safeUrl && <Text size="small"><a href={safeUrl} target="_blank" rel="noopener noreferrer" className="text-ui-fg-interactive underline focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2">{zh ? "查看承运商物流" : "Track with the carrier"}</a></Text>}
        </div>
      }) : <Text size="small" className="text-ui-fg-subtle">{zh ? "物流单号暂未提供" : "Tracking number not provided yet"}</Text>}
      {demonstration ? <>
        <ol className="space-y-4" aria-label={zh ? "模拟物流轨迹" : "Simulated transit events"}>{demo.events.map((event, index) => <li key={event.time} className="flex gap-3"><Badge color={index === 0 ? "blue" : "grey"}>{index + 1}</Badge><div><Text size="small">{zh ? event.zh : event.en}</Text><Text size="small" className="text-ui-fg-subtle"><time dateTime={event.time}>{new Date(event.time).toLocaleString(zh ? "zh-CN" : "en-GB", { timeZone: "Asia/Shanghai", timeZoneName: "short", hour12: false })}</time></Text></div></li>)}</ol>
        <Text size="small"><a className="text-ui-fg-interactive underline" href={demo.source_url} target="_blank" rel="noopener noreferrer">{zh ? "官方测试单号说明" : "Official test number documentation"}</a></Text>
      </> : <Text size="small" className="text-ui-fg-subtle">{zh ? "暂未接入站内运输轨迹；如提供查询链接，可前往承运商页面查看。" : "In-site transit events are not connected yet. Use the carrier link, if provided, for tracking details."}</Text>}
    </div>)}
  </Container>
}
