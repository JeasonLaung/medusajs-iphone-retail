"use client"

import { ArrowUpMini } from "@medusajs/icons"
import { theme } from "antd"
import { type CSSProperties, useEffect, useState } from "react"
import LocalizedClientLink from "@modules/common/components/localized-client-link"
import StorefrontTheme from "@modules/common/components/storefront-theme"
import { getLayoutCopy } from "@modules/layout/i18n"
import styles from "./styles.module.css"

export default function FloatingActions({ locale }: { locale: string | null }) {
  return <StorefrontTheme locale={locale}><FloatingActionButtons locale={locale} /></StorefrontTheme>
}

function FloatingActionButtons({ locale }: { locale: string | null }) {
  const copy = getLayoutCopy(locale)
  const { token } = theme.useToken()
  const [showBackToTop, setShowBackToTop] = useState(false)

  useEffect(() => {
    const update = () => setShowBackToTop(window.scrollY > 300)
    update()
    window.addEventListener("scroll", update, { passive: true })
    return () => window.removeEventListener("scroll", update)
  }, [])

  return (
    <div className={styles.actions} data-testid="floating-actions" data-development={process.env.NODE_ENV === "development"} style={{
      "--floating-brand": token.colorPrimary,
      "--floating-hover": token.colorPrimaryHover,
      "--floating-active": token.colorPrimaryActive,
      "--floating-surface": token.colorBgContainer,
      "--floating-text": token.colorText,
      "--floating-border": token.colorBorder,
      "--floating-selected": token.colorPrimaryBg,
      "--floating-whatsapp": "#25d366",
      "--floating-whatsapp-hover": "#1ebe5d",
    } as CSSProperties}>
      {showBackToTop && (
        <button type="button" className={styles.action} aria-label={copy.backToTop} data-label={copy.backToTop}
          onClick={() => window.scrollTo({ top: 0, behavior: window.matchMedia("(prefers-reduced-motion: reduce)").matches ? "instant" : "smooth" })}>
          <ArrowUpMini width={28} height={28} aria-hidden="true" />
        </button>
      )}
      <LocalizedClientLink href="/help#service-title" className={`${styles.action} ${styles.contact}`} aria-label={copy.whatsappContact} data-label={copy.whatsappContact}>
        <svg viewBox="0 0 32 32" aria-hidden="true" focusable="false">
          <path fill="currentColor" d="M16.02 3C8.85 3 3.02 8.83 3.02 16c0 2.29.6 4.53 1.74 6.5L3 29l6.68-1.71A12.94 12.94 0 0 0 16.02 29c7.17 0 13-5.83 13-13S23.19 3 16.02 3Zm0 23.62a10.58 10.58 0 0 1-5.4-1.48l-.39-.23-3.96 1.01 1.06-3.85-.25-.4A10.55 10.55 0 1 1 16.02 26.62Zm5.79-7.88c-.32-.16-1.89-.93-2.18-1.04-.29-.1-.5-.16-.72.16-.21.32-.82 1.04-1 1.25-.18.21-.37.24-.69.08-1.86-.93-3.08-1.66-4.3-3.76-.32-.55.32-.51.92-1.7.11-.21.05-.4-.03-.56-.08-.16-.72-1.73-.98-2.37-.26-.62-.53-.54-.72-.55h-.61c-.21 0-.56.08-.85.4-.29.32-1.12 1.09-1.12 2.66s1.15 3.09 1.31 3.3c.16.21 2.26 3.45 5.47 4.84.76.33 1.36.53 1.82.68.77.24 1.47.2 2.03.12.62-.09 1.89-.77 2.16-1.52.27-.75.27-1.39.19-1.52-.08-.13-.29-.21-.61-.37Z" />
        </svg>
      </LocalizedClientLink>
    </div>
  )
}
