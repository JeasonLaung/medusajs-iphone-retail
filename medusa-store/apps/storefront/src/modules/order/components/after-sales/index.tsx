"use client"

import { useAccountCopy } from "@modules/account/use-account-copy"
import { useEffect, useState } from "react"
import { HttpTypes } from "@medusajs/types"
import {
  AfterSalesRequest,
  listAfterSales,
  submitAfterSales,
  listOrderDevices,
  OrderDevice,
} from "@lib/data/after-sales"
import { Button } from "@modules/common/components/ui"
import LocalizedClientLink from "@modules/common/components/localized-client-link"

export default function AfterSales({ order }: { order: HttpTypes.StoreOrder }) {
  const { t } = useAccountCopy()
  const types = {
    return: t("returnType"),
    exchange: t("exchangeType"),
    repair: t("repairType"),
  }
  const statuses = {
    pending: t("awaitingReview"),
    processing: t("processing"),
    completed: t("completedRequest"),
    rejected: t("declined"),
  }
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
  const hasOpenRequest = requests.some((request) =>
    ["pending", "processing"].includes(request.status)
  )
  const unavailable = order.status === "canceled" || order.status === "draft"

  useEffect(() => {
    if (!open) return
    let active = true
    setLoading(true)
    listAfterSales(order.id)
      .then(async (result) => {
        if (!active) return
        setRequests(result.requests)
        setLoginRequired(result.loginRequired)
        setLoadError(result.error ? "load-failed" : null)
        if (!result.loginRequired && !result.error) {
          const loaded = await listOrderDevices(order.id)
          if (active) setDevices(loaded)
        }
      })
      .catch(() => {
        if (active) setLoadError("load-failed")
      })
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      active = false
    }
  }, [open, order.id, retry])

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setError(null)
    const items = Object.entries(selected).map(([item_id, quantity]) => {
      const itemDevices = devices.filter(
        (d) => d.item_id === item_id && d.status === "shipped"
      )
      const device_ids = itemDevices
        .filter((d) => selectedDevices.includes(d.id))
        .map((d) => d.id)
      return {
        item_id,
        quantity: itemDevices.length ? device_ids.length : quantity,
        ...(itemDevices.length ? { device_ids } : {}),
      }
    })
    if (!items.length) {
      setError(t("selectItemError"))
      return
    }
    if (items.some((i) => i.quantity < 1)) {
      setError(t("selectDeviceError"))
      return
    }
    if (reason.trim().length < 10) {
      setError(t("reasonError"))
      return
    }
    setSubmitting(true)
    try {
      const result = await submitAfterSales(order.id, {
        type,
        reason: reason.trim(),
        items,
      })
      if (result.error) {
        setError(t("submitRequestError"))
        return
      }
      if (result.request) {
        setRequests((current) => [result.request!, ...current])
        setSuccess(true)
        setReason("")
        setSelected({})
        setSelectedDevices([])
      }
    } catch {
      setError(t("submitRequestError"))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <section
      className="mt-6 rounded-xl border border-ui-border-base p-6"
      data-testid="after-sales-section"
    >
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold">{t("afterSales")}</h2>
          <p className="mt-1 text-sm text-ui-fg-subtle">
            {t("afterSalesDescription")}
          </p>
        </div>
        <Button
          variant="secondary"
          onClick={() => setOpen(!open)}
          aria-expanded={open}
          aria-controls={`after-sales-${order.id}`}
        >
          {open ? t("close") : t("requestAfterSales")}
        </Button>
      </div>
      {open && (
        <div id={`after-sales-${order.id}`} className="mt-5 space-y-5">
          {loading ? (
            <p role="status">{t("loadingRequests")}</p>
          ) : loginRequired ? (
            <p className="text-sm">
              <LocalizedClientLink href="/account" className="underline">
                {t("afterSalesLogin")}
              </LocalizedClientLink>
            </p>
          ) : loadError ? (
            <div role="alert">
              <p>{t("loadRequestsError")}</p>
              <Button
                variant="secondary"
                className="mt-3"
                onClick={() => setRetry(retry + 1)}
              >
                {t("retry")}
              </Button>
            </div>
          ) : (
            <>
              {success && (
                <p role="status" className="text-sm text-green-700">
                  {t("requestSuccess")}
                </p>
              )}
              {requests.length > 0 && (
                <div className="space-y-3">
                  <h3 className="font-medium">{t("yourRequests")}</h3>
                  {requests.map((request) => (
                    <div
                      key={request.id}
                      className="rounded-lg bg-ui-bg-subtle p-4 text-sm"
                    >
                      <div className="flex flex-wrap justify-between gap-2">
                        <span className="font-medium">
                          {types[request.type]}
                        </span>
                        <span>{statuses[request.status]}</span>
                      </div>
                      <p className="mt-1 text-ui-fg-subtle">
                        {t("reference", { id: request.id })}
                      </p>
                      <p className="mt-2 whitespace-pre-wrap break-words">
                        {request.reason}
                      </p>
                      <ul className="mt-2">
                        {request.items.map((item) => (
                          <li key={item.item_id}>
                            {item.title} × {item.quantity}
                          </li>
                        ))}
                      </ul>
                    </div>
                  ))}
                </div>
              )}
              {unavailable ? (
                <p className="text-sm">{t("afterSalesUnavailable")}</p>
              ) : hasOpenRequest ? (
                <p className="text-sm text-ui-fg-subtle">{t("openRequest")}</p>
              ) : (
                <form onSubmit={submit} className="space-y-4">
                  <fieldset disabled={submitting} className="space-y-4">
                    <legend className="mb-3 font-medium">
                      {t("newRequest")}
                    </legend>
                    <div>
                      <label
                        htmlFor={`request-type-${order.id}`}
                        className="mb-1 block text-sm font-medium"
                      >
                        {t("requestType")}
                      </label>
                      <select
                        id={`request-type-${order.id}`}
                        value={type}
                        onChange={(event) =>
                          setType(
                            event.target.value as AfterSalesRequest["type"]
                          )
                        }
                        className="w-full rounded-md border border-ui-border-base bg-white p-3 text-sm"
                      >
                        {Object.entries(types).map(([value, label]) => (
                          <option value={value} key={value}>
                            {label}
                          </option>
                        ))}
                      </select>
                    </div>
                    <fieldset className="space-y-3">
                      <legend className="mb-2 text-sm font-medium">
                        {t("selectItems")}
                      </legend>
                      {order.items?.map((item) => (
                        <div
                          key={item.id}
                          className="flex flex-wrap items-center justify-between gap-3 text-sm"
                        >
                          <label className="flex items-center gap-2">
                            <input
                              type="checkbox"
                              checked={item.id in selected}
                              onChange={(event) =>
                                setSelected((current) => {
                                  const next = { ...current }
                                  if (event.target.checked) next[item.id] = 1
                                  else delete next[item.id]
                                  return next
                                })
                              }
                            />
                            {item.title}
                            {item.variant_title &&
                            item.variant_title !== "Default variant"
                              ? ` — ${item.variant_title}`
                              : ""}
                          </label>
                          {item.id in selected &&
                            !devices.some((d) => d.item_id === item.id) && (
                              <input
                                aria-label={t("quantityFor", {
                                  title: item.title || "",
                                })}
                                type="number"
                                min={1}
                                max={Number(item.quantity)}
                                required
                                value={selected[item.id]}
                                onChange={(event) =>
                                  setSelected({
                                    ...selected,
                                    [item.id]: Number(event.target.value),
                                  })
                                }
                                className="w-20 rounded-md border border-ui-border-base p-2"
                              />
                            )}
                          {item.id in selected && (
                            <div className="w-full space-y-2 pl-5">
                              {devices
                                .filter(
                                  (d) =>
                                    d.item_id === item.id &&
                                    d.status === "shipped"
                                )
                                .map((d) => (
                                  <label
                                    key={d.assignment_id}
                                    className="flex items-center gap-2 break-all"
                                  >
                                    <input
                                      type="checkbox"
                                      checked={selectedDevices.includes(d.id)}
                                      onChange={(e) =>
                                        setSelectedDevices(
                                          e.target.checked
                                            ? [...selectedDevices, d.id]
                                            : selectedDevices.filter(
                                                (id) => id !== d.id
                                              )
                                        )
                                      }
                                    />
                                    {t("serial")} {d.serial_number || d.imei}
                                  </label>
                                ))}
                            </div>
                          )}
                        </div>
                      ))}
                    </fieldset>
                    <div>
                      <label
                        htmlFor={`request-reason-${order.id}`}
                        className="mb-1 block text-sm font-medium"
                      >
                        {t("describeIssue")}
                      </label>
                      <textarea
                        id={`request-reason-${order.id}`}
                        required
                        minLength={10}
                        maxLength={2000}
                        rows={4}
                        value={reason}
                        onChange={(event) => setReason(event.target.value)}
                        placeholder={t("issuePlaceholder")}
                        className="w-full rounded-md border border-ui-border-base p-3 text-sm"
                      />
                    </div>
                    <p className="text-xs text-ui-fg-subtle">
                      {t("returnInstructions")}
                    </p>
                    {error && (
                      <p role="alert" className="text-sm text-red-700">
                        {error}
                      </p>
                    )}
                    <Button type="submit" isLoading={submitting}>
                      {t("submitRequest")}
                    </Button>
                  </fieldset>
                </form>
              )}
            </>
          )}
        </div>
      )}
    </section>
  )
}
