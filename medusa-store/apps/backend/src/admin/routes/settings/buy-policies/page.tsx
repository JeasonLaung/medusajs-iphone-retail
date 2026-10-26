import { useEffect, useState, type FormEvent } from "react"
import { defineRouteConfig } from "@medusajs/admin-sdk"
import { Button, Container, Drawer, Heading, Input, Label, Text, Textarea, toast } from "@medusajs/ui"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { sdk } from "../../../lib/sdk"
import { notifyError } from "../../../lib/notifications"
import en from "../../../i18n/json/buy-policies-en.json"
import zh from "../../../i18n/json/buy-policies-zh.json"

type Policies = {
  warranty_months: number
  return_days: number
  warranty_note: { en: string; zh: string }
  return_note: { en: string; zh: string }
}
type Response = { policies: Policies }
const api = "/admin/buy-policies"
const queryKey = ["buy-policies"]
const translationOptions = { bindI18n: "languageChanged", bindI18nStore: "added" }
const noteFields = [
  { key: "warranty_note", locale: "en", label: "warrantyNoteEn" },
  { key: "warranty_note", locale: "zh", label: "warrantyNoteZh" },
  { key: "return_note", locale: "en", label: "returnNoteEn" },
  { key: "return_note", locale: "zh", label: "returnNoteZh" },
] as const

const BuyPoliciesPage = () => {
  const { t, i18n } = useTranslation(undefined, translationOptions)
  const client = useQueryClient()
  const [draft, setDraft] = useState<Policies | null>(null)
  const [invalid, setInvalid] = useState(false)
  useEffect(() => {
    i18n.addResourceBundle("en", "translation", { buyPolicies: en }, true, true)
    i18n.addResourceBundle("zhCN", "translation", { buyPolicies: zh }, true, true)
  }, [i18n])
  const copy = (key: string) => String(t(`buyPolicies.${key}`))
  const query = useQuery({ queryKey, queryFn: () => sdk.client.fetch<Response>(api) })
  useEffect(() => { if (query.error) notifyError(String(t("buyPolicies.requestError"))) }, [query.error, t])
  const close = () => { setDraft(null); setInvalid(false) }
  const save = useMutation({
    mutationFn: (policies: Policies) => sdk.client.fetch<Response>(api, { method: "POST", body: policies }),
    onSuccess: async data => {
      client.setQueryData(queryKey, data)
      close()
      toast.success(copy("saved"))
      await client.invalidateQueries({ queryKey })
    },
    onError: () => notifyError(copy("requestError")),
  })
  const submit = (event: FormEvent) => {
    event.preventDefault()
    if (!draft) return
    if (!Number.isInteger(draft.warranty_months) || draft.warranty_months < 0 || draft.warranty_months > 120 ||
      !Number.isInteger(draft.return_days) || draft.return_days < 0 || draft.return_days > 365 ||
      noteFields.some(({ key, locale }) => draft[key][locale].length > 200)) {
      setInvalid(true)
      return
    }
    save.mutate(draft)
  }
  const data = query.data?.policies
  return <Container className="flex flex-col gap-6 p-6">
    <div className="flex items-start justify-between gap-4">
      <div className="max-w-2xl space-y-2"><Heading>{copy("title")}</Heading><Text size="small" className="text-ui-fg-subtle">{copy("description")}</Text></div>
      <Button size="small" variant="secondary" disabled={!data || save.isPending} onClick={() => {
        if (data) setDraft({ ...data, warranty_note: { ...data.warranty_note }, return_note: { ...data.return_note } })
      }}>{copy("edit")}</Button>
    </div>
    {query.isPending && <Text size="small">{copy("loading")}</Text>}
    {query.isError && <Button className="self-start" variant="secondary" onClick={() => query.refetch()}>{copy("retry")}</Button>}
    {data && <dl className="grid gap-6 sm:grid-cols-2">
      <div><dt className="text-ui-fg-subtle text-sm">{copy("warrantyMonths")}</dt><dd>{data.warranty_months}</dd></div>
      <div><dt className="text-ui-fg-subtle text-sm">{copy("returnDays")}</dt><dd>{data.return_days}</dd></div>
      {noteFields.map(({ key, locale, label }) => <div key={label} className="min-w-0"><dt className="text-ui-fg-subtle text-sm">{copy(label)}</dt><dd className="whitespace-pre-wrap break-words">{data[key][locale] || copy("empty")}</dd></div>)}
    </dl>}
    <Drawer open={Boolean(draft)} onOpenChange={open => { if (!open && !save.isPending) close() }}>
      <Drawer.Content><Drawer.Header><Drawer.Title>{copy("title")}</Drawer.Title></Drawer.Header>
        <form onSubmit={submit} className="flex min-h-0 flex-1 flex-col">
          <Drawer.Body className="overflow-y-auto"><fieldset disabled={save.isPending} className="flex flex-col gap-5">
            <Text size="small" className="text-ui-fg-subtle">{copy("help")}</Text>
            {draft && <>
              {([{ key: "warranty_months", label: "warrantyMonths", max: 120 }, { key: "return_days", label: "returnDays", max: 365 }] as const).map(({ key, label, max }) => <div key={key} className="space-y-2"><Label htmlFor={key}>{copy(label)}</Label><Input id={key} type="number" required min={0} max={max} step={1} value={Number.isNaN(draft[key]) ? "" : draft[key]} onChange={event => { setInvalid(false); setDraft({ ...draft, [key]: event.target.value === "" ? NaN : Number(event.target.value) }) }} /></div>)}
              {noteFields.map(({ key, locale, label }) => <div key={label} className="space-y-2"><Label htmlFor={label}>{copy(label)}</Label><Textarea id={label} maxLength={200} value={draft[key][locale]} onChange={event => { setInvalid(false); setDraft({ ...draft, [key]: { ...draft[key], [locale]: event.target.value } }) }} /></div>)}
            </>}
            {invalid && <Text size="small" role="alert" className="text-ui-fg-error">{copy("validation")}</Text>}
          </fieldset></Drawer.Body>
          <Drawer.Footer><Button type="button" variant="secondary" disabled={save.isPending} onClick={close}>{copy("cancel")}</Button><Button type="submit" isLoading={save.isPending}>{copy("save")}</Button></Drawer.Footer>
        </form>
      </Drawer.Content>
    </Drawer>
  </Container>
}

export const config = defineRouteConfig({ label: "buyPolicies.title", translationNs: "translation" })
export default BuyPoliciesPage
