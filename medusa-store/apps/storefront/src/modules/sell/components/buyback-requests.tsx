"use client"

import { useEffect, useState } from "react"
import { Alert, Button, Collapse, Empty, Image as AntImage, Input, Modal, Pagination, Skeleton, Tag, Timeline, Upload } from "antd"
import { useParams } from "next/navigation"
import SellTheme from "./sell-theme"
import { getBuybackPhoto, getBuybackRequest, listBuybackRequests, updateBuybackRequest, uploadBuybackPhoto, type BuybackRequest, type BuybackPhoto, type BuybackCommand } from "@lib/data/buyback"
import { getBuybackCopy, type BuybackCopy } from "../buyback-copy"
import styles from "../request.module.css"

function statusColor(status: string) {
  if (["paid", "returned"].includes(status)) return "success"
  if (["final_offer", "return_requested"].includes(status)) return "warning"
  if (status === "canceled") return "default"
  return "processing"
}

function PhotoUpload({ requestId, photo, copy, label, canUpload, busy, onUpload }: { requestId: string; photo?: BuybackPhoto; copy: BuybackCopy; label: string; canUpload: boolean; busy: boolean; onUpload: (file: File) => void }) {
  const [source, setSource] = useState("")
  const [failed, setFailed] = useState(false)
  const [revision, setRevision] = useState(0)
  const photoId = photo?.id
  const reviewStatus = photo?.review_status
  useEffect(() => {
    let active = true
    setSource(""); setFailed(false)
    if (photoId && reviewStatus !== "expired") getBuybackPhoto(requestId, photoId).then(({ data }) => { if (active) { if (data) setSource(`data:${data.mime_type};base64,${data.content}`); else setFailed(true) } }).catch(() => { if (active) setFailed(true) })
    return () => { active = false }
  }, [requestId, photoId, reviewStatus, revision])
  const preview = source ? <AntImage width="100%" height={200} src={source} alt={label} preview /> : photo && photo.review_status !== "expired" ? <p className={styles.notice}>{failed ? copy.errors.BUYBACK_PHOTO_NOT_FOUND : copy.loading}</p> : null
  return <>
    {preview}
    {failed && <Button type="link" disabled={busy} onClick={() => setRevision(revision + 1)}>{copy.retry}</Button>}
    {photo && <><Tag color={photo.review_status === "approved" ? "success" : photo.review_status === "retake" ? "warning" : "default"}>{copy.reviews[photo.review_status as keyof typeof copy.reviews]}</Tag>{photo.review_note && <p>{photo.review_note}</p>}</>}
    {canUpload && <Upload accept="image/jpeg,image/png,image/webp" capture="environment" disabled={busy} showUploadList={false} multiple={false} beforeUpload={(file) => { onUpload(file); return Upload.LIST_IGNORE }}><Button loading={busy} disabled={busy}>{copy.choosePhoto}</Button></Upload>}
  </>
}
async function compressedPhoto(file: File) {
  if (!["image/jpeg", "image/png", "image/webp"].includes(file.type) || file.size > 15 * 1024 * 1024) throw new Error("BUYBACK_INVALID_PHOTO")
  const url = URL.createObjectURL(file)
  try {
    const image = new Image()
    image.src = url
    await image.decode()
    const scale = Math.min(1, 1800 / Math.max(image.width, image.height))
    const canvas = document.createElement("canvas")
    canvas.width = Math.max(1, Math.round(image.width * scale)); canvas.height = Math.max(1, Math.round(image.height * scale))
    const context = canvas.getContext("2d")
    if (!context) throw new Error("BUYBACK_INVALID_PHOTO")
    context.fillStyle = "#fff"; context.fillRect(0, 0, canvas.width, canvas.height)
    context.drawImage(image, 0, 0, canvas.width, canvas.height)
    const dataUrl = canvas.toDataURL("image/webp", .95)
    if (!dataUrl.startsWith("data:image/webp;base64,")) throw new Error("BUYBACK_INVALID_PHOTO")
    const content = dataUrl.split(",")[1]
    if (content.length > 4194304) throw new Error("BUYBACK_INVALID_PHOTO")
    return { content, mime_type: "image/webp" }
  } finally { URL.revokeObjectURL(url) }
}

