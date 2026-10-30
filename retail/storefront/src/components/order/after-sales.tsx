// Preserved from the original Medusa after-sales component; transport uses Gin.
import {Alert,Checkbox,Input,Select,Spin} from 'antd'
import {useEffect,useMemo,useRef,useState} from 'react'
import {getAccountCopy} from '../account/i18n'
import {Button} from '../account/ui'
import {customerClient,CustomerError} from '../../lib/customer-client'
import type {AfterSalesRequest,OrderDevice,AfterSalesInput} from '../../lib/customer-after-sales'
import type {OrderConfirmation} from '../../lib/cart-client'
export default function AfterSales({ order,segment }: { order: OrderConfirmation;segment:string }) {
  const { t } = getAccountCopy(segment)
  const api=useMemo(()=>customerClient(),[])
  useEffect(()=>()=>api.clear(),[api])
  const pending=useRef<AfterSalesInput|null>(null)
  const [uncertain,setUncertain]=useState(false)
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
    api.afterSales(order.id).then(requests=>({requests,loginRequired:false,error:null})).catch((e:unknown)=>({requests:[],loginRequired:e instanceof CustomerError&&e.status===401,error:e instanceof CustomerError&&e.status===401?null:'unavailable'}))
      .then(async (result) => {
        if (!active) return
        setRequests(result.requests)
        setLoginRequired(result.loginRequired)
        setLoadError(result.error ? "load-failed" : null)
        if (!result.loginRequired && !result.error) {
          const loaded = await api.orderDevices(order.id)
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
  }, [open, order.id, retry,api])

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
      const payload=pending.current??{operation_id:crypto.randomUUID(),type,reason:reason.trim(),items}
      pending.current=payload
      const request=await api.createAfterSales(order.id,payload)
      const result={request}
      if (result.request) {
        setRequests((current) => [result.request!, ...current])
        pending.current=null
        setUncertain(false)
        setSuccess(true)
        setReason("")
        setSelected({})
        setSelectedDevices([])
      }
    } catch(e) {
      const known=e instanceof CustomerError&&[400,401,403,404,409].includes(e.status)
      if(known){pending.current=null;setUncertain(false)}else setUncertain(true)
      setError(t(e instanceof CustomerError&&e.status===409?'openRequest':e instanceof CustomerError&&e.status===400?'afterSalesInvalidItems':e instanceof CustomerError&&e.status===401?'afterSalesSessionExpired':e instanceof CustomerError&&e.status===404?'afterSalesOrderNotLinked':'submitRequestError'))
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
            <div role="status">
              <Spin /> {t("loadingRequests")}
            </div>
          ) : loginRequired ? (
            <p className="text-sm">
              <a href={`/${segment}/account`} className="underline">
                {t("afterSalesLogin")}
              </a>
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
                <Alert type="success" showIcon message={t("requestSuccess")} />
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
                      <Select
                        id={`request-type-${order.id}`}
                        value={type}
                        disabled={submitting||uncertain}
                        onChange={setType}
                        style={{ width: "100%" }}
                        options={Object.entries(types).map(
                          ([value, label]) => ({ value, label })
                        )}
                      />
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
                            <Checkbox
                              disabled={submitting||uncertain}
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
                              <Input
                                disabled={submitting||uncertain}
                                aria-label={t("quantityFor", {
                                  title: item.title || "",
                                })}
                                type="number"
                                min={1}
                                max={Math.min(10000,Number(order.amounts.lines.find(l=>l.item_id===item.id)?.quantity??0))}
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
                                    <Checkbox
                                      disabled={submitting||uncertain}
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
                      <Input.TextArea
                        disabled={submitting||uncertain}
                        id={`request-reason-${order.id}`}
                        required
                        minLength={10}
                        maxLength={2000}
                        rows={4}
                        value={reason}
                        onChange={(event) => setReason(event.target.value)}
                        placeholder={t("issuePlaceholder")}
                        className="w-full"
                      />
                    </div>
                    <p className="text-xs text-ui-fg-subtle">
                      {t("returnInstructions")}
                    </p>
                    {uncertain&&<p role="status">{segment.endsWith('-zh')?'提交结果尚未确认。请重试查询同一次申请；确认前不能修改内容。':'The result is not yet confirmed. Retry the same request before editing its contents.'}</p>}
                    {error && <Alert type="error" showIcon message={error} />}
                    <Button type="submit" isLoading={submitting}>
                      {uncertain?t("retry"):t("submitRequest")}
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
