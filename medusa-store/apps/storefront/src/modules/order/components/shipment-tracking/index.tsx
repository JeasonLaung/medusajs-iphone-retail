"use client"

import { Alert, Tag, Timeline } from "antd"
import { useAccountCopy } from "@modules/account/use-account-copy"
import type { OrderShipment } from "types/order-timeline"
import demo from "../../../../../../../shared/shipping-demo.json"

export default function ShipmentTracking({ shipments = [], demonstration = false }: {
  shipments?: OrderShipment[]
  demonstration?: boolean
}) {
  const { locale } = useAccountCopy()
  const zh = locale.startsWith("zh")
  if (!shipments.length) return null

  return <section className="mt-6 min-w-0 space-y-4" aria-label={zh ? "包裹物流" : "Shipment tracking"} data-testid="shipment-tracking">
    <h3 className="text-lg font-semibold">{zh ? "包裹物流" : "Shipment tracking"}</h3>
    {demonstration && <Alert type="info" showIcon title={zh ? "演示数据，不关联真实订单" : "Demo data, not linked to a real order"} description={zh ? "使用 Ship24 官方测试单号；以下时间和轨迹节点为本地模拟，未调用物流 API。" : "Uses an official Ship24 test number. The times and tracking events below are locally simulated; no tracking API has been called."} />}
    {shipments.map((shipment, index) => <div key={shipment.id} className="min-w-0 space-y-3 rounded-md bg-ui-bg-subtle p-4">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-medium">{zh ? `包裹 ${index + 1}` : `Package ${index + 1}`}</span>
        <Tag color={shipment.delivered_at ? "success" : "processing"}>{demonstration ? (zh ? "派送中（模拟）" : "Out for delivery (simulated)") : shipment.delivered_at ? (zh ? "已送达" : "Delivered") : (zh ? "已发货" : "Shipped")}</Tag>
      </div>
      {shipment.labels.length ? shipment.labels.map((label, labelIndex) => {
        const safeUrl = /^https?:\/\//i.test(label.tracking_url) ? label.tracking_url : null
        return <div key={labelIndex} className="min-w-0 space-y-1">
          <p className="text-sm break-all">{zh ? "物流单号：" : "Tracking number: "}{label.tracking_number || (zh ? "暂未提供" : "Not provided")}</p>
          {safeUrl && <a href={safeUrl} target="_blank" rel="noopener noreferrer" className="text-ui-fg-interactive underline focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2">{zh ? "查看承运商物流" : "Track with the carrier"}</a>}
        </div>
      }) : <p className="text-sm text-ui-fg-subtle">{zh ? "物流单号暂未提供" : "Tracking number not provided yet"}</p>}
      {demonstration ? <>
        <Timeline className="pt-3" items={demo.events.map((event) => ({ content: <div><p>{zh ? event.zh : event.en}</p><time className="text-sm text-ui-fg-subtle" dateTime={event.time}>{new Date(event.time).toLocaleString(locale, { timeZone: "Asia/Shanghai", timeZoneName: "short", hour12: false })}</time></div> }))} />
        <a className="text-sm text-ui-fg-interactive underline" href={demo.source_url} target="_blank" rel="noopener noreferrer">{zh ? "官方测试单号说明" : "Official test number documentation"}</a>
      </> : <p className="text-sm text-ui-fg-subtle">{zh ? "暂未接入站内运输轨迹；如提供查询链接，可前往承运商页面查看。" : "In-site transit events are not connected yet. Use the carrier link, if provided, for tracking details."}</p>}
    </div>)}
  </section>
}
