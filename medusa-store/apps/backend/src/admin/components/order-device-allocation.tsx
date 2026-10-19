import { useEffect, useId, useState, type ReactNode } from "react"
import { useTranslation } from "react-i18next"
import { useQuery } from "@tanstack/react-query"
import { Button, Input, Label, Select, Text } from "@medusajs/ui"
import DeviceIdentityInput from "./device-identity-input"
import type {
  OrderDeviceData,
  RunOrderDeviceCommand,
} from "../lib/order-devices"
import { sdk } from "../lib/sdk"
import { orderItemQuantities } from "../../lib/order-item-quantities"
import { isImeiOrEid } from "../../lib/device-identity"
import { mergeDeviceScans } from "../lib/device-scan-values"
import { notifyError } from "../lib/notifications"

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
}: {
  orderId: string
  data: OrderDeviceData
  busy: boolean
  run: RunOrderDeviceCommand
}) {
  const { t } = useTranslation()
  const items =
    data.order?.items.filter((item) => Boolean(item.variant_id)) || []
  const [itemId, setItemId] = useState(items.length === 1 ? items[0].id : "")
  const [mode, setMode] = useState("identifier")
  const [serial, setSerial] = useState("")
  const [imei, setImei] = useState("")
  const [imei2, setImei2] = useState("")
  const onScanMany = (values: string[]) => {
    const next = mergeDeviceScans({ serial, imei, imei2 }, values)
    setSerial(next.serial)
    setImei(next.imei)
    setImei2(next.imei2)
    if (next.conflict) notifyError(t("deviceScan.scannerConflict"))
  }
  const [deviceId, setDeviceId] = useState("")
  const [sourceId, setSourceId] = useState("auto")
  const [search, setSearch] = useState("")
  const [query, setQuery] = useState("")
  const [offset, setOffset] = useState(0)
  const item = items.find((i) => i.id === itemId)
  const reserved = data.assignments.filter(
    (a) => a.item_id === itemId && a.status === "reserved",
  ).length
  const quantities = item
    ? orderItemQuantities(item)
    : { quantity: 0, fulfilled_quantity: 0 }
  const remaining = item
    ? Math.max(
        0,
        quantities.quantity - quantities.fulfilled_quantity - reserved,
      )
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
    ],
    queryFn: () =>
      sdk.client.fetch("/admin/devices", {
        query: {
          order_id: orderId,
          available_variant_id: item!.variant_id,
          q: query,
          offset,
        },
      }),
    enabled: Boolean(item && mode === "stock"),
  })
  const searching = search.trim() !== query

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
                  setOffset(0)
                  setSerial("")
                  setImei("")
                  setDeviceId("")
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
              <Field label={t("orderDevices.allocationMode")}>
                {(id) => (
                  <Select value={mode} onValueChange={setMode} disabled={busy}>
                    <Select.Trigger id={id}>
                      <Select.Value />
                    </Select.Trigger>
                    <Select.Content>
                      <Select.Item value="identifier">
                        {t("orderDevices.scanOrEnter")}
                      </Select.Item>
                      <Select.Item value="stock">
                        {t("orderDevices.existingDevice")}
                      </Select.Item>
                    </Select.Content>
                  </Select>
                )}
              </Field>
              {mode === "identifier" ? (
                <>
                  <form
                    className="space-y-3"
                    onSubmit={async (event) => {
                      event.preventDefault()
                      if (
                        await run({
                          action: "order_assign",
                          order_id: orderId,
                          item_id: itemId,
                          serial_number: serial.trim().toUpperCase(),
                          ...(imei.trim() ? { imei: imei.trim() } : {}),
                          ...(imei2.trim() ? { imei2: imei2.trim() } : {}),
                          ...(deviceId ? { device_id: deviceId } : {}),
                          ...(sourceId !== "auto"
                            ? { source_id: sourceId }
                            : {}),
                        })
                      ) {
                        setSerial("")
                        setImei("")
                        setImei2("")
                        setDeviceId("")
                      }
                    }}
                  >
                    <Field label={t("orderDevices.serial")}>
                      {(id) => (
                        <DeviceIdentityInput
                          id={id}
                          scanLabel={t("orderDevices.serial")}
                          onScan={setSerial}
                          onScanMany={onScanMany}
                          disabled={busy}
                          autoComplete="off"
                          required
                          maxLength={100}
                          value={serial}
                          onChange={(e) => setSerial(e.target.value)}
                        />
                      )}
                    </Field>
                    {[{ value: imei, set: setImei, index: 1 }, { value: imei2, set: setImei2, index: 2 }].map((field) => (
                    <Field key={field.index} label={t(field.index === 1 ? "orderDevices.imei" : "orderDevices.imei2")}>
                      {(id) => (
                        <DeviceIdentityInput
                          id={id}
                          scanLabel={t(field.index === 1 ? "orderDevices.imei" : "orderDevices.imei2")}
                          onScan={field.set}
                          onScanMany={onScanMany}
                          disabled={busy}
                          autoComplete="off"
                          inputMode="numeric"
                          pattern="([0-9]{15}|[0-9]{32})"
                          maxLength={32}
                          value={field.value}
                          onChange={(e) => field.set(e.target.value)}
                        />
                      )}
                    </Field>
                    ))}
                    {item.requires_imei && (
                      <Text size="small" className="text-ui-fg-subtle">
                        {t("orderDevices.iphoneIdentityHelp")}
                      </Text>
                    )}
                    {data.sources.length > 0 && (
                      <Field label={t("orderDevices.source")}>
                        {(id) => (
                          <Select
                            value={sourceId}
                            onValueChange={setSourceId}
                            disabled={busy}
                          >
                            <Select.Trigger id={id}>
                              <Select.Value />
                            </Select.Trigger>
                            <Select.Content>
                              <Select.Item value="auto">
                                {t("orderDevices.autoSource")}
                              </Select.Item>
                              {data.sources.map((source) => (
                                <Select.Item key={source.id} value={source.id}>
                                  {source.name}
                                </Select.Item>
                              ))}
                            </Select.Content>
                          </Select>
                        )}
                      </Field>
                    )}
                    <Text size="small" className="text-ui-fg-subtle">
                      {t("orderDevices.identifierHelp")}
                    </Text>
                    <Button
                      size="small"
                      type="submit"
                      disabled={
                        busy ||
                        remaining === 0 ||
                        !serial.trim() ||
                        (item.requires_imei && !imei.trim() && !imei2.trim()) ||
                        (!!imei.trim() && !isImeiOrEid(imei)) ||
                        (!!imei2.trim() && !isImeiOrEid(imei2)) ||
                        (!!imei.trim() && imei.trim() === imei2.trim())
                      }
                      isLoading={busy}
                    >
                      {t("deviceScan.allocate")}
                    </Button>
                  </form>
                </>
              ) : (
                <>
                  <Field label={t("deviceScan.search")}>
                    {(id) => (
                      <Input
                        id={id}
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
                      <Text
                        size="small"
                        role="alert"
                        className="text-ui-fg-error"
                      >
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
                          <Text
                            size="small"
                            className="break-all"
                            weight="plus"
                          >
                            {device.serial_number ||
                              t("orderDevices.serialMissing")}
                          </Text>
                          {device.imei && (
                            <Text size="small" className="break-all">
                              {t("orderDevices.imei")}: {device.imei}
                            </Text>
                          )}
                          {device.imei2 && <Text size="small" className="break-all">{t("orderDevices.imei2")}: {device.imei2}</Text>}
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
                            busy || remaining === 0 || candidates.isFetching
                          }
                          onClick={() => {
                            if (
                              !device.serial_number ||
                              (item.requires_imei && !device.imei && !device.imei2)
                            ) {
                              setSerial(device.serial_number || "")
                              setImei(device.imei || "")
                              setImei2(device.imei2 || "")
                              setDeviceId(device.id)
                              setMode("identifier")
                              return
                            }
                            void run({
                              action: "assign",
                              order_id: orderId,
                              item_id: itemId,
                              device_id: device.id,
                            })
                          }}
                        >
                          {t(
                            !device.serial_number ||
                              (item.requires_imei && !device.imei && !device.imei2)
                              ? "orderDevices.completeAndAllocate"
                              : "deviceScan.allocate",
                          )}
                        </Button>
                      </div>
                    ))}
                  {candidates.data && candidates.data.count > 50 && (
                    <div className="flex gap-2">
                      <Button
                        size="small"
                        variant="secondary"
                        disabled={
                          busy ||
                          candidates.isFetching ||
                          searching ||
                          offset === 0
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
        </>
      )}
    </fieldset>
  )
}