function RequestDetails({ initial, copy, locale }: { initial: BuybackRequest; copy: BuybackCopy; locale: string | null }) {
  const { countryCode } = useParams<{ countryCode: string }>()
  const [request, setRequest] = useState(initial)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const [tracking, setTracking] = useState("")
  const [choice, setChoice] = useState<BuybackCommand | null>(null)
  const [validationTarget, setValidationTarget] = useState<{ itemId: string; field: string; message: string } | null>(null)
  const [identities, setIdentities] = useState(Object.fromEntries(initial.items.map((item) => [item.id, { item_id: item.id, serial_number: item.serial_number, imei: item.imei }])))
  const money = (value: number | null) => value == null ? copy.pricePending : new Intl.NumberFormat(locale?.startsWith("zh") ? "zh-CN" : "en-GB", { style: "currency", currency: request.currency_code }).format(value)
  const message = (key: string | null) => copy.errors[key as keyof typeof copy.errors] || copy.errors.BUYBACK_REQUEST_FAILED
  async function refresh() {
    setBusy(true); setError("")
    try { const result = await getBuybackRequest(request.id); if (result.data) setRequest(result.data.request); else setError(message(result.error)) }
    finally { setBusy(false) }
  }
  async function command(body: BuybackCommand) {
    setBusy(true); setError("")
    try { const result = await updateBuybackRequest(request.id, body); if (result.data) { setRequest(result.data.request); setChoice(null) } else setError(message(result.error)) }
    finally { setBusy(false) }
  }
  async function upload(itemId: string, photoType: string, file?: File) {
    if (!file || busy) return
    setBusy(true); setError("")
    try {
      const compressed = await compressedPhoto(file)
      const result = await uploadBuybackPhoto(request.id, { item_id: itemId, photo_type: photoType, ...compressed })
      if (!result.data) { setError(message(result.error)); return }
      const updated = await getBuybackRequest(request.id)
      if (updated.data) setRequest(updated.data.request)
      else setError(message(updated.error))
    } catch { setError(copy.errors.BUYBACK_INVALID_PHOTO) }
    finally { setBusy(false) }
  }
  const editable = request.status === "awaiting_photos"
  const fieldId = (itemId: string, field: string) => `${request.id}-${itemId}-${field}`
  function submitPhotos() {
    if (busy) return
    setError("")
    for (const item of request.items) {
      const identity = identities[item.id]
      const types = ["front", "back", ...(["damaged", "visible"].includes(item.assessment.frame) ? ["edges"] : [])]
      const missingPhoto = types.find((type) => !request.photos.some((photo) => photo.item_id === item.id && photo.photo_type === type && !["retake", "expired"].includes(photo.review_status)))
      const issue = !identity?.serial_number.trim() ? { field: "serial_number", message: copy.serialRequired }
        : !/^\d{15}$/.test(identity?.imei || "") ? { field: "imei", message: copy.imeiInvalid }
        : missingPhoto ? { field: missingPhoto, message: copy.photoRequired } : null
      if (issue) {
        setValidationTarget({ itemId: item.id, ...issue })
        requestAnimationFrame(() => {
          const target = document.getElementById(fieldId(item.id, issue.field))
          target?.scrollIntoView({ behavior: window.matchMedia("(prefers-reduced-motion: reduce)").matches ? "instant" : "smooth", block: "center" })
          target?.querySelector<HTMLElement>("input:not([type=file]), button")?.focus({ preventScroll: true })
        })
        return
      }
    }
    setValidationTarget(null)
    void command({ action: "submit_photos", items: Object.values(identities) })
  }
  const slot = (itemId: string, type: string) => {
    const photo = request.photos.find((photo) => photo.item_id === itemId && photo.photo_type === type)
    const canUpload = editable || request.status === "ready_to_ship" && ["packaging", "sealed"].includes(type)
    const label = copy.photoTypes[type as keyof typeof copy.photoTypes]
    const invalid = validationTarget?.itemId === itemId && validationTarget.field === type
    return <div key={type} id={fieldId(itemId, type)} className={styles.photo}><strong>{label}</strong>{invalid && <Alert type="error" showIcon title={validationTarget.message} />}<PhotoUpload requestId={request.id} photo={photo} copy={copy} label={label} canUpload={canUpload} busy={busy} onUpload={(file) => { if (invalid) setValidationTarget(null); void upload(itemId, type, file) }} /></div>
  }
  return <div data-testid="buyback-request-details">
    <div className={styles.actions}><Button type="link" href={`/${countryCode}/account/buyback`}>{copy.myRequests}</Button><Button type="default" disabled={busy} onClick={() => void refresh()}>{copy.refresh}</Button></div>
    <section className={styles.section}><h2>{request.id}</h2><Tag color={statusColor(request.status)}>{copy.statuses[request.status as keyof typeof copy.statuses]}</Tag><p>{copy.reference}: {money(request.reference_amount)}</p><p>{copy.final}: {request.final_amount == null ? "—" : money(request.final_amount)}</p><p className={styles.notice}>{copy.referenceHelp}</p>{request.review_note && <p>{copy.reviewNote}: {request.review_note}</p>}</section>
    <section className={styles.section}><h2>{copy.contact}</h2><address className={styles.address}>{[request.contact_name, request.phone, request.email, request.return_address.address_1, request.return_address.address_2, `${request.return_address.postal_code} ${request.return_address.city}`, request.return_address.country_code.toUpperCase()].filter(Boolean).join("\n")}</address><p className={styles.notice}>{copy.originalAddress}</p></section>
    {["awaiting_photos", "photo_review", "ready_to_ship"].includes(request.status) && <><p className={styles.notice}>{copy.photoHelp}</p><p className={styles.notice}>{copy.photoEvidence}</p><p className={styles.notice}>{copy.privacy}</p></>}
    {request.items.map((item, index) => <section className={styles.section} key={item.id}>
      <h2>{index + 1}. {item.model_name} · {item.capacity}</h2><p>{copy.reference}: {money(item.reference_amount)}</p>
      {editable ? <><p className={styles.notice}>{copy.identityHelp}</p><div className={styles.grid}>{(["serial_number", "imei"] as const).map((field) => {
        const invalid = validationTarget?.itemId === item.id && validationTarget.field === field
        return <label key={field} id={fieldId(item.id, field)} className={styles.label}>{field === "imei" ? copy.imei : copy.serial}<Input value={identities[item.id]?.[field] || ""} maxLength={field === "imei" ? 15 : 100} inputMode={field === "imei" ? "numeric" : "text"} disabled={busy} status={invalid ? "error" : undefined} aria-invalid={invalid} aria-describedby={invalid ? `${fieldId(item.id, field)}-error` : undefined} onChange={(event) => { if (invalid) setValidationTarget(null); setIdentities({ ...identities, [item.id]: { ...identities[item.id], [field]: event.target.value.trim().toUpperCase() } }) }} />{invalid && <div id={`${fieldId(item.id, field)}-error`}><Alert type="error" showIcon title={validationTarget.message} /></div>}</label>
      })}</div></> : <p>{copy.serial}: {item.serial_number || "—"} · {copy.imei}: {item.imei || "—"}</p>}
      <div className={styles.photos}>{slot(item.id, "front")}{slot(item.id, "back")}{["damaged", "visible"].includes(item.assessment.frame) && slot(item.id, "edges")}</div>
      <Collapse items={[{ key: "extra", label: <>{copy.photoTypes.edges} / {copy.photoTypes.packaging} / {copy.photoTypes.sealed}</>, children: <><div className={styles.photos}>{!["damaged", "visible"].includes(item.assessment.frame) && slot(item.id, "edges")}{slot(item.id, "damage")}{slot(item.id, "packaging")}{slot(item.id, "sealed")}</div><p className={styles.notice}>{copy.fileHelp}</p></> }]} />
      {item.inspection_notes && <div><h3>{copy.inspection}</h3><p>{item.inspection_notes}</p><p>{copy.final}: {money(item.final_amount)}</p></div>}
    </section>)}
    {editable && <><p className={styles.notice}>{copy.submitPhotosHelp}</p><Button type="primary" disabled={busy} loading={busy} onClick={submitPhotos}>{copy.submitPhotos}</Button></>}
    {request.shipping_instructions && <section className={styles.section}><h2>{copy.shipping}</h2><p className={styles.address}>{request.shipping_instructions}</p>{request.status === "ready_to_ship" && <form onSubmit={(event) => { event.preventDefault(); void command({ action: "mark_shipped", tracking_number: tracking.trim() }) }}><label className={styles.label}>{copy.tracking}<Input value={tracking} minLength={3} maxLength={150} required disabled={busy} onChange={(event) => setTracking(event.target.value)} /></label><Button type="primary" htmlType="submit" loading={busy} disabled={busy || tracking.trim().length < 3}>{copy.markShipped}</Button></form>}</section>}
    {request.tracking_number && <p>{copy.tracking}: {request.tracking_number}</p>}
    {request.status === "received" && <p className={styles.notice}>{copy.receivedHelp}</p>}
    {request.status === "final_offer" && <section className={styles.section}><h2>{copy.final}: {money(request.final_amount)}</h2><p>{copy.finalHelp}</p><div className={styles.actions}><Button type="primary" disabled={busy} onClick={() => setChoice({ action: "accept_offer", final_amount: request.final_amount! })}>{copy.accept}</Button><Button type="default" disabled={busy} onClick={() => setChoice({ action: "decline_offer" })}>{copy.decline}</Button></div></section>}
    {request.status === "accepted" && <p className={styles.notice}>{copy.paymentHelp}</p>}
    {request.payment_reference && <section className={styles.section}><h2>{copy.payment}</h2><p>{request.payment_reference}</p></section>}
    {request.return_tracking && <section className={styles.section}><h2>{copy.returnTracking}</h2><p>{request.return_tracking}</p><address className={styles.address}>{[request.return_address.name, request.return_address.address_1, request.return_address.address_2, request.return_address.postal_code, request.return_address.city, request.return_address.country_code.toUpperCase()].filter(Boolean).join("\n")}</address></section>}
    {["awaiting_photos", "photo_review", "ready_to_ship"].includes(request.status) && <div className={styles.actions}><Button type="default" disabled={busy} onClick={() => setChoice({ action: "cancel" })}>{copy.cancel}</Button></div>}
    <Modal open={Boolean(choice)} title={choice?.action === "cancel" ? copy.cancelConfirm : copy.offerConfirm} onOk={() => { if (choice) void command(choice) }} onCancel={() => { if (!busy) setChoice(null) }} okText={copy.confirmChoice} cancelText={copy.cancelChoice} confirmLoading={busy} cancelButtonProps={{ disabled: busy }} closable={!busy} mask={{ closable: !busy }} okButtonProps={{ danger: choice?.action === "cancel" || choice?.action === "decline_offer" }} />
    {error && <Alert type="error" showIcon title={error} />}
    <section className={styles.section}><h2>{copy.history}</h2><Timeline items={request.history.map(event => ({ title: new Date(event.at).toLocaleString(locale?.startsWith("zh") ? "zh-CN" : "en-GB"), content: <>{copy.statuses[event.status as keyof typeof copy.statuses]}{event.note ? ` · ${event.note}` : ""}</>, color: ["paid", "returned"].includes(event.status) ? "green" : event.status === "canceled" ? "gray" : "blue" }))} /></section>
  </div>
}

