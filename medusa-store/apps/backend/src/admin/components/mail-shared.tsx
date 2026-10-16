import { useMutation, useQueryClient } from "@tanstack/react-query"
import { Badge, Button, Text, toast } from "@medusajs/ui"
import { useTranslation } from "react-i18next"
import { mailCommand, mailError, mailKey } from "../lib/mail"

export function useMailAction() {
  const client = useQueryClient()
  const { t } = useTranslation()
  return useMutation({
    mutationFn: (body: Record<string, unknown>) => mailCommand(body),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: mailKey })
    },
    onError: (error) => toast.error(mailError(error, t)),
  })
}

export function MailStatus({ status }: { status: string }) {
  const { t } = useTranslation()
  return (
    <Badge
      size="2xsmall"
      color={
        ["failed", "bounced", "complained"].includes(status)
          ? "red"
          : ["delivered", "completed", "subscribed"].includes(status)
            ? "green"
            : "grey"
      }
    >
      {t(`mail.state.${status}`, { defaultValue: status })}
    </Badge>
  )
}

export function MailDeliveryError({ error }: { error: string }) {
  const { t } = useTranslation()
  const key =
    error === "retry_expired"
      ? "mail.retryExpired"
      : error === "unsubscribed"
        ? "mail.unsubscribed"
        : ["bounced", "complained", "suppressed"].includes(error)
          ? `mail.state.${error}`
          : "mail.deliveryFailed"
  return error ? (
    <Text size="xsmall" className="text-ui-fg-error" title={error}>
      {t(key)}
    </Text>
  ) : null
}

export function MailPagination({
  count,
  offset,
  limit,
  setOffset,
}: {
  count: number
  offset: number
  limit: number
  setOffset: (n: number) => void
}) {
  const { t } = useTranslation()
  return (
    <div className="mail-footer">
      <Text size="xsmall" className="text-ui-fg-muted">
        {t("mail.pagination", {
          start: count ? offset + 1 : 0,
          end: Math.min(offset + limit, count),
          count,
        })}
      </Text>
      <div className="flex gap-1">
        <Button
          variant="transparent"
          size="small"
          disabled={!offset}
          onClick={() => setOffset(Math.max(0, offset - limit))}
        >
          {t("mail.previous")}
        </Button>
        <Button
          variant="transparent"
          size="small"
          disabled={offset + limit >= count}
          onClick={() => setOffset(offset + limit)}
        >
          {t("mail.next")}
        </Button>
      </div>
    </div>
  )
}

export function MailEmpty({
  title = "empty",
  description = "emptyDescription",
}: {
  title?: string
  description?: string
}) {
  const { t } = useTranslation()
  return (
    <div className="mail-empty">
      <svg
        aria-hidden="true"
        width="44"
        height="44"
        viewBox="0 0 48 48"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.5"
      >
        <rect x="5" y="11" width="38" height="28" rx="6" />
        <path d="m7 15 17 13 17-13" />
      </svg>
      <Text weight="plus">{t(`mail.${title}`)}</Text>
      <Text size="small">{t(`mail.${description}`)}</Text>
    </div>
  )
}
