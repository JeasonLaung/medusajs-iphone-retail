import { useState } from "react"
import { useTranslation } from "react-i18next"
import { defineWidgetConfig } from "@medusajs/admin-sdk"
import type { AdminOrder, DetailWidgetProps } from "@medusajs/framework/types"
import {
  Button,
  Checkbox,
  Container,
  Input,
  Label,
  Text,
  toast,
} from "@medusajs/ui"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import OrderDeviceAllocation from "../components/order-device-allocation"
import { localizeDeviceMessage } from "../lib/device-messages"
import type {
  OrderDeviceCommand,
  OrderDeviceData,
  RunOrderDeviceCommand,
} from "../lib/order-devices"
import { sdk } from "../lib/sdk"

function Shipment({
  id,
  busy,
  run,
}: {
  id: string
  busy: boolean
  run: RunOrderDeviceCommand
}) {
  const { t } = useTranslation()
  const [tracking, setTracking] = useState("")
  const [url, setUrl] = useState("")
  return (
    <form
      className="space-y-3 border-t border-ui-border-base pt-3"
      onSubmit={async (event) => {
        event.preventDefault()
        await run({
          action: "ship",
          fulfillment_id: id,
          tracking_number: tracking.trim(),
          ...(url.trim() ? { tracking_url: url.trim() } : {}),
        })
      }}
    >
      <Text size="small" className="break-all text-ui-fg-subtle">
        {t("deviceScan.fulfillmentPrefix")}
        {id}
      </Text>
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="space-y-1">
          <Label size="small" htmlFor={`tracking-${id}`}>
            {t("deviceScan.tracking")}
          </Label>
          <Input
            id={`tracking-${id}`}
            required
            maxLength={200}
            value={tracking}
            disabled={busy}
            onChange={(e) => setTracking(e.target.value)}
          />
        </div>
        <div className="space-y-1">
          <Label size="small" htmlFor={`url-${id}`}>
            {t("deviceScan.trackingUrl")}
          </Label>
          <Input
            id={`url-${id}`}
            type="url"
            pattern="https?://.*"
            value={url}
            disabled={busy}
            onChange={(e) => setUrl(e.target.value)}
          />
        </div>
      </div>
      <div className="flex flex-wrap gap-2">
        <Button
          size="small"
          type="submit"
          disabled={busy || !tracking.trim()}
          isLoading={busy}
        >
          {t("orderDevices.ship")}
        </Button>
        <Button
          size="small"
          type="button"
          variant="secondary"
          disabled={busy}
          onClick={() =>
            run({ action: "cancel_fulfillment", fulfillment_id: id })
          }
        >
          {t("deviceScan.cancelFulfillment")}
        </Button>
      </div>
    </form>
  )
}

