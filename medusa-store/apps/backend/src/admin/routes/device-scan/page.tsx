import { defineRouteConfig } from "@medusajs/admin-sdk"
import { Button, Container, Heading } from "@medusajs/ui"
import { useCallback, useEffect, useState } from "react"
import { useSearchParams } from "react-router-dom"
import DeviceScanner from "../../components/device-scanner"
import { parseDeviceCsv } from "../../lib/device-csv"

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
const states: Record<string, string> = {
  inspection: "待检测",
  available: "可售",
  reserved: "已配货",
  packed: "待发货",
  shipped: "已发货",
  repair: "维修中",
  repair_ready: "维修完成待返还",
  retired: "报废",
  supplier_returned: "已退供应商",
  returned: "已收回",
  canceled: "已解除",
}
const modes = {
  lookup: "设备查询",
  intake: "扫码入库 / 导入",
  orders: "订单配货 / 发货",
  inspection: "售后收回 / 检测",
  sources: "仓库 / 供应商库存",
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
    if (!response.ok) throw new Error("无法加载设备数据，请确认后台已登录")
    const next: Data = await response.json()
    setData(next)
    if (selected)
      setSelected(next.devices.find((d) => d.id === selected.id) || null)
    if (orderId && (mode === "orders" || mode === "inspection")) {
      const r = await fetch(
        `/admin/orders/${encodeURIComponent(orderId)}/after-sales`,
        { credentials: "include" },
      )
      if (!r.ok) throw new Error("无法加载该订单的售后申请")
      setRequests((await r.json()).requests)
    }
  }, [mode, search, orderId, offset, selected?.id])
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
      if (!r.ok) throw new Error(json.message || "操作失败，请核对信息后重试")
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
    d?.serial_number || d?.imei || d?.id || "未找到设备"
  const variantLabel = (v: Variant) =>
    `${v.product?.title || ""} · ${v.options?.map((o) => o.value).join(" / ") || v.title} · ${v.sku || v.id}`
  const sourceSelect = (
    <Field label="供货来源 / 收回仓库">
      <select
        aria-label="供货来源 / 收回仓库"
        className={box}
        value={sourceId}
        onChange={(e) => {
          setSourceId(e.target.value)
          setBatch([])
        }}
      >
        <option value="">请选择</option>
        {data?.sources.map((s) => (
          <option key={s.id} value={s.id}>
            {s.name}（{s.type === "self" ? "本仓" : "供应商"}）
          </option>
        ))}
      </select>
    </Field>
  )
  const variantSelect = (
    <Field label="商品 SKU">
      <select
        aria-label="商品 SKU"
        className={box}
        value={variantId}
        onChange={(e) => {
          setVariantId(e.target.value)
          setAcknowledged(false)
        }}
      >
        <option value="">请选择</option>
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
      <Field label="苹果序列号">
        <input
          aria-label="苹果序列号"
          className={box}
          value={serial}
          onChange={(e) => setSerial(e.target.value.toUpperCase())}
          autoComplete="off"
        />
      </Field>
      <Field label="IMEI（15 位，可选）">
        <input
          aria-label="IMEI"
          className={box}
          value={imei}
          onChange={(e) => setImei(e.target.value)}
          inputMode="numeric"
        />
      </Field>
      <Field label="成色">
        <select
          aria-label="成色"
          className={box}
          value={grade}
          onChange={(e) => setGrade(e.target.value)}
        >
          {["S", "A", "B", "C"].map((g) => (
            <option key={g}>{g}</option>
          ))}
        </select>
      </Field>
      <Field label="实际电池健康度（%）">
        <input
          aria-label="实际电池健康度"
          className={box}
          type="number"
          min="0"
          max="100"
          value={battery}
          onChange={(e) => setBattery(e.target.value)}
        />
      </Field>
      <Field label="采购成本（EUR）">
        <input
          aria-label="采购成本"
          className={box}
          type="number"
          min="0"
          step="0.01"
          value={cost}
          onChange={(e) => setCost(e.target.value)}
        />
      </Field>
      <Field label="仓位">
        <input
          aria-label="仓位"
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
      if (file.size > 1024 * 1024) throw new Error("CSV 文件请小于 1 MB")
      const rows = parseDeviceCsv(await file.text())
      const devices = rows.map((r, i) => {
        const v =
          r.variant_id ||
          data?.variants.find((v) => v.sku === r.sku)?.id ||
          variantId
        if (!v) throw new Error(`第 ${i + 2} 行 SKU 未找到`)
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
        <Heading level="h1">设备扫码工作台</Heading>
        <p className="mt-1 text-sm text-ui-fg-subtle">
          一机一档 · 本仓与供应商代发 · 入库、配货、发货、售后与再次销售
        </p>
      </div>
      <div className="flex flex-wrap gap-2">
        {Object.entries(modes).map(([key, title]) => (
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
            {title}
          </Button>
        ))}
      </div>
      {error && (
        <div
          role="alert"
          className="rounded-lg bg-ui-bg-subtle p-3 text-sm text-ui-fg-error"
        >
          {error}
        </div>
      )}
      {success && (
        <div role="status" className="rounded-lg bg-ui-bg-subtle p-3 text-sm">
          {success}
        </div>
      )}
      {mode !== "sources" && <DeviceScanner key={mode} onScan={onScan} />}
      {!data && <p>正在加载…</p>}
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
                <input
                  aria-label="订单 ID"
                  className={`${box} flex-1`}
                  placeholder="订单 ID（order_…），也可从订单详情进入"
                  value={orderInput}
                  onChange={(e) => setOrderInput(e.target.value)}
                />
                <Button type="submit" variant="secondary">
                  加载订单
                </Button>
              </form>
              {data.order ? (
                <>
                  <p>
                    订单 #{data.order.display_id} · {data.order.status} ·{" "}
                    <a
                      className="underline"
                      href={`/app/orders/${data.order.id}`}
                    >
                      打开订单 / 退款操作
                    </a>
                  </p>
                  <Field label="订单商品">
                    <select
                      aria-label="订单商品"
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
                      <option value="">请选择</option>
                      {data.order.items.map((i) => (
                        <option key={i.id} value={i.id}>
                          {i.title} · {i.variant_title} · 数量{" "}
                          {Number(i.detail.quantity)} / 已履约{" "}
                          {Number(i.detail.fulfilled_quantity || 0)}
                        </option>
                      ))}
                    </select>
                  </Field>
                </>
              ) : (
                orderId && <p>未找到订单。</p>
              )}
            </div>
          )}
          {mode === "sources" && (
            <>
              <div className="space-y-3 rounded-lg border p-4">
                <h2 className="font-medium">新增本仓 / 供应商</h2>
                <div className="grid gap-3 md:grid-cols-2">
                  <Field label="来源名称">
                    <input
                      className={box}
                      value={sourceName}
                      onChange={(e) => setSourceName(e.target.value)}
                    />
                  </Field>
                  <Field label="来源类型">
                    <select
                      className={box}
                      value={sourceType}
                      onChange={(e) => setSourceType(e.target.value)}
                    >
                      <option value="self">自有仓库</option>
                      <option value="supplier">供应商代发</option>
                    </select>
                  </Field>
                </div>
                <Button
                  disabled={!sourceName.trim()}
                  onClick={() =>
                    command(
                      { action: "source", name: sourceName, type: sourceType },
                      "供货来源已创建，并关联默认销售渠道",
                    )
                  }
                >
                  创建来源
                </Button>
              </div>
              <div className="space-y-3 rounded-lg border p-4">
                <h2 className="font-medium">启用 SKU 设备库存</h2>
                {variantSelect}
                {variant && (
                  <p className="text-sm">
                    现有数量：
                    {variant.inventory_items
                      ?.flatMap((i) => i.inventory?.location_levels || [])
                      .map(
                        (l) =>
                          `${data.sources.find((s) => s.location_id === l.location_id)?.name || l.location_id}：${Number(l.stocked_quantity)}（预留 ${Number(l.reserved_quantity)}）`,
                      )
                      .join("；") || "0"}
                  </p>
                )}
                {managed ? (
                  <p>该 SKU 已启用设备管理。</p>
                ) : (
                  <>
                    <p className="text-sm">
                      接管会把该 SKU
                      现有数量库存清零，再按本仓实际设备及供应商可供数量建立库存。存在订单预留时无法接管。请核对后执行。
                    </p>
                    <label className="flex gap-2 text-sm">
                      <input
                        type="checkbox"
                        checked={acknowledged}
                        onChange={(e) => setAcknowledged(e.target.checked)}
                      />
                      已核对现有库存，同意改用设备库存
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
                          "SKU 已启用设备库存，请登记实际设备或供应商数量",
                        )
                      }
                    >
                      接管该 SKU 库存
                    </Button>
                  </>
                )}
              </div>
              <div className="space-y-3 rounded-lg border p-4">
                <h2 className="font-medium">供应商未分配设备的可供数量</h2>
                {sourceSelect}
                {variantSelect}
                <Field label="未分配可供数量">
                  <input
                    className={box}
                    type="number"
                    min="0"
                    value={quantity}
                    onChange={(e) => setQuantity(e.target.value)}
                  />
                </Field>
                <p className="text-sm">
                  这里填写供应商尚未登记为具体机器的数量，已登记设备单独计数。重复设置同一数量不会重复增加库存。
                </p>
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
                      "供应商可供库存已更新",
                    )
                  }
                >
                  更新可供数量
                </Button>
              </div>
              <div className="space-y-2">
                {data.sources.map((s) => (
                  <div key={s.id} className="rounded-lg border p-3">
                    <p>
                      {s.name} · {s.type === "self" ? "本仓" : "供应商代发"}
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
                          ：未分配 {p.quantity} 台
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
              <Field label="入库备注">
                <textarea
                  className={box}
                  value={notes}
                  onChange={(e) => setNotes(e.target.value)}
                />
              </Field>
              <p className="text-sm">
                入库后进入待检测；检测通过才增加可售库存。
              </p>
              <Button
                disabled={
                  source?.type !== "self" || !managed || (!serial && !imei)
                }
                onClick={async () => {
                  if (
                    await command(
                      { action: "intake", devices: [newDevice()] },
                      "设备已入库，等待检测",
                    )
                  ) {
                    setSerial("")
                    setImei("")
                  }
                }}
              >
                登记入库
              </Button>
              <div className="space-y-3 border-t pt-4">
                <h2 className="font-medium">批量 CSV 导入</h2>
                <p className="break-words text-sm">
                  表头：serial_number,imei,sku,grade,battery_health,cost_eur,currency_code,bin,notes。来源使用上方选项；SKU、成色、电池为空时使用上方默认值。号码列请保存为文本。
                </p>
                <a
                  className="text-sm underline"
                  download="device-import-template.csv"
                  href={`data:text/csv;charset=utf-8,${encodeURIComponent("\uFEFFserial_number,imei,sku,grade,battery_health,cost_eur,currency_code,bin,notes\r\n")}`}
                >
                  下载空白 CSV 模板
                </a>
                <input
                  aria-label="导入设备 CSV"
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
                      待导入 {batch.length}{" "}
                      台，核对预览后提交。任一行不合格则整批不执行。
                    </p>
                    <div className="max-h-64 overflow-auto rounded border p-2 text-xs">
                      {batch.map((d, i) => (
                        <p key={i}>
                          {i + 1}.{" "}
                          {String(d.serial_number || d.imei || "缺少标识")} ·{" "}
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
                            "整批设备已入库，等待检测",
                          )
                        )
                          setBatch([])
                      }}
                    >
                      确认导入 {batch.length} 台
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
                <Field label="换机原始销售记录（普通配货留空）">
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
                        setVariantId(findDevice(a.device_id)?.variant_id || "")
                      }
                    }}
                  >
                    <option value="">普通订单配货</option>
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
                            "供应商设备已登记并绑定订单",
                          )
                        ) {
                          setSerial("")
                          setImei("")
                        }
                      }}
                    >
                      登记代发设备并配货
                    </Button>
                  </>
                )}
                {source?.type === "self" && (
                  <p className="text-sm">
                    扫描或从下方选择可售设备，再点击配货。可以从不同来源分批发货。
                  </p>
                )}
              </div>
              <div className="space-y-3 rounded-lg border p-4">
                <h2 className="font-medium">本次订单设备</h2>
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
                      <p className="break-all">替换原机：{a.replacement_of}</p>
                    )}
                    {a.tracking_number && <p>物流：{a.tracking_number}</p>}
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
                          选中创建履约
                        </label>
                        <Button
                          size="small"
                          variant="secondary"
                          onClick={() =>
                            command(
                              { action: "release", assignment_id: a.id },
                              "配货已解除，设备恢复可售",
                            )
                          }
                        >
                          解除配货
                        </Button>
                      </div>
                    )}
                    {a.fulfillment_id && (
                      <p className="break-all text-xs">
                        履约：{a.fulfillment_id}
                      </p>
                    )}
                  </div>
                ))}
                <Button
                  disabled={!checked.length}
                  onClick={() =>
                    command(
                      { action: "fulfill", assignment_ids: checked },
                      "原生订单履约已创建，库存已扣减，请填写物流发货",
                    )
                  }
                >
                  为选中的 {checked.length} 台创建履约
                </Button>
                <Field label="物流单号">
                  <input
                    aria-label="物流单号"
                    className={box}
                    value={tracking}
                    onChange={(e) => setTracking(e.target.value)}
                  />
                </Field>
                <Field label="物流查询链接（可选）">
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
                          "设备与原生订单均已标记发货",
                        )
                      }
                    >
                      发出履约 {id}
                    </Button>
                    <Button
                      variant="secondary"
                      onClick={() =>
                        command(
                          { action: "cancel_fulfillment", fulfillment_id: id },
                          "履约已取消，库存恢复，设备保留配货；可解除或重新创建履约",
                        )
                      }
                    >
                      取消未发出的履约
                    </Button>
                  </div>
                ))}
              </div>
            </>
          )}
          {mode === "inspection" && (
            <>
              <div className="space-y-3 rounded-lg border p-4">
                <h2 className="font-medium">售后审核 / 完成</h2>
                {requests.map((r) => (
                  <div
                    className="space-y-2 rounded border p-3 text-sm"
                    key={r.id}
                  >
                    <p className="break-all">
                      {r.type} · {r.status} · {r.id}
                    </p>
                    <div className="flex flex-wrap gap-2">
                      {r.status === "pending" && (
                        <Button
                          size="small"
                          variant="secondary"
                          onClick={() =>
                            command(
                              { request_id: r.id, status: "processing" },
                              "售后申请已审核，开始处理",
                              `/admin/orders/${orderId}/after-sales`,
                            )
                          }
                        >
                          审核并处理
                        </Button>
                      )}
                      {r.status === "processing" && (
                        <Button
                          size="small"
                          variant="secondary"
                          onClick={() =>
                            command(
                              { request_id: r.id, status: "completed" },
                              "售后申请已完成",
                              `/admin/orders/${orderId}/after-sales`,
                            )
                          }
                        >
                          完成申请
                        </Button>
                      )}
                      {r.status === "pending" && (
                        <Button
                          size="small"
                          variant="secondary"
                          onClick={() =>
                            command(
                              { request_id: r.id, status: "rejected" },
                              "申请已拒绝",
                              `/admin/orders/${orderId}/after-sales`,
                            )
                          }
                        >
                          拒绝申请
                        </Button>
                      )}
                    </div>
                  </div>
                ))}
                {requests.length === 0 && (
                  <p className="text-sm">客户提交申请后可在这里审核处理。</p>
                )}
              </div>
              <div className="space-y-3 rounded-lg border p-4">
                <h2 className="font-medium">售后扫码收回</h2>
                {sourceSelect}
                <Field label="待收回销售记录">
                  <select
                    aria-label="待收回销售记录"
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
                    <option value="">请选择</option>
                    {data.assignments
                      .filter((a) => a.status === "shipped")
                      .map((a) => (
                        <option key={a.id} value={a.id}>
                          {identity(findDevice(a.device_id)!)} · {a.id}
                        </option>
                      ))}
                  </select>
                </Field>
                <Field label="处理中售后申请">
                  <select
                    className={box}
                    value={requestId}
                    onChange={(e) => setRequestId(e.target.value)}
                  >
                    <option value="">请选择已审核的申请</option>
                    {requests
                      .filter((r) => r.status === "processing")
                      .map((r) => (
                        <option key={r.id} value={r.id}>
                          {r.type} · {r.id}
                        </option>
                      ))}
                  </select>
                </Field>
                <p className="break-all text-sm">
                  本次扫描：{scan || "请扫描实际收回机器的序列号 / IMEI"}
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
                      "机器已核对并收回，等待检测；退货已同步原生订单，尚未自动退款",
                    )
                  }
                >
                  核对并收回机器
                </Button>
              </div>
              {selected && (
                <div className="space-y-3 rounded-lg border p-4">
                  <h2 className="font-medium">
                    检测：{identity(selected)} · {states[selected.state]}
                  </h2>
                  {variantSelect}
                  <div className="grid gap-3 md:grid-cols-2">
                    <Field label="检测后成色">
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
                    <Field label="检测后电池健康度">
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
                  <Field label="检测结果">
                    <select
                      aria-label="检测结果"
                      className={box}
                      value={result}
                      onChange={(e) => setResult(e.target.value)}
                    >
                      <option value="pass">
                        通过（退货重新上架 / 维修待返还）
                      </option>
                      <option value="repair">需要维修</option>
                      <option value="retire">报废，不上架</option>
                      <option value="supplier_return">
                        退还原供应商，不上架
                      </option>
                    </select>
                  </Field>
                  <Field label="检测 / 维修说明">
                    <textarea
                      aria-label="检测 / 维修说明"
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
                        "检测结果已保存，可售库存按检测结果同步",
                      )
                    }
                  >
                    保存检测结果
                  </Button>
                  {selected.state === "repair_ready" && (
                    <>
                      <Field label="维修返还物流单号">
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
                              "维修原机已返还，物流记录已保存",
                            )
                        }}
                      >
                        原机维修返还
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
                <h2 className="font-medium">退货退款</h2>
                <p className="text-sm">
                  先选择上方退货申请并收回全部机器，再核对实收余额和退款金额。点击后会通过订单原支付渠道实际退款。
                </p>
                <Field label="退款支付记录">
                  <select
                    className={box}
                    value={paymentId}
                    onChange={(e) => {
                      setPaymentId(e.target.value)
                      setRefundConfirmed(false)
                    }}
                  >
                    <option value="">请选择已收款支付</option>
                    {data.order.payment_collections
                      ?.flatMap((c) => c.payments || [])
                      .map((p) => {
                        const balance =
                          p.captures.reduce((s, c) => s + Number(c.amount), 0) -
                          p.refunds.reduce((s, r) => s + Number(r.amount), 0)
                        return (
                          <option key={p.id} value={p.id}>
                            {p.id} · 剩余可退 {balance.toFixed(2)}{" "}
                            {data.order!.currency_code.toUpperCase()}
                          </option>
                        )
                      })}
                  </select>
                </Field>
                <Field
                  label={`退款金额（${data.order.currency_code.toUpperCase()}）`}
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
                  已核对申请、收回机器和金额，确认执行退款
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
                      "退款已执行并关联售后申请，可以在完成检测后关闭申请",
                      "/admin/devices/refunds",
                    )
                  }
                >
                  确认执行退款
                </Button>
              </div>
            )}
          {mode !== "sources" && (
            <div className="space-y-3">
              <h2 className="font-medium">设备列表（{data.count}）</h2>
              <div className="flex flex-wrap gap-2">
                <input
                  aria-label="搜索设备"
                  className={`${box} flex-1`}
                  placeholder="序列号 / IMEI / 设备编号"
                  value={search}
                  onChange={(e) => {
                    setSearch(e.target.value)
                    setSelected(null)
                    setOffset(0)
                  }}
                />
                {selected && (
                  <Button variant="secondary" onClick={() => setSelected(null)}>
                    返回列表
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
                      {d.grade} · 电池 {d.battery_health}%
                    </p>
                    <p className="text-sm">
                      所在：
                      {data.sources.find((s) => s.location_id === d.location_id)
                        ?.name || d.location_id}{" "}
                      · 仓位 {d.bin || "—"}
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
                        {mode === "inspection" ? "选择检测设备" : "查看设备"}
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
                              "设备已绑定订单",
                            )
                          }
                        >
                          配货到订单
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
                          去检测
                        </Button>
                      )}
                    </div>
                  </div>
                ))}
              </div>
              {!data.devices.length && (
                <p className="text-sm">没有匹配设备。</p>
              )}
              {!selected && (
                <div className="flex gap-2">
                  <Button
                    variant="secondary"
                    disabled={offset === 0}
                    onClick={() => setOffset(Math.max(0, offset - 50))}
                  >
                    上一页
                  </Button>
                  <Button
                    variant="secondary"
                    disabled={offset + 50 >= data.count}
                    onClick={() => setOffset(offset + 50)}
                  >
                    下一页
                  </Button>
                </div>
              )}
              {mode === "lookup" && selected && (
                <div className="space-y-3 rounded-lg border p-4">
                  <h2 className="font-medium">设备完整流水</h2>
                  <p className="text-sm">
                    采购成本：{(selected.cost_cents / 100).toFixed(2)}{" "}
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
                        {new Date(e.created_at).toLocaleString()} · {e.action}
                      </summary>
                      <p className="break-all">操作人：{e.actor_id}</p>
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

export const config = defineRouteConfig({ label: "设备扫码工作台", rank: 5 })
