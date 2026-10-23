import { Button, Container, DataTable, FocusModal, Heading, Input, Label, Select, Text, Textarea, createDataTableColumnHelper, toast, useDataTable, usePrompt } from "@medusajs/ui"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useEffect, useMemo, useState, type ReactNode } from "react"
import { Link, useSearchParams } from "react-router-dom"
import { sdk } from "../lib/sdk"
import { notifyError } from "../lib/notifications"
import { useBuybackTranslations } from "../lib/use-buyback-translations"
import type { BuybackAdminCommand } from "../../api/buyback-validators"
import type { RecycledAssessment } from "../../lib/recycled-assessment"
import RecycledAssessmentSummary from "./recycled-assessment-summary"
import RecycledDeviceIntake from "./recycled-device-intake"

type Request = {
  id: string; status: string; contact_name: string; email: string; phone: string; currency_code: string
  reference_amount: number | null; final_amount: number | null; created_at: string
  return_address: Record<string, string>; shipping_instructions: string; tracking_number: string
  return_tracking: string; payment_reference: string; review_note: string
  history: { at: string; action: string; status: string; note?: string }[]
  items: { id: string; model_name: string; capacity: string; assessment: RecycledAssessment; serial_number: string; imei: string; device_id: string | null; reference_amount: number | null; final_amount: number | null; inspection_notes: string }[]
  photos: { id: string; item_id: string; photo_type: string; review_status: string; review_note: string; uploaded_at: string; mime_type: string }[]
}
type Photo = Request["photos"][number]
const statuses = ["awaiting_photos", "photo_review", "ready_to_ship", "in_transit", "received", "final_offer", "accepted", "paid", "return_requested", "returning", "returned", "canceled"]
const selectClass = "w-full min-w-0 rounded-md border border-ui-border-base bg-ui-bg-field px-3 py-2 text-sm focus:border-ui-border-interactive focus:shadow-borders-interactive-with-focus"
const field = (label: string, children: ReactNode) => <Label className="block min-w-0 space-y-2"><Text size="small" weight="plus">{label}</Text>{children}</Label>
const errorCode = (error: Error) => error.message.match(/BUYBACK_[A-Z_]+/)?.[0] || "BUYBACK_REQUEST_FAILED"

function PhotoReview({ requestId, photo, busy, review }: { requestId: string; photo: Photo; busy: boolean; review: (command: BuybackAdminCommand) => void }) {
  const { t } = useBuybackTranslations()
  const [note, setNote] = useState("")
  const query = useQuery({ queryKey: ["buyback-photo", requestId, photo.id], queryFn: () => sdk.client.fetch<{ content: string; mime_type: string }>(`/admin/buyback/${requestId}/photos/${photo.id}`), enabled: photo.review_status !== "expired" })
  return <div className="min-w-0 space-y-3 rounded-md bg-ui-bg-subtle p-4">
    <Text size="small" weight="plus">{t(`buyback.photoTypes.${photo.photo_type}`)}</Text>
    {query.data ? <img src={`data:${query.data.mime_type};base64,${query.data.content}`} className="h-48 w-full object-contain" alt={t(`buyback.photoTypes.${photo.photo_type}`)} /> : photo.review_status === "expired" ? null : query.isError ? <Button size="small" variant="secondary" onClick={() => query.refetch()}>{t("buyback.retry")}</Button> : <Text role="status">{t("buyback.loading")}</Text>}
    <Text size="small">{t(`buyback.reviews.${photo.review_status}`)}</Text>
    <Text size="small" className="text-ui-fg-subtle">{new Date(String(photo.uploaded_at)).toLocaleString()}</Text>
    {photo.review_note && <Text size="small">{photo.review_note}</Text>}
    {photo.review_status !== "expired" && <>
      {field(t("buyback.note"), <Input value={note} maxLength={500} disabled={busy} aria-label={t("buyback.note")} onChange={(event) => setNote(event.target.value)} />)}
      <div className="flex flex-wrap gap-2"><Button size="small" variant="secondary" disabled={busy} onClick={() => review({ action: "review_photo", photo_id: photo.id, review_status: "approved", note })}>{t("buyback.approve")}</Button><Button size="small" variant="secondary" disabled={busy || note.trim().length < 3} onClick={() => review({ action: "review_photo", photo_id: photo.id, review_status: "retake", note: note.trim() })}>{t("buyback.retake")}</Button></div>
    </>}
  </div>
}