const OrderDevices = ({ data: order }: DetailWidgetProps<AdminOrder>) => {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [allocating, setAllocating] = useState(false)
  const [checked, setChecked] = useState<string[]>([])
  const [error, setError] = useState("")
  const key = ["order-devices", order.id]
  const { data, isPending, isError, refetch } = useQuery<OrderDeviceData>({
    queryKey: key,
    queryFn: () =>
      sdk.client.fetch("/admin/devices", { query: { order_id: order.id } }),
  })
  const mutation = useMutation({
    mutationFn: (body: OrderDeviceCommand) =>
      sdk.client.fetch("/admin/devices", { method: "POST", body }),
    onSuccess: async (_result, body) => {
      setChecked([])
      const messages = {
        assign: "allocated",
        order_assign: "allocated",
        supplier_assign: "supplierAllocated",
        release: "released",
        fulfill: "fulfillmentCreated",
        ship: "shipSuccess",
        cancel_fulfillment: "cancelSuccess",
      }
      toast.success(t(`deviceScan.${messages[body.action]}`))
      await Promise.all([
        client.invalidateQueries({ queryKey: key }),
        client.invalidateQueries({
          queryKey: ["order-device-candidates", order.id],
        }),
        client.invalidateQueries({ queryKey: ["orders"] }),
      ])
    },
  })
  const run: RunOrderDeviceCommand = async (body) => {
    if (mutation.isPending) return false
    setError("")
    try {
      await mutation.mutateAsync(body)
      return true
    } catch (cause) {
      setError(
        localizeDeviceMessage(
          cause instanceof Error ? cause.message : t("deviceScan.commandError"),
          t,
        ),
      )
      return false
    }
  }
  const editable = Boolean(
    data?.order && !["canceled", "draft"].includes(data.order.status),
  )
  const busy = mutation.isPending
  const selected =
    data?.assignments.filter(
      (a) => a.status === "reserved" && checked.includes(a.id),
    ) || []
  const sameSource = new Set(selected.map((a) => a.source_id)).size <= 1
  const pendingFulfillments = [
    ...new Set(
      data?.assignments
        .filter((a) => a.status === "packed" && a.fulfillment_id)
        .map((a) => a.fulfillment_id!) || [],
    ),
  ]

  return (
    <Container
      className="divide-y divide-ui-border-base p-0"
      data-testid="order-devices"
    >
      <div className="flex flex-wrap items-center justify-between gap-3 px-6 py-4">
        <div className="space-y-1">
          <Text size="small" leading="compact" weight="plus">
            {t("deviceScan.deviceWidgetTitle")}
          </Text>
          <Text size="small" className="text-ui-fg-subtle">
            {t("orderDevices.help")}
          </Text>
        </div>
        {data && (
          <Button
            size="small"
            variant="secondary"
            disabled={busy || !editable}
            onClick={() => setAllocating(!allocating)}
          >
            {t(
              allocating
                ? "orderDevices.closeAllocation"
                : "orderDevices.allocate",
            )}
          </Button>
        )}
      </div>
      <div className="space-y-3 px-6 py-4">
        {isPending && (
          <Text size="small" role="status">
            {t("deviceScan.loading")}
          </Text>
        )}
        {isError && (
          <div className="space-y-2">
            <Text size="small" role="alert" className="text-ui-fg-error">
              {t("deviceScan.deviceLoadError")}
            </Text>
            <Button size="small" variant="secondary" onClick={() => refetch()}>
              {t("orderDevices.retry")}
            </Button>
          </div>
        )}
        {error && (
          <Text size="small" role="alert" className="text-ui-fg-error">
            {error}
          </Text>
        )}
        {data && !isError && (
          <>
            {allocating && (
              <OrderDeviceAllocation
                key={order.id}
                orderId={order.id}
                data={data}
                busy={busy || !editable}
                run={run}
              />
            )}
            {data.assignments.length === 0 && (
              <Text size="small" className="text-ui-fg-subtle">
                {t("deviceScan.noAssignedDevices")}
              </Text>
            )}
            {data.assignments.map((a) => {
              const device = data.assigned_devices.find(
                (d) => d.id === a.device_id,
              )
              const item = data.order?.items.find((i) => i.id === a.item_id)
              return (
                <div
                  key={a.id}
                  className="space-y-2 rounded-lg border border-ui-border-base p-3"
                >
                  <Text size="small" weight="plus" className="break-all">
                    {t("orderDevices.serial")}:{" "}
                    {device?.serial_number || t("orderDevices.notRecorded")}
                  </Text>
                  {device?.imei && (
                    <Text size="small" className="break-all">
                      {t("orderDevices.imei")}: {device.imei}
                    </Text>
                  )}
                  {device?.imei2 && <Text size="small" className="break-all">{t("orderDevices.imei2")}: {device.imei2}</Text>}
                  <Text size="small" className="text-ui-fg-subtle">
                    {item?.title} · {item?.variant_title}
                  </Text>
                  <Text size="small">
                    {t(`deviceScan.${a.status}`, { defaultValue: a.status })} ·{" "}
                    {data.sources.find((s) => s.id === a.source_id)?.name}
                  </Text>
                  {a.tracking_number && (
                    <Text size="small" className="break-all">
                      {t("deviceScan.trackingPrefix")}
                      {a.tracking_number}
                    </Text>
                  )}
                  {a.replacement_of && (
                    <Text size="small" className="break-all">
                      {t("deviceScan.originalSalePrefix")}
                      {a.replacement_of}
                    </Text>
                  )}
                  {a.status === "reserved" && editable && (
                    <div className="flex flex-wrap items-center gap-3">
                      <div className="flex items-center gap-2">
                        <Checkbox
                          id={`assignment-${a.id}`}
                          disabled={
                            busy ||
                            (!checked.includes(a.id) &&
                              selected.length > 0 &&
                              selected[0].source_id !== a.source_id)
                          }
                          checked={checked.includes(a.id)}
                          onCheckedChange={(value) =>
                            setChecked((ids) =>
                              value === true
                                ? [...ids, a.id]
                                : ids.filter((id) => id !== a.id),
                            )
                          }
                        />
                        <Label size="small" htmlFor={`assignment-${a.id}`}>
                          {t("deviceScan.selectFulfillment")}
                        </Label>
                      </div>
                      <Button
                        size="small"
                        variant="secondary"
                        disabled={busy}
                        onClick={() =>
                          run({ action: "release", assignment_id: a.id })
                        }
                      >
                        {t("deviceScan.release")}
                      </Button>
                    </div>
                  )}
                </div>
              )
            })}
            {data.assignments.some((a) => a.status === "reserved") &&
              editable && (
                <div className="space-y-2">
                  <Text size="small" className="text-ui-fg-subtle">
                    {t("orderDevices.splitHelp")}
                  </Text>
                  <Button
                    size="small"
                    disabled={busy || !selected.length || !sameSource}
                    isLoading={busy}
                    onClick={() =>
                      run({
                        action: "fulfill",
                        assignment_ids: selected.map((a) => a.id),
                      })
                    }
                  >
                    {t("deviceScan.createFulfillment", {
                      count: selected.length,
                    })}
                  </Button>
                </div>
              )}
            {editable &&
              pendingFulfillments.map((id) => (
                <Shipment key={id} id={id} busy={busy} run={run} />
              ))}
          </>
        )}
      </div>
    </Container>
  )
}

export default OrderDevices
export const config = defineWidgetConfig({ zone: "order.details.after" })
