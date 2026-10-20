import { Button, Checkbox, toast, DataTable, Heading, Select, Text, createDataTableColumnHelper, useDataTable, usePrompt } from "@medusajs/ui"
import { Link } from "react-router-dom"
import { useQueryClient, useQuery } from "@tanstack/react-query"
import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { useDeviceTranslations } from "../lib/use-device-translations"
import { sdk } from "../lib/sdk"
import { notifyError } from "../lib/notifications"
import { ClearableInput } from "./clearable-input"
import DeviceVariantSelect, { type DeviceVariantChoice } from "./device-variant-select"
import DeviceDetailsModal from "./device-details-modal"
import ScanIntakeMenu from "./scan-intake-menu"
import { localizeDeviceMessage } from "../lib/device-messages"
import { buildDeviceExport } from "../lib/device-xlsx"

type LookupDevice = {
  category: string
  can_delete: boolean
  can_retire: boolean
  id: string
  serial_number: string | null
  imei: string | null
  imei2: string | null
  eid: string | null
  variant_id: string
  source_id: string
  location_id: string
  state: string
  grade: string | null
  battery_health: number | null
  cost_cents: number
  currency_code: string
  bin: string
  notes: string
  created_at: string
  updated_at: string
}
type Results = {
  devices: LookupDevice[]
  count: number
  variants: DeviceVariantChoice[]
  sources: { id: string; location_id: string; name: string }[]
}
const helper = createDataTableColumnHelper<LookupDevice>()

