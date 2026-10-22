import { useEffect, useState, type FormEvent } from "react"
import { defineRouteConfig } from "@medusajs/admin-sdk"
import { Button, Checkbox, Container, Drawer, Heading, Label, Select, Table, Text, toast } from "@medusajs/ui"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { sdk } from "../../../lib/sdk"
import { notifyError } from "../../../lib/notifications"
import en from "../../../i18n/json/warehouse-en.json"
import zh from "../../../i18n/json/warehouse-zh.json"

type Scope = "platform" | "all" | "bound"
type User = { id: string; email: string; scope: Scope; location_ids: string[] }
type BindingData = { actor_id: string; users: User[]; locations: { id: string; name: string }[] }
type Draft = { user_id: string; scope: Scope; location_ids: string[] }
const api = "/admin/warehouse-access"
const queryKey = ["warehouse-access-settings"]
const translationOptions = { bindI18n: "languageChanged", bindI18nStore: "added" }
const scopes: Scope[] = ["platform", "all", "bound"]

const WarehouseAccessPage = () => {
  const { t, i18n } = useTranslation(undefined, translationOptions)
  const client = useQueryClient()
  const [draft, setDraft] = useState<Draft | null>(null)
  const [submitted, setSubmitted] = useState(false)
  useEffect(() => {
    i18n.addResourceBundle("en", "translation", { warehouseAccess: en }, true, true)
    i18n.addResourceBundle("zhCN", "translation", { warehouseAccess: zh }, true, true)
  }, [i18n])
  const query = useQuery({ queryKey, queryFn: () => sdk.client.fetch<BindingData>(api) })
  const errorMessage = (error: unknown) => {
    const message = error instanceof Error ? error.message : ""
    const codes: Record<string, string> = {
      WAREHOUSE_ACCESS_DENIED: "denied", WAREHOUSE_SELF_CHANGE_DENIED: "self",
      WAREHOUSE_LOCATION_REQUIRED: "locationsRequired", WAREHOUSE_LOCATION_INVALID: "locationsInvalid",
      WAREHOUSE_SUPPLIER_ACCOUNT_DENIED: "supplier", WAREHOUSE_ROLE_NOT_INITIALIZED: "roleMissing",
    }
    const code = Object.keys(codes).find(value => message.includes(value))
    return String(t(`warehouseAccess.errors.${code ? codes[code] : "request"}`))
  }
  useEffect(() => {
    if (query.error) notifyError(errorMessage(query.error))
  }, [query.error, t])
  const close = () => { setDraft(null); setSubmitted(false) }
  const save = useMutation({
    mutationFn: (input: Draft) => sdk.client.fetch(api, { method: "POST", body: { ...input, location_ids: input.scope === "bound" ? input.location_ids : [] } }),
    onSuccess: async () => {
      close()
      await Promise.all([
        client.invalidateQueries({ queryKey }),
        client.invalidateQueries({ queryKey: ["warehouse-menu"] }),
        client.invalidateQueries({ queryKey: ["users"] }),
      ])
      toast.success(String(t("warehouseAccess.saved")))
    },
    onError: error => notifyError(errorMessage(error)),
  })
  const editing = query.data?.users.find(user => user.id === draft?.user_id)
  const locationName = (id: string) => query.data?.locations.find(location => location.id === id)?.name || id
  const summary = (user: User) => user.scope === "bound" ? user.location_ids.length ? user.location_ids.map(locationName).join(" / ") : t("warehouseAccess.unbound") : t(`warehouseAccess.scope.${user.scope}`)
  const changed = draft && editing && (draft.scope !== editing.scope || draft.scope === "bound" && [...draft.location_ids].sort().join(",") !== [...editing.location_ids].sort().join(","))
  const submit = (event: FormEvent) => {
    event.preventDefault()
    setSubmitted(true)
    if (!draft || draft.scope === "bound" && !draft.location_ids.length || !changed) return
    save.mutate(draft)
  }
  return <Container className="flex max-w-5xl flex-col gap-6 p-6">
    <div className="space-y-2">
      <Heading>{t("warehouseAccess.title")}</Heading>
      <Text size="small" className="text-ui-fg-subtle">{t("warehouseAccess.description")}</Text>
    </div>
    {query.isPending && <Text size="small">{t("warehouseAccess.loading")}</Text>}
    {query.isError && <Button variant="secondary" className="self-start" onClick={() => query.refetch()}>{t("warehouseAccess.retry")}</Button>}
    {query.data && <>
      <div className="overflow-x-auto">
        <Table>
          <Table.Header><Table.Row><Table.HeaderCell>{t("warehouseAccess.account")}</Table.HeaderCell><Table.HeaderCell>{t("warehouseAccess.access")}</Table.HeaderCell><Table.HeaderCell>{t("warehouseAccess.warehouses")}</Table.HeaderCell><Table.HeaderCell>{t("warehouseAccess.actions")}</Table.HeaderCell></Table.Row></Table.Header>
          <Table.Body>{query.data.users.map(user => <Table.Row key={user.id}>
            <Table.Cell><div className="py-2"><Text size="small" className="break-all">{user.email}</Text>{user.id === query.data.actor_id && <Text size="xsmall" className="text-ui-fg-subtle">{t("warehouseAccess.currentAccount")}</Text>}</div></Table.Cell>
            <Table.Cell>{t(`warehouseAccess.scope.${user.scope}`)}</Table.Cell>
            <Table.Cell><Text size="small" className="max-w-sm whitespace-normal">{summary(user)}</Text></Table.Cell>
            <Table.Cell><Button size="small" variant="secondary" disabled={user.id === query.data.actor_id || save.isPending} aria-label={String(t("warehouseAccess.editAccount", { email: user.email }))} onClick={() => { setDraft({ user_id: user.id, scope: user.scope, location_ids: [...user.location_ids] }); setSubmitted(false) }}>{t("warehouseAccess.edit")}</Button></Table.Cell>
          </Table.Row>)}</Table.Body>
        </Table>
      </div>
      {!query.data.users.length && <Text size="small">{t("warehouseAccess.empty")}</Text>}
      <Text size="small" className="text-ui-fg-subtle">{t("warehouseAccess.selfHelp")}</Text>
      <Text size="small" className="text-ui-fg-subtle">{t("warehouseAccess.menuHelp")}</Text>
    </>}
    <Drawer open={Boolean(draft)} onOpenChange={open => { if (!open && !save.isPending) close() }}>
      <Drawer.Content>
        <Drawer.Header><Drawer.Title>{t("warehouseAccess.editTitle")}</Drawer.Title><Drawer.Description>{editing?.email}</Drawer.Description></Drawer.Header>
        {draft && <form onSubmit={submit} className="flex min-h-0 flex-1 flex-col">
          <Drawer.Body className="flex flex-col gap-5 overflow-y-auto">
            <div className="flex flex-col gap-2">
              <Label htmlFor="warehouse-access-scope">{t("warehouseAccess.access")}</Label>
              <Select value={draft.scope} disabled={save.isPending} onValueChange={value => { setDraft({ ...draft, scope: value as Scope }); setSubmitted(false) }}><Select.Trigger id="warehouse-access-scope"><Select.Value /></Select.Trigger><Select.Content>{scopes.map(scope => <Select.Item key={scope} value={scope}>{t(`warehouseAccess.scope.${scope}`)}</Select.Item>)}</Select.Content></Select>
              <Text size="small" className="text-ui-fg-subtle">{t(`warehouseAccess.help.${draft.scope}`)}</Text>
            </div>
            {draft.scope === "bound" && <fieldset className="flex flex-col gap-3" disabled={save.isPending}>
              <legend className="mb-3 text-sm font-medium">{t("warehouseAccess.warehouses")}</legend>
              {query.data?.locations.map(location => <div className="flex items-start gap-3" key={location.id}><Checkbox id={`warehouse-${location.id}`} checked={draft.location_ids.includes(location.id)} disabled={save.isPending} onCheckedChange={checked => setDraft({ ...draft, location_ids: checked === true ? [...new Set([...draft.location_ids, location.id])] : draft.location_ids.filter(id => id !== location.id) })} /><Label htmlFor={`warehouse-${location.id}`} className="cursor-pointer break-all">{location.name}</Label></div>)}
              {!query.data?.locations.length && <Text size="small">{t("warehouseAccess.noWarehouses")}</Text>}
              {submitted && !draft.location_ids.length && <Text size="small" role="alert" className="text-ui-fg-error">{t("warehouseAccess.errors.locationsRequired")}</Text>}
            </fieldset>}
            <Text size="small" className="text-ui-fg-subtle">{t("warehouseAccess.sessionHelp")}</Text>
          </Drawer.Body>
          <Drawer.Footer><Button type="button" variant="secondary" disabled={save.isPending} onClick={close}>{t("warehouseAccess.cancel")}</Button><Button type="submit" isLoading={save.isPending} disabled={save.isPending || !changed || draft.user_id === query.data?.actor_id}>{t("warehouseAccess.save")}</Button></Drawer.Footer>
        </form>}
      </Drawer.Content>
    </Drawer>
  </Container>
}

export const config = defineRouteConfig({ label: "warehouseAccess.title", translationNs: "translation" })
export default WarehouseAccessPage
