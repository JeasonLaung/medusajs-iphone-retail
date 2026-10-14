import { useTranslation } from "react-i18next"
import { defineRouteConfig } from "@medusajs/admin-sdk"
import { Button, Container, Heading } from "@medusajs/ui"
import { useCallback, useEffect, useState } from "react"
import { useSearchParams } from "react-router-dom"
import DeviceScanner from "../../components/device-scanner"
import { parseDeviceCsv } from "../../lib/device-csv"
import { localizeDeviceMessage } from "../../lib/device-messages"

type Source = {
  id: string
  name: string
  type: "self" | "supplier"
  location_id: string
}
type Variant = {
  id: string
  sku: string
  title: string
  product: { title: string }
  options: { value: string }[]
  inventory_items: {
    inventory: {
      location_levels: {
        location_id: string
        stocked_quantity: number
        reserved_quantity: number
      }[]
    }
  }[]
}
type Device = {
  id: string
  serial_number: string | null
  imei: string | null
  variant_id: string
  source_id: string
  location_id: string
  state: string
  grade: string
  battery_health: number
  cost_cents: number
  currency_code: string
  bin: string
}
type Assignment = {
  id: string
  device_id: string
  order_id: string
  item_id: string
  source_id: string
  status: string
  fulfillment_id: string | null
  return_id: string | null
  replacement_of: string | null
  after_sales_request_id: string | null
  tracking_number: string | null
}
type Order = {
  shipping_address?: {
    first_name?: string
    last_name?: string
    company?: string
    address_1?: string
    address_2?: string
    city?: string
    province?: string
    postal_code?: string
    country_code?: string
    phone?: string
  } | null
  currency_code: string
  payment_collections: {
    payments: {
      id: string
      captures: { amount: number }[]
      refunds: { amount: number; note: string }[]
    }[]
  }[]
  id: string
  display_id: number
  status: string
  items: {
    id: string
    title: string
    variant_title: string
    variant_id: string
    detail: { quantity: number; fulfilled_quantity: number }
  }[]
}
type Request = {
  id: string
  type: string
  status: string
  items: { item_id: string; quantity: number }[]
}
type Data = {
  access: {
    role: "admin" | "supplier"
    source_id: string | null
    order_ids: string[]
  }
  devices: Device[]
  assigned_devices: Device[]
  count: number
  sources: Source[]
  variants: Variant[]
  managed_variants: { variant_id: string }[]
  supplies: { source_id: string; variant_id: string; quantity: number }[]
  assignments: Assignment[]
  order: Order | null
  events: {
    id: string
    action: string
    actor_id: string
    created_at: string
    data: Record<string, unknown>
  }[]
}
const box = "w-full min-w-0 rounded-md border bg-ui-bg-base px-3 py-2 text-sm"
function Field({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <label className="block min-w-0 space-y-1 text-sm">
      <span>{label}</span>
      {children}
    </label>
  )
}