function RequestEditor({ request, onClose }: { request: Request; onClose: () => void }) {
  const { t, i18n } = useBuybackTranslations()
  const client = useQueryClient()
  const prompt = usePrompt()
  const [note, setNote] = useState("")
  const [instructions, setInstructions] = useState(request.shipping_instructions)
  const [reference, setReference] = useState("")
  const [tracking, setTracking] = useState("")
  const [registering, setRegistering] = useState<Request["items"][number]>()
  const [links, setLinks] = useState<Record<string, string>>({})
  const [offers, setOffers] = useState(Object.fromEntries(request.items.map((item) => [item.id, { amount: item.final_amount == null ? "" : String(item.final_amount), notes: item.inspection_notes }])))
  const mutation = useMutation({
    mutationFn: (command: BuybackAdminCommand) => sdk.client.fetch<{ request: Request }>(`/admin/buyback/${request.id}`, { method: "POST", body: command }),
    onSuccess: async (data) => { client.setQueryData(["buyback-request", request.id], data); await client.invalidateQueries({ queryKey: ["buyback-requests"] }); toast.success(t("buyback.success")) },
    onError: (error: Error) => notifyError(t(`buyback.errors.${errorCode(error)}`)),
  })
  const busy = mutation.isPending
  const money = (value: unknown) => value == null ? t("buyback.pricePending") : new Intl.NumberFormat(i18n.language.startsWith("zh") ? "zh-CN" : "en-GB", { style: "currency", currency: request.currency_code }).format(Number(value))
  const address = request.return_address as { name: string; address_1: string; address_2: string; postal_code: string; city: string; country_code: string }
  const review = ["awaiting_photos", "photo_review"].includes(request.status)
  const canOffer = request.items.every((item) => item.device_id && offers[item.id]?.amount !== "" && Number.isFinite(Number(offers[item.id]?.amount)) && Number(offers[item.id]?.amount) >= 0 && offers[item.id]?.notes.trim().length >= 3)
  async function paid() {
    if (await prompt({ title: t("buyback.paid"), description: `${t("buyback.paidHelp")} ${money(request.final_amount)}`, confirmText: t("buyback.confirmChoice"), cancelText: t("buyback.cancelChoice") })) mutation.mutate({ action: "mark_paid", payment_reference: reference.trim() })
  }
  return <FocusModal open onOpenChange={(open) => { if (!open && !busy && !registering) onClose() }}>
    <FocusModal.Content aria-describedby={undefined} onEscapeKeyDown={(event) => { if (busy || registering) event.preventDefault() }}>
      <FocusModal.Header><FocusModal.Title className="min-w-0 break-words">{t("buyback.adminTitle")} · {request.id}</FocusModal.Title><Button size="small" variant="secondary" className="shrink-0 whitespace-nowrap" disabled={busy || !!registering} onClick={onClose}>{t("deviceScan.lookupClose")}</Button></FocusModal.Header>
      <FocusModal.Body className="overflow-y-auto px-6 py-4"><div className="mx-auto w-full max-w-5xl space-y-6" data-testid="buyback-application-details">
        <section className="space-y-3"><Heading level="h2">{t(`buyback.statuses.${request.status}`)}</Heading><Text size="small">{t("buyback.reference")}: {money(request.reference_amount)} · {t("buyback.final")}: {request.final_amount == null ? "—" : money(request.final_amount)}</Text><Text size="small" className="text-ui-fg-subtle">{t("buyback.referenceHelp")}</Text>{request.review_note && <Text size="small">{t("buyback.reviewNote")}: {request.review_note}</Text>}</section>
        <section className="space-y-2 rounded-md bg-ui-bg-subtle p-4"><Text weight="plus">{t("buyback.contact")}</Text><Text size="small">{request.contact_name} · {request.email} · {request.phone}</Text><Text size="small" className="whitespace-pre-line">{[address.name, address.address_1, address.address_2, `${address.postal_code} ${address.city}`, address.country_code.toUpperCase()].filter(Boolean).join("\n")}</Text><Text size="small" className="text-ui-fg-subtle">{t("buyback.originalAddress")}</Text></section>
        {request.items.map((item, index) => <section key={item.id} className="space-y-4">
          <Heading level="h2">{index + 1}. {item.model_name} · {item.capacity}</Heading>
          <Text size="small">{t("buyback.serial")}: {item.serial_number || "—"} · {t("buyback.imei")}: {item.imei || "—"}</Text>
          <Text size="small">{t("buyback.reference")}: {money(item.reference_amount)} · {t("buyback.final")}: {item.final_amount == null ? "—" : money(item.final_amount)}</Text>
          <RecycledAssessmentSummary value={item.assessment} touchId={item.model_name.includes("SE")} />
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">{request.photos.filter((photo) => photo.item_id === item.id).map((photo) => review ? <PhotoReview key={photo.id} requestId={request.id} photo={photo} busy={busy} review={(command) => mutation.mutate(command)} /> : <ReadOnlyPhoto key={photo.id} requestId={request.id} photo={photo} />)}</div>
          {request.status === "received" && <>
            {item.device_id ? <Link className="text-sm text-ui-fg-interactive underline" to={`/device-scan/recycled?device_id=${item.device_id}`}>{item.device_id}</Link> : <div className="space-y-3 rounded-md bg-ui-bg-subtle p-4"><Text size="small">{t("buyback.linkHelp")}</Text><div className="flex flex-wrap items-end gap-3"><Button size="small" disabled={busy} onClick={() => setRegistering(item)}>{t("buyback.register")}</Button><div className="min-w-0 flex-1">{field(t("buyback.deviceId"), <Input value={links[item.id] || ""} maxLength={100} disabled={busy} aria-label={t("buyback.deviceId")} onChange={(event) => setLinks({ ...links, [item.id]: event.target.value })} />)}</div><Button size="small" variant="secondary" disabled={busy || !links[item.id]?.trim()} onClick={() => mutation.mutate({ action: "link_device", item_id: item.id, device_id: links[item.id].trim() })}>{t("buyback.link")}</Button></div></div>}
            <div className="grid gap-4 sm:grid-cols-2">{field(t("buyback.final"), <Input type="number" min={0} max={1000000} step="0.01" value={offers[item.id]?.amount || ""} disabled={busy} aria-label={`${t("buyback.final")} ${index + 1}`} onChange={(event) => setOffers({ ...offers, [item.id]: { ...offers[item.id], amount: event.target.value } })} />)}{field(t("buyback.inspection"), <Textarea maxLength={1000} value={offers[item.id]?.notes || ""} disabled={busy} aria-label={`${t("buyback.inspection")} ${index + 1}`} onChange={(event) => setOffers({ ...offers, [item.id]: { ...offers[item.id], notes: event.target.value } })} />)}</div>
          </>}
          {item.inspection_notes && <Text size="small" className="whitespace-pre-line">{item.inspection_notes}</Text>}
        </section>)}
        <fieldset disabled={busy} className="space-y-4">
          {request.status === "photo_review" && <>{field(t("buyback.shipping"), <Textarea aria-label={t("buyback.shipping")} maxLength={2000} value={instructions} onChange={(event) => setInstructions(event.target.value)} />)}<Button size="small" disabled={busy || instructions.trim().length < 5} onClick={() => mutation.mutate({ action: "approve_shipping", shipping_instructions: instructions.trim() })}>{t("buyback.approveShipping")}</Button></>}
          {request.shipping_instructions && <Text size="small" className="whitespace-pre-line">{request.shipping_instructions}</Text>}
          {request.tracking_number && <Text size="small">{t("buyback.tracking")}: {request.tracking_number}</Text>}
          {["ready_to_ship", "in_transit"].includes(request.status) && <Button size="small" disabled={busy} onClick={() => mutation.mutate({ action: "mark_received" })}>{t("buyback.markReceived")}</Button>}
          {request.status === "received" && <Button size="small" disabled={busy || !canOffer} onClick={() => mutation.mutate({ action: "final_offer", items: request.items.map((item) => ({ item_id: item.id, amount: Number(offers[item.id].amount), notes: offers[item.id].notes.trim() })) })}>{t("buyback.offer")}</Button>}
          {request.status === "accepted" && <><Text size="small">{t("buyback.paidHelp")}</Text>{field(t("buyback.payment"), <Input maxLength={200} aria-label={t("buyback.payment")} value={reference} onChange={(event) => setReference(event.target.value)} />)}<Button size="small" disabled={busy || reference.trim().length < 3} onClick={() => void paid()}>{t("buyback.paid")}</Button></>}
          {request.payment_reference && <Text size="small">{t("buyback.payment")}: {request.payment_reference}</Text>}
          {request.status === "return_requested" && <><Text size="small">{t("buyback.returnHelp")}</Text>{field(t("buyback.returnTracking"), <Input maxLength={150} aria-label={t("buyback.returnTracking")} value={tracking} onChange={(event) => setTracking(event.target.value)} />)}<Button size="small" disabled={busy || tracking.trim().length < 3} onClick={() => mutation.mutate({ action: "mark_returning", return_tracking: tracking.trim() })}>{t("buyback.returning")}</Button></>}
          {request.return_tracking && <Text size="small">{t("buyback.returnTracking")}: {request.return_tracking}</Text>}
          {request.status === "returning" && <Button size="small" disabled={busy} onClick={() => mutation.mutate({ action: "mark_returned" })}>{t("buyback.returned")}</Button>}
          {["received", "final_offer", "awaiting_photos", "photo_review", "ready_to_ship"].includes(request.status) && <>{field(t("buyback.note"), <Textarea maxLength={1000} aria-label={t("buyback.note")} value={note} onChange={(event) => setNote(event.target.value)} />)}<Button size="small" variant="secondary" disabled={busy || note.trim().length < 3} onClick={() => mutation.mutate(["received", "final_offer"].includes(request.status) ? { action: "request_return", note: note.trim() } : { action: "cancel", note: note.trim() })}>{t(["received", "final_offer"].includes(request.status) ? "buyback.requestReturn" : "buyback.cancel")}</Button></>}
        </fieldset>
        <section className="space-y-2"><Heading level="h2">{t("buyback.history")}</Heading>{(Array.isArray(request.history) ? request.history : []).map((event, index) => <Text key={index} size="small">{new Date(String(event.at)).toLocaleString(i18n.language.startsWith("zh") ? "zh-CN" : "en-GB")} · {t(`buyback.statuses.${event.status}`)}{event.note ? ` · ${event.note}` : ""}</Text>)}</section>
        {registering && <RecycledDeviceIntake initial={{ modelName: registering.model_name, capacity: registering.capacity, assessment: registering.assessment, serial: registering.serial_number, imei: registering.imei }} onClose={() => setRegistering(undefined)} onCreated={async (id) => { setLinks((previous) => ({ ...previous, [registering.id]: id })); await mutation.mutateAsync({ action: "link_device", item_id: registering.id, device_id: id }) }} />}
      </div></FocusModal.Body>
    </FocusModal.Content>
  </FocusModal>
}

