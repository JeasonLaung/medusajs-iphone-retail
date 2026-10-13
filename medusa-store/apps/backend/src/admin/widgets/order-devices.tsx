import { defineWidgetConfig } from "@medusajs/admin-sdk"
import type { AdminOrder, DetailWidgetProps } from "@medusajs/framework/types"
import { Container, Heading } from "@medusajs/ui"
import { useQuery } from "@tanstack/react-query"

const OrderDevices = ({ data: order }: DetailWidgetProps<AdminOrder>) => {
  const { data, isError } = useQuery<{
    assigned_devices: {
      id: string
      serial_number: string | null
      imei: string | null
    }[]
    assignments: {
      id: string
      device_id: string
      status: string
      source_id: string
      replacement_of: string | null
      tracking_number: string | null
    }[]
    sources: { id: string; name: string }[]
  }>({
    queryKey: ["order-devices", order.id],
    queryFn: async () => {
      const r = await fetch(
        `/admin/devices?order_id=${encodeURIComponent(order.id)}`,
        { credentials: "include" },
      )
      if (!r.ok) throw new Error("Unable to load devices")
      return r.json()
    },
  })
  return (
    <Container className="space-y-3">
      <Heading level="h2">设备与履约</Heading>
      <a
        className="text-sm underline"
        href={`/app/device-scan?order_id=${encodeURIComponent(order.id)}`}
      >
        进入扫码工作台：配货、发货、退回检测
      </a>
      {isError && <p role="alert">设备记录加载失败。</p>}
      {data?.assignments.map((a) => {
        const d = data.assigned_devices.find((d) => d.id === a.device_id)
        return (
          <div key={a.id} className="rounded-lg border p-3 text-sm">
            <p className="break-all">
              {d?.serial_number || d?.imei || a.device_id} · {a.status} ·{" "}
              {data.sources.find((s) => s.id === a.source_id)?.name}
            </p>
            {a.tracking_number && <p>物流：{a.tracking_number}</p>}
            {a.replacement_of && (
              <p className="break-all">原机销售记录：{a.replacement_of}</p>
            )}
          </div>
        )
      })}
      {data?.assignments.length === 0 && (
        <p className="text-sm text-ui-fg-subtle">尚未绑定具体设备。</p>
      )}
    </Container>
  )
}
export default OrderDevices
export const config = defineWidgetConfig({ zone: "order.details.after" })