export default function BuybackRequests({ locale, requestId, signedIn }: { locale: string | null; requestId?: string; signedIn: boolean }) {
  const copy = getBuybackCopy(locale)
  const { countryCode } = useParams<{ countryCode: string }>()
  const [data, setData] = useState<{ requests: BuybackRequest[]; count: number } | null>(null)
  const [request, setRequest] = useState<BuybackRequest | null>(null)
  const [offset, setOffset] = useState(0)
  const [loading, setLoading] = useState(signedIn)
  const [error, setError] = useState("")
  const [revision, setRevision] = useState(0)
  useEffect(() => {
    if (!signedIn) return
    let active = true
    setLoading(true); setError("")
    const work = requestId ? getBuybackRequest(requestId).then((result) => { if (!active) return; if (result.data) setRequest(result.data.request); else setError(copy.errors[result.error as keyof typeof copy.errors] || copy.errors.BUYBACK_REQUEST_FAILED) }) : listBuybackRequests(offset).then((result) => { if (!active) return; if (result.data) setData(result.data); else setError(copy.errors[result.error as keyof typeof copy.errors] || copy.errors.BUYBACK_REQUEST_FAILED) })
    work.finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [signedIn, requestId, offset, revision, copy])
  return <SellTheme locale={locale}><div className={styles.page}><h1 className="text-2xl-semi">{copy.title}</h1>{!signedIn ? <><p className={styles.notice}>{copy.login}</p><Button type="primary" href={`/${countryCode}/account`}>{copy.signIn}</Button></> : loading ? <Skeleton active paragraph={{ rows: 5 }} aria-label={copy.loading} /> : error ? <><Alert type="error" showIcon title={error} /><Button type="default" onClick={() => setRevision(revision + 1)}>{copy.retry}</Button></> : request ? <RequestDetails key={request.id} initial={request} copy={copy} locale={locale} /> : <>
    <div className={styles.actions}><Button type="primary" href={`/${countryCode}/sell/iphone/model`}>{copy.newRequest}</Button><Button type="default" onClick={() => setRevision(revision + 1)}>{copy.refresh}</Button></div>
    <div className={styles.list}>{data?.requests.map((item) => <article className={styles.row} key={item.id}><div><strong>{item.items.map((phone) => `${phone.model_name} ${phone.capacity}`).join(" · ")}</strong><Tag color={statusColor(item.status)}>{copy.statuses[item.status as keyof typeof copy.statuses]}</Tag><p>{new Date(item.created_at).toLocaleString(locale?.startsWith("zh") ? "zh-CN" : "en-GB")}</p><p>{item.id}</p></div><Button href={`/${countryCode}/account/buyback/${item.id}`}>{copy.view}</Button></article>)}{data?.count === 0 && <Empty description={copy.empty} />}</div>
    <div className={styles.actions}><Pagination current={offset / 20 + 1} pageSize={20} total={data?.count || 0} showSizeChanger={false} onChange={page => setOffset((page - 1) * 20)} /></div>
  </>}</div></SellTheme>
}