function ReadOnlyPhoto({ requestId, photo }: { requestId: string; photo: Photo }) {
  const { t } = useBuybackTranslations()
  const query = useQuery({ queryKey: ["buyback-photo", requestId, photo.id], queryFn: () => sdk.client.fetch<{ content: string; mime_type: string }>(`/admin/buyback/${requestId}/photos/${photo.id}`), enabled: photo.review_status !== "expired" })
  return <div className="space-y-2"><Text size="small">{t(`buyback.photoTypes.${photo.photo_type}`)}</Text>{query.data ? <img className="h-40 w-full object-contain" src={`data:${query.data.mime_type};base64,${query.data.content}`} alt={t(`buyback.photoTypes.${photo.photo_type}`)} /> : query.isError ? <Button size="small" variant="secondary" onClick={() => query.refetch()}>{t("buyback.retry")}</Button> : <Text size="small">{t(photo.review_status === "expired" ? "buyback.reviews.expired" : "buyback.loading")}</Text>}<Text size="small">{t(`buyback.reviews.${photo.review_status}`)}</Text></div>
}

function ApplicationDetails({ id, onClose }: { id: string; onClose: () => void }) {
  const { t } = useBuybackTranslations()
  const query = useQuery({ queryKey: ["buyback-request", id], queryFn: () => sdk.client.fetch<{ request: Request }>(`/admin/buyback/${id}`) })
  useEffect(() => { if (query.isError) notifyError(t("buyback.errors.BUYBACK_REQUEST_FAILED")) }, [query.isError, t])
  return query.data ? <RequestEditor request={query.data.request} onClose={onClose} /> : <FocusModal open onOpenChange={(open) => { if (!open) onClose() }}><FocusModal.Content aria-describedby={undefined}><FocusModal.Header><FocusModal.Title>{t("buyback.adminTitle")}</FocusModal.Title></FocusModal.Header><FocusModal.Body className="p-6">{query.isError ? <Button size="small" variant="secondary" onClick={() => query.refetch()}>{t("buyback.retry")}</Button> : <Text role="status">{t("buyback.loading")}</Text>}</FocusModal.Body></FocusModal.Content></FocusModal>
}

