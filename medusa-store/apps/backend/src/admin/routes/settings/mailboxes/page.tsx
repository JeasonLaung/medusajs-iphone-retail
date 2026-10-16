import { useState } from "react"
import { defineRouteConfig } from "@medusajs/admin-sdk"
import {
  Badge,
  Button,
  Checkbox,
  Container,
  Drawer,
  FocusModal,
  Heading,
  IconButton,
  Input,
  Label,
  Popover,
  Text,
  toast,
} from "@medusajs/ui"
import { useQuery } from "@tanstack/react-query"
import { Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { mailGet, mailKey, type Account } from "../../../lib/mail"
import { MailEmpty, useMailAction } from "../../../components/mail-shared"
import "../../mail/mail.css"

function WebhookAddress({ url }: { url?: string }) {
  const { t } = useTranslation()
  const isPublicHttps = (() => {
    try {
      const parsed = new URL(url || "")
      return (
        parsed.protocol === "https:" &&
        !["localhost", "127.0.0.1", "[::1]"].includes(parsed.hostname)
      )
    } catch {
      return false
    }
  })()
  return (
    <div className="flex flex-col gap-2 rounded bg-ui-bg-subtle px-3 py-3">
      <Text size="small" weight="plus">
        {t("mail.webhookUrl")}
      </Text>
      {url ? (
        <>
          <Text size="small" className="break-all font-mono">
            {url}
          </Text>
          <Button
            size="small"
            variant="secondary"
            className="self-start"
            onClick={async () => {
              try {
                await navigator.clipboard.writeText(url)
                toast.success(t("mail.webhookCopied"))
              } catch {
                toast.error(t("mail.webhookCopyFailed"))
              }
            }}
            type="button"
          >
            {t("mail.copyWebhook")}
          </Button>
          {!isPublicHttps && (
            <Text size="small" className="text-ui-fg-error">
              {t("mail.webhookLocalWarning")}
            </Text>
          )}
        </>
      ) : (
        <Text size="small" className="text-ui-fg-subtle">
          {t("mail.webhookSaveFirst")}
        </Text>
      )}
    </div>
  )
}

function AccountForm({
  account,
  onClose,
}: {
  account?: Account
  onClose: () => void
}) {
  const { t } = useTranslation()
  const action = useMailAction()
  const [form, setForm] = useState({
    name: account?.name || "",
    email: account?.email || "",
    reply_to: account?.reply_to || "",
    enabled: account?.enabled ?? true,
    api_key: "",
    webhook_secret: "",
  })
  async function save(event: React.FormEvent) {
    event.preventDefault()
    try {
      await action.mutateAsync({
        action: "save_account",
        id: account?.id,
        ...form,
      })
      toast.success(t("mail.saved"))
      onClose()
    } catch {}
  }
  const content = (
    <form
      id="mail-account-form"
      className="flex flex-col gap-4 px-6 py-6"
      onSubmit={(event) => void save(event)}
    >
      {(
        ["name", "email", "reply_to", "api_key", "webhook_secret"] as const
      ).map((key) => (
        <div key={key} className="flex flex-col gap-2">
          <Label htmlFor={`mail-account-${key}`}>
            {t(
              `mail.${({ reply_to: "replyTo", api_key: "apiKey", webhook_secret: "webhookSecret" } as Record<string, string>)[key] || key}`
            )}
          </Label>
          <Input
            id={`mail-account-${key}`}
            type={
              ["api_key", "webhook_secret"].includes(key)
                ? "password"
                : key === "email" || key === "reply_to"
                  ? "email"
                  : "text"
            }
            autoComplete="off"
            required={
              key === "name" ||
              key === "email" ||
              (key === "api_key" && !account?.has_api_key)
            }
            value={form[key]}
            disabled={action.isPending}
            onChange={(e) => setForm({ ...form, [key]: e.target.value })}
          />
          {key === "api_key" && account?.has_api_key && (
            <Text size="xsmall" className="text-ui-fg-muted">
              {t("mail.configured")}
            </Text>
          )}
          {key === "webhook_secret" && account?.has_webhook_secret && (
            <Text size="xsmall" className="text-ui-fg-muted">
              {t("mail.configured")}
            </Text>
          )}
        </div>
      ))}
      <Text size="small" className="text-ui-fg-subtle">
        {t("mail.secretHint")}
      </Text>
      <Label className="flex items-center gap-2">
        <Checkbox
          disabled={action.isPending}
          checked={form.enabled}
          onCheckedChange={(checked) =>
            setForm({ ...form, enabled: checked === true })
          }
        />
        {t("mail.enabled")}
      </Label>
      <div className="flex flex-col gap-3 border-t pt-4">
        <WebhookAddress url={account?.webhook_url} />
        <Text size="small" className="text-ui-fg-subtle">
          {t("mail.webhookSetupSteps")}
        </Text>
      </div>
    </form>
  )
  const button = (
    <Button
      type="submit"
      form="mail-account-form"
      size="small"
      isLoading={action.isPending}
    >
      {t("mail.save")}
    </Button>
  )
  if (account)
    return (
      <Drawer
        open
        onOpenChange={(open) => {
          if (!open && !action.isPending) onClose()
        }}
      >
        <Drawer.Content>
          <Drawer.Header>
            <Drawer.Title>{t("mail.edit")}</Drawer.Title>
          </Drawer.Header>
          <Drawer.Body className="overflow-y-auto p-0">{content}</Drawer.Body>
          <Drawer.Footer>{button}</Drawer.Footer>
        </Drawer.Content>
      </Drawer>
    )
  return (
    <FocusModal
      open
      onOpenChange={(open) => {
        if (!open && !action.isPending) onClose()
      }}
    >
      <FocusModal.Content>
        <FocusModal.Header>{button}</FocusModal.Header>
        <FocusModal.Body className="overflow-y-auto">
          <div className="mx-auto max-w-xl">
            <Text className="px-6 pt-8" weight="plus">
              {t("mail.newAccount")}
            </Text>
            {content}
          </div>
        </FocusModal.Body>
      </FocusModal.Content>
    </FocusModal>
  )
}

const MailboxSettings = () => {
  const { t } = useTranslation()
  const action = useMailAction()
  const [editor, setEditor] = useState<Account | "new" | null>(null)
  const { data, isPending, error } = useQuery({
    queryKey: [...mailKey, "accounts"],
    queryFn: () =>
      mailGet<{ accounts: Account[]; webhook_base_url: string }>({
        resource: "accounts",
      }),
  })
  return (
    <Container className="p-0">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b px-6 py-4">
        <div className="flex items-center gap-2">
          <Heading level="h1">{t("mail.settings")}</Heading>
          <Popover>
            <Popover.Trigger asChild>
              <IconButton
                size="small"
                variant="transparent"
                aria-label={t("mail.webhookSetupTitle")}
              >
                <svg
                  width="20"
                  height="20"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="1.6"
                  aria-hidden="true"
                >
                  <circle cx="12" cy="12" r="9" />
                  <path d="M12 11v6" />
                  <circle
                    cx="12"
                    cy="7.5"
                    r="0.8"
                    fill="currentColor"
                    stroke="none"
                  />
                </svg>
              </IconButton>
            </Popover.Trigger>
            <Popover.Content
              align="start"
              className="max-h-[70vh] w-[min(28rem,calc(100vw-2rem))] overflow-y-auto p-4"
              aria-label={t("mail.webhookSetupTitle")}
            >
              <div className="flex flex-col gap-3">
                <Text weight="plus">{t("mail.webhookSetupTitle")}</Text>
                <Text size="small" className="text-ui-fg-subtle">
                  {t("mail.webhookSetupSteps")}
                </Text>
                {data?.webhook_base_url && (
                  <div className="rounded bg-ui-bg-subtle px-3 py-2">
                    <Text size="xsmall" className="text-ui-fg-subtle">
                      {t("mail.webhookCurrentBase")}
                    </Text>
                    <Text size="small" className="break-all font-mono">
                      {data.webhook_base_url}
                    </Text>
                  </div>
                )}
                <Text size="small" className="text-ui-fg-subtle">
                  {t("mail.webhookHint")}
                </Text>
                <Text size="small" className="text-ui-fg-subtle">
                  {t("mail.webhookEvents")}
                </Text>
                <Text size="small" className="text-ui-fg-subtle">
                  {t("mail.webhookLocalWarning")}
                </Text>
              </div>
            </Popover.Content>
          </Popover>
        </div>
        <div className="flex gap-2">
          <Button variant="secondary" size="small" asChild>
            <Link to="/mail">{t("mail.title")}</Link>
          </Button>
          <Button size="small" onClick={() => setEditor("new")}>
            {t("mail.newAccount")}
          </Button>
        </div>
      </div>
      {isPending ? (
        <MailEmpty title="loading" />
      ) : error ? (
        <MailEmpty title="requestFailed" />
      ) : !data?.accounts.length ? (
        <MailEmpty title="noAccounts" />
      ) : (
        <div className="mail-card-grid">
          {data.accounts.map((account) => (
            <div className="mail-card" key={account.id}>
              <div className="flex items-center justify-between gap-2">
                <Text weight="plus">{account.name}</Text>
                <Badge
                  color={account.enabled ? "green" : "grey"}
                  size="2xsmall"
                >
                  {t(account.enabled ? "mail.enabled" : "mail.disabled")}
                </Badge>
              </div>
              <Text size="small">{account.email}</Text>
              <div className="flex flex-wrap gap-2">
                <Text size="xsmall" className="text-ui-fg-muted">
                  {t("mail.apiKey")}:{" "}
                  {t(
                    account.has_api_key
                      ? "mail.configured"
                      : "mail.notConfigured"
                  )}
                </Text>
                <Text size="xsmall" className="text-ui-fg-muted">
                  {t("mail.webhookSecret")}:{" "}
                  {t(
                    account.has_webhook_secret
                      ? "mail.configured"
                      : "mail.notConfigured"
                  )}
                </Text>
              </div>
              <WebhookAddress url={account.webhook_url} />
              <div className="flex flex-wrap gap-2">
                <Button
                  size="small"
                  variant="secondary"
                  onClick={() => setEditor(account)}
                >
                  {t("mail.edit")}
                </Button>
                {account.enabled && (
                  <>
                    <Button size="small" variant="secondary" asChild>
                      <Link to={`/mail?account=${account.id}&compose=1`}>
                        {t("mail.testEmail")}
                      </Link>
                    </Button>
                    <Button
                      size="small"
                      variant="transparent"
                      disabled={action.isPending}
                      onClick={() =>
                        action.mutate({
                          action: "delete_account",
                          id: account.id,
                        })
                      }
                    >
                      {t("mail.disable")}
                    </Button>
                  </>
                )}
              </div>
            </div>
          ))}
        </div>
      )}
      {editor && (
        <AccountForm
          account={editor === "new" ? undefined : editor}
          onClose={() => setEditor(null)}
        />
      )}
    </Container>
  )
}

export const config = defineRouteConfig({
  label: "mail.settings",
  translationNs: "translation",
})
export default MailboxSettings
