import type { PropsWithChildren } from "react"
import { Button, Divider, Text } from "@medusajs/ui"
import { useQuery } from "@tanstack/react-query"
import { Link, useLocation, useSearchParams } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { folders, mailGet, mailKey, type Account } from "../lib/mail"
import "../routes/mail/mail.css"

export const MailSidebarLayout = ({ children, closeMobile }: PropsWithChildren<{ closeMobile: () => void }>) => {
  const { pathname } = useLocation()
  return pathname === "/mail" || pathname.startsWith("/mail/")
    ? <MailSidebar closeMobile={closeMobile} />
    : children
}

const MailSidebar = ({ closeMobile }: { closeMobile: () => void }) => {
  const { t } = useTranslation()
  const [params, setParams] = useSearchParams()
  const view = params.get("view") || "inbox"
  const accountId = params.get("account") || ""
  const accounts = useQuery({
    queryKey: [...mailKey, "accounts"],
    queryFn: () => mailGet<{ accounts: Account[] }>({ resource: "accounts" }),
  })
  const summary = useQuery({
    queryKey: [...mailKey, "summary", accountId],
    queryFn: () => mailGet<{ counts: Record<string, number>; unread: number }>({
      resource: "summary", account_id: accountId || undefined,
    }),
    refetchInterval: 30_000,
  })
  const navItems = [...folders, "contacts", "campaigns", "templates", "events"]

  return (
    <aside className="mail-sidebar flex flex-1 flex-col overflow-y-auto bg-ui-bg-subtle" aria-label={t("mail.title")}>
      <div className="p-3">
        <Link to="/orders" className="mail-nav-button justify-start gap-2.5">
          <svg width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden="true"><path d="m7 4-4 4 4 4M3 8h9a4 4 0 0 1 0 8H9" /></svg>
          <Text as="span" size="small" weight="plus">{t("mail.workspace")}</Text>
        </Link>
        <Divider variant="dashed" className="mt-3" />
      </div>
      <nav className="mail-nav" aria-label={t("mail.title")}>
        <Button size="small" className="mail-compose-button mb-3" disabled={!accounts.data?.accounts.some(a => a.enabled)} onClick={() => {
          const next = new URLSearchParams(params)
          next.set("compose", "1")
          setParams(next)
          closeMobile()
        }}>{t("mail.compose")}</Button>
        {navItems.map((item, index) => {
          const count = item === "inbox" ? summary.data?.unread : summary.data?.counts[item]
          return (
            <Link key={item} onClick={closeMobile} className={`mail-nav-button ${index === 6 ? "mt-5" : ""}`} aria-current={view === item ? "page" : undefined}
              to={{ pathname: "/mail", search: new URLSearchParams({ view: item, ...(accountId ? { account: accountId } : {}) }).toString() }}>
              <Text as="span" size="small">{t(`mail.${item}`)}</Text>
              {!!count && <Text as="span" size="xsmall" className={item === "inbox" ? "rounded bg-ui-tag-blue-bg px-1.5 text-ui-tag-blue-text" : "text-ui-fg-muted"}>{count}</Text>}
            </Link>
          )
        })}
      </nav>
      <div className="mt-auto p-3">
        <Divider variant="dashed" className="mb-3" />
        <Link className="mail-nav-button" to="/settings/mailboxes">{t("mail.openSettings")}</Link>
      </div>
    </aside>
  )
}