function ReferencePrices({ onClose }: { onClose: () => void }) {
  const { t } = useBuybackTranslations()
  const client = useQueryClient()
  const [productId, setProductId] = useState("")
  const [capacity, setCapacity] = useState("")
  const [amount, setAmount] = useState("")
  const models = useQuery({ queryKey: ["buyback-models"], queryFn: async () => {
    const products = []
    let offset = 0
    while (true) { const page = await sdk.admin.product.list({ fields: "id,title,*options,*options.values", limit: 100, offset }); products.push(...page.products.filter((product) => /^iPhone\b/i.test(product.title))); offset += page.products.length; if (!page.products.length || offset >= page.count) break }
    return products.sort((a, b) => b.title.localeCompare(a.title, "en", { numeric: true }))
  } })
  const prices = useQuery({ queryKey: ["buyback-prices", productId], queryFn: () => sdk.client.fetch<{ prices: { capacity: string; amount: number }[] }>("/admin/buyback-prices", { query: { product_id: productId } }), enabled: Boolean(productId) })
  const model = models.data?.find((product) => product.id === productId)
  const capacities = [...new Set((model?.options || []).filter((option) => /storage|capacity|memory|容量/i.test(option.title)).flatMap((option) => option.values || []).map((value) => value.value.replace(/\s+/g, "").toUpperCase()).filter((value) => /^\d+(GB|TB)$/.test(value)))]
  const mutation = useMutation({ mutationFn: () => sdk.client.fetch("/admin/buyback-prices", { method: "POST", body: { product_id: productId, capacity, amount: Number(amount) } }), onSuccess: async () => { await client.invalidateQueries({ queryKey: ["buyback-prices"] }); toast.success(t("buyback.success")) }, onError: (error: Error) => notifyError(t(`buyback.errors.${errorCode(error)}`)) })
  useEffect(() => { if (models.isError || prices.isError) notifyError(t("buyback.errors.BUYBACK_REQUEST_FAILED")) }, [models.isError, prices.isError, t])
  return <FocusModal open onOpenChange={(open) => { if (!open && !mutation.isPending) onClose() }}><FocusModal.Content aria-describedby={undefined}><FocusModal.Header><FocusModal.Title>{t("buyback.prices")}</FocusModal.Title><Button size="small" variant="secondary" disabled={mutation.isPending} onClick={onClose}>{t("deviceScan.lookupClose")}</Button></FocusModal.Header><FocusModal.Body className="overflow-y-auto p-6"><div className="mx-auto max-w-2xl space-y-6"><Text size="small">{t("buyback.pricesHelp")}</Text>{models.isPending ? <Text>{t("buyback.loading")}</Text> : models.isError ? <Button size="small" variant="secondary" onClick={() => models.refetch()}>{t("buyback.retry")}</Button> : <form className="space-y-4" onSubmit={(event) => { event.preventDefault(); mutation.mutate() }}>
    {field(t("buyback.model"), <select className={selectClass} value={productId} aria-label={t("buyback.model")} disabled={mutation.isPending} required onChange={(event) => { setProductId(event.target.value); setCapacity(""); setAmount("") }}><option value="">{t("buyback.select")}</option>{models.data?.map((product) => <option value={product.id} key={product.id}>{product.title}</option>)}</select>)}
    {field(t("buyback.capacity"), <select className={selectClass} value={capacity} aria-label={t("buyback.capacity")} disabled={mutation.isPending || !productId} required onChange={(event) => { setCapacity(event.target.value); setAmount("") }}><option value="">{t("buyback.select")}</option>{capacities.map((value) => <option key={value}>{value}</option>)}</select>)}
    <Text size="small">{t("buyback.reference")}: {prices.isFetching ? t("buyback.loading") : prices.data?.prices.find((price) => price.capacity === capacity)?.amount ?? t("buyback.noPrice")}</Text>
    {field(t("buyback.amount"), <Input aria-label={t("buyback.amount")} type="number" min={0} max={1000000} step="0.01" required value={amount} disabled={mutation.isPending} onChange={(event) => setAmount(event.target.value)} />)}
    <Button size="small" type="submit" isLoading={mutation.isPending} disabled={mutation.isPending || !productId || !capacity || amount === ""}>{t("buyback.save")}</Button>
    </form>}</div></FocusModal.Body></FocusModal.Content></FocusModal>
}

