import { useEffect, useId, useState, type ReactNode } from "react"
import { useDeviceTranslations } from "../lib/use-device-translations"
import { useQuery } from "@tanstack/react-query"
import { Button, Label, Select, Text } from "@medusajs/ui"
import DeviceIdentityInput from "./device-identity-input"
import DeviceIntakeShortcut from "./device-intake-shortcut"
import type {
  OrderDeviceData,
  RunOrderDeviceCommand,
} from "../lib/order-devices"
import { sdk } from "../lib/sdk"
import { orderDeviceCapacity } from "../../lib/order-device-capacity"

function Field({
  label,
  children,
}: {
  label: string
  children: (id: string) => ReactNode
}) {
  const id = useId()
  return (
    <div className="space-y-1">
      <Label size="small" htmlFor={id}>
        {label}
      </Label>
      {children(id)}
    </div>
  )
}

export default function OrderDeviceAllocation({
  orderId,
  data,
  busy,
  run,
  fulfillmentId,
  selectionMode = false,
}: {
  orderId: string
  data: OrderDeviceData
  busy: boolean
  run: RunOrderDeviceCommand
  fulfillmentId?: string
  selectionMode?: boolean
}) {
  const { t } = useDeviceTranslations()
  const items =
    data.order?.items.filter((item) => Boolean(item.variant_id)) || []
  const [itemId, setItemId] = useState(items.length === 1 ? items[0].id : "")
  const [search, setSearch] = useState("")
  const [query, setQuery] = useState("")
  const [offset, setOffset] = useState(0)
  const fulfillmentLocation = data.order?.fulfillments?.find((f) => f.id === fulfillmentId)?.location_id
  const [warehouse, setWarehouse] = useState("all")
  const item = items.find((i) => i.id === itemId)
  const capacity = item ? orderDeviceCapacity(item, data.assignments, data.order?.fulfillments) : null
  const remaining = item
    ? fulfillmentId ? capacity?.slots.find((f) => f.id === fulfillmentId)?.remaining || 0 : capacity?.remaining || 0
    : 0
  useEffect(() => {
    const timeout = setTimeout(() => {
      setQuery(search.trim())
      setOffset(0)
    }, 250)
    return () => clearTimeout(timeout)
  }, [search])
  const candidates = useQuery<OrderDeviceData>({
    queryKey: [
      "order-device-candidates",
      orderId,
      item?.variant_id,
      query,
      offset,
      fulfillmentId,
      warehouse,
    ],
    queryFn: () =>
      sdk.client.fetch("/admin/devices", {
        query: {
          order_id: orderId,
          available_variant_id: item!.variant_id,
          category: "sale",
          ...(warehouse !== "all" ? { lookup_location_id: warehouse } : {}),
          q: query,
          offset,
        },
      }),
    enabled: Boolean(item),
  })
  const searching = search.trim() !== query
  const stock = candidates.data?.allocation_stock
  const selectedIds = new Set(data.assignments.filter((a) => ["reserved", "packed", "shipped"].includes(a.status)).map((a) => a.device_id))

  return (
    <fieldset
      disabled={busy}
      className="min-w-0 space-y-3 rounded-lg border border-ui-border-base p-3"
    >
      <Text size="small" weight="plus">
        {t("orderDevices.allocate")}
      </Text>
      {!items.length ? (
        <Text size="small" className="text-ui-fg-subtle">
          {t("orderDevices.noItems")}
        </Text>
      ) : (
        <>
          <Field label={t("deviceScan.orderItem")}>
            {(id) => (
              <Select
                value={itemId}
                onValueChange={(value) => {
                  setItemId(value)
                  setWarehouse("all")
                  setOffset(0)
                }}
                disabled={busy}
              >
                <Select.Trigger id={id}>
                  <Select.Value placeholder={t("deviceScan.select")} />
                </Select.Trigger>
                <Select.Content>
                  {items.map((i) => (
                    <Select.Item key={i.id} value={i.id}>
                      {i.title} · {i.variant_title}
                    </Select.Item>
                  ))}
                </Select.Content>
              </Select>
            )}
          </Field>
          {item && (
            <>
              <Text size="small" className="text-ui-fg-subtle">
                {t("orderDevices.remaining", { count: remaining })}
              </Text>
              <DeviceIntakeShortcut orderId={orderId} busy={busy} />
              {fulfillmentId && <Text size="small">{t("orderDevices.fulfillmentWarehouseHelp")}</Text>}
              {stock && <div className="space-y-2">
                <Text size="small" weight="plus">{t("orderDevices.availableStock", { count: stock.reduce((total, location) => total + location.count, 0) })}</Text>
                {fulfillmentLocation && <Text size="small">{t("orderDevices.currentWarehouse", { name: stock.find((location) => location.location_id === fulfillmentLocation)?.name || fulfillmentLocation })}</Text>}
                <Field label={t("orderDevices.stockWarehouse")}>
                  {(id) => <Select value={warehouse} disabled={busy} onValueChange={(value) => { setWarehouse(value); setOffset(0) }}>
                    <Select.Trigger id={id}><Select.Value /></Select.Trigger>
                    <Select.Content>
                      <Select.Item value="all">{t("orderDevices.allWarehouses", { count: stock.reduce((total, location) => total + location.count, 0) })}</Select.Item>
                      {stock.map((location) => <Select.Item key={location.location_id} value={location.location_id}>{t("orderDevices.warehouseStock", { name: location.name, count: location.count })}</Select.Item>)}
                    </Select.Content>
                  </Select>}
                </Field>
              </div>}
              <Field label={t("deviceScan.search")}>
                {(id) => (
                  <DeviceIdentityInput
                    id={id}
                    scanLabel={t("deviceScan.search")}
                    scanTarget="any"
                    onScan={setSearch}
                    disabled={busy}
                    value={search}
                    onChange={(e) => setSearch(e.target.value)}
                    placeholder={t("deviceScan.searchPlaceholder")}
                  />
                )}
              </Field>
              {(candidates.isPending || searching) && (
                <Text size="small" role="status">
                  {t("deviceScan.loading")}
                </Text>
              )}
              {candidates.isError && (
                <div className="space-y-2">
                  <Text size="small" role="alert" className="text-ui-fg-error">
                    {t("deviceScan.deviceLoadError")}
                  </Text>
                  <Button
                    size="small"
                    variant="secondary"
                    onClick={() => candidates.refetch()}
                  >
                    {t("orderDevices.retry")}
                  </Button>
                </div>
              )}
              {!searching && !candidates.isError && candidates.data && <Text size="small" role="status">{t("orderDevices.matchingDevices", { count: candidates.data.count })}</Text>}
              {!searching &&
                !candidates.isError &&
                candidates.data?.devices.length === 0 && (
                  <Text size="small">{t("deviceScan.noDevices")}</Text>
                )}
              {!searching &&
                !candidates.isError &&
                candidates.data?.devices.map((device) => (
                  <div
                    key={device.id}
                    className="flex flex-wrap items-center justify-between gap-3 border-t border-ui-border-base py-3"
                  >
                    <div className="min-w-0 space-y-1">
                      <Text size="small" className="break-all" weight="plus">
                        {device.serial_number ||
                          t("orderDevices.serialMissing")}
                      </Text>
                      {device.imei && (
                        <Text size="small" className="break-all">
                          {t("orderDevices.imei")}: {device.imei}
                        </Text>
                      )}
                      {device.imei2 && (
                        <Text size="small" className="break-all">
                          {t("orderDevices.imei2")}: {device.imei2}
                        </Text>
                      )}
                      {device.eid && (
                        <Text size="small" className="break-all">
                          EID: {device.eid}
                        </Text>
                      )}
                      <Text size="small">{t("orderDevices.deviceWarehouse", { name: stock?.find((location) => location.location_id === device.location_id)?.name || device.location_id })}</Text>
                      <Text size="small" className="text-ui-fg-subtle">
                        {
                          data.sources.find(
                            (source) => source.id === device.source_id,
                          )?.name
                        }{" "}
                        · {device.grade || t("orderDevices.notRecorded")} ·{" "}
                        {device.battery_health == null
                          ? t("orderDevices.notRecorded")
                          : device.battery_health + "%"}{" "}
                        {device.bin}
                      </Text>
                    </div>
                    <Button
                      size="small"
                      variant="secondary"
                      disabled={
                        busy ||
                        selectedIds.has(device.id) ||
                        remaining === 0 ||
                        candidates.isFetching ||
                        !device.serial_number ||
                        (item.requires_imei &&
                          !device.imei &&
                          !device.imei2 &&
                          !device.eid)
                      }
                      onClick={() => {
                        void run({
                          action: "assign",
                          order_id: orderId,
                          item_id: itemId,
                          device_id: device.id,
                        })
                      }}
                    >
                      {t(selectedIds.has(device.id) ? "orderDevices.deviceSelected" : selectionMode ? "orderDevices.selectDevice" : "deviceScan.allocate")}
                    </Button>
                  </div>
                ))}
              {candidates.data && candidates.data.count > 50 && (
                <div className="flex gap-2">
                  <Button
                    size="small"
                    variant="secondary"
                    disabled={
                      busy || candidates.isFetching || searching || offset === 0
                    }
                    onClick={() => setOffset(offset - 50)}
                  >
                    {t("deviceScan.previous")}
                  </Button>
                  <Button
                    size="small"
                    variant="secondary"
                    disabled={
                      busy ||
                      candidates.isFetching ||
                      searching ||
                      offset + 50 >= candidates.data.count
                    }
                    onClick={() => setOffset(offset + 50)}
                  >
                    {t("deviceScan.next")}
                  </Button>
                </div>
              )}
            </>
          )}
        </>
      )}
    </fieldset>
  )
}
