import { useEffect, useState, type FormEvent } from "react"
import { defineRouteConfig } from "@medusajs/admin-sdk"
import { Button, Checkbox, Container, Drawer, FocusModal, Heading, Input, Label, Select, Table, Text, toast } from "@medusajs/ui"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { sdk } from "../../../lib/sdk"
import { notifyError } from "../../../lib/notifications"
import en from "../../../i18n/json/customer-service-en.json"
import zh from "../../../i18n/json/customer-service-zh.json"

type Contact = {
  id: string; name: string; department: "pre_sales" | "after_sales"; whatsapp: string; email: string
  timezone: string; days: number[]; start: string; end: string
  availability: "schedule" | "online" | "offline"; enabled: boolean
}
type Settings = { contacts: Contact[] }
const api = "/admin/customer-service"
const queryKey = ["customer-service-settings"]

const translationOptions = { bindI18n: "languageChanged", bindI18nStore: "added" }

const CustomerServicePage = () => {
  const { t, i18n } = useTranslation(undefined, translationOptions)
  const client = useQueryClient()
  const [draft, setDraft] = useState<Contact | null>(null)
  const [creating, setCreating] = useState(false)
  const [invalid, setInvalid] = useState(false)
  useEffect(() => {
    i18n.addResourceBundle("en", "translation", { customerService: en }, true, true)
    i18n.addResourceBundle("zhCN", "translation", { customerService: zh }, true, true)
  }, [i18n])
  const copy = (key: string) => String(t(`customerService.${key}`))
  const query = useQuery({ queryKey, queryFn: () => sdk.client.fetch<Settings>(api) })
  useEffect(() => { if (query.error) notifyError(copy("requestError")) }, [query.error, t])
  const close = () => { setDraft(null); setInvalid(false) }
  const save = useMutation({
    mutationFn: (contacts: Contact[]) => sdk.client.fetch<Settings>(api, { method: "POST", body: { contacts } }),
    onSuccess: async data => {
      client.setQueryData(queryKey, data)
      close()
      toast.success(copy("saved"))
      await client.invalidateQueries({ queryKey })
    },
    onError: () => notifyError(copy("requestError")),
  })
  const change = (patch: Partial<Contact>) => { setDraft(value => value && { ...value, ...patch }); setInvalid(false) }
  const submit = (event: FormEvent) => {
    event.preventDefault()
    if (!draft || !query.data) return
    const contact = { ...draft, name: draft.name.trim(), email: draft.email.trim(), whatsapp: draft.whatsapp.trim(), timezone: draft.timezone.trim() }
    let timezoneValid = false
    try { new Intl.DateTimeFormat("en", { timeZone: contact.timezone }); timezoneValid = Boolean(contact.timezone) } catch {}
    if (!contact.name || !timezoneValid || !contact.days.length || contact.start >= contact.end || !(contact.whatsapp || contact.email) || contact.whatsapp && !/^\+?[1-9]\d{6,14}$/.test(contact.whatsapp)) {
      setInvalid(true)
      return
    }
    save.mutate(creating ? [...query.data.contacts, contact] : query.data.contacts.map(value => value.id === contact.id ? contact : value))
  }
  const fields = draft && <div className="flex flex-col gap-5">
    <div className="flex flex-col gap-2"><Label htmlFor="service-name">{copy("name")}</Label><Input id="service-name" required maxLength={100} value={draft.name} onChange={event => change({ name: event.target.value })} /></div>
    <div className="flex flex-col gap-2"><Label htmlFor="service-department">{copy("department")}</Label><Select value={draft.department} onValueChange={value => change({ department: value as Contact["department"] })}><Select.Trigger id="service-department"><Select.Value /></Select.Trigger><Select.Content>{["pre_sales", "after_sales"].map(value => <Select.Item key={value} value={value}>{copy(value)}</Select.Item>)}</Select.Content></Select></div>
    <div className="flex flex-col gap-2"><Label htmlFor="service-whatsapp">{copy("whatsapp")}</Label><Input id="service-whatsapp" type="tel" maxLength={30} value={draft.whatsapp} onChange={event => change({ whatsapp: event.target.value })} /></div>
    <div className="flex flex-col gap-2"><Label htmlFor="service-email">{copy("email")}</Label><Input id="service-email" type="email" maxLength={254} value={draft.email} onChange={event => change({ email: event.target.value })} /></div>
    <div className="flex flex-col gap-2"><Label htmlFor="service-timezone">{copy("timezone")}</Label><Input id="service-timezone" required maxLength={80} value={draft.timezone} onChange={event => change({ timezone: event.target.value })} /><Text size="small" className="text-ui-fg-subtle">{copy("hoursHelp")}</Text></div>
    <fieldset><legend className="mb-3 text-sm font-medium">{copy("days")}</legend><div className="flex flex-wrap gap-4">{[1, 2, 3, 4, 5, 6, 0].map(day => <div key={day} className="flex items-center gap-2"><Checkbox id={`service-day-${day}`} checked={draft.days.includes(day)} onCheckedChange={checked => change({ days: checked === true ? [...draft.days, day] : draft.days.filter(value => value !== day) })} /><Label htmlFor={`service-day-${day}`}>{copy(`daysShort.${day}`)}</Label></div>)}</div></fieldset>
    <div className="grid grid-cols-2 gap-4">{(["start", "end"] as const).map(key => <div key={key} className="flex flex-col gap-2"><Label htmlFor={`service-${key}`}>{copy(key)}</Label><Input id={`service-${key}`} type="time" required value={draft[key]} onChange={event => change({ [key]: event.target.value })} /></div>)}</div>
    <div className="flex flex-col gap-2"><Label htmlFor="service-availability">{copy("availability")}</Label><Select value={draft.availability} onValueChange={value => change({ availability: value as Contact["availability"] })}><Select.Trigger id="service-availability"><Select.Value /></Select.Trigger><Select.Content>{["schedule", "online", "offline"].map(value => <Select.Item key={value} value={value}>{copy(value)}</Select.Item>)}</Select.Content></Select></div>
    <div className="flex items-center gap-2"><Checkbox id="service-enabled" checked={draft.enabled} onCheckedChange={checked => change({ enabled: checked === true })} /><Label htmlFor="service-enabled">{copy("enabled")}</Label></div>
    {invalid && <Text size="small" role="alert" className="text-ui-fg-error">{copy("validation")}</Text>}
  </div>
  const buttons = <><Button type="button" variant="secondary" disabled={save.isPending} onClick={close}>{copy("cancel")}</Button><Button type="submit" disabled={save.isPending} isLoading={save.isPending}>{copy("save")}</Button></>
  return <Container className="flex flex-col gap-6 p-6">
    <div className="flex flex-wrap items-start justify-between gap-4"><div className="max-w-2xl space-y-2"><Heading>{copy("title")}</Heading><Text size="small" className="text-ui-fg-subtle">{copy("description")}</Text></div><Button size="small" disabled={!query.data || save.isPending || query.data.contacts.length >= 100} onClick={() => { setCreating(true); setInvalid(false); setDraft({ id: crypto.randomUUID(), name: "", department: "pre_sales", whatsapp: "", email: "", timezone: "Asia/Shanghai", days: [1, 2, 3, 4, 5], start: "09:00", end: "18:00", availability: "schedule", enabled: true }) }}>{copy("add")}</Button></div>
    {query.isPending && <Text size="small">{copy("loading")}</Text>}
    {query.isError && <Button variant="secondary" className="self-start" onClick={() => query.refetch()}>{copy("retry")}</Button>}
    {query.data && (query.data.contacts.length ? <div className="overflow-x-auto"><Table><Table.Header><Table.Row>{["name", "department", "channels", "availability", "enabled", "actions"].map(key => <Table.HeaderCell key={key}>{copy(key)}</Table.HeaderCell>)}</Table.Row></Table.Header><Table.Body>{query.data.contacts.map(contact => <Table.Row key={contact.id}>
      <Table.Cell>{contact.name}</Table.Cell><Table.Cell>{copy(contact.department)}</Table.Cell><Table.Cell><Text size="small" className="break-all">{[contact.whatsapp, contact.email].filter(Boolean).join(" / ")}</Text></Table.Cell><Table.Cell>{copy(contact.availability)}</Table.Cell><Table.Cell>{copy(contact.enabled ? "visible" : "hidden")}</Table.Cell><Table.Cell><div className="flex gap-2"><Button size="small" variant="secondary" disabled={save.isPending} aria-label={String(t("customerService.editLabel", { name: contact.name }))} onClick={() => { setCreating(false); setDraft({ ...contact, days: [...contact.days] }); setInvalid(false) }}>{copy("edit")}</Button><Button size="small" variant="secondary" disabled={save.isPending} aria-label={String(t("customerService.removeLabel", { name: contact.name }))} onClick={() => save.mutate(query.data.contacts.filter(value => value.id !== contact.id))}>{copy("remove")}</Button></div></Table.Cell>
    </Table.Row>)}</Table.Body></Table></div> : <Text size="small">{copy("empty")}</Text>)}
    <FocusModal open={Boolean(draft && creating)} onOpenChange={open => { if (!open && !save.isPending) close() }}><FocusModal.Content><form onSubmit={submit} className="flex h-full min-h-0 flex-col"><FocusModal.Header>{buttons}</FocusModal.Header><FocusModal.Body className="overflow-y-auto"><fieldset disabled={save.isPending} className="mx-auto flex max-w-2xl flex-col gap-6 px-6 py-8"><Heading>{copy("createTitle")}</Heading>{creating && fields}</fieldset></FocusModal.Body></form></FocusModal.Content></FocusModal>
    <Drawer open={Boolean(draft && !creating)} onOpenChange={open => { if (!open && !save.isPending) close() }}><Drawer.Content><Drawer.Header><Drawer.Title>{copy("editTitle")}</Drawer.Title></Drawer.Header><form onSubmit={submit} className="flex min-h-0 flex-1 flex-col"><Drawer.Body className="overflow-y-auto"><fieldset disabled={save.isPending}>{!creating && fields}</fieldset></Drawer.Body><Drawer.Footer>{buttons}</Drawer.Footer></form></Drawer.Content></Drawer>
  </Container>
}

export const config = defineRouteConfig({ label: "customerService.title", translationNs: "translation" })

export default CustomerServicePage
