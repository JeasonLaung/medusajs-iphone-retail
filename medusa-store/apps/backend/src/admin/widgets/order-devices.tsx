import { useTranslation } from "react-i18next"
import { defineWidgetConfig } from "@medusajs/admin-sdk"
import type { AdminOrder, DetailWidgetProps } from "@medusajs/framework/types"
import { Container, Heading } from "@medusajs/ui"
import { useQuery } from "@tanstack/react-query"

const OrderDevices = ({ data: order }: DetailWidgetProps<AdminOrder>) => {
  const { t } = useTranslation()
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
      if (!r.ok) throw new Error(t("deviceScan.deviceLoadError"))
      return r.json()
    },
  })
  return (
    <Container className="space-y-3">
      <Heading level="h2">{t("deviceScan.deviceWidgetTitle")}</Heading>
      <a
        className="text-sm underline"
        href={`/app/device-scan?order_id=${encodeURIComponent(order.id)}`}
      >
        {t("deviceScan.openWorkspace")}
      </a>
      {isError && <p role="alert">{t("deviceScan.deviceLoadError")}</p>}
      {data?.assignments.map((a) => {
        const d = data.assigned_devices.find((d) => d.id === a.device_id)
        return (
          <div key={a.id} className="rounded-lg border p-3 text-sm">
            <p className="break-all">
              {d?.serial_number || d?.imei || a.device_id} ·{" "}
              {t(
                `deviceScan.${a.status === "repair_ready" ? "repairReady" : a.status}`,
                { defaultValue: a.status },
              )}{" "}
              · {data.sources.find((s) => s.id === a.source_id)?.name}
            </p>
            {a.tracking_number && (
              <p>
                {t("deviceScan.trackingPrefix")}
                {a.tracking_number}
              </p>
            )}
            {a.replacement_of && (
              <p className="break-all">
                {t("deviceScan.originalSalePrefix")}
                {a.replacement_of}
              </p>
            )}
          </div>
        )
      })}
      {data?.assignments.length === 0 && (
        <p className="text-sm text-ui-fg-subtle">
          {t("deviceScan.noAssignedDevices")}
        </p>
      )}
    </Container>
  )
}
export default OrderDevices
export const config = defineWidgetConfig({ zone: "order.details.after" })
