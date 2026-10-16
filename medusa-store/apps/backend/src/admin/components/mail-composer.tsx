import { useEffect, useRef, useState } from "react"
import {
  Button,
  FocusModal,
  Input,
  Label,
  Text,
  Textarea,
  toast,
} from "@medusajs/ui"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import {
  mailCommand,
  mailError,
  mailGet,
  mailKey,
  recipientList,
  emailDocument,
  type Account,
  type Attachment,
  type Message,
  type Template,
} from "../lib/mail"

type Props = {
  accounts: Account[]
  message?: Message
  mode?: "draft" | "reply" | "replyAll" | "forward" | "new"
  onClose: () => void
  accountId?: string
  initial?: Partial<Template>
}
type Draft = {
  account_id: string
  to: string
  cc: string
  bcc: string
  subject: string
  text_body: string
  html_body: string
  attachments: Attachment[]
  in_reply_to: string | null
}
const address = (value: string) => value.match(/<([^>]+)>/)?.[1] || value.trim()
const emailValid = (value: string) =>
  /^[^\s@<>]+@[^\s@<>]+\.[^\s@<>]+$/.test(value)

export default function MailComposer({
  accounts,
  message,
  mode = "new",
  onClose,
  accountId,
  initial,
}: Props) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const replying = mode === "reply" || mode === "replyAll"
  const originalText = message
    ? message.text_body || message.html_body.replace(/<[^>]*>/g, " ")
    : ""
  const own = new Set(accounts.map((a) => a.email.toLowerCase()))
  const sender = message
    ? address(message.reply_to?.[0] || message.from_address)
    : ""
  const initialDraft: Draft =
    mode === "draft" && message
      ? {
          account_id: message.account_id,
          to: message.to.join(", "),
          cc: message.cc.join(", "),
          bcc: message.bcc.join(", "),
          subject: message.subject,
          text_body: message.text_body,
          html_body: message.html_body,
          attachments: message.attachments,
          in_reply_to: message.in_reply_to,
        }
      : {
          account_id:
            message?.account_id ||
            accountId ||
            accounts.find((a) => a.enabled)?.id ||
            "",
          to: replying
            ? [
                ...new Set([
                  sender,
                  ...(mode === "replyAll"
                    ? message!.to
                        .map(address)
                        .filter((a) => !own.has(a.toLowerCase()))
                    : []),
                ]),
              ].join(", ")
            : "",
          cc:
            mode === "replyAll"
              ? message!.cc
                  .map(address)
                  .filter((a) => !own.has(a.toLowerCase()))
                  .join(", ")
              : "",
          bcc: "",
          subject: message
            ? `${replying ? "Re: " : "Fwd: "}${message.subject.replace(/^(Re|Fwd):\s*/i, "")}`
            : initial?.subject || "",
          text_body: message
            ? `\n\n-------- ${message.from_address} --------\n${originalText}`
            : initial?.text_body || "",
          html_body: initial?.html_body || "",
          attachments: [],
          in_reply_to: replying ? message?.message_id || null : null,
        }
  const [draft, setDraft] = useState(initialDraft)
  const [tab, setTab] = useState(initialDraft.html_body ? "html" : "text")
  const [busy, setBusy] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [saveState, setSaveState] = useState("")
  const [templateName, setTemplateName] = useState("")
  const id = useRef(mode === "draft" ? message?.id : undefined)
  const chain = useRef<Promise<unknown>>(Promise.resolve())
  const latest = useRef(draft)
  const mounted = useRef(true)
  const saving = useRef(false)
  const sent = useRef(false)
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  latest.current = draft
  const { data: templates } = useQuery({
    queryKey: [...mailKey, "templates", "compose"],
    queryFn: () =>
      mailGet<{ templates: Template[] }>({ resource: "templates", limit: 100 }),
  })
  const update = (key: keyof Draft, value: string | Attachment[]) =>
    setDraft((previous) => ({ ...previous, [key]: value }))

  async function persist(snapshot: Draft) {
    if (!snapshot.account_id) return undefined
    const recipients = [
      ...recipientList(snapshot.to),
      ...recipientList(snapshot.cc),
      ...recipientList(snapshot.bcc),
    ]
    if (recipients.some((email) => !emailValid(email)))
      throw new Error("mail.invalidRecipients")
    if (mounted.current) setSaveState("saving")
    const { id: savedId } = await mailCommand<{ id: string }>({
      action: "save_draft",
      id: id.current,
      ...snapshot,
      to: recipientList(snapshot.to),
      cc: recipientList(snapshot.cc),
      bcc: recipientList(snapshot.bcc),
    })
    id.current = savedId
    if (mounted.current) setSaveState("saved")
    await client.invalidateQueries({ queryKey: mailKey })
    return savedId
  }
  const save = (snapshot: Draft) => {
    const request = chain.current
      .catch(() => undefined)
      .then(() => persist(snapshot))
    chain.current = request
    return request
  }
  useEffect(() => {
    if (sent.current || saving.current) return
    timer.current = setTimeout(() => {
      if (
        !latest.current.to &&
        !latest.current.subject &&
        !latest.current.text_body &&
        !latest.current.html_body &&
        !latest.current.attachments.length
      )
        return
      const addresses = [
        ...recipientList(latest.current.to),
        ...recipientList(latest.current.cc),
        ...recipientList(latest.current.bcc),
      ]
      if (addresses.some((email) => !emailValid(email))) return
      void save(latest.current).catch((error) => {
        if (mounted.current) {
          setSaveState("")
          toast.error(mailError(error, t))
        }
      })
    }, 1500)
    return () => clearTimeout(timer.current)
  }, [draft])
  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
      clearTimeout(timer.current)
    }
  }, [])

  async function close() {
    if (busy || uploading) return
    clearTimeout(timer.current)
    setBusy(true)
    try {
      if (
        draft.to ||
        draft.subject ||
        draft.text_body ||
        draft.html_body ||
        draft.attachments.length
      )
        await save(draft)
      onClose()
    } catch (error) {
      toast.error(mailError(error, t))
      setBusy(false)
    }
  }
  async function send() {
    if (
      !recipientList(draft.to).length ||
      !draft.subject.trim() ||
      (!draft.text_body.trim() && !draft.html_body.trim())
    )
      return toast.error(t("mail.messageIncomplete"))
    clearTimeout(timer.current)
    saving.current = true
    setBusy(true)
    try {
      const savedId = await save(draft)
      await mailCommand({ action: "send", id: savedId })
      sent.current = true
      await client.invalidateQueries({ queryKey: mailKey })
      toast.success(t("mail.queuedNotice"))
      onClose()
    } catch (error) {
      toast.error(mailError(error, t))
      saving.current = false
      setBusy(false)
    }
  }
  async function upload(files: FileList | null) {
    if (!files) return
    const picked = Array.from(files)
    const existingSize = draft.attachments.reduce(
      (size, a) => size + (a.content?.length || 0) * 0.75,
      0
    )
    if (
      picked.length + draft.attachments.length > 10 ||
      picked.reduce((size, f) => size + f.size, existingSize) > 10_000_000
    )
      return toast.error(t("mail.attachmentsTooLarge"))
    setUploading(true)
    try {
      const items = await Promise.all(
        picked.map(async (file) => ({
          filename: file.name,
          content: await new Promise<string>((resolve, reject) => {
            const reader = new FileReader()
            reader.onload = () => resolve(String(reader.result).split(",")[1])
            reader.onerror = reject
            reader.readAsDataURL(file)
          }),
        }))
      )
      setDraft((previous) => ({
        ...previous,
        attachments: [...previous.attachments, ...items],
      }))
    } catch (error) {
      toast.error(mailError(error, t))
    } finally {
      setUploading(false)
    }
  }
  async function saveTemplate() {
    if (!templateName.trim()) return
    setBusy(true)
    try {
      await mailCommand({
        action: "save_template",
        name: templateName,
        subject: draft.subject,
        text_body: draft.text_body,
        html_body: draft.html_body,
      })
      await client.invalidateQueries({ queryKey: [...mailKey, "templates"] })
      toast.success(t("mail.templateSaved"))
      setTemplateName("")
    } catch (error) {
      toast.error(mailError(error, t))
    } finally {
      setBusy(false)
    }
  }
  return (
    <FocusModal
      open
      onOpenChange={(open) => {
        if (!open) void close()
      }}
    >
      <FocusModal.Content>
        <FocusModal.Header>
          <div className="flex items-center gap-3">
            <Text size="small" className="text-ui-fg-muted">
              {saveState ? t(`mail.${saveState}`) : t("mail.unsaved")}
            </Text>
            <Button
              size="small"
              variant="secondary"
              disabled={busy || uploading}
              onClick={() => void close()}
            >
              {t("mail.saveDraft")}
            </Button>
            <Button
              size="small"
              isLoading={busy}
              disabled={uploading}
              onClick={() => void send()}
            >
              {t("mail.send")}
            </Button>
          </div>
        </FocusModal.Header>
        <FocusModal.Body className="overflow-y-auto">
          <div className="mx-auto flex max-w-4xl flex-col gap-4 px-6 py-8">
            <Text weight="plus">
              {t(
                replying
                  ? "mail.reply"
                  : mode === "forward"
                    ? "mail.forward"
                    : "mail.compose"
              )}
            </Text>
            <div className="flex flex-wrap gap-3">
              <Label htmlFor="mail-compose-account">{t("mail.from")}</Label>
              <select
                id="mail-compose-account"
                className="mail-select flex-1"
                disabled={busy}
                value={draft.account_id}
                onChange={(e) => update("account_id", e.target.value)}
              >
                {accounts
                  .filter((a) => a.enabled)
                  .map((a) => (
                    <option value={a.id} key={a.id}>
                      {a.name} &lt;{a.email}&gt;
                    </option>
                  ))}
              </select>
              <select
                className="mail-select max-w-56"
                aria-label={t("mail.useTemplate")}
                value=""
                onChange={(e) => {
                  const template = templates?.templates.find(
                    (item) => item.id === e.target.value
                  )
                  if (template) {
                    setDraft((previous) => ({
                      ...previous,
                      subject: template.subject,
                      text_body: template.text_body,
                      html_body: template.html_body,
                    }))
                    setTab(template.html_body ? "html" : "text")
                  }
                }}
              >
                <option value="">{t("mail.useTemplate")}</option>
                {templates?.templates.map((item) => (
                  <option value={item.id} key={item.id}>
                    {item.name}
                  </option>
                ))}
              </select>
            </div>
            {(["to", "cc", "bcc", "subject"] as const).map((field) => (
              <div
                key={field}
                className="grid grid-cols-[72px_1fr] items-center gap-3"
              >
                <Label htmlFor={`mail-compose-${field}`}>
                  {t(`mail.${field}`)}
                </Label>
                <Input
                  id={`mail-compose-${field}`}
                  value={draft[field]}
                  disabled={busy}
                  onChange={(e) => update(field, e.target.value)}
                />
              </div>
            ))}
            <div className="flex gap-2">
              {["text", "html", "preview"].map((value) => (
                <Button
                  type="button"
                  size="small"
                  variant={tab === value ? "primary" : "secondary"}
                  key={value}
                  onClick={() => setTab(value)}
                >
                  {t(`mail.${value}`)}
                </Button>
              ))}
            </div>
            {tab === "preview" ? (
              <iframe
                className="min-h-80 w-full rounded border"
                sandbox=""
                title={t("mail.preview")}
                srcDoc={emailDocument(
                  draft.html_body ||
                    `<pre>${draft.text_body.replace(/&/g, "&amp;").replace(/</g, "&lt;")}</pre>`
                )}
              />
            ) : (
              <Textarea
                className="min-h-80"
                aria-label={t(tab === "html" ? "mail.html" : "mail.body")}
                value={tab === "html" ? draft.html_body : draft.text_body}
                disabled={busy}
                onChange={(e) =>
                  update(
                    tab === "html" ? "html_body" : "text_body",
                    e.target.value
                  )
                }
              />
            )}
            <Text size="small" className="text-ui-fg-muted">
              {t("mail.attachmentLimit")}
            </Text>
            <Input
              type="file"
              multiple
              aria-label={t("mail.attachments")}
              disabled={busy || uploading}
              onChange={(e) => {
                void upload(e.target.files)
                e.target.value = ""
              }}
            />
            {draft.attachments.map((item, index) => (
              <div
                className="flex items-center justify-between gap-3"
                key={`${item.filename}-${index}`}
              >
                <Text size="small">{item.filename}</Text>
                <Button
                  size="small"
                  variant="transparent"
                  disabled={busy}
                  onClick={() =>
                    update(
                      "attachments",
                      draft.attachments.filter((_, i) => index !== i)
                    )
                  }
                >
                  {t("mail.delete")}
                </Button>
              </div>
            ))}
            <div className="mt-4 flex gap-2 border-t pt-4">
              <Input
                placeholder={t("mail.templateName")}
                aria-label={t("mail.templateName")}
                value={templateName}
                onChange={(e) => setTemplateName(e.target.value)}
              />
              <Button
                size="small"
                variant="secondary"
                disabled={busy || !templateName.trim()}
                onClick={() => void saveTemplate()}
              >
                {t("mail.saveTemplate")}
              </Button>
            </div>
          </div>
        </FocusModal.Body>
      </FocusModal.Content>
    </FocusModal>
  )
}
