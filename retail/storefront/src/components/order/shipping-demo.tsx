"use client"

import { useEffect, useState } from "react"
import { Alert, Button, Timeline } from "antd"
import Theme from "../products/storefront-theme"
import copy from "./dhl-demo-copy.json"
import {dhlDemo,type DhlDemo as Demo} from '../../lib/shipping-demo'

// Ported from medusa-store/modules/order/components/dhl-official-tracking.
export default function ShippingDemo({segment}:{segment:string}) {
  const locale=segment.endsWith("-zh")?"zh-CN":"en"
  return <Theme locale={locale}><div className="content-container py-8"><DhlOfficialTracking locale={locale}/></div></Theme>
}

function DhlOfficialTracking({locale}:{locale:string}) {
  const t = copy[locale.startsWith("zh") ? "zh" : "en"]
  const [data, setData] = useState<Demo>()
  const [pending, setPending] = useState(true)
  const [error, setError] = useState(false)
  const [attempt, setAttempt] = useState(0)
  useEffect(() => {
    let active = true
    setPending(true); setError(false)
    fetch("/api/store/shipping-demo/dhl", {cache:"no-store", credentials:"omit", redirect:"error"}).then(async(response)=>{if(!response.ok)throw new Error("DHL demo unavailable");return dhlDemo(await response.json())}).then((value) => { if (active) setData(value) }).catch(() => { if (active) setError(true) }).finally(() => { if (active) setPending(false) })
    return () => { active = false }
  }, [attempt])
  return <section className="mt-6 min-w-0 space-y-4" aria-label={t.title} data-testid="dhl-official-tracking">
    <h3 className="text-lg font-semibold">{t.title}</h3><Alert type="info" showIcon description={t.notice} />
    <Button loading={pending} disabled={pending} onClick={() => setAttempt(attempt + 1)}>{t.refresh}</Button>
    {pending && <p role="status">{t.loading}</p>}{error && <Alert type="error" showIcon message={t.error} />}
    {data && <>
      <p>{t.requested}: {data.requested_tracking_number}</p><p>{t.returned}: {data.shipment.id}</p>
      <p className="text-sm text-ui-fg-subtle">{t.checked}: {new Date(data.fetched_at).toLocaleString(locale)}</p>
      <p>{t.status}: {data.shipment.status?.statusCode} · {data.shipment.status?.description}</p>
      <p className="text-sm text-ui-fg-subtle">{t.timezone}</p>
      <Timeline items={data.shipment.events.map((event) => ({ children: <div><p>{event.description || event.status}</p><p className="text-sm text-ui-fg-subtle">{event.timestamp} · {event.location?.address?.addressLocality} {event.location?.address?.countryCode}</p></div> }))} />
      <a className="text-sm text-ui-fg-interactive underline" href={data.source} target="_blank" rel="noopener noreferrer">{t.source}</a>
    </>}
  </section>
}