const helper = createDataTableColumnHelper<Request>()
export default function BuybackApplications() {
  const { t, i18n } = useBuybackTranslations()
  const [params, setParams] = useSearchParams()
  const [input, setInput] = useState("")
  const [search, setSearch] = useState("")
  const [status, setStatus] = useState("all")
  const [pagination, setPagination] = useState({ pageIndex: 0, pageSize: 20 })
  const [pricesOpen, setPricesOpen] = useState(false)
  const id = params.get("request_id")
  const query = useQuery({ queryKey: ["buyback-requests", search, status, pagination.pageIndex], queryFn: () => sdk.client.fetch<{ requests: Request[]; count: number }>("/admin/buyback", { query: { q: search || undefined, status: status === "all" ? undefined : status, offset: pagination.pageIndex * 20 } }), refetchInterval: 30000 })
  useEffect(() => { if (query.isError) notifyError(t("buyback.errors.BUYBACK_REQUEST_FAILED")) }, [query.isError, t])
  const columns = useMemo(() => [
    helper.accessor("id", { header: t("buyback.adminTitle"), cell: ({ getValue }) => <Button size="small" variant="transparent" className="text-ui-fg-interactive underline" onClick={() => setParams({ request_id: getValue() })}>{getValue()}</Button> }),
    helper.accessor("contact_name", { header: t("buyback.customer"), cell: ({ row }) => <div className="whitespace-normal py-2"><Text size="small" weight="plus">{row.original.contact_name}</Text><Text size="small" className="text-ui-fg-subtle">{row.original.email}</Text></div> }),
    helper.accessor("items", { header: t("buyback.devices"), cell: ({ getValue }) => <Text size="small" className="min-w-40 whitespace-normal">{getValue().map((item) => `${item.model_name} ${item.capacity}`).join(" · ")}</Text> }),
    helper.accessor("status", { header: t("deviceScan.lookupState"), cell: ({ getValue }) => <Text size="small" className="whitespace-normal">{t(`buyback.statuses.${getValue()}`)}</Text> }),
    helper.accessor("reference_amount", { header: t("buyback.reference"), cell: ({ getValue }) => getValue() == null ? t("buyback.pricePending") : new Intl.NumberFormat(i18n.language.startsWith("zh") ? "zh-CN" : "en-GB", { style: "currency", currency: "EUR" }).format(Number(getValue())) }),
    helper.accessor("final_amount", { header: t("buyback.final"), cell: ({ getValue }) => getValue() == null ? "—" : new Intl.NumberFormat(i18n.language.startsWith("zh") ? "zh-CN" : "en-GB", { style: "currency", currency: "EUR" }).format(Number(getValue())) }),
    helper.accessor("created_at", { header: t("buyback.created"), cell: ({ getValue }) => <Text size="small" className="whitespace-nowrap">{new Date(String(getValue())).toLocaleString(i18n.language.startsWith("zh") ? "zh-CN" : "en-GB")}</Text> }),
  ], [t, i18n.language, setParams])
  const table = useDataTable({ data: query.data?.requests || [], columns, getRowId: (row) => row.id, rowCount: query.data?.count || 0, isLoading: query.isPending, pagination: { state: pagination, onPaginationChange: setPagination } })
  return <Container className="min-w-0 space-y-4" data-testid="buyback-applications"><div className="flex flex-wrap items-center justify-between gap-3"><Heading level="h1">{t("buyback.adminTitle")}</Heading><div className="flex flex-wrap items-center gap-3"><Link className="text-sm text-ui-fg-interactive underline" to="/device-scan/prices">{t("buyback.catalog.title")}</Link><Button size="small" variant="secondary" onClick={() => setPricesOpen(true)}>{t("buyback.prices")}</Button></div></div><Text size="small" className="text-ui-fg-subtle">{t("buyback.adminHelp")}</Text>
    <form className="flex flex-wrap items-end gap-3" onSubmit={(event) => { event.preventDefault(); setSearch(input.trim()); setPagination({ pageIndex: 0, pageSize: 20 }); if (input.trim() === search) void query.refetch() }}><div className="min-w-0 flex-1">{field(t("buyback.search"), <Input aria-label={t("buyback.search")} value={input} maxLength={100} onChange={(event) => setInput(event.target.value)} />)}</div><Select value={status} onValueChange={(value) => { setStatus(value); setPagination({ pageIndex: 0, pageSize: 20 }) }}><Select.Trigger className="w-56" aria-label={t("deviceScan.lookupState")}><Select.Value /></Select.Trigger><Select.Content><Select.Item value="all">{t("buyback.all")}</Select.Item>{statuses.map((value) => <Select.Item key={value} value={value}>{t(`buyback.statuses.${value}`)}</Select.Item>)}</Select.Content></Select><Button size="small" type="submit" variant="secondary" isLoading={query.isFetching}>{t("deviceScan.lookupSearch")}</Button></form>
    {query.isError ? <Button size="small" variant="secondary" onClick={() => query.refetch()}>{t("buyback.retry")}</Button> : <DataTable instance={table}><DataTable.Table emptyState={{ empty: { heading: t("buyback.empty") } }} /><DataTable.Pagination /></DataTable>}
    {id && <ApplicationDetails id={id} onClose={() => setParams({})} />}{pricesOpen && <ReferencePrices onClose={() => setPricesOpen(false)} />}
  </Container>
}
