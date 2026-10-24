import { Badge, Button, Container, Text } from "@medusajs/ui"
import { useQuery } from "@tanstack/react-query"
import { useEffect } from "react"
import { useTranslation } from "react-i18next"
import type { DhlOfficialDemo } from "../../lib/dhl-official-demo"
import copy from "../../../../../shared/dhl-demo-copy.json"
import { sdk } from "../lib/sdk"
import { notifyError } from "../lib/notifications"

const DhlOfficialTracking = () => {
  const { i18n } = useTranslation()
  const t = copy[i18n.language.startsWith("zh") ? "zh" : "en"]
  const dateLocale = i18n.language.startsWith("zh") ? "zh-CN" : "en-GB"
  const query = useQuery({ queryKey: ["shipping-demo", "dhl"], queryFn: () => sdk.client.fetch<DhlOfficialDemo>("/admin/shipping-demo/dhl"), staleTime: 30_000, retry: false })
  useEffect(() => { if (query.isError) notifyError(t.error) }, [query.isError, t.error])
  return <Container className="space-y-4 px-6 py-4" data-testid="dhl-official-tracking">
    <Text weight="plus">{t.title}</Text><Text size="small" className="text-ui-fg-subtle">{t.notice}</Text>
    <Button size="small" variant="secondary" isLoading={query.isFetching} disabled={query.isFetching} onClick={() => void query.refetch()}>{t.refresh}</Button>
    {query.isPending && <Text role="status">{t.loading}</Text>}
    {query.data && <>
      <Text size="small">{t.requested}: {query.data.requested_tracking_number}</Text>
      <Text size="small">{t.returned}: {query.data.shipment.id}</Text>
      <Text size="small">{t.checked}: {new Date(query.data.fetched_at).toLocaleString(dateLocale)}</Text>
      <Text size="small">{t.status}: <Badge color="grey">{query.data.shipment.status?.statusCode}</Badge> {query.data.shipment.status?.description}</Text>
      <Text size="small" className="text-ui-fg-subtle">{t.timezone}</Text>
      <ol className="space-y-4">{query.data.shipment.events.map((event, index) => <li key={index}><Text size="small">{event.description || event.status}</Text><Text size="small" className="text-ui-fg-subtle">{event.timestamp} · {event.location?.address?.addressLocality} {event.location?.address?.countryCode}</Text></li>)}</ol>
      <Text size="small"><a className="text-ui-fg-interactive underline" href={query.data.source} target="_blank" rel="noopener noreferrer">{t.source}</a></Text>
    </>}
  </Container>
}
export default DhlOfficialTracking
