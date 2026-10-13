"use client"

import { useEffect, useState } from "react"
import { HttpTypes } from "@medusajs/types"
import { AfterSalesRequest, listAfterSales, submitAfterSales, listOrderDevices, OrderDevice } from "@lib/data/after-sales"
import { Button } from "@modules/common/components/ui"
import LocalizedClientLink from "@modules/common/components/localized-client-link"

const types = { return: "Return & refund", exchange: "Exchange", repair: "Repair / warranty" }
const statuses = { pending: "Awaiting review", processing: "In progress", completed: "Completed", rejected: "Declined" }

export default function AfterSales({ order }: { order: HttpTypes.StoreOrder }) {
  const [open, setOpen] = useState(false)
  const [requests, setRequests] = useState<AfterSalesRequest[]>([])
  const [loading, setLoading] = useState(false)
  const [loginRequired, setLoginRequired] = useState(false)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [type, setType] = useState<AfterSalesRequest["type"]>("return")
  const [reason, setReason] = useState("")
  const [selected, setSelected] = useState<Record<string, number>>({})
  const [devices, setDevices] = useState<OrderDevice[]>([])
  const [selectedDevices, setSelectedDevices] = useState<string[]>([])
  const [retry, setRetry] = useState(0)
  const hasOpenRequest = requests.some((request) => ["pending", "processing"].includes(request.status))
  const unavailable = order.status === "canceled" || order.status === "draft"

  useEffect(() => {
    if (!open) return
    let active = true
    setLoading(true)
    listAfterSales(order.id).then(async (result) => {
      if (!active) return
      setRequests(result.requests)
      setLoginRequired(result.loginRequired)
      setLoadError(result.error)
      if (!result.loginRequired && !result.error) {
        const loaded = await listOrderDevices(order.id)
        if (active) setDevices(loaded)
      }
    }).catch(() => {
      if (active) setLoadError("Unable to load requests. Please try again.")
    }).finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [open, order.id, retry])

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setError(null)
    const items = Object.entries(selected).map(([item_id, quantity]) => {
      const itemDevices = devices.filter((d) => d.item_id === item_id && d.status === "shipped")
      const device_ids = itemDevices.filter((d) => selectedDevices.includes(d.id)).map((d) => d.id)
      return { item_id, quantity: itemDevices.length ? device_ids.length : quantity, ...(itemDevices.length ? { device_ids } : {}) }
    })
    if (!items.length) { setError("Select at least one item."); return }
    if (items.some((i) => i.quantity < 1)) { setError("Select the device you need help with."); return }
    if (reason.trim().length < 10) { setError("Describe the issue in at least 10 characters."); return }
    setSubmitting(true)
    try {
      const result = await submitAfterSales(order.id, { type, reason: reason.trim(), items })
      if (result.error) { setError(result.error); return }
      if (result.request) {
        setRequests((current) => [result.request!, ...current])
        setSuccess(true)
        setReason("")
        setSelected({})
        setSelectedDevices([])
      }
    } catch {
      setError("Unable to submit your request. Please try again.")
    } finally { setSubmitting(false) }
  }

  return (
    <section className="mt-6 rounded-xl border border-ui-border-base p-6" data-testid="after-sales-section">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold">After-sales support</h2>
          <p className="mt-1 text-sm text-ui-fg-subtle">Request a return, exchange or repair for this order.</p>
        </div>
        <Button variant="secondary" onClick={() => setOpen(!open)} aria-expanded={open} aria-controls={`after-sales-${order.id}`}>
          {open ? "Close" : "Request after-sales support"}
        </Button>
      </div>
      {open && <div id={`after-sales-${order.id}`} className="mt-5 space-y-5">
        {loading ? <p role="status">Loading requests…</p> : loginRequired ? (
          <p className="text-sm">Please <LocalizedClientLink href="/account" className="underline">sign in</LocalizedClientLink> to submit a request. Guest orders can be linked to your account in My orders.</p>
        ) : loadError ? (
          <div role="alert"><p>{loadError}</p><Button variant="secondary" className="mt-3" onClick={() => setRetry(retry + 1)}>Try again</Button></div>
        ) : <>
          {success && <p role="status" className="text-sm text-green-700">Your request has been submitted. Our team will review it before arranging the next steps.</p>}
          {requests.length > 0 && <div className="space-y-3">
            <h3 className="font-medium">Your requests</h3>
            {requests.map((request) => <div key={request.id} className="rounded-lg bg-ui-bg-subtle p-4 text-sm">
              <div className="flex flex-wrap justify-between gap-2"><span className="font-medium">{types[request.type]}</span><span>{statuses[request.status]}</span></div>
              <p className="mt-1 text-ui-fg-subtle">Reference: {request.id}</p>
              <p className="mt-2 whitespace-pre-wrap break-words">{request.reason}</p>
              <ul className="mt-2">{request.items.map((item) => <li key={item.item_id}>{item.title} × {item.quantity}</li>)}</ul>
            </div>)}
          </div>}
          {unavailable ? <p className="text-sm">After-sales requests are unavailable for this order.</p> : hasOpenRequest ? <p className="text-sm text-ui-fg-subtle">You already have an open request for this order. You can check its status here.</p> : (
            <form onSubmit={submit} className="space-y-4">
              <fieldset disabled={submitting} className="space-y-4">
                <legend className="mb-3 font-medium">New request</legend>
                <div><label htmlFor={`request-type-${order.id}`} className="mb-1 block text-sm font-medium">Request type</label>
                  <select id={`request-type-${order.id}`} value={type} onChange={(event) => setType(event.target.value as AfterSalesRequest["type"])} className="w-full rounded-md border border-ui-border-base bg-white p-3 text-sm">
                    {Object.entries(types).map(([value, label]) => <option value={value} key={value}>{label}</option>)}
                  </select>
                </div>
                <fieldset className="space-y-3"><legend className="mb-2 text-sm font-medium">Select items and quantities</legend>
                  {order.items?.map((item) => <div key={item.id} className="flex flex-wrap items-center justify-between gap-3 text-sm">
                    <label className="flex items-center gap-2"><input type="checkbox" checked={item.id in selected} onChange={(event) => setSelected((current) => {
                      const next = { ...current }
                      if (event.target.checked) next[item.id] = 1
                      else delete next[item.id]
                      return next
                    })} />{item.title}{item.variant_title && item.variant_title !== "Default variant" ? ` — ${item.variant_title}` : ""}</label>
                    {item.id in selected && !devices.some((d) => d.item_id === item.id) && <input aria-label={`Quantity for ${item.title}`} type="number" min={1} max={Number(item.quantity)} required value={selected[item.id]} onChange={(event) => setSelected({ ...selected, [item.id]: Number(event.target.value) })} className="w-20 rounded-md border border-ui-border-base p-2" />}
                    {item.id in selected && <div className="w-full space-y-2 pl-5">{devices.filter((d) => d.item_id === item.id && d.status === "shipped").map((d) => <label key={d.assignment_id} className="flex items-center gap-2 break-all"><input type="checkbox" checked={selectedDevices.includes(d.id)} onChange={(e) => setSelectedDevices(e.target.checked ? [...selectedDevices, d.id] : selectedDevices.filter((id) => id !== d.id))} />Serial / IMEI: {d.serial_number || d.imei}</label>)}</div>}
                  </div>)}
                </fieldset>
                <div><label htmlFor={`request-reason-${order.id}`} className="mb-1 block text-sm font-medium">Describe the issue</label>
                  <textarea id={`request-reason-${order.id}`} required minLength={10} maxLength={2000} rows={4} value={reason} onChange={(event) => setReason(event.target.value)} placeholder="Tell us what happened and how you would like us to help." className="w-full rounded-md border border-ui-border-base p-3 text-sm" />
                </div>
                <p className="text-xs text-ui-fg-subtle">Our team will review your request. Please wait for return instructions before shipping any items.</p>
                {error && <p role="alert" className="text-sm text-red-700">{error}</p>}
                <Button type="submit" isLoading={submitting}>Submit request</Button>
              </fieldset>
            </form>
          )}
        </>}
      </div>}
    </section>
  )
}
