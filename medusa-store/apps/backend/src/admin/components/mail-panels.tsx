import { useState } from "react"
import {
  Badge,
  Button,
  Checkbox,
  FocusModal,
  Input,
  Label,
  Text,
  Textarea,
  toast,
  usePrompt,
} from "@medusajs/ui"
import { useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import {
  mailGet,
  mailKey,
  emailDocument,
  type Account,
  type Campaign,
  type Contact,
  type Delivery,
  type Template,
} from "../lib/mail"
import {
  MailEmpty,
  MailDeliveryError,
  MailPagination,
  MailStatus,
  useMailAction,
} from "./mail-shared"

export function MailContacts({
  selected,
  setSelected,
}: {
  selected: string[]
  setSelected: (ids: string[]) => void
}) {
  const { t } = useTranslation()
  const [search, setSearch] = useState("")
  const [offset, setOffset] = useState(0)
  const [open, setOpen] = useState(false)
  const [csv, setCsv] = useState("")
  const [consent, setConsent] = useState(false)
  const [source, setSource] = useState("")
  const action = useMailAction()
  const { data, isPending, error } = useQuery({
    queryKey: [...mailKey, "contacts", search, offset],
    queryFn: () =>
      mailGet<{ contacts: Contact[]; count: number }>({
        resource: "contacts",
        q: search,
        offset,
      }),
  })
  const toggle = (id: string) =>
    setSelected(
      selected.includes(id)
        ? selected.filter((item) => item !== id)
        : [...selected, id]
    )
  async function importContacts() {
    // CSV supports quoted names and escaped quotes without treating commas in names as delimiters.
    const lines = csv
      .replace(/^\uFEFF/, "")
      .split(/\r?\n/)
      .filter((line) => line.trim())
    const rows = lines
      .map((line) => {
        const cells: string[] = []
        let value = ""
        let quoted = false
        for (let i = 0; i < line.length; i++) {
          if (line[i] === '"') {
            if (quoted && line[i + 1] === '"') {
              value += '"'
              i++
            } else quoted = !quoted
          } else if (line[i] === "," && !quoted) {
            cells.push(value.trim())
            value = ""
          } else value += line[i]
        }
        cells.push(value.trim())
        return cells
      })
      .filter((row, i) => !(i === 0 && row[0]?.toLowerCase() === "email"))
    if (
      !rows.length ||
      rows.length > 1000 ||
      rows.some((row) => !/^[^\s@<>]+@[^\s@<>]+\.[^\s@<>]+$/.test(row[0]))
    )
      return toast.error(t("mail.invalidCsv"))
    try {
      await action.mutateAsync({
        action: "contacts",
        contacts: rows.map(([email, name]) => ({
          email,
          name: name || "",
          subscribed: consent,
          consent_source: source,
        })),
      })
      setOpen(false)
      setCsv("")
      setConsent(false)
      setSource("")
      toast.success(t("mail.saved"))
    } catch {
      /* Mutation displays the localized error. */
    }
  }
  return (
    <>
      <div className="mail-toolbar">
        <Input
          className="max-w-xs"
          value={search}
          aria-label={t("mail.search")}
          placeholder={t("mail.search")}
          onChange={(e) => {
            setSearch(e.target.value)
            setOffset(0)
          }}
        />
        <div className="flex-1" />
        <Text size="small">
          {t("mail.selected", { count: selected.length })}
        </Text>
        <Button size="small" onClick={() => setOpen(true)}>
          {t("mail.newContact")}
        </Button>
      </div>
      {isPending ? (
        <MailEmpty title="loading" />
      ) : error ? (
        <MailEmpty title="requestFailed" />
      ) : !data?.contacts.length ? (
        <MailEmpty title="emptyContacts" />
      ) : (
        <div className="mail-table-wrap">
          <table className="mail-table">
            <thead>
              <tr>
                <th>
                  <Checkbox
                    aria-label={t("mail.selected", {
                      count: data.contacts.length,
                    })}
                    checked={data.contacts.every((c) =>
                      selected.includes(c.id)
                    )}
                    onCheckedChange={(checked) =>
                      setSelected(
                        checked
                          ? [
                              ...new Set([
                                ...selected,
                                ...data.contacts.map((c) => c.id),
                              ]),
                            ]
                          : selected.filter(
                              (id) => !data.contacts.some((c) => c.id === id)
                            )
                      )
                    }
                  />
                </th>
                <th>{t("mail.name")}</th>
                <th>{t("mail.email")}</th>
                <th>{t("mail.status")}</th>
                <th>{t("mail.consentSource")}</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {data.contacts.map((contact) => (
                <tr key={contact.id}>
                  <td>
                    <Checkbox
                      aria-label={contact.email}
                      checked={selected.includes(contact.id)}
                      onCheckedChange={() => toggle(contact.id)}
                    />
                  </td>
                  <td>{contact.name}</td>
                  <td>{contact.email}</td>
                  <td>
                    <Badge
                      size="2xsmall"
                      color={
                        contact.subscribed && !contact.suppressed
                          ? "green"
                          : "grey"
                      }
                    >
                      {t(
                        `mail.${contact.suppressed ? "suppressed" : contact.subscribed ? "subscribed" : "unsubscribed"}`
                      )}
                    </Badge>
                  </td>
                  <td>{contact.consent_source}</td>
                  <td>
                    {contact.subscribed && (
                      <Button
                        variant="transparent"
                        size="small"
                        disabled={action.isPending}
                        onClick={() =>
                          action.mutate({
                            action: "unsubscribe_contact",
                            id: contact.id,
                          })
                        }
                      >
                        {t("mail.unsubscribe")}
                      </Button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <MailPagination
        count={data?.count || 0}
        offset={offset}
        limit={30}
        setOffset={setOffset}
      />
      <FocusModal
        open={open}
        onOpenChange={(value) => {
          if (!action.isPending) setOpen(value)
        }}
      >
        <FocusModal.Content>
          <FocusModal.Header>
            <Button
              size="small"
              isLoading={action.isPending}
              disabled={!csv.trim() || (consent && !source.trim())}
              onClick={() => void importContacts()}
            >
              {t("mail.import")}
            </Button>
          </FocusModal.Header>
          <FocusModal.Body>
            <div className="mx-auto flex max-w-xl flex-col gap-4 px-6 py-8">
              <Text weight="plus">{t("mail.newContact")}</Text>
              <Text size="small" className="text-ui-fg-subtle">
                {t("mail.contactImport")}
              </Text>
              <Input
                type="file"
                accept=".csv,text/csv"
                aria-label={t("mail.importFile")}
                onChange={(e) => {
                  const file = e.target.files?.[0]
                  if (file && file.size <= 1_000_000)
                    void file.text().then(setCsv)
                  e.target.value = ""
                }}
              />
              <Textarea
                className="min-h-64"
                value={csv}
                aria-label={t("mail.contactImport")}
                onChange={(e) => setCsv(e.target.value)}
              />
              <Label className="flex items-center gap-2">
                <Checkbox
                  checked={consent}
                  onCheckedChange={(value) => setConsent(value === true)}
                />
                {t("mail.consent")}
              </Label>
              {consent && (
                <Input
                  value={source}
                  aria-label={t("mail.consentSource")}
                  placeholder={t("mail.consentSource")}
                  onChange={(e) => setSource(e.target.value)}
                />
              )}
            </div>
          </FocusModal.Body>
        </FocusModal.Content>
      </FocusModal>
    </>
  )
}

export function CampaignEditor({
  accounts,
  campaign,
  onClose,
}: {
  accounts: Account[]
  campaign?: Campaign
  onClose: () => void
}) {
  const { t } = useTranslation()
  const action = useMailAction()
  const [form, setForm] = useState({
    account_id:
      campaign?.account_id || accounts.find((a) => a.enabled)?.id || "",
    name: campaign?.name || "",
    subject: campaign?.subject || "",
    text_body: campaign?.text_body || "",
    html_body: campaign?.html_body || "",
  })
  const [mode, setMode] = useState(campaign?.html_body ? "html" : "text")
  const { data } = useQuery({
    queryKey: [...mailKey, "templates", "campaign"],
    queryFn: () =>
      mailGet<{ templates: Template[] }>({ resource: "templates", limit: 100 }),
  })
  return (
    <FocusModal
      open
      onOpenChange={(open) => {
        if (!open && !action.isPending) onClose()
      }}
    >
      <FocusModal.Content>
        <FocusModal.Header>
          <Button
            size="small"
            isLoading={action.isPending}
            disabled={
              !form.account_id || !form.name.trim() || !form.subject.trim()
            }
            onClick={async () => {
              try {
                await action.mutateAsync({
                  action: "save_campaign",
                  id: campaign?.id,
                  ...form,
                })
                onClose()
              } catch {}
            }}
          >
            {t("mail.save")}
          </Button>
        </FocusModal.Header>
        <FocusModal.Body className="overflow-y-auto">
          <div className="mx-auto flex max-w-3xl flex-col gap-4 px-6 py-8">
            <Text weight="plus">{t("mail.newCampaign")}</Text>
            <Label htmlFor="campaign-account">{t("mail.from")}</Label>
            <select
              className="mail-select"
              id="campaign-account"
              value={form.account_id}
              onChange={(e) => setForm({ ...form, account_id: e.target.value })}
            >
              {accounts
                .filter((a) => a.enabled)
                .map((a) => (
                  <option value={a.id} key={a.id}>
                    {a.name} &lt;{a.email}&gt;
                  </option>
                ))}
            </select>
            {(["name", "subject"] as const).map((field) => (
              <div key={field}>
                <Label htmlFor={`campaign-${field}`}>
                  {t(field === "name" ? "mail.campaignName" : "mail.subject")}
                </Label>
                <Input
                  id={`campaign-${field}`}
                  value={form[field]}
                  onChange={(e) =>
                    setForm({ ...form, [field]: e.target.value })
                  }
                />
              </div>
            ))}
            <select
              aria-label={t("mail.useTemplate")}
              value=""
              className="mail-select"
              onChange={(e) => {
                const item = data?.templates.find(
                  (tpl) => tpl.id === e.target.value
                )
                if (item) {
                  setForm({
                    ...form,
                    subject: item.subject,
                    text_body: item.text_body,
                    html_body: item.html_body,
                  })
                  setMode(item.html_body ? "html" : "text")
                }
              }}
            >
              <option value="">{t("mail.useTemplate")}</option>
              {data?.templates.map((tpl) => (
                <option key={tpl.id} value={tpl.id}>
                  {tpl.name}
                </option>
              ))}
            </select>
            <div className="flex gap-2">
              {["text", "html", "preview"].map((value) => (
                <Button
                  size="small"
                  variant={mode === value ? "primary" : "secondary"}
                  key={value}
                  onClick={() => setMode(value)}
                >
                  {t(`mail.${value}`)}
                </Button>
              ))}
            </div>
            {mode === "preview" ? (
              <iframe
                sandbox=""
                title={t("mail.preview")}
                className="min-h-80 w-full border"
                srcDoc={emailDocument(
                  form.html_body ||
                    `<pre>${form.text_body.replace(/&/g, "&amp;").replace(/</g, "&lt;")}</pre>`
                )}
              />
            ) : (
              <Textarea
                className="min-h-80"
                aria-label={t("mail.body")}
                value={mode === "html" ? form.html_body : form.text_body}
                onChange={(e) =>
                  setForm({
                    ...form,
                    [mode === "html" ? "html_body" : "text_body"]:
                      e.target.value,
                  })
                }
              />
            )}
            <Text size="small" className="text-ui-fg-subtle">
              {t("mail.audienceHint")}
            </Text>
          </div>
        </FocusModal.Body>
      </FocusModal.Content>
    </FocusModal>
  )
}

export function MailCampaigns({
  accounts,
  selectedContacts,
}: {
  accounts: Account[]
  selectedContacts: string[]
}) {
  const { t } = useTranslation()
  const prompt = usePrompt()
  const action = useMailAction()
  const [offset, setOffset] = useState(0)
  const [editor, setEditor] = useState<Campaign | "new" | null>(null)
  const [details, setDetails] = useState<string | null>(null)
  const [audience, setAudience] = useState("all")
  const [deliveryOffset, setDeliveryOffset] = useState(0)
  const { data, isPending, error } = useQuery({
    queryKey: [...mailKey, "campaigns", offset],
    queryFn: () =>
      mailGet<{ campaigns: Campaign[]; count: number }>({
        resource: "campaigns",
        offset,
      }),
    refetchInterval: 30_000,
  })
  const deliveries = useQuery({
    queryKey: [...mailKey, "deliveries", details, deliveryOffset],
    queryFn: () =>
      mailGet<{ deliveries: Delivery[]; count: number }>({
        resource: "deliveries",
        campaign_id: details!,
        offset: deliveryOffset,
      }),
    enabled: !!details,
    refetchInterval: 30_000,
  })
  async function queue(campaign: Campaign) {
    if (
      await prompt({
        title: t("mail.startCampaign"),
        description: t("mail.startCampaignConfirm", {
          audience: t(
            audience === "selected"
              ? "mail.selectedContacts"
              : "mail.allSubscribers",
            { count: selectedContacts.length }
          ),
        }),
        confirmText: t("mail.confirm"),
        cancelText: t("mail.cancel"),
      })
    )
      action.mutate({
        action: "campaign",
        id: campaign.id,
        operation: "queue",
        ...(audience === "selected" ? { contact_ids: selectedContacts } : {}),
      })
  }
  return (
    <>
      <div className="mail-toolbar">
        <Text weight="plus">{t("mail.campaigns")}</Text>
        <div className="flex-1" />
        <select
          className="mail-select"
          aria-label={t("mail.audience")}
          value={audience}
          onChange={(e) => setAudience(e.target.value)}
        >
          <option value="all">{t("mail.allSubscribers")}</option>
          <option value="selected" disabled={!selectedContacts.length}>
            {t("mail.selectedContacts", { count: selectedContacts.length })}
          </option>
        </select>
        <Button
          size="small"
          disabled={!accounts.some((a) => a.enabled)}
          onClick={() => setEditor("new")}
        >
          {t("mail.newCampaign")}
        </Button>
      </div>
      <Text size="small" className="px-6 py-4 text-ui-fg-muted">
        {t("mail.audienceHint")}
      </Text>
      {isPending ? (
        <MailEmpty title="loading" />
      ) : error ? (
        <MailEmpty title="requestFailed" />
      ) : !data?.campaigns.length ? (
        <MailEmpty title="emptyCampaigns" />
      ) : (
        <div className="mail-card-grid">
          {data.campaigns.map((campaign) => (
            <div key={campaign.id} className="mail-card">
              <div className="flex items-center justify-between gap-2">
                <Text weight="plus">{campaign.name}</Text>
                <MailStatus status={campaign.status} />
              </div>
              <Text size="small" className="text-ui-fg-subtle">
                {campaign.subject}
              </Text>
              <div className="grid grid-cols-3 gap-3">
                {[
                  "sent",
                  "delivered",
                  "failed",
                  "opened",
                  "clicked",
                  "queued",
                ].map((status) => (
                  <div key={status}>
                    <Text size="xsmall" className="text-ui-fg-muted">
                      {t(
                        ["opened", "clicked"].includes(status)
                          ? `mail.${status}`
                          : `mail.state.${status}`
                      )}
                    </Text>
                    <Text weight="plus">{campaign.stats?.[status] || 0}</Text>
                  </div>
                ))}
              </div>
              <div className="mt-auto flex flex-wrap gap-2 border-t pt-3">
                {campaign.status === "draft" && (
                  <>
                    <Button
                      size="small"
                      variant="secondary"
                      disabled={action.isPending}
                      onClick={() => setEditor(campaign)}
                    >
                      {t("mail.edit")}
                    </Button>
                    <Button
                      size="small"
                      disabled={
                        action.isPending ||
                        (audience === "selected" && !selectedContacts.length)
                      }
                      onClick={() => void queue(campaign)}
                    >
                      {t("mail.startCampaign")}
                    </Button>
                  </>
                )}
                {["queued", "sending"].includes(campaign.status) && (
                  <Button
                    size="small"
                    variant="secondary"
                    disabled={action.isPending}
                    onClick={() =>
                      action.mutate({
                        action: "campaign",
                        id: campaign.id,
                        operation: "pause",
                      })
                    }
                  >
                    {t("mail.pause")}
                  </Button>
                )}
                {campaign.status === "paused" && (
                  <Button
                    size="small"
                    disabled={action.isPending}
                    onClick={() =>
                      action.mutate({
                        action: "campaign",
                        id: campaign.id,
                        operation: "resume",
                      })
                    }
                  >
                    {t("mail.resume")}
                  </Button>
                )}
                {["completed", "paused"].includes(campaign.status) &&
                  !!campaign.stats.failed && (
                    <Button
                      size="small"
                      variant="secondary"
                      disabled={action.isPending}
                      onClick={() =>
                        action.mutate({
                          action: "campaign",
                          id: campaign.id,
                          operation: "retry",
                        })
                      }
                    >
                      {t("mail.retry")}
                    </Button>
                  )}
                <Button
                  size="small"
                  variant="transparent"
                  onClick={() => {
                    setDetails(campaign.id)
                    setDeliveryOffset(0)
                  }}
                >
                  {t("mail.details")}
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}
      <MailPagination
        count={data?.count || 0}
        offset={offset}
        limit={30}
        setOffset={setOffset}
      />
      {editor && (
        <CampaignEditor
          accounts={accounts}
          campaign={editor === "new" ? undefined : editor}
          onClose={() => setEditor(null)}
        />
      )}
      <FocusModal
        open={!!details}
        onOpenChange={(open) => {
          if (!open) setDetails(null)
        }}
      >
        <FocusModal.Content>
          <FocusModal.Header>
            <Text weight="plus">{t("mail.details")}</Text>
          </FocusModal.Header>
          <FocusModal.Body className="overflow-y-auto">
            <Text size="small" className="px-6 py-4 text-ui-fg-muted">
              {t("mail.deliveryHelp")}
            </Text>
            {deliveries.isPending ? (
              <MailEmpty title="loading" />
            ) : deliveries.error ? (
              <MailEmpty title="requestFailed" />
            ) : (
              <div className="mail-table-wrap">
                <table className="mail-table">
                  <thead>
                    <tr>
                      {[
                        "email",
                        "status",
                        "attempts",
                        "opened",
                        "clicked",
                        "error",
                      ].map((key) => (
                        <th key={key}>{t(`mail.${key}`)}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {deliveries.data?.deliveries.map((row) => (
                      <tr key={row.id}>
                        <td>{row.email}</td>
                        <td>
                          <MailStatus status={row.status} />
                        </td>
                        <td>{row.attempts}</td>
                        <td>{row.opened_at ? "✓" : "—"}</td>
                        <td>{row.clicked_at ? "✓" : "—"}</td>
                        <td>
                          <MailDeliveryError error={row.error} />
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
            <MailPagination
              count={deliveries.data?.count || 0}
              offset={deliveryOffset}
              limit={30}
              setOffset={setDeliveryOffset}
            />
          </FocusModal.Body>
        </FocusModal.Content>
      </FocusModal>
    </>
  )
}

export function MailTemplates({
  onUse,
}: {
  onUse: (template: Template) => void
}) {
  const { t } = useTranslation()
  const prompt = usePrompt()
  const action = useMailAction()
  const [offset, setOffset] = useState(0)
  const { data, isPending, error } = useQuery({
    queryKey: [...mailKey, "templates", offset],
    queryFn: () =>
      mailGet<{ templates: Template[]; count: number }>({
        resource: "templates",
        offset,
      }),
  })
  return (
    <>
      <div className="mail-toolbar">
        <Text weight="plus">{t("mail.templates")}</Text>
      </div>
      {isPending ? (
        <MailEmpty title="loading" />
      ) : error ? (
        <MailEmpty title="requestFailed" />
      ) : !data?.templates.length ? (
        <MailEmpty title="emptyTemplates" />
      ) : (
        <div className="mail-card-grid">
          {data.templates.map((item) => (
            <div key={item.id} className="mail-card">
              <Text weight="plus">{item.name}</Text>
              <Text size="small" className="text-ui-fg-muted">
                {item.subject}
              </Text>
              <div className="flex gap-2">
                <Button size="small" onClick={() => onUse(item)}>
                  {t("mail.useTemplate")}
                </Button>
                <Button
                  size="small"
                  variant="secondary"
                  disabled={action.isPending}
                  onClick={async () => {
                    if (
                      await prompt({
                        title: t("mail.delete"),
                        description: t("mail.deleteTemplateConfirm"),
                        confirmText: t("mail.confirm"),
                        cancelText: t("mail.cancel"),
                      })
                    )
                      action.mutate({ action: "delete_template", id: item.id })
                  }}
                >
                  {t("mail.delete")}
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}
      <MailPagination
        count={data?.count || 0}
        offset={offset}
        limit={30}
        setOffset={setOffset}
      />
    </>
  )
}

export function MailEvents() {
  const { t, i18n } = useTranslation()
  const [offset, setOffset] = useState(0)
  const { data, isPending, error } = useQuery({
    queryKey: [...mailKey, "events", offset],
    queryFn: () =>
      mailGet<{
        events: {
          id: string
          type: string
          provider_id: string
          occurred_at: string
        }[]
        count: number
      }>({ resource: "events", offset }),
    refetchInterval: 30_000,
  })
  return (
    <>
      <div className="mail-toolbar">
        <Text weight="plus">{t("mail.events")}</Text>
      </div>
      {isPending ? (
        <MailEmpty title="loading" />
      ) : error ? (
        <MailEmpty title="requestFailed" />
      ) : !data?.events.length ? (
        <MailEmpty />
      ) : (
        <div className="mail-table-wrap">
          <table className="mail-table">
            <thead>
              <tr>
                {["eventType", "providerId", "createdAt"].map((key) => (
                  <th key={key}>{t(`mail.${key}`)}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {data.events.map((event) => (
                <tr key={event.id}>
                  <td>{event.type}</td>
                  <td className="font-mono text-xs">{event.provider_id}</td>
                  <td>
                    {new Date(event.occurred_at).toLocaleString(
                      i18n.language.startsWith("zh") ? "zh-CN" : "en-GB"
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <MailPagination
        count={data?.count || 0}
        offset={offset}
        limit={30}
        setOffset={setOffset}
      />
    </>
  )
}
