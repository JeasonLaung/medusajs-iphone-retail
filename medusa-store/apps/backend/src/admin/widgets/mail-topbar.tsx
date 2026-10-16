import { defineWidgetConfig } from "@medusajs/admin-sdk"
import { IconButton, Text, Tooltip } from "@medusajs/ui"
import { useQuery } from "@tanstack/react-query"
import { Link } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { mailGet } from "../lib/mail"

export const MailIcon = () => (
  <svg
    width="19"
    height="19"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="1.6"
    aria-hidden="true"
  >
    <rect x="3" y="5" width="18" height="14" rx="3" />
    <path d="m4 7 8 6 8-6" />
  </svg>
)

const MailTopbar = () => {
  const { t } = useTranslation()
  const { data } = useQuery({
    queryKey: ["mail", "summary"],
    queryFn: () => mailGet<{ unread: number }>({ resource: "summary" }),
    refetchInterval: 30_000,
    retry: false,
  })
  const count = data?.unread || 0
  return (
    <Tooltip content={t("mail.unreadCount", { count })}>
      <div className="relative mr-1">
        <IconButton size="small" variant="transparent" asChild>
          <Link to="/mail" aria-label={t("mail.unreadCount", { count })}>
            <MailIcon />
          </Link>
        </IconButton>
        {count > 0 && (
          <Text
            as="span"
            size="xsmall"
            className="pointer-events-none absolute -right-1 -top-1 flex h-4 min-w-4 items-center justify-center rounded-full bg-ui-tag-red-bg px-1 text-ui-tag-red-text"
            aria-live="polite"
          >
            {count > 99 ? "99+" : count}
          </Text>
        )}
      </div>
    </Tooltip>
  )
}

export const config = defineWidgetConfig({ zone: "topbar", id: "mail:topbar" })
export default MailTopbar
