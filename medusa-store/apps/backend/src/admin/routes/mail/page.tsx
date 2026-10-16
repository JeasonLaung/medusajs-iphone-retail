import { useEffect, useState } from "react"
import { defineRouteConfig } from "@medusajs/admin-sdk"
import {
  Button,
  Checkbox,
  Input,
  Label,
  Text,
  toast,
  usePrompt,
} from "@medusajs/ui"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Link, useSearchParams } from "react-router-dom"
import { useTranslation } from "react-i18next"
import MailComposer from "../../components/mail-composer"
import {
  MailEmpty,
  MailDeliveryError,
  MailPagination,
  MailStatus,
  useMailAction,
} from "../../components/mail-shared"
import {
  MailCampaigns,
  MailContacts,
  MailEvents,
  MailTemplates,
} from "../../components/mail-panels"
import {
  emailDocument,
  folders,
  mailError,
  mailGet,
  mailKey,
  mailSdk,
  type Account,
  type Message,
  type Template,
} from "../../lib/mail"
import "./mail.css"

type Compose = {
  mode: "new" | "draft" | "reply" | "replyAll" | "forward"
  message?: Message
  initial?: Partial<Template>
}

const MailPage = () => {
  const { t, i18n } = useTranslation()
  const client = useQueryClient()
  const prompt = usePrompt()
  const action = useMailAction()
  const [params, setParams] = useSearchParams()
  const view = params.get("view") || "inbox"
  const accountId = params.get("account") || ""
  const selectedId = params.get("message") || ""
  const [search, setSearch] = useState("")
  const [debouncedSearch, setDebouncedSearch] = useState("")
  const [offset, setOffset] = useState(0)
  const [selected, setSelected] = useState<string[]>([])
  const [contacts, setContacts] = useState<string[]>([])
  const [compose, setCompose] = useState<Compose | null>(null)
  const [unread, setUnread] = useState(false)
  const [starred, setStarred] = useState(false)
  const [images, setImages] = useState(false)
  const [downloading, setDownloading] = useState(false)
  const isFolder = folders.includes(view as (typeof folders)[number])
  const locale = i18n.language.startsWith("zh") ? "zh-CN" : "en-GB"
  const accounts = useQuery({
    queryKey: [...mailKey, "accounts"],
    queryFn: () => mailGet<{ accounts: Account[] }>({ resource: "accounts" }),
  })
  const summary = useQuery({
    queryKey: [...mailKey, "summary", accountId],
    queryFn: () =>
      mailGet<{ counts: Record<string, number>; unread: number }>({
        resource: "summary",
        account_id: accountId || undefined,
      }),
    refetchInterval: 30_000,
  })
  const messages = useQuery({
    queryKey: [
      ...mailKey,
      "messages",
      view,
      accountId,
      debouncedSearch,
      unread,
      starred,
      offset,
    ],
    queryFn: () =>
      mailGet<{ messages: Message[]; count: number }>({
        resource: "messages",
        folder: view,
        account_id: accountId || undefined,
        q: debouncedSearch,
        unread: unread ? "true" : undefined,
        starred: starred ? "true" : undefined,
        offset,
      }),
    enabled: isFolder,
    refetchInterval: 30_000,
  })
  const detail = useQuery({
    queryKey: [...mailKey, "message", selectedId],
    queryFn: () =>
      mailSdk.client.fetch<{ message: Message }>(
        `/admin/mail/messages/${selectedId}`
      ),
    enabled: !!selectedId && isFolder,
  })
  const message = detail.data?.message
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearch(search)
      setOffset(0)
    }, 300)
    return () => clearTimeout(timer)
  }, [search])
  useEffect(() => {
    setImages(false)
  }, [selectedId])
  useEffect(() => {
    if (
      params.get("compose") === "1" &&
      accounts.data?.accounts.some((a) => a.enabled)
    ) {
      setCompose({ mode: "new" })
      const next = new URLSearchParams(params)
      next.delete("compose")
      setParams(next, { replace: true })
    }
  }, [accounts.data, params])
  const navigate = (next: Record<string, string>) => {
    setParams(next)
    setSelected([])
    setOffset(0)
  }
  const open = (item: Message) => {
    if (item.folder === "draft") {
      void mailSdk.client
        .fetch<{ message: Message }>(`/admin/mail/messages/${item.id}`)
        .then((data) => setCompose({ mode: "draft", message: data.message }))
        .catch((error) => toast.error(mailError(error, t)))
      return
    }
    setParams({
      view,
      ...(accountId ? { account: accountId } : {}),
      message: item.id,
    })
    if (!item.read)
      action.mutate({ action: "messages", ids: [item.id], operation: "read" })
  }
  async function change(operation: string, ids = selected) {
    if (!ids.length) return
    if (
      operation === "delete" &&
      !(await prompt({
        title: t("mail.delete"),
        description: t("mail.deleteConfirm"),
        confirmText: t("mail.confirm"),
        cancelText: t("mail.cancel"),
      }))
    )
      return
    try {
      await action.mutateAsync({ action: "messages", ids, operation })
      if (["trash", "archive", "restore", "delete"].includes(operation)) {
        setSelected([])
        if (ids.includes(selectedId))
          setParams({ view, ...(accountId ? { account: accountId } : {}) })
      }
    } catch {}
  }
  async function download(index: number) {
    if (!message) return
    const item = message.attachments[index]
    setDownloading(true)
    try {
      if (item.content) {
        const bytes = Uint8Array.from(atob(item.content), (char) =>
          char.charCodeAt(0)
        )
        const url = URL.createObjectURL(
          new Blob([bytes], { type: "application/octet-stream" })
        )
        const anchor = document.createElement("a")
        anchor.href = url
        anchor.download = item.filename
        anchor.click()
        setTimeout(() => URL.revokeObjectURL(url), 30_000)
      } else {
        const { url } = await mailSdk.client.fetch<{ url: string }>(
          `/admin/mail/messages/${message.id}/attachments/${item.id || index}`
        )
        window.open(url, "_blank", "noopener,noreferrer")
      }
    } catch (error) {
      toast.error(mailError(error, t))
    } finally {
      setDownloading(false)
    }
  }
  const count = summary.data?.counts || {}
  const list = messages.data?.messages || []
  const navItems = [...folders, "contacts", "campaigns", "templates", "events"]
  return (
    <div className="mail-workspace">
      <aside className="mail-nav" aria-label={t("mail.title")}>
        <div className="mail-nav-caption px-3 pb-3">
          <Text
            size="xsmall"
            weight="plus"
            className="tracking-widest text-ui-fg-muted"
          >
            {t("mail.workspace")}
          </Text>
        </div>
        <Button
          size="small"
          className="mb-3"
          disabled={!accounts.data?.accounts.some((a) => a.enabled)}
          onClick={() => setCompose({ mode: "new" })}
        >
          {t("mail.compose")}
        </Button>
        {navItems.map((item, index) => (
          <button
            className={`mail-nav-button ${index === 6 ? "mt-5" : ""}`}
            aria-current={view === item ? "page" : undefined}
            key={item}
            onClick={() =>
              navigate({
                view: item,
                ...(accountId ? { account: accountId } : {}),
              })
            }
          >
            <Text as="span" size="small">
              {t(`mail.${item}`)}
            </Text>
            {item === "inbox" && !!summary.data?.unread ? (
              <Text
                as="span"
                size="xsmall"
                weight="plus"
                className="rounded bg-ui-tag-blue-bg px-1.5 text-ui-tag-blue-text"
              >
                {summary.data.unread}
              </Text>
            ) : !!count[item] ? (
              <Text as="span" size="xsmall" className="text-ui-fg-muted">
                {count[item]}
              </Text>
            ) : null}
          </button>
        ))}
        <div className="mail-nav-settings mt-auto border-t pt-4">
          <Link className="mail-nav-button" to="/settings/mailboxes">
            {t("mail.openSettings")}
          </Link>
        </div>
      </aside>
      <main className="mail-main">
        {accounts.isPending ? (
          <MailEmpty title="loading" />
        ) : accounts.error ? (
          <MailEmpty title="requestFailed" />
        ) : !accounts.data?.accounts.length ? (
          <div className="mail-empty">
            <Text>{t("mail.noAccounts")}</Text>
            <Button size="small" asChild>
              <Link to="/settings/mailboxes">{t("mail.openSettings")}</Link>
            </Button>
          </div>
        ) : (
          <>
            {isFolder && (
              <>
                <div className="mail-toolbar">
                  <Text weight="plus" className="mr-1">
                    {t(`mail.${view}`)}
                  </Text>
                  <Input
                    className="min-w-28 flex-1"
                    value={search}
                    aria-label={t("mail.search")}
                    placeholder={t("mail.search")}
                    onChange={(e) => setSearch(e.target.value)}
                  />
                  <select
                    className="mail-select max-w-48"
                    aria-label={t("mail.mailbox")}
                    value={accountId}
                    onChange={(e) =>
                      navigate({
                        view,
                        ...(e.target.value ? { account: e.target.value } : {}),
                      })
                    }
                  >
                    <option value="">{t("mail.allAccounts")}</option>
                    {accounts.data.accounts.map((account) => (
                      <option key={account.id} value={account.id}>
                        {account.email}
                      </option>
                    ))}
                  </select>
                  <Button
                    size="small"
                    variant="transparent"
                    onClick={() =>
                      void client.invalidateQueries({ queryKey: mailKey })
                    }
                  >
                    {t("mail.refresh")}
                  </Button>
                </div>
                <div className="flex flex-wrap items-center gap-4 border-b px-5 py-3">
                  <Checkbox
                    aria-label={t("mail.selected", { count: list.length })}
                    checked={
                      !!list.length &&
                      list.every((item) => selected.includes(item.id))
                    }
                    onCheckedChange={(checked) =>
                      setSelected(checked ? list.map((item) => item.id) : [])
                    }
                  />
                  <Label className="flex items-center gap-2 text-xs">
                    <Checkbox
                      checked={unread}
                      onCheckedChange={(value) => {
                        setUnread(value === true)
                        setOffset(0)
                      }}
                    />
                    {t("mail.onlyUnread")}
                  </Label>
                  <Label className="flex items-center gap-2 text-xs">
                    <Checkbox
                      checked={starred}
                      onCheckedChange={(value) => {
                        setStarred(value === true)
                        setOffset(0)
                      }}
                    />
                    {t("mail.onlyStarred")}
                  </Label>
                  {selected.length > 0 && (
                    <div className="flex flex-wrap gap-1">
                      {(view === "trash"
                        ? ["restore", "delete"]
                        : ["read", "unread", "archive", "trash"]
                      ).map((operation) => (
                        <Button
                          key={operation}
                          size="small"
                          variant="transparent"
                          disabled={action.isPending}
                          onClick={() => void change(operation)}
                        >
                          {t(`mail.${operation}`)}
                        </Button>
                      ))}
                    </div>
                  )}
                </div>
                <div className="mail-split" data-selected={!!selectedId}>
                  <div className="mail-list">
                    <div className="mail-list-scroll">
                      {messages.isPending ? (
                        <MailEmpty title="loading" />
                      ) : messages.error ? (
                        <MailEmpty title="requestFailed" />
                      ) : !list.length ? (
                        <MailEmpty />
                      ) : (
                        list.map((item) => (
                          <div
                            className="mail-message-row"
                            key={item.id}
                            data-active={selectedId === item.id}
                          >
                            <div className="flex flex-col items-center gap-3 pt-1">
                              <Checkbox
                                aria-label={item.subject || t("mail.noSubject")}
                                checked={selected.includes(item.id)}
                                onCheckedChange={(checked) =>
                                  setSelected(
                                    checked
                                      ? [...selected, item.id]
                                      : selected.filter((id) => id !== item.id)
                                  )
                                }
                              />
                              <button
                                aria-label={t(
                                  item.starred ? "mail.unstar" : "mail.star"
                                )}
                                className={
                                  item.starred
                                    ? "text-ui-tag-orange-text"
                                    : "text-ui-fg-muted"
                                }
                                disabled={action.isPending}
                                onClick={() =>
                                  void change(
                                    item.starred ? "unstar" : "star",
                                    [item.id]
                                  )
                                }
                              >
                                <svg
                                  width="15"
                                  height="15"
                                  viewBox="0 0 24 24"
                                  fill={item.starred ? "currentColor" : "none"}
                                  stroke="currentColor"
                                  strokeWidth="1.6"
                                  aria-hidden="true"
                                >
                                  <path d="m12 3 2.8 5.7 6.2.9-4.5 4.4 1.1 6.2-5.6-3-5.6 3 1.1-6.2L3 9.6l6.2-.9Z" />
                                </svg>
                              </button>
                            </div>
                            <button
                              className="mail-message-open"
                              onClick={() => open(item)}
                            >
                              <div className="flex w-full items-center gap-2">
                                {!item.read && <i className="mail-dot" />}
                                <Text
                                  as="span"
                                  size="small"
                                  weight={item.read ? "regular" : "plus"}
                                  className="truncate"
                                >
                                  {item.direction === "inbound"
                                    ? item.from_address
                                    : item.to.join(", ")}
                                </Text>
                                <Text
                                  as="span"
                                  size="xsmall"
                                  className="ml-auto shrink-0 text-ui-fg-muted"
                                >
                                  {new Date(
                                    item.received_at ||
                                      item.sent_at ||
                                      item.created_at
                                  ).toLocaleDateString(locale, {
                                    month: "short",
                                    day: "numeric",
                                  })}
                                </Text>
                              </div>
                              <Text
                                as="span"
                                size="small"
                                weight={item.read ? "regular" : "plus"}
                                className="w-full truncate"
                              >
                                {item.subject || t("mail.noSubject")}
                              </Text>
                              {["outbox", "sent"].includes(item.folder) && (
                                <MailStatus status={item.status} />
                              )}
                            </button>
                          </div>
                        ))
                      )}
                    </div>
                    <MailPagination
                      count={messages.data?.count || 0}
                      offset={offset}
                      limit={30}
                      setOffset={(value) => {
                        setOffset(value)
                        setSelected([])
                      }}
                    />
                  </div>
                  <div className="mail-reader">
                    {!selectedId ? (
                      <MailEmpty
                        title="selectMessage"
                        description="selectDescription"
                      />
                    ) : detail.isPending ? (
                      <MailEmpty title="loading" />
                    ) : detail.error || !message ? (
                      <MailEmpty title="requestFailed" />
                    ) : (
                      <>
                        <div className="mail-reader-head">
                          <div className="mb-5 flex flex-wrap gap-2">
                            <Button
                              size="small"
                              variant="transparent"
                              onClick={() =>
                                setParams({
                                  view,
                                  ...(accountId ? { account: accountId } : {}),
                                })
                              }
                            >
                              {t("mail.back")}
                            </Button>
                            {["trash", "outbox"].includes(message.folder) ? (
                              message.folder === "trash" ? (
                                <Button
                                  size="small"
                                  variant="secondary"
                                  disabled={action.isPending}
                                  onClick={() =>
                                    void change("restore", [message.id])
                                  }
                                >
                                  {t("mail.restore")}
                                </Button>
                              ) : (
                                message.status === "failed" && (
                                  <Button
                                    size="small"
                                    variant="secondary"
                                    disabled={action.isPending}
                                    onClick={() =>
                                      void change("retry", [message.id])
                                    }
                                  >
                                    {t("mail.retry")}
                                  </Button>
                                )
                              )
                            ) : (
                              <>
                                <Button
                                  size="small"
                                  variant="secondary"
                                  onClick={() =>
                                    setCompose({ mode: "reply", message })
                                  }
                                >
                                  {t("mail.reply")}
                                </Button>
                                <Button
                                  size="small"
                                  variant="secondary"
                                  onClick={() =>
                                    setCompose({ mode: "replyAll", message })
                                  }
                                >
                                  {t("mail.replyAll")}
                                </Button>
                                <Button
                                  size="small"
                                  variant="transparent"
                                  onClick={() =>
                                    setCompose({ mode: "forward", message })
                                  }
                                >
                                  {t("mail.forward")}
                                </Button>
                              </>
                            )}
                            <Button
                              className="ml-auto"
                              size="small"
                              variant="transparent"
                              disabled={action.isPending}
                              onClick={() =>
                                void change(
                                  message.folder === "trash"
                                    ? "delete"
                                    : "trash",
                                  [message.id]
                                )
                              }
                            >
                              {t(
                                message.folder === "trash"
                                  ? "mail.delete"
                                  : "mail.trash"
                              )}
                            </Button>
                          </div>
                          <Text
                            size="large"
                            weight="plus"
                            className="mb-4 break-words"
                          >
                            {message.subject || t("mail.noSubject")}
                          </Text>
                          {["from", "to", "cc", "bcc"].map((key) => {
                            const value =
                              key === "from"
                                ? message.from_address
                                : message[key as "to" | "cc" | "bcc"].join(", ")
                            return value ? (
                              <div
                                key={key}
                                className="mb-1 grid grid-cols-[60px_1fr] gap-2"
                              >
                                <Text
                                  size="xsmall"
                                  className="text-ui-fg-muted"
                                >
                                  {t(`mail.${key}`)}
                                </Text>
                                <Text size="xsmall" className="break-all">
                                  {value}
                                </Text>
                              </div>
                            ) : null
                          })}
                          <div className="mt-3 flex flex-wrap items-center gap-3">
                            <Text size="xsmall" className="text-ui-fg-muted">
                              {new Date(
                                message.received_at ||
                                  message.sent_at ||
                                  message.created_at
                              ).toLocaleString(locale)}
                            </Text>
                            <MailStatus status={message.status} />
                          </div>
                          <MailDeliveryError error={message.error} />
                        </div>
                        {message.html_body ? (
                          <>
                            <div className="flex flex-wrap items-center justify-between gap-2 border-b px-5 py-2">
                              <Text size="xsmall" className="text-ui-fg-muted">
                                {t("mail.imagesBlocked")}
                              </Text>
                              <Button
                                size="small"
                                variant="transparent"
                                disabled={images}
                                onClick={() => setImages(true)}
                              >
                                {t("mail.loadImages")}
                              </Button>
                            </div>
                            <iframe
                              title={message.subject || t("mail.body")}
                              sandbox=""
                              referrerPolicy="no-referrer"
                              srcDoc={emailDocument(message.html_body, images)}
                            />
                          </>
                        ) : (
                          <div className="mail-reader-body">
                            {message.text_body}
                          </div>
                        )}
                        {!!message.attachments?.length && (
                          <div className="border-t px-6 py-4">
                            <Text size="small" weight="plus" className="mb-2">
                              {t("mail.attachments")}
                            </Text>
                            <div className="flex flex-wrap gap-2">
                              {message.attachments.map((attachment, index) => (
                                <Button
                                  key={index}
                                  size="small"
                                  variant="secondary"
                                  disabled={downloading}
                                  onClick={() => void download(index)}
                                >
                                  {attachment.filename || t("mail.download")}
                                </Button>
                              ))}
                            </div>
                          </div>
                        )}
                      </>
                    )}
                  </div>
                </div>
              </>
            )}
            {view === "contacts" && (
              <MailContacts selected={contacts} setSelected={setContacts} />
            )}
            {view === "campaigns" && (
              <MailCampaigns
                accounts={accounts.data.accounts}
                selectedContacts={contacts}
              />
            )}
            {view === "templates" && (
              <MailTemplates
                onUse={(template) =>
                  setCompose({ mode: "new", initial: template })
                }
              />
            )}
            {view === "events" && <MailEvents />}
          </>
        )}
      </main>
      {compose && accounts.data && (
        <MailComposer
          accounts={accounts.data.accounts}
          accountId={accountId}
          {...compose}
          onClose={() => setCompose(null)}
        />
      )}
    </div>
  )
}

export const config = defineRouteConfig({})
export default MailPage