const DeviceScanPage = () => {
  const { t, i18n } = useTranslation()
  const states: Record<string, string> = {
    inspection: t("deviceScan.inspection"),
    available: t("deviceScan.available"),
    reserved: t("deviceScan.reserved"),
    packed: t("deviceScan.packed"),
    shipped: t("deviceScan.shipped"),
    repair: t("deviceScan.repair"),
    repair_ready: t("deviceScan.repairReady"),
    retired: t("deviceScan.retired"),
    supplier_returned: t("deviceScan.supplierReturned"),
    returned: t("deviceScan.returned"),
    canceled: t("deviceScan.canceled"),
  }
  const modes = {
    lookup: t("deviceScan.lookup"),
    intake: t("deviceScan.intake"),
    orders: t("deviceScan.orders"),
    inspection: t("deviceScan.afterSales"),
    sources: t("deviceScan.sources"),
  }

  const [params] = useSearchParams()
  const [mode, setMode] = useState<keyof typeof modes>(
    params.get("order_id") ? "orders" : "lookup",
  )
  const [data, setData] = useState<Data | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const [success, setSuccess] = useState("")
  const [scan, setScan] = useState("")
  const [search, setSearch] = useState("")
  const [offset, setOffset] = useState(0)
  const [selected, setSelected] = useState<Device | null>(null)
  const [orderInput, setOrderInput] = useState(params.get("order_id") || "")
  const [orderId, setOrderId] = useState(params.get("order_id") || "")
  const [requests, setRequests] = useState<Request[]>([])
  const [sourceId, setSourceId] = useState("")
  const [variantId, setVariantId] = useState("")
  const [serial, setSerial] = useState("")
  const [imei, setImei] = useState("")
  const [grade, setGrade] = useState("A")
  const [battery, setBattery] = useState("85")
  const [cost, setCost] = useState("0")
  const [bin, setBin] = useState("")
  const [notes, setNotes] = useState("")
  const [itemId, setItemId] = useState("")
  const [replacement, setReplacement] = useState("")
  const [assignmentId, setAssignmentId] = useState("")
  const [requestId, setRequestId] = useState("")
  const [tracking, setTracking] = useState("")
  const [trackingUrl, setTrackingUrl] = useState("")
  const [result, setResult] = useState("pass")
  const [sourceName, setSourceName] = useState("")
  const [sourceType, setSourceType] = useState("self")
  const [quantity, setQuantity] = useState("0")
  const [acknowledged, setAcknowledged] = useState(false)
  const [batch, setBatch] = useState<Record<string, unknown>[]>([])
  const [checked, setChecked] = useState<string[]>([])
  const [paymentId, setPaymentId] = useState("")
  const [refundAmount, setRefundAmount] = useState("")
  const [refundConfirmed, setRefundConfirmed] = useState(false)
  const [supplierEmail, setSupplierEmail] = useState("")
  const [supplierPassword, setSupplierPassword] = useState("")
  const [supplierOrders, setSupplierOrders] = useState("")
  const [supplierAccounts, setSupplierAccounts] = useState<
    { email: string; source_id: string; order_ids: string[] }[]
  >([])
  const supplier = data?.access?.role === "supplier"

  const load = useCallback(async () => {
    const query = new URLSearchParams({ offset: String(offset) })
    if (mode === "lookup" && selected) query.set("device_id", selected.id)
    else if (mode === "orders" || mode === "inspection") {
      if (orderId) query.set("order_id", orderId)
      if (search) query.set("q", search)
    } else if (search) query.set("q", search)
    const response = await fetch(`/admin/devices?${query}`, {
      credentials: "include",
    })
    if (!response.ok) throw new Error(t("deviceScan.loadError"))
    const next: Data = await response.json()
    setData(next)
    if (next.access?.role === "supplier")
      setSourceId(next.access.source_id || "")
    if (mode === "sources" && next.access?.role !== "supplier") {
      const accounts = await fetch("/admin/devices/supplier-accounts", {
        credentials: "include",
      })
      if (!accounts.ok) throw new Error(t("deviceScan.loadError"))
      setSupplierAccounts((await accounts.json()).accounts)
    }
    if (selected)
      setSelected(next.devices.find((d) => d.id === selected.id) || null)
    if (next.access?.role === "supplier") setRequests([])
    if (
      next.access?.role !== "supplier" &&
      orderId &&
      (mode === "orders" || mode === "inspection")
    ) {
      const r = await fetch(
        `/admin/orders/${encodeURIComponent(orderId)}/after-sales`,
        { credentials: "include" },
      )
      if (!r.ok) throw new Error(t("deviceScan.requestLoadError"))
      setRequests((await r.json()).requests)
    }
  }, [mode, search, orderId, offset, selected?.id, t])
  useEffect(() => {
    setError("")
    load().catch((e) => setError(e.message))
  }, [load])
  async function command(
    body: Record<string, unknown>,
    message: string,
    path = "/admin/devices",
  ) {
    setBusy(true)
    setError("")
    setSuccess("")
    try {
      const r = await fetch(path, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      })
      const json = await r.json()
      if (!r.ok) throw new Error(json.message || t("deviceScan.commandError"))
      setSuccess(message)
      setChecked([])
      setRefundConfirmed(false)
      await load()
      return true
    } catch (e) {
      setError((e as Error).message)
      return false
    } finally {
      setBusy(false)
    }
  }
  function onScan(value: string) {
    const normalized = value.trim().toUpperCase()
    setScan(normalized)
    if (
      mode === "intake" ||
      (mode === "orders" &&
        data?.sources.find((s) => s.id === sourceId)?.type === "supplier")
    ) {
      if (/^\d{15}$/.test(normalized)) setImei(normalized)
      else setSerial(normalized)
    } else {
      setSearch(normalized)
      setSelected(null)
      setOffset(0)
      const d = data?.devices.find(
        (d) => d.serial_number === normalized || d.imei === normalized,
      )
      if (d) choose(d)
    }
  }
  function choose(d: Device) {
    setSelected(d)
    setGrade(d.grade)
    setBattery(String(d.battery_health))
    setVariantId(d.variant_id)
    setNotes("")
    const a = data?.assignments.find(
      (a) => a.device_id === d.id && a.status === "shipped",
    )
    if (a) {
      setAssignmentId(a.id)
      setOrderId(a.order_id)
      setOrderInput(a.order_id)
    }
  }
  const source = data?.sources.find((s) => s.id === sourceId)
  const variant = data?.variants.find((v) => v.id === variantId)
  const managed = data?.managed_variants.some((v) => v.variant_id === variantId)
  const findDevice = (id?: string) =>
    data?.devices.find((d) => d.id === id) ||
    data?.assigned_devices.find((d) => d.id === id)
  const identity = (d?: Device) =>
    d?.serial_number || d?.imei || d?.id || t("deviceScan.deviceNotFound")
  const variantLabel = (v: Variant) =>
    `${v.product?.title || ""} · ${v.options?.map((o) => o.value).join(" / ") || v.title} · ${v.sku || v.id}`
  const sourceSelect = (
    <Field label={t("deviceScan.sourceLabel")}>
      <select
        aria-label={t("deviceScan.sourceLabel")}
        className={box}
        value={sourceId}
        onChange={(e) => {
          setSourceId(e.target.value)
          setBatch([])
        }}
      >
        <option value="">{t("deviceScan.select")}</option>
        {data?.sources.map((s) => (
          <option key={s.id} value={s.id}>
            {s.name}（
            {s.type === "self"
              ? t("deviceScan.ownWarehouse")
              : t("deviceScan.supplier")}
            ）
          </option>
        ))}
      </select>
    </Field>
  )
  const variantSelect = (
    <Field label={t("deviceScan.sku")}>
      <select
        aria-label={t("deviceScan.sku")}
        className={box}
        value={variantId}
        onChange={(e) => {
          setVariantId(e.target.value)
          setAcknowledged(false)
        }}
      >
        <option value="">{t("deviceScan.select")}</option>
        {data?.variants.map((v) => (
          <option key={v.id} value={v.id}>
            {variantLabel(v)}
          </option>
        ))}
      </select>
    </Field>
  )
  const deviceFields = (
    <div className="grid gap-3 md:grid-cols-2">
      <Field label={t("deviceScan.serial")}>
        <input
          aria-label={t("deviceScan.serial")}
          className={box}
          value={serial}
          onChange={(e) => setSerial(e.target.value.toUpperCase())}
          autoComplete="off"
        />
      </Field>
      <Field label={t("deviceScan.imei")}>
        <input
          aria-label="IMEI"
          className={box}
          value={imei}
          onChange={(e) => setImei(e.target.value)}
          inputMode="numeric"
        />
      </Field>
      <Field label={t("deviceScan.grade")}>
        <select
          aria-label={t("deviceScan.grade")}
          className={box}
          value={grade}
          onChange={(e) => setGrade(e.target.value)}
        >
          {["S", "A", "B", "C"].map((g) => (
            <option key={g}>{g}</option>
          ))}
        </select>
      </Field>
      <Field label={t("deviceScan.batteryPercent")}>
        <input
          aria-label={t("deviceScan.battery")}
          className={box}
          type="number"
          min="0"
          max="100"
          value={battery}
          onChange={(e) => setBattery(e.target.value)}
        />
      </Field>
      <Field label={t("deviceScan.costEur")}>
        <input
          aria-label={t("deviceScan.cost")}
          className={box}
          type="number"
          min="0"
          step="0.01"
          value={cost}
          onChange={(e) => setCost(e.target.value)}
        />
      </Field>
      <Field label={t("deviceScan.bin")}>
        <input
          aria-label={t("deviceScan.bin")}
          className={box}
          value={bin}
          onChange={(e) => setBin(e.target.value)}
        />
      </Field>
    </div>
  )
  const newDevice = () => ({
    ...(serial.trim() ? { serial_number: serial.trim() } : {}),
    ...(imei.trim() ? { imei: imei.trim() } : {}),
    source_id: sourceId,
    variant_id: variantId,
    grade,
    battery_health: Number(battery),
    cost_cents: Math.round(Number(cost) * 100),
    bin,
    notes,
  })
  async function importCsv(file?: File) {
    if (!file) return
    setError("")
    setBatch([])
    try {
      if (file.size > 1024 * 1024) throw new Error(t("deviceScan.csvSize"))
      const rows = parseDeviceCsv(await file.text(), t)
      const devices = rows.map((r, i) => {
        const v =
          r.variant_id ||
          data?.variants.find((v) => v.sku === r.sku)?.id ||
          variantId
        if (!v) throw new Error(t("deviceScan.csvSku", { row: i + 2 }))
        return {
          ...(r.serial_number
            ? { serial_number: r.serial_number.toUpperCase() }
            : {}),
          ...(r.imei ? { imei: r.imei } : {}),
          variant_id: v,
          source_id: sourceId,
          grade: r.grade || grade,
          battery_health: Number(r.battery_health || battery),
          cost_cents: Math.round(Number(r.cost_eur || "0") * 100),
          currency_code: r.currency_code || "eur",
          bin: r.bin || "",
          notes: r.notes || "",
        }
      })
      setBatch(devices)
    } catch (e) {
      setError((e as Error).message)
    }
  }

  return (
    <Container className="space-y-5" data-testid="device-scan-page">
      <div>
        <Heading level="h1">{t("deviceScan.title")}</Heading>
        <p className="mt-1 text-sm text-ui-fg-subtle">
          {t("deviceScan.subtitle")}
        </p>
      </div>
      <div className="flex flex-wrap gap-2">
        {Object.entries(modes)
          .filter(
            ([key]) => !supplier || !["intake", "inspection"].includes(key),
          )
          .map(([key, title]) => (
            <Button
              key={key}
              variant={mode === key ? "primary" : "secondary"}
              disabled={busy}
              onClick={() => {
                setMode(key as keyof typeof modes)
                setSelected(null)
                setSearch("")
                setOffset(0)
                setError("")
                setSuccess("")
              }}
            >
              {supplier && key === "sources"
                ? t("deviceScan.supplierStock")
                : title}
            </Button>
          ))}
      </div>
      {supplier && (
        <p className="rounded-lg border p-3 text-sm">
          {t("deviceScan.supplierScope")}
        </p>
      )}
      {error && (
        <div
          role="alert"
          className="rounded-lg bg-ui-bg-subtle p-3 text-sm text-ui-fg-error"
        >
          {localizeDeviceMessage(error, t)}
        </div>
      )}
      {success && (
        <div role="status" className="rounded-lg bg-ui-bg-subtle p-3 text-sm">
          {success}
        </div>
      )}
      {mode !== "sources" && <DeviceScanner key={mode} onScan={onScan} />}
      {!data && <p>{t("deviceScan.loading")}</p>}
      {data && (
        <fieldset disabled={busy} className="min-w-0 space-y-5">
          {(mode === "orders" || mode === "inspection") && (
            <div className="space-y-3 rounded-lg border p-4">
              <form
                className="flex flex-wrap gap-2"
                onSubmit={(e) => {
                  e.preventDefault()
                  setOrderId(orderInput.trim())
                  setSelected(null)
                  setItemId("")
                  setAssignmentId("")
                  setChecked([])
                }}
              >
                {supplier && (
                  <select
                    className={box}
                    aria-label={t("deviceScan.authorizedOrders")}
                    value={orderInput}
                    onChange={(e) => setOrderInput(e.target.value)}
                  >
                    <option value="">{t("deviceScan.authorizedOrders")}</option>
                    {data.access.order_ids.map((id) => (
                      <option key={id} value={id}>
                        {id}
                      </option>
                    ))}
                  </select>
                )}
                <input
                  aria-label={t("deviceScan.orderId")}
                  className={`${box} flex-1`}
                  placeholder={t("deviceScan.orderPlaceholder")}
                  value={orderInput}
                  onChange={(e) => setOrderInput(e.target.value)}
                />
                <Button type="submit" variant="secondary">
                  {t("deviceScan.loadOrder")}
                </Button>
              </form>
              {data.order ? (
                <>
                  <p>
                    {t("deviceScan.orderPrefix")}
                    {data.order.display_id} ·{" "}
                    {t(`deviceScan.orderStatuses.${data.order.status}`, {
                      defaultValue: data.order.status,
                    })}{" "}
                    ·{" "}
                    {!supplier && (
                      <a
                        className="underline"
                        href={`/app/orders/${data.order.id}`}
                      >
                        {t("deviceScan.openOrder")}
                      </a>
                    )}
                  </p>
                  <Field label={t("deviceScan.orderItem")}>
                    <select
                      aria-label={t("deviceScan.orderItem")}
                      className={box}
                      value={itemId}
                      onChange={(e) => {
                        setItemId(e.target.value)
                        const i = data.order?.items.find(
                          (i) => i.id === e.target.value,
                        )
                        if (i) setVariantId(i.variant_id)
                      }}
                    >
                      <option value="">{t("deviceScan.select")}</option>
                      {data.order.items.map((i) => (
                        <option key={i.id} value={i.id}>
                          {i.title} · {i.variant_title}{" "}
                          {t("deviceScan.quantityPrefix")}{" "}
                          {Number(i.detail.quantity)}{" "}
                          {t("deviceScan.fulfilledPrefix")}{" "}
                          {Number(i.detail.fulfilled_quantity || 0)}
                        </option>
                      ))}
                    </select>
                  </Field>
                  {supplier && data.order.shipping_address && (
                    <div className="rounded border p-3 text-sm">
                      <p className="font-medium">
                        {t("deviceScan.shippingAddress")}
                      </p>
                      <p>
                        {[
                          data.order.shipping_address.first_name,
                          data.order.shipping_address.last_name,
                        ]
                          .filter(Boolean)
                          .join(" ")}
                      </p>
                      <p>
                        {[
                          data.order.shipping_address.company,
                          data.order.shipping_address.address_1,
                          data.order.shipping_address.address_2,
                        ]
                          .filter(Boolean)
                          .join(", ")}
                      </p>
                      <p>
                        {[
                          data.order.shipping_address.postal_code,
                          data.order.shipping_address.city,
                          data.order.shipping_address.province,
                          data.order.shipping_address.country_code?.toUpperCase(),
                        ]
                          .filter(Boolean)
                          .join(" ")}
                      </p>
                      {data.order.shipping_address.phone && (
                        <p>
                          {t("deviceScan.recipientPhone", {
                            phone: data.order.shipping_address.phone,
                          })}
                        </p>
                      )}
                    </div>
                  )}
                </>
              ) : (
                orderId && <p>{t("deviceScan.orderNotFound")}</p>
              )}
            </div>
          )}
          {mode === "sources" && (
            <>
              {!supplier && (
                <>
                  <div className="space-y-3 rounded-lg border p-4">
                    <h2 className="font-medium">{t("deviceScan.addSource")}</h2>
                    <div className="grid gap-3 md:grid-cols-2">
                      <Field label={t("deviceScan.sourceName")}>
                        <input
                          className={box}
                          value={sourceName}
                          onChange={(e) => setSourceName(e.target.value)}
                        />
                      </Field>
                      <Field label={t("deviceScan.sourceType")}>
                        <select
                          className={box}
                          value={sourceType}
                          onChange={(e) => setSourceType(e.target.value)}
                        >
                          <option value="self">
                            {t("deviceScan.selfSource")}
                          </option>
                          <option value="supplier">
                            {t("deviceScan.supplierShipping")}
                          </option>
                        </select>
                      </Field>
                    </div>
                    <Button
                      disabled={!sourceName.trim()}
                      onClick={() =>
                        command(
                          {
                            action: "source",
                            name: sourceName,
                            type: sourceType,
                          },
                          t("deviceScan.sourceCreated"),
                        )
                      }
                    >
                      {t("deviceScan.createSource")}
                    </Button>
                  </div>
                  <div className="space-y-3 rounded-lg border p-4">
                    <h2 className="font-medium">
                      {t("deviceScan.enableInventory")}
                    </h2>
                    {variantSelect}
                    {variant && (
                      <p className="text-sm">
                        {t("deviceScan.stockPrefix")}
                        {variant.inventory_items
                          ?.flatMap((i) => i.inventory?.location_levels || [])
                          .map((l) =>
                            t("deviceScan.inventorySummary", {
                              source:
                                data.sources.find(
                                  (s) => s.location_id === l.location_id,
                                )?.name || l.location_id,
                              stock: Number(l.stocked_quantity),
                              reserved: Number(l.reserved_quantity),
                            }),
                          )
                          .join("；") || "0"}
                      </p>
                    )}
                    {managed ? (
                      <p>{t("deviceScan.inventoryEnabled")}</p>
                    ) : (
                      <>
                        <p className="text-sm">
                          {t("deviceScan.takeoverWarning")}
                        </p>
                        <label className="flex gap-2 text-sm">
                          <input
                            type="checkbox"
                            checked={acknowledged}
                            onChange={(e) => setAcknowledged(e.target.checked)}
                          />
                          {t("deviceScan.takeoverConfirm")}
                        </label>
                        <Button
                          disabled={!variantId || !acknowledged}
                          onClick={() =>
                            command(
                              {
                                action: "enroll",
                                variant_id: variantId,
                                acknowledged: true,
                              },
                              t("deviceScan.takeoverSuccess"),
                            )
                          }
                        >
                          {t("deviceScan.takeover")}
                        </Button>
                      </>
                    )}
                  </div>
                </>
              )}
              {!supplier && (
                <div className="space-y-3 rounded-lg border p-4">
                  <h2 className="font-medium">
                    {t("deviceScan.supplierAccounts")}
                  </h2>
                  <p className="text-sm">
                    {t("deviceScan.supplierAccountHelp")}
                  </p>
                  <Field label={t("deviceScan.supplierEmail")}>
                    <input
                      className={box}
                      type="email"
                      value={supplierEmail}
                      onChange={(e) => setSupplierEmail(e.target.value)}
                    />
                  </Field>
                  {sourceSelect}
                  <Field label={t("deviceScan.authorizedOrders")}>
                    <textarea
                      className={box}
                      value={supplierOrders}
                      onChange={(e) => setSupplierOrders(e.target.value)}
                      placeholder={t("deviceScan.supplierOrdersHelp")}
                    />
                  </Field>
                  <Field label={t("deviceScan.supplierPassword")}>
                    <input
                      className={box}
                      type="password"
                      autoComplete="new-password"
                      value={supplierPassword}
                      onChange={(e) => setSupplierPassword(e.target.value)}
                      placeholder={t("deviceScan.supplierPasswordHelp")}
                    />
                  </Field>
                  <Button
                    disabled={
                      !supplierEmail.trim() ||
                      source?.type !== "supplier" ||
                      (supplierPassword.length > 0 &&
                        supplierPassword.length < 12)
                    }
                    onClick={async () => {
                      if (
                        await command(
                          {
                            email: supplierEmail.trim(),
                            ...(supplierPassword
                              ? { password: supplierPassword }
                              : {}),
                            source_id: sourceId,
                            order_ids: supplierOrders
                              .split(/[\s,，]+/)
                              .filter(Boolean),
                          },
                          t("deviceScan.supplierAccountSaved"),
                          "/admin/devices/supplier-accounts",
                        )
                      )
                        setSupplierPassword("")
                    }}
                  >
                    {t("deviceScan.saveSupplierAccount")}
                  </Button>
                  {supplierAccounts.map((account) => (
                    <button
                      type="button"
                      className="block w-full rounded border p-3 text-left text-sm"
                      key={account.email}
                      onClick={() => {
                        setSupplierEmail(account.email)
                        setSupplierPassword("")
                        setSourceId(account.source_id)
                        setSupplierOrders(account.order_ids.join("\n"))
                      }}
                    >
                      {account.email} ·{" "}
                      {
                        data.sources.find((s) => s.id === account.source_id)
                          ?.name
                      }{" "}
                      ·{" "}
                      {t("deviceScan.authorizedOrderCount", {
                        count: account.order_ids.length,
                      })}
                    </button>
                  ))}
                </div>
              )}
              <div className="space-y-3 rounded-lg border p-4">
                <h2 className="font-medium">
                  {t("deviceScan.supplierAvailability")}
                </h2>
                {sourceSelect}
                {variantSelect}
                <Field label={t("deviceScan.unallocatedQuantity")}>
                  <input
                    className={box}
                    type="number"
                    min="0"
                    value={quantity}
                    onChange={(e) => setQuantity(e.target.value)}
                  />
                </Field>
                <p className="text-sm">{t("deviceScan.supplierHelp")}</p>
                <Button
                  disabled={!managed || source?.type !== "supplier"}
                  onClick={() =>
                    command(
                      {
                        action: "supply",
                        source_id: sourceId,
                        variant_id: variantId,
                        quantity: Number(quantity),
                      },
                      t("deviceScan.supplierUpdated"),
                    )
                  }
                >
                  {t("deviceScan.updateAvailability")}
                </Button>
              </div>
              <div className="space-y-2">
                {data.sources.map((s) => (
                  <div key={s.id} className="rounded-lg border p-3">
                    <p>
                      {s.name} ·{" "}
                      {s.type === "self"
                        ? t("deviceScan.ownWarehouse")
                        : t("deviceScan.supplierShipping")}
                    </p>
                    <p className="break-all text-xs text-ui-fg-subtle">
                      {s.location_id}
                    </p>
                    {data.supplies
                      .filter((p) => p.source_id === s.id)
                      .map((p) => (
                        <p key={p.variant_id} className="text-sm">
                          {data.variants.find((v) => v.id === p.variant_id)
                            ?.sku || p.variant_id}
                          {t("deviceScan.unallocatedPrefix")}
                          {p.quantity} {t("deviceScan.units")}
                        </p>
                      ))}
                  </div>
                ))}
              </div>
            </>
          )}
          {mode === "intake" && (
            <div className="space-y-4 rounded-lg border p-4">
              {sourceSelect}
              {variantSelect}
              {deviceFields}
              <Field label={t("deviceScan.intakeNotes")}>
                <textarea
                  className={box}
                  value={notes}
                  onChange={(e) => setNotes(e.target.value)}
                />
              </Field>
              <p className="text-sm">{t("deviceScan.intakeHelp")}</p>
              <Button
                disabled={
                  source?.type !== "self" || !managed || (!serial && !imei)
                }
                onClick={async () => {
                  if (
                    await command(
                      { action: "intake", devices: [newDevice()] },
                      t("deviceScan.intakeSuccess"),
                    )
                  ) {
                    setSerial("")
                    setImei("")
                  }
                }}
              >
                {t("deviceScan.registerIntake")}
              </Button>
              <div className="space-y-3 border-t pt-4">
                <h2 className="font-medium">{t("deviceScan.csvTitle")}</h2>
                <p className="break-words text-sm">{t("deviceScan.csvHelp")}</p>
                <a
                  className="text-sm underline"
                  download="device-import-template.csv"
                  href={`data:text/csv;charset=utf-8,${encodeURIComponent("\uFEFFserial_number,imei,sku,grade,battery_health,cost_eur,currency_code,bin,notes\r\n")}`}
                >
                  {t("deviceScan.csvTemplate")}
                </a>
                <input
                  aria-label={t("deviceScan.csvUpload")}
                  type="file"
                  accept=".csv,text/csv"
                  onChange={(e) => {
                    void importCsv(e.target.files?.[0])
                    e.target.value = ""
                  }}
                />
                {batch.length > 0 && (
                  <>
                    <p>
                      {t("deviceScan.batchPreview", { count: batch.length })}
                    </p>
                    <div className="max-h-64 overflow-auto rounded border p-2 text-xs">
                      {batch.map((d, i) => (
                        <p key={i}>
                          {i + 1}.{" "}
                          {String(
                            d.serial_number ||
                              d.imei ||
                              t("deviceScan.missingIdentity"),
                          )}{" "}
                          ·{" "}
                          {
                            data.variants.find((v) => v.id === d.variant_id)
                              ?.sku
                          }{" "}
                          · {String(d.grade)} · {String(d.battery_health)}%
                        </p>
                      ))}
                    </div>
                    <Button
                      disabled={source?.type !== "self"}
                      onClick={async () => {
                        if (
                          await command(
                            { action: "intake", devices: batch },
                            t("deviceScan.batchSuccess"),
                          )
                        )
                          setBatch([])
                      }}
                    >
                      {t("deviceScan.batchConfirm", { count: batch.length })}
                    </Button>
                  </>
                )}
              </div>
            </div>
          )}
          {mode === "orders" && (
            <>
              <div className="space-y-3 rounded-lg border p-4">
                {sourceSelect}
                {!supplier && (
                  <Field label={t("deviceScan.replacementLabel")}>
                    <select
                      className={box}
                      value={replacement}
                      onChange={(e) => {
                        setReplacement(e.target.value)
                        const a = data.assignments.find(
                          (a) => a.id === e.target.value,
                        )
                        if (a) {
                          setItemId(a.item_id)
                          setVariantId(
                            findDevice(a.device_id)?.variant_id || "",
                          )
                        }
                      }}
                    >
                      <option value="">
                        {t("deviceScan.normalAllocation")}
                      </option>
                      {data.assignments
                        .filter(
                          (a) =>
                            a.status === "returned" &&
                            requests.some(
                              (r) =>
                                r.id === a.after_sales_request_id &&
                                r.type === "exchange" &&
                                r.status === "processing",
                            ),
                        )
                        .map((a) => (
                          <option key={a.id} value={a.id}>
                            {identity(findDevice(a.device_id)!)} · {a.id}
                          </option>
                        ))}
                    </select>
                  </Field>
                )}
                {source?.type === "supplier" && (
                  <>
                    {variantSelect}
                    {deviceFields}
                    <Button
                      disabled={
                        !orderId || !itemId || !managed || (!serial && !imei)
                      }
                      onClick={async () => {
                        if (
                          await command(
                            {
                              action: "supplier_assign",
                              device: newDevice(),
                              order_id: orderId,
                              item_id: itemId,
                              ...(replacement
                                ? { replacement_of: replacement }
                                : {}),
                            },
                            t("deviceScan.supplierAllocated"),
                          )
                        ) {
                          setSerial("")
                          setImei("")
                        }
                      }}
                    >
                      {t("deviceScan.registerSupplier")}
                    </Button>
                  </>
                )}
                {source?.type === "self" && (
                  <p className="text-sm">{t("deviceScan.allocateHelp")}</p>
                )}
              </div>
              <div className="space-y-3 rounded-lg border p-4">
                <h2 className="font-medium">{t("deviceScan.orderDevices")}</h2>
                {data.assignments.map((a) => (
                  <div
                    key={a.id}
                    className="space-y-2 rounded border p-3 text-sm"
                  >
                    <p className="break-all">
                      {identity(findDevice(a.device_id)!)} · {states[a.status]}{" "}
                      · {data.sources.find((s) => s.id === a.source_id)?.name}
                    </p>
                    {a.replacement_of && (
                      <p className="break-all">
                        {t("deviceScan.replacementPrefix")}
                        {a.replacement_of}
                      </p>
                    )}
                    {a.tracking_number && (
                      <p>
                        {t("deviceScan.trackingPrefix")}
                        {a.tracking_number}
                      </p>
                    )}
                    {a.status === "reserved" && (
                      <div className="flex flex-wrap gap-3">
                        <label className="flex gap-2">
                          <input
                            type="checkbox"
                            checked={checked.includes(a.id)}
                            onChange={(e) =>
                              setChecked(
                                e.target.checked
                                  ? [...checked, a.id]
                                  : checked.filter((id) => id !== a.id),
                              )
                            }
                          />
                          {t("deviceScan.selectFulfillment")}
                        </label>
                        <Button
                          size="small"
                          variant="secondary"
                          onClick={() =>
                            command(
                              { action: "release", assignment_id: a.id },
                              t("deviceScan.released"),
                            )
                          }
                        >
                          {t("deviceScan.release")}
                        </Button>
                      </div>
                    )}
                    {a.fulfillment_id && (
                      <p className="break-all text-xs">
                        {t("deviceScan.fulfillmentPrefix")}
                        {a.fulfillment_id}
                      </p>
                    )}
                  </div>
                ))}
                <Button
                  disabled={!checked.length}
                  onClick={() =>
                    command(
                      { action: "fulfill", assignment_ids: checked },
                      t("deviceScan.fulfillmentCreated"),
                    )
                  }
                >
                  {t("deviceScan.createFulfillment", { count: checked.length })}
                </Button>
                <Field label={t("deviceScan.tracking")}>
                  <input
                    aria-label={t("deviceScan.tracking")}
                    className={box}
                    value={tracking}
                    onChange={(e) => setTracking(e.target.value)}
                  />
                </Field>
                <Field label={t("deviceScan.trackingUrl")}>
                  <input
                    className={box}
                    type="url"
                    value={trackingUrl}
                    onChange={(e) => setTrackingUrl(e.target.value)}
                  />
                </Field>
                {[
                  ...new Set(
                    data.assignments
                      .filter((a) => a.status === "packed")
                      .map((a) => a.fulfillment_id!),
                  ),
                ].map((id) => (
                  <div key={id} className="flex flex-wrap gap-2">
                    <Button
                      disabled={!tracking.trim()}
                      onClick={() =>
                        command(
                          {
                            action: "ship",
                            fulfillment_id: id,
                            tracking_number: tracking,
                            ...(trackingUrl
                              ? { tracking_url: trackingUrl }
                              : {}),
                          },
                          t("deviceScan.shipSuccess"),
                        )
                      }
                    >
                      {t("deviceScan.shipFulfillment", { id })}
                    </Button>
                    <Button
                      variant="secondary"
                      onClick={() =>
                        command(
                          { action: "cancel_fulfillment", fulfillment_id: id },
                          t("deviceScan.cancelSuccess"),
                        )
                      }
                    >
                      {t("deviceScan.cancelFulfillment")}
                    </Button>
                  </div>
                ))}
              </div>
            </>
          )}
          {mode === "inspection" && (
            <>
              <div className="space-y-3 rounded-lg border p-4">
                <h2 className="font-medium">{t("deviceScan.reviewTitle")}</h2>
                {requests.map((r) => (
                  <div
                    className="space-y-2 rounded border p-3 text-sm"
                    key={r.id}
                  >
                    <p className="break-all">
                      {t(`deviceScan.requestTypes.${r.type}`, {
                        defaultValue: r.type,
                      })}{" "}
                      ·{" "}
                      {t(`deviceScan.requestStatuses.${r.status}`, {
                        defaultValue: r.status,
                      })}{" "}
                      · {r.id}
                    </p>
                    <div className="flex flex-wrap gap-2">
                      {r.status === "pending" && (
                        <Button
                          size="small"
                          variant="secondary"
                          onClick={() =>
                            command(
                              { request_id: r.id, status: "processing" },
                              t("deviceScan.reviewSuccess"),
                              `/admin/orders/${orderId}/after-sales`,
                            )
                          }
                        >
                          {t("deviceScan.review")}
                        </Button>
                      )}
                      {r.status === "processing" && (
                        <Button
                          size="small"
                          variant="secondary"
                          onClick={() =>
                            command(
                              { request_id: r.id, status: "completed" },
                              t("deviceScan.completedSuccess"),
                              `/admin/orders/${orderId}/after-sales`,
                            )
                          }
                        >
                          {t("deviceScan.complete")}
                        </Button>
                      )}
                      {r.status === "pending" && (
                        <Button
                          size="small"
                          variant="secondary"
                          onClick={() =>
                            command(
                              { request_id: r.id, status: "rejected" },
                              t("deviceScan.rejectedSuccess"),
                              `/admin/orders/${orderId}/after-sales`,
                            )
                          }
                        >
                          {t("deviceScan.reject")}
                        </Button>
                      )}
                    </div>
                  </div>
                ))}
                {requests.length === 0 && (
                  <p className="text-sm">{t("deviceScan.reviewHelp")}</p>
                )}
              </div>
              <div className="space-y-3 rounded-lg border p-4">
                <h2 className="font-medium">{t("deviceScan.receiveTitle")}</h2>
                {sourceSelect}
                <Field label={t("deviceScan.saleToReceive")}>
                  <select
                    aria-label={t("deviceScan.saleToReceive")}
                    className={box}
                    value={assignmentId}
                    onChange={(e) => {
                      setAssignmentId(e.target.value)
                      const a = data.assignments.find(
                        (a) => a.id === e.target.value,
                      )
                      const d = findDevice(a?.device_id)
                      if (d) choose(d)
                    }}
                  >
                    <option value="">{t("deviceScan.select")}</option>
                    {data.assignments
                      .filter((a) => a.status === "shipped")
                      .map((a) => (
                        <option key={a.id} value={a.id}>
                          {identity(findDevice(a.device_id)!)} · {a.id}
                        </option>
                      ))}
                  </select>
                </Field>
                <Field label={t("deviceScan.processingRequest")}>
                  <select
                    className={box}
                    value={requestId}
                    onChange={(e) => setRequestId(e.target.value)}
                  >
                    <option value="">{t("deviceScan.selectReviewed")}</option>
                    {requests
                      .filter((r) => r.status === "processing")
                      .map((r) => (
                        <option key={r.id} value={r.id}>
                          {t(`deviceScan.requestTypes.${r.type}`, {
                            defaultValue: r.type,
                          })}{" "}
                          · {r.id}
                        </option>
                      ))}
                  </select>
                </Field>
                <p className="break-all text-sm">
                  {t("deviceScan.scanPrefix")}
                  {scan || t("deviceScan.scanReturned")}
                </p>
                <Button
                  disabled={!assignmentId || !requestId || !sourceId || !scan}
                  onClick={() =>
                    command(
                      {
                        action: "receive",
                        assignment_id: assignmentId,
                        request_id: requestId,
                        source_id: sourceId,
                        scan,
                      },
                      t("deviceScan.receivedSuccess"),
                    )
                  }
                >
                  {t("deviceScan.receive")}
                </Button>
              </div>
              {selected && (
                <div className="space-y-3 rounded-lg border p-4">
                  <h2 className="font-medium">
                    {t("deviceScan.inspectionPrefix")}
                    {identity(selected)} · {states[selected.state]}
                  </h2>
                  {variantSelect}
                  <div className="grid gap-3 md:grid-cols-2">
                    <Field label={t("deviceScan.inspectedGrade")}>
                      <select
                        className={box}
                        value={grade}
                        onChange={(e) => setGrade(e.target.value)}
                      >
                        {["S", "A", "B", "C"].map((g) => (
                          <option key={g}>{g}</option>
                        ))}
                      </select>
                    </Field>
                    <Field label={t("deviceScan.inspectedBattery")}>
                      <input
                        className={box}
                        type="number"
                        min="0"
                        max="100"
                        value={battery}
                        onChange={(e) => setBattery(e.target.value)}
                      />
                    </Field>
                  </div>
                  <Field label={t("deviceScan.inspectionResult")}>
                    <select
                      aria-label={t("deviceScan.inspectionResult")}
                      className={box}
                      value={result}
                      onChange={(e) => setResult(e.target.value)}
                    >
                      <option value="pass">{t("deviceScan.pass")}</option>
                      <option value="repair">
                        {t("deviceScan.needsRepair")}
                      </option>
                      <option value="retire">{t("deviceScan.retire")}</option>
                      <option value="supplier_return">
                        {t("deviceScan.returnSupplier")}
                      </option>
                    </select>
                  </Field>
                  <Field label={t("deviceScan.inspectionNotes")}>
                    <textarea
                      aria-label={t("deviceScan.inspectionNotes")}
                      className={box}
                      value={notes}
                      onChange={(e) => setNotes(e.target.value)}
                    />
                  </Field>
                  <Button
                    disabled={
                      !notes.trim() ||
                      !["inspection", "repair"].includes(selected.state)
                    }
                    onClick={() =>
                      command(
                        {
                          action: "inspect",
                          device_id: selected.id,
                          result,
                          grade,
                          battery_health: Number(battery),
                          variant_id: variantId,
                          notes,
                        },
                        t("deviceScan.inspectionSaved"),
                      )
                    }
                  >
                    {t("deviceScan.saveInspection")}
                  </Button>
                  {selected.state === "repair_ready" && (
                    <>
                      <Field label={t("deviceScan.repairTracking")}>
                        <input
                          className={box}
                          value={tracking}
                          onChange={(e) => setTracking(e.target.value)}
                        />
                      </Field>
                      <Button
                        disabled={!tracking.trim()}
                        onClick={() => {
                          const a = data.assignments.find(
                            (a) =>
                              a.device_id === selected.id &&
                              a.status === "returned",
                          )
                          if (a)
                            void command(
                              {
                                action: "repair_ship",
                                assignment_id: a.id,
                                tracking_number: tracking,
                              },
                              t("deviceScan.repairReturned"),
                            )
                        }}
                      >
                        {t("deviceScan.returnRepair")}
                      </Button>
                    </>
                  )}
                </div>
              )}
            </>
          )}
          {mode === "inspection" &&
            data.order &&
            requests.some(
              (r) => r.type === "return" && r.status === "processing",
            ) && (
              <div className="space-y-3 rounded-lg border p-4">
                <h2 className="font-medium">{t("deviceScan.refundTitle")}</h2>
                <p className="text-sm">{t("deviceScan.refundHelp")}</p>
                <Field label={t("deviceScan.refundPayment")}>
                  <select
                    className={box}
                    value={paymentId}
                    onChange={(e) => {
                      setPaymentId(e.target.value)
                      setRefundConfirmed(false)
                    }}
                  >
                    <option value="">{t("deviceScan.selectCaptured")}</option>
                    {data.order.payment_collections
                      ?.flatMap((c) => c.payments || [])
                      .map((p) => {
                        const balance =
                          p.captures.reduce((s, c) => s + Number(c.amount), 0) -
                          p.refunds.reduce((s, r) => s + Number(r.amount), 0)
                        return (
                          <option key={p.id} value={p.id}>
                            {p.id} {t("deviceScan.refundablePrefix")}
                            {balance.toFixed(2)}{" "}
                            {data.order!.currency_code.toUpperCase()}
                          </option>
                        )
                      })}
                  </select>
                </Field>
                <Field
                  label={t("deviceScan.refundAmount", {
                    currency: data.order.currency_code.toUpperCase(),
                  })}
                >
                  <input
                    className={box}
                    type="number"
                    step="0.01"
                    min="0.01"
                    value={refundAmount}
                    onChange={(e) => {
                      setRefundAmount(e.target.value)
                      setRefundConfirmed(false)
                    }}
                  />
                </Field>
                <label className="flex gap-2 text-sm">
                  <input
                    type="checkbox"
                    checked={refundConfirmed}
                    onChange={(e) => setRefundConfirmed(e.target.checked)}
                  />
                  {t("deviceScan.refundConfirm")}
                </label>
                <Button
                  disabled={
                    !requestId ||
                    !paymentId ||
                    !refundConfirmed ||
                    Number(refundAmount) <= 0
                  }
                  onClick={() =>
                    command(
                      {
                        request_id: requestId,
                        payment_id: paymentId,
                        amount: Number(refundAmount),
                        confirmed: true,
                      },
                      t("deviceScan.refundSuccess"),
                      "/admin/devices/refunds",
                    )
                  }
                >
                  {t("deviceScan.refund")}
                </Button>
              </div>
            )}
          {mode !== "sources" && (
            <div className="space-y-3">
              <h2 className="font-medium">
                {t("deviceScan.deviceList", { count: data.count })}
              </h2>
              <div className="flex flex-wrap gap-2">
                <input
                  aria-label={t("deviceScan.search")}
                  className={`${box} flex-1`}
                  placeholder={t("deviceScan.searchPlaceholder")}
                  value={search}
                  onChange={(e) => {
                    setSearch(e.target.value)
                    setSelected(null)
                    setOffset(0)
                  }}
                />
                {selected && (
                  <Button variant="secondary" onClick={() => setSelected(null)}>
                    {t("deviceScan.backToList")}
                  </Button>
                )}
              </div>
              <div className="grid gap-3 lg:grid-cols-2">
                {data.devices.map((d) => (
                  <div
                    key={d.id}
                    className={`min-w-0 space-y-2 rounded-lg border p-4 ${selected?.id === d.id ? "border-ui-border-interactive" : ""}`}
                  >
                    <p className="break-all font-medium">
                      {identity(d)} · {states[d.state]}
                    </p>
                    <p className="text-sm">
                      {data.variants.find((v) => v.id === d.variant_id)?.sku} ·{" "}
                      {d.grade} {t("deviceScan.batteryPrefix")}
                      {d.battery_health}%
                    </p>
                    <p className="text-sm">
                      {t("deviceScan.locationPrefix")}
                      {data.sources.find((s) => s.location_id === d.location_id)
                        ?.name || d.location_id}{" "}
                      {t("deviceScan.binPrefix")}
                      {d.bin || "—"}
                    </p>
                    <p className="break-all text-xs text-ui-fg-subtle">
                      {d.id}
                      {d.serial_number && d.imei ? ` · IMEI ${d.imei}` : ""}
                    </p>
                    <div className="flex flex-wrap gap-2">
                      <Button
                        size="small"
                        variant="secondary"
                        onClick={() => choose(d)}
                      >
                        {mode === "inspection"
                          ? t("deviceScan.selectInspection")
                          : t("deviceScan.viewDevice")}
                      </Button>
                      {mode === "orders" && d.state === "available" && (
                        <Button
                          size="small"
                          disabled={
                            !orderId ||
                            !itemId ||
                            d.location_id !== source?.location_id
                          }
                          onClick={() =>
                            command(
                              {
                                action: "assign",
                                device_id: d.id,
                                order_id: orderId,
                                item_id: itemId,
                                ...(replacement
                                  ? { replacement_of: replacement }
                                  : {}),
                              },
                              t("deviceScan.allocated"),
                            )
                          }
                        >
                          {t("deviceScan.allocate")}
                        </Button>
                      )}
                      {mode === "intake" && d.state === "inspection" && (
                        <Button
                          size="small"
                          variant="secondary"
                          onClick={() => {
                            choose(d)
                            setMode("inspection")
                            setOrderId("")
                          }}
                        >
                          {t("deviceScan.inspect")}
                        </Button>
                      )}
                    </div>
                  </div>
                ))}
              </div>
              {!data.devices.length && (
                <p className="text-sm">{t("deviceScan.noDevices")}</p>
              )}
              {!selected && (
                <div className="flex gap-2">
                  <Button
                    variant="secondary"
                    disabled={offset === 0}
                    onClick={() => setOffset(Math.max(0, offset - 50))}
                  >
                    {t("deviceScan.previous")}
                  </Button>
                  <Button
                    variant="secondary"
                    disabled={offset + 50 >= data.count}
                    onClick={() => setOffset(offset + 50)}
                  >
                    {t("deviceScan.next")}
                  </Button>
                </div>
              )}
              {mode === "lookup" && selected && (
                <div className="space-y-3 rounded-lg border p-4">
                  <h2 className="font-medium">{t("deviceScan.history")}</h2>
                  <p className="text-sm">
                    {t("deviceScan.costPrefix")}
                    {(selected.cost_cents / 100).toFixed(2)}{" "}
                    {selected.currency_code.toUpperCase()}
                  </p>
                  {data.assignments.map((a) => (
                    <p key={a.id} className="break-all text-sm">
                      <a
                        className="underline"
                        href={`/app/orders/${a.order_id}`}
                      >
                        {a.order_id}
                      </a>{" "}
                      · {states[a.status]} · {a.tracking_number || ""}
                    </p>
                  ))}
                  {data.events.map((e) => (
                    <details key={e.id} className="rounded border p-2 text-sm">
                      <summary>
                        {new Date(e.created_at).toLocaleString(
                          i18n.language === "zhCN" ? "zh-CN" : "en-GB",
                        )}{" "}
                        ·{" "}
                        {t(`deviceScan.actions.${e.action}`, {
                          defaultValue: e.action,
                        })}
                      </summary>
                      <p className="break-all">
                        {t("deviceScan.actorPrefix")}
                        {e.actor_id}
                      </p>
                      <pre className="overflow-auto whitespace-pre-wrap break-all text-xs">
                        {JSON.stringify(e.data, null, 2)}
                      </pre>
                    </details>
                  ))}
                </div>
              )}
            </div>
          )}
        </fieldset>
      )}
    </Container>
  )
}
export default DeviceScanPage

export const config = defineRouteConfig({
  label: "deviceScan.title",
  translationNs: "translation",
  rank: 5,
})
