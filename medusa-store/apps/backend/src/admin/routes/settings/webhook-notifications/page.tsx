import { useEffect, useState, type FormEvent } from "react"
import { defineRouteConfig } from "@medusajs/admin-sdk"
import { Button, Container, Heading, Input, Label, Select, Text, toast } from "@medusajs/ui"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { sdk } from "../../../lib/sdk"
import { notifyError } from "../../../lib/notifications"
import { webhookEvents as eventNames } from "../../../../lib/webhook-events"

const platforms = ["feishu", "wecom", "dingtalk", "slack", "mattermost", "generic"] as const
type Provider = typeof platforms[number]
type Endpoint = {
  id: string; name: string; provider: Provider; keyword: string; enabled: boolean
  language: "zh" | "en"; events: { values: string[] }; has_url: boolean
  has_secret: boolean; has_token: boolean; last_status: string; last_sent_at: string | null
}
type Draft = {
  name: string; provider: Provider; url: string; secret: string; token: string
  keyword: string; enabled: boolean; language: "zh" | "en"; events: string[]
}
const api = "/admin/webhook-notifications"
const queryKey = ["webhook-notifications"]

const WebhookNotifications = () => {
  const { t, i18n } = useTranslation()
  const client = useQueryClient()
  const [editing, setEditing] = useState<Endpoint | "new" | null>(null)
  const [draft, setDraft] = useState<Draft | null>(null)
  const [clearSecret, setClearSecret] = useState(false)
  const [clearToken, setClearToken] = useState(false)
  const [submitted, setSubmitted] = useState(false)
  const [deleting, setDeleting] = useState<string | null>(null)
  const saved = editing && editing !== "new" ? editing : null
  const query = useQuery({ queryKey, queryFn: () => sdk.client.fetch<{ endpoints: Endpoint[] }>(api) })
  const errorMessage = (error: unknown) => {
    const message = error instanceof Error ? error.message : ""
    const key = message.match(/webhooks\.errors\.(validation|url|delivery|encryption|missing)/)?.[0]
    return String(t(key || "webhooks.errors.request"))
  }
  useEffect(() => {
    if (query.error) notifyError(String(t("webhooks.errors.load")))
  }, [query.error, t])
  const refresh = () => client.invalidateQueries({ queryKey })
  const close = () => { setEditing(null); setDraft(null); setSubmitted(false) }
  const open = (endpoint: Endpoint | "new") => {
    setEditing(endpoint)
    setSubmitted(false)
    setClearSecret(false)
    setClearToken(false)
    setDraft(endpoint === "new"
      ? { name: "", provider: "feishu", url: "", secret: "", token: "", keyword: "", enabled: true, language: i18n.language.toLowerCase().startsWith("zh") ? "zh" : "en", events: ["order.placed"] }
      : { ...endpoint, url: "", secret: "", token: "", events: endpoint.events.values })
  }
  const save = useMutation({
    mutationFn: () => sdk.client.fetch(saved ? `${api}/${saved.id}` : api, {
      method: "POST", body: { ...draft, url: draft!.url.trim() || undefined, secret: clearSecret ? "" : draft!.secret.trim() || undefined, token: clearToken ? "" : draft!.token.trim() || undefined },
    }),
    onSuccess: () => { close(); refresh(); toast.success(String(t("webhooks.saved"))) },
    onError: error => notifyError(errorMessage(error)),
  })
  const remove = useMutation({
    mutationFn: (id: string) => sdk.client.fetch(`${api}/${id}`, { method: "DELETE" }),
    onSuccess: () => { setDeleting(null); refresh(); toast.success(String(t("webhooks.deleted"))) },
    onError: error => notifyError(errorMessage(error)),
  })
  const test = useMutation({
    mutationFn: (id: string) => sdk.client.fetch(`${api}/${id}/test`, { method: "POST" }),
    onSuccess: () => toast.success(String(t("webhooks.tested"))),
    onError: error => notifyError(errorMessage(error)),
    onSettled: refresh,
  })
  const busy = save.isPending || remove.isPending || test.isPending
  const change = <K extends keyof Draft>(key: K, value: Draft[K]) => setDraft(current => current ? { ...current, [key]: value } : current)
  const needUrl = !saved || saved.provider !== draft?.provider
  const submit = (event: FormEvent) => {
    event.preventDefault()
    setSubmitted(true)
    if (!draft?.name.trim() || (needUrl && !draft.url.trim()) || !draft.events.length) return
    save.mutate()
  }
  const field = (key: "name" | "url" | "secret" | "token" | "keyword", password = false) => (
    <div className="flex flex-col gap-2">
      <Label htmlFor={`webhook-${key}`}>{t(`webhooks.${key}`)}</Label>
      <Input id={`webhook-${key}`} type={password ? "password" : "text"} autoComplete={password ? "new-password" : "off"} value={draft![key]} maxLength={key === "url" ? 4096 : key === "name" || key === "keyword" ? 100 : key === "token" ? 2048 : 1024} onChange={event => change(key, event.target.value)} placeholder={saved && (key === "url" || key === "secret" && saved.has_secret || key === "token" && saved.has_token) ? String(t("webhooks.keep")) : undefined} disabled={save.isPending || key === "secret" && clearSecret || key === "token" && clearToken} aria-invalid={submitted && (key === "name" && !draft!.name.trim() || key === "url" && needUrl && !draft!.url.trim()) || undefined} />
      {submitted && (key === "name" && !draft!.name.trim() || key === "url" && needUrl && !draft!.url.trim()) && <Text size="small" className="text-ui-fg-error">{t("webhooks.required")}</Text>}
    </div>
  )
  return <Container className="flex max-w-4xl flex-col gap-6 p-6">
    <div className="flex flex-wrap items-start justify-between gap-4">
      <div><Heading>{t("webhooks.title")}</Heading><Text size="small" className="text-ui-fg-subtle">{t("webhooks.description")}</Text></div>
      {!draft && <Button onClick={() => open("new")} disabled={busy || query.isError || query.isPending}>{t("webhooks.add")}</Button>}
    </div>
    {query.isPending && <Text>{t("webhooks.loading")}</Text>}
    {query.isError && <Button variant="secondary" className="self-start" onClick={() => query.refetch()}>{t("webhooks.retry")}</Button>}
    {draft && <form onSubmit={submit} noValidate className="flex flex-col gap-4 rounded-lg border border-ui-border-base p-4">
      <Heading level="h2">{t(saved ? "webhooks.edit" : "webhooks.add")}</Heading>
      {field("name")}
      <div className="flex flex-col gap-2"><Label htmlFor="webhook-provider">{t("webhooks.provider")}</Label><Select value={draft.provider} onValueChange={value => { change("provider", value as Provider); change("url", ""); change("secret", ""); change("token", "") }} disabled={save.isPending}><Select.Trigger id="webhook-provider"><Select.Value /></Select.Trigger><Select.Content>{platforms.map(provider => <Select.Item key={provider} value={provider}>{t(`webhooks.${provider}`)}</Select.Item>)}</Select.Content></Select></div>
      {field("url", true)}
      {(draft.provider === "feishu" || draft.provider === "dingtalk") && <>{field("secret", true)}{saved?.has_secret && <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={clearSecret} onChange={event => setClearSecret(event.target.checked)} />{t("webhooks.clearSecret")}</label>}</>}
      {draft.provider === "generic" && <>{field("token", true)}{saved?.has_token && <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={clearToken} onChange={event => setClearToken(event.target.checked)} />{t("webhooks.clearToken")}</label>}<Text size="small" className="text-ui-fg-subtle">{t("webhooks.genericHelp")}</Text></>}
      {field("keyword")}
      <div className="flex flex-col gap-2"><Label htmlFor="webhook-language">{t("webhooks.language")}</Label><Select value={draft.language} onValueChange={value => change("language", value as "zh" | "en")} disabled={save.isPending}><Select.Trigger id="webhook-language"><Select.Value /></Select.Trigger><Select.Content>{["zh", "en"].map(language => <Select.Item key={language} value={language}>{t(`webhooks.${language}`)}</Select.Item>)}</Select.Content></Select></div>
      <fieldset className="flex flex-col gap-2" disabled={save.isPending}><legend className="mb-2 text-sm font-medium">{t("webhooks.events")}</legend>{eventNames.map(name => <label className="flex items-center gap-2 text-sm" key={name}><input type="checkbox" checked={draft.events.includes(name)} onChange={event => change("events", event.target.checked ? [...draft.events, name] : draft.events.filter(value => value !== name))} />{t(`webhooks.eventLabels.${name}`)}</label>)}{submitted && !draft.events.length && <Text className="text-ui-fg-error" size="small">{t("webhooks.chooseEvents")}</Text>}</fieldset>
      <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={draft.enabled} disabled={save.isPending} onChange={event => change("enabled", event.target.checked)} />{t("webhooks.enabled")}</label>
      <Text size="small" className="text-ui-fg-subtle">{t("webhooks.security")}</Text>
      <div className="flex gap-2"><Button type="submit" isLoading={save.isPending} disabled={save.isPending}>{t("webhooks.save")}</Button><Button type="button" variant="secondary" onClick={close} disabled={save.isPending}>{t("webhooks.cancel")}</Button></div>
    </form>}
    {!query.isPending && !query.isError && !query.data?.endpoints.length && <Text className="text-ui-fg-subtle">{t("webhooks.empty")}</Text>}
    {query.data?.endpoints.map(endpoint => <section key={endpoint.id} className="flex flex-col gap-3 rounded-lg border border-ui-border-base p-4">
      <div className="flex flex-wrap justify-between gap-2"><Heading level="h2">{endpoint.name}</Heading><Text size="small">{t(`webhooks.${endpoint.enabled ? "enabledStatus" : "disabled"}`)}</Text></div>
      <Text size="small">{t(`webhooks.${endpoint.provider}`)} · {t(`webhooks.${endpoint.language}`)}</Text>
      <Text size="small" className="text-ui-fg-subtle">{endpoint.events.values.map(name => t(`webhooks.eventLabels.${name}`)).join(" / ")}</Text>
      <Text size="small">{t("webhooks.status")}: {t(`webhooks.${endpoint.last_status || "never"}`)}{endpoint.last_sent_at ? ` · ${new Date(endpoint.last_sent_at).toLocaleString(i18n.language === "zhCN" ? "zh-CN" : i18n.language)}` : ""}</Text>
      {deleting === endpoint.id ? <div className="flex flex-wrap items-center gap-2"><Text>{t("webhooks.confirmDelete")}</Text><Button variant="danger" onClick={() => remove.mutate(endpoint.id)} isLoading={remove.isPending} disabled={busy}>{t("webhooks.delete")}</Button><Button variant="secondary" onClick={() => setDeleting(null)} disabled={busy}>{t("webhooks.cancel")}</Button></div> : <div className="flex flex-wrap gap-2"><Button variant="secondary" onClick={() => open(endpoint)} disabled={busy || !!draft}>{t("webhooks.edit")}</Button><Button variant="secondary" onClick={() => test.mutate(endpoint.id)} disabled={busy || !!draft} isLoading={test.isPending && test.variables === endpoint.id}>{t("webhooks.test")}</Button><Button variant="secondary" onClick={() => setDeleting(endpoint.id)} disabled={busy || !!draft}>{t("webhooks.delete")}</Button></div>}
    </section>)}
    <Text size="small" className="text-ui-fg-subtle">{t("webhooks.deliveryHelp")}</Text>
  </Container>
}

export const config = defineRouteConfig({ label: "webhooks.title", translationNs: "translation" })

export default WebhookNotifications