export default function DeviceLookupTable({ onIntake, onDirectIntake, intakeOpening, canIntake, initialDeviceId, category }: {
  category: "sale" | "returned" | "recycled"
  onIntake: (method: "image" | "scan" | "import") => void
  onDirectIntake: () => void
  intakeOpening?: boolean
  canIntake: boolean
  initialDeviceId?: string
}) {
  const { t, i18n } = useDeviceTranslations()
  const prompt = usePrompt()
  const promptRef = useRef(prompt)
  promptRef.current = prompt
  const queryClient = useQueryClient()
  const [acting, setActing] = useState<string>()
  const act = useCallback(async (device: LookupDevice, action: "delete" | "retire") => {
    const confirmed = await promptRef.current({ title: t(`deviceScan.${action}Device`), description: t(`deviceScan.${action}Confirm`, { serial: device.serial_number || device.id }), confirmText: t(`deviceScan.${action}Device`), cancelText: t("deviceScan.lookupClose") })
    if (!confirmed) return
    setActing(device.id)
    try {
      await sdk.client.fetch("/admin/devices", { method: "POST", body: { action, device_id: device.id } })
      await queryClient.invalidateQueries({ queryKey: ["device-lookup"] })
      setPagination((previous) => ({ ...previous, pageIndex: 0 }))
    } catch (error) { notifyError(localizeDeviceMessage(error instanceof Error ? error.message : t("deviceScan.operationError"), t)) }
    finally { setActing(undefined) }
  }, [queryClient, t])
  const [exporting, setExporting] = useState(false)
  const [exportCount, setExportCount] = useState(0)
  const [input, setInput] = useState("")
  const [search, setSearch] = useState("")
  const [variant, setVariant] = useState<DeviceVariantChoice>()
  const [state, setState] = useState("all")
  const [location, setLocation] = useState("all")
  const [deviceId, setDeviceId] = useState(initialDeviceId)
  const [editingDevice, setEditingDevice] = useState(false)
  const [pagination, setPagination] = useState({ pageIndex: 0, pageSize: 20 })
  const results = useQuery<Results>({
    queryKey: ["device-lookup", category, search, variant?.id, state, location, pagination],
    queryFn: () => sdk.client.fetch("/admin/devices", {
      query: { category, q: search || undefined, lookup_variant_id: variant?.id,
        lookup_state: state === "all" ? undefined : state,
        lookup_location_id: location === "all" ? undefined : location,
        limit: pagination.pageSize, offset: pagination.pageIndex * pagination.pageSize },
    }),
  })
  useEffect(() => {
    if (results.isError) notifyError(t("deviceScan.loadError"))
  }, [results.isError, t])
  const [selectedIds, setSelectedIds] = useState<string[]>([])
  const [publishing, setPublishing] = useState(false)
  useEffect(() => { setSelectedIds([]) }, [category, search, variant?.id, state, location, pagination.pageIndex, pagination.pageSize])
  const pendingDevices = (results.data?.devices || []).filter((device) => device.state === "pending" && device.category === "sale")
  const selectedDevices = pendingDevices.filter((device) => selectedIds.includes(device.id))
  async function makeAvailable() {
    const ids = selectedDevices.map((device) => device.id)
    if (!ids.length) return
    const confirmed = await promptRef.current({ title: t("deviceScan.makeAvailable"), description: t("deviceScan.makeAvailableConfirm", { count: ids.length }), confirmText: t("deviceScan.makeAvailable"), cancelText: t("deviceScan.lookupClose") })
    if (!confirmed) return
    setPublishing(true)
    try {
      await sdk.client.fetch("/admin/devices", { method: "POST", body: { action: "make_available", device_ids: ids } })
      setSelectedIds([])
      await Promise.all(["device-lookup", "device-details", "devices", "order-device-candidates"].map((key) => queryClient.invalidateQueries({ queryKey: [key] })))
      toast.success(t("deviceScan.makeAvailableSuccess", { count: ids.length }))
    } catch (error) { notifyError(localizeDeviceMessage(error instanceof Error ? error.message : t("deviceScan.operationError"), t)) }
    finally { setPublishing(false) }
  }
  const columns = useMemo(() => {
    const stateKeys: Record<string, string> = { repair_ready: "repairReady", supplier_returned: "supplierReturned" }
    return [
      ...(canIntake && category === "sale" ? [helper.display({ id: "selection", size: 50, header: () => <Checkbox aria-label={t("deviceScan.selectPendingPage")} checked={pendingDevices.length > 0 && pendingDevices.every((device) => selectedIds.includes(device.id))} disabled={publishing || !pendingDevices.length} onCheckedChange={(checked) => setSelectedIds(checked === true ? pendingDevices.map((device) => device.id) : [])} />, cell: ({ row }) => row.original.state === "pending" ? <Checkbox aria-label={t("deviceScan.selectPendingDevice", { serial: row.original.serial_number || row.original.id })} disabled={publishing} checked={selectedIds.includes(row.original.id)} onCheckedChange={(checked) => setSelectedIds((previous) => checked === true ? [...new Set([...previous, row.original.id])] : previous.filter((id) => id !== row.original.id))} /> : null })] : []),
      helper.display({ id: "sequence", size: 75, header: t("deviceScan.lookupSequence"), cell: ({ row }) => (results.data?.count || 0) - pagination.pageIndex * pagination.pageSize - row.index }),
      helper.accessor("serial_number", { size: 155, header: t("orderDevices.serial"), cell: ({ getValue }) => <Text size="small" className="whitespace-normal break-all">{getValue() || "—"}</Text> }),
      helper.accessor("variant_id", { size: 220, header: t("deviceScan.sku"), cell: ({ getValue }) => {
        const sku = results.data?.variants.find((item) => item.id === getValue())
        return <div className="whitespace-normal break-words py-2">
          <Text size="small" weight="plus">{sku?.sku || "—"}</Text>
          <Text size="small" className="text-ui-fg-subtle">{[sku?.product?.title, sku?.title].filter(Boolean).join(" · ")}</Text>
        </div>
      } }),
      helper.accessor("imei", { size: 155, header: t("orderDevices.imei"), cell: ({ getValue }) => <Text size="small" className="whitespace-normal break-all">{getValue() || "—"}</Text> }),
      helper.accessor("imei2", { size: 155, header: t("orderDevices.imei2"), cell: ({ getValue }) => <Text size="small" className="whitespace-normal break-all">{getValue() || "—"}</Text> }),
      helper.accessor("eid", { size: 220, header: t("orderDevices.eid"), cell: ({ getValue }) => <Text size="small" className="whitespace-normal break-all">{getValue() || "—"}</Text> }),
      helper.accessor("state", { size: 100, header: t("deviceScan.lookupState"), cell: ({ getValue }) => t(`deviceScan.${stateKeys[getValue()] || getValue()}`) }),
      helper.accessor("grade", { size: 75, header: t("deviceScan.grade"), cell: ({ getValue }) => getValue() || t("orderDevices.notRecorded") }),
      helper.accessor("battery_health", { size: 85, header: t("deviceScan.lookupBattery"), cell: ({ getValue }) => getValue() == null ? t("orderDevices.notRecorded") : `${getValue()}%` }),
      helper.accessor("location_id", { size: 120, header: t("deviceScan.lookupLocation"), cell: ({ getValue }) => <Text size="small" className="whitespace-normal break-words">{results.data?.sources.find((source) => source.location_id === getValue())?.name || getValue()}</Text> }),
      helper.accessor("bin", { size: 80, header: t("deviceScan.bin"), cell: ({ getValue }) => getValue() || "—" }),
      helper.accessor("created_at", { size: 180, header: t("deviceScan.lookupCreated"), cell: ({ getValue }) => <Text size="small" className="whitespace-nowrap">{getValue() ? new Date(getValue()).toLocaleString(i18n.language.startsWith("zh") ? "zh-CN" : "en-GB", { year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false }) : "—"}</Text> }),
      helper.display({ id: "actions", size: 230, header: t("deviceScan.lookupActions"), cell: ({ row }) =>
        <div className="flex w-full flex-wrap items-center justify-center gap-x-3 gap-y-1 py-2 text-sm">
          <a className="whitespace-nowrap text-ui-fg-interactive hover:underline" href={`?device_id=${encodeURIComponent(row.original.id)}`} onClick={(event) => { event.preventDefault(); setEditingDevice(false); setDeviceId(row.original.id) }}>{t("deviceScan.viewDevice")}</a>
          {canIntake && <Button size="small" variant="secondary" onClick={() => { setEditingDevice(true); setDeviceId(row.original.id) }}>{t("deviceScan.editDevice")}</Button>}
          {canIntake && ["inspection", "repair"].includes(row.original.state) && <Link className="whitespace-nowrap text-ui-fg-interactive hover:underline" to="/device-scan/inspection" state={{ device: row.original }}>{t("deviceScan.inspectDevice")}</Link>}
          {row.original.can_retire && <a className="whitespace-nowrap text-ui-fg-interactive hover:underline aria-disabled:cursor-wait aria-disabled:opacity-50" href="#" aria-disabled={!!acting} onClick={(event) => { event.preventDefault(); if (!acting) void act(row.original, "retire") }}>{t("deviceScan.retireDevice")}</a>}
          {row.original.can_delete && <a className="whitespace-nowrap text-ui-fg-error hover:underline aria-disabled:cursor-wait aria-disabled:opacity-50" href="#" aria-disabled={!!acting} onClick={(event) => { event.preventDefault(); if (!acting) void act(row.original, "delete") }}>{t("deviceScan.deleteDevice")}</a>}
        </div> }),
    ]
  }, [results.data?.variants, results.data?.sources, results.data?.count, pagination.pageIndex, pagination.pageSize, t, i18n.language, canIntake, acting, act, category, pendingDevices, selectedIds, publishing])
  const table = useDataTable({
    data: results.data?.devices || [], columns, getRowId: (device) => device.id,
    rowCount: results.data?.count || 0, isLoading: results.isPending,
    pagination: { state: pagination, onPaginationChange: setPagination },
  })
  async function exportDevices() {
    setExporting(true)
    setExportCount(0)
    try {
      const rows: Record<string, string | number | null>[] = []
      const seen = new Set<string>()
      let offset = 0
      while (true) {
        if (offset > 100000) throw new Error("Export exceeds pagination range")
        const page = await sdk.client.fetch<Results>("/admin/devices", { query: {
          category, q: search || undefined, lookup_variant_id: variant?.id,
          lookup_state: state === "all" ? undefined : state,
          lookup_location_id: location === "all" ? undefined : location,
          limit: 100, offset,
        } })
        for (const device of page.devices) {
          if (seen.has(device.id)) throw new Error("Device list changed during export")
          seen.add(device.id)
          const sku = page.variants.find((entry) => entry.id === device.variant_id)
          const stateKey = ({ repair_ready: "repairReady", supplier_returned: "supplierReturned" } as Record<string, string>)[device.state] || device.state
          rows.push({
            id: device.id, serial_number: device.serial_number, imei: device.imei, imei2: device.imei2, eid: device.eid,
            sku: sku?.sku || "", product: [sku?.product?.title, sku?.title].filter(Boolean).join(" · "),
            state: t(`deviceScan.${stateKey}`), grade: device.grade, battery_health: device.battery_health,
            location: page.sources.find((entry) => entry.location_id === device.location_id)?.name || device.location_id,
            source: page.sources.find((entry) => entry.id === device.source_id)?.name || device.source_id,
            cost: device.cost_cents / 100, currency_code: device.currency_code.toUpperCase(),
            bin: device.bin, notes: device.notes || "", created_at: device.created_at, updated_at: device.updated_at,
          })
        }
        offset += page.devices.length
        setExportCount(rows.length)
        if (offset >= page.count || !page.devices.length) break
      }
      const fields = [
        ["id", "lookupId"], ["serial_number", "serial"], ["imei", "imei"], ["imei2", "imei2"], ["eid", "eid"],
        ["sku", "sku"], ["product", "lookupProduct"], ["state", "lookupState"], ["grade", "grade"],
        ["battery_health", "battery"], ["location", "lookupLocation"], ["source", "lookupSource"],
        ["cost", "cost"], ["currency_code", "lookupCurrency"], ["bin", "bin"], ["notes", "intakeNotes"],
        ["created_at", "lookupCreated"], ["updated_at", "lookupUpdated"],
      ]
      const buffer = await buildDeviceExport(fields.map(([key, label]) => ({
        key, header: t(`${(key === "imei" || key === "imei2" || key === "eid") ? "orderDevices" : "deviceScan"}.${label}`),
        width: key === "id" || key === "imei" || key === "imei2" || key === "eid" ? 36 : 24,
        numFmt: key === "cost" ? "0.00" : key === "battery_health" ? "0.##" : "@",
      })), rows)
      const url = URL.createObjectURL(new Blob([buffer], { type: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" }))
      const link = document.createElement("a")
      link.href = url
      link.download = `devices-${new Date().toISOString().slice(0, 10)}-${i18n.language}.xlsx`
      document.body.appendChild(link)
      link.click()
      link.remove()
      setTimeout(() => URL.revokeObjectURL(url), 60000)
    } catch {
      notifyError(t("deviceScan.lookupExportError"))
    } finally {
      setExporting(false)
    }
  }
  return <div className="space-y-4" data-testid="device-lookup-table">
    <div className="flex flex-wrap items-center justify-between gap-3">
      <Heading level="h1">{t(category === "sale" ? "deviceScan.lookup" : `deviceScan.${category}Devices`)}</Heading>
      <div className="flex flex-wrap gap-2">
        <Button size="small" variant="secondary" isLoading={exporting} disabled={results.isPending || results.isError || !results.data?.count || exporting} onClick={() => void exportDevices()}>
          {exporting ? t("deviceScan.lookupExporting", { count: exportCount }) : t("deviceScan.lookupExport")}
        </Button>
        {canIntake && <>
        <Button size="small" variant="secondary" onClick={() => onIntake("import")}>{t("deviceScan.lookupImportIntake")}</Button>
        <Button size="small" variant="secondary" onClick={() => onIntake("image")}>{t("deviceScan.lookupImageIntake")}</Button>
        <ScanIntakeMenu onDirect={onDirectIntake} onQr={() => onIntake("scan")} busy={intakeOpening} />
        </>}
      </div>
    </div>
    <form style={{ display: "flex", flexWrap: "wrap", alignItems: "flex-end", gap: 12 }} onSubmit={(event) => {
        event.preventDefault()
        setSearch(input.trim())
        setPagination((previous) => ({ ...previous, pageIndex: 0 }))
        if (search === input.trim() && pagination.pageIndex === 0) void results.refetch()
      }}>
      <div className="min-w-0 space-y-1" style={{ flex: "2 1 280px" }}>
        <Text size="small" weight="plus">{t("deviceScan.search")}</Text>
        <div className="flex gap-2">
          <ClearableInput medusa className="flex-1" aria-label={t("deviceScan.search")} placeholder={t("deviceScan.searchPlaceholder")}
            value={input} onChange={(event) => setInput(event.target.value)} />
          <Button size="small" className="shrink-0 whitespace-nowrap" type="submit" isLoading={results.isFetching}>{t("deviceScan.lookupSearch")}</Button>
        </div>
      </div>
      <div className="min-w-0" style={{ flex: "2 1 260px" }}>
      <DeviceVariantSelect value={variant} onChange={(next) => {
        setVariant(next)
        setPagination((previous) => ({ ...previous, pageIndex: 0 }))
      }} />
      </div>
      <div className="space-y-1" style={{ flex: "1 1 140px" }}>
        <Text size="small" weight="plus">{t("deviceScan.lookupState")}</Text>
        <Select value={state} onValueChange={(value) => { setState(value); setPagination((previous) => ({ ...previous, pageIndex: 0 })) }}>
          <Select.Trigger aria-label={t("deviceScan.lookupState")}><Select.Value /></Select.Trigger>
          <Select.Content><Select.Item value="all">{t("deviceScan.lookupAllStates")}</Select.Item>
            {(category === "sale" ? ["pending", "inspection", "available", "reserved", "packed", "shipped", "retired"] : ["inspection", "repair", "repair_ready", "retired", "supplier_returned"]).map((value) =>
              <Select.Item key={value} value={value}>{t(`deviceScan.${({ repair_ready: "repairReady", supplier_returned: "supplierReturned" } as Record<string, string>)[value] || value}`)}</Select.Item>)}
          </Select.Content>
        </Select>
      </div>
      <div className="space-y-1" style={{ flex: "1 1 160px" }}>
        <Text size="small" weight="plus">{t("deviceScan.lookupLocation")}</Text>
        <Select value={location} onValueChange={(value) => { setLocation(value); setPagination((previous) => ({ ...previous, pageIndex: 0 })) }}>
          <Select.Trigger aria-label={t("deviceScan.lookupLocation")}><Select.Value /></Select.Trigger>
          <Select.Content><Select.Item value="all">{t("deviceScan.lookupAllLocations")}</Select.Item>
            {results.data?.sources.map((source) => <Select.Item key={source.location_id} value={source.location_id}>{source.name}</Select.Item>)}
          </Select.Content>
        </Select>
      </div>
    </form>
    <div className="flex flex-wrap items-center justify-between gap-2">
      <Text size="small" className="text-ui-fg-subtle">{t("deviceScan.deviceList", { count: results.data?.count || 0 })}</Text>
      {canIntake && category === "sale" && <Button size="small" variant="secondary" disabled={!selectedDevices.length || publishing || results.isFetching} isLoading={publishing} onClick={() => void makeAvailable()}>{t("deviceScan.makeAvailableSelected", { count: selectedDevices.length })}</Button>}
      <Button size="small" variant="secondary" onClick={() => {
        setInput(""); setSearch(""); setVariant(undefined)
        setState("all"); setLocation("all")
        setPagination((previous) => ({ ...previous, pageIndex: 0 }))
      }}>{t("deviceScan.lookupReset")}</Button>
    </div>
    {results.isError ? <Button size="small" variant="secondary" onClick={() => results.refetch()}>{t("orderDevices.retry")}</Button> :
      <div className="[&_table]:min-w-[1600px] [&_th:last-child]:sticky [&_th:last-child]:right-0 [&_th:last-child]:z-10 [&_th:last-child]:bg-ui-bg-subtle [&_td:last-child]:sticky [&_td:last-child]:right-0 [&_td:last-child]:bg-ui-bg-base [&_td:last-child]:border-l"><DataTable instance={table}>
        <DataTable.Table emptyState={{ empty: { heading: t("deviceScan.noDevices") } }} />
        <DataTable.Pagination translations={{ of: t("general.of"), results: t("deviceScan.lookupResults"), pages: t("deviceScan.lookupPages"), prev: t("deviceScan.previous"), next: t("deviceScan.next") }} />
      </DataTable></div>}
    {deviceId && <DeviceDetailsModal deviceId={deviceId} initialEditing={editingDevice} onClose={() => setDeviceId(undefined)} />}
  </div>
}
