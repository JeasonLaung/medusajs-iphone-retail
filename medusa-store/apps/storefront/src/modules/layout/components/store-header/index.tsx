"use client"

import BrandLogo from "@modules/common/components/brand-logo"
import NotificationBell from "@modules/notifications/notification-bell"

import { ChevronDown, XMark } from "@medusajs/icons"
import { Popover, PopoverButton, PopoverPanel } from "@headlessui/react"
import { useParams, usePathname } from "next/navigation"
import type { StoreRegion } from "@medusajs/types"
import type { Locale } from "@lib/data/locales"
import RegionLanguage from "../region-language"
import LocalizedClientLink from "@modules/common/components/localized-client-link"
import { getLayoutCopy } from "@modules/layout/i18n"
import styles from "./styles.module.css"

export default function StoreHeader({ regions, locales, locale, cartCount, isLoggedIn }: {
  regions: StoreRegion[]
  locales: Locale[]
  locale: string | null
  cartCount: number
  isLoggedIn: boolean
}) {
  const copy = getLayoutCopy(locale)
  const { countryCode } = useParams<{ countryCode: string }>()
  const pathname = usePathname()
  const items = [
    { label: copy.ui, href: "/ui" },
    { label: copy.buy, href: "/store" },
    { label: copy.sell, href: "/sell" },
    { label: copy.about, href: "/about", children: [
      { label: copy.about, href: "/about" }, { label: copy.focus, href: "/about#our-focus" },
    ] },
    { label: copy.reviews, href: "/reviews" },
    { label: copy.help, href: "/help", children: [
      { label: copy.shopping, href: "/help#shopping" }, { label: copy.orders, href: "/help#account" },
    ] },
  ]
  const isActive = (href: string) => pathname === `/${countryCode}${href}` ||
    (href === "/store" && /\/(categories|products)\//.test(pathname))

  return (
    <header className={styles.header} data-signed-in={isLoggedIn}>
      <nav className={styles.inner} aria-label={copy.navigation}>
        <LocalizedClientLink href="/" className={styles.brand} aria-label="EuroRefurb" data-testid="nav-store-link"><BrandLogo /></LocalizedClientLink>
        <div className={styles.desktop}>
          {items.map((item) => item.children ? (
            <Popover className={styles.dropdown} key={item.href}>
              <PopoverButton className={styles.navLink} data-active={isActive(item.href)}>
                {item.label}<ChevronDown className={styles.chevron} aria-hidden="true" />
              </PopoverButton>
              <PopoverPanel className={styles.panel}>
                {({ close }) => <>{item.children.map((child) => (
                  <LocalizedClientLink key={child.href} href={child.href} onClick={() => close()}>{child.label}</LocalizedClientLink>
                ))}</>}
              </PopoverPanel>
            </Popover>
          ) : (
            <LocalizedClientLink key={item.href} href={item.href} className={styles.navLink}
              aria-current={isActive(item.href) ? "page" : undefined} data-active={isActive(item.href)}>{item.label}</LocalizedClientLink>
          ))}
        </div>
        <div className={styles.actions}>
          {isLoggedIn && <NotificationBell locale={locale} />}
          <LocalizedClientLink href="/account" className={styles.iconButton} aria-label={copy.account} data-testid="nav-account-link">
            <svg viewBox="0 0 24 24" fill={isLoggedIn ? "#000" : "none"} stroke={isLoggedIn ? "#000" : "currentColor"} strokeWidth="1.4" aria-hidden="true"><circle cx="12" cy="7" r="4" /><path d="M3 22v-2a9 9 0 0 1 18 0v2Z" /></svg>
          </LocalizedClientLink>
          <LocalizedClientLink href="/cart" className={styles.iconButton} aria-label={`${copy.cart} (${cartCount})`} data-testid="nav-cart-link">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.4" aria-hidden="true"><path d="M1 2h3l3 15h13l3-11H5" /><circle cx="9" cy="21" r="1" /><circle cx="19" cy="21" r="1" /></svg>
            {cartCount > 0 && <span className={styles.count} aria-hidden="true">{cartCount > 99 ? "99+" : cartCount}</span>}
          </LocalizedClientLink>
          <RegionLanguage regions={regions} locales={locales} locale={locale} />
          <Popover className={styles.mobile}>
            <PopoverButton className={styles.iconButton} aria-label={copy.menu} data-testid="nav-menu-button">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden="true"><path d="M3 6h18M3 12h18M3 18h18" /></svg>
            </PopoverButton>
            <PopoverPanel className={styles.mobilePanel} modal focus>
              {({ close }) => <>
                <button onClick={() => close()} className={styles.close} aria-label={copy.close}><XMark aria-hidden="true" width={24} height={24} className="inline-block align-middle shrink-0" /></button>
                {items.map((item) => <LocalizedClientLink key={item.href} href={item.href} onClick={() => close()} aria-current={isActive(item.href) ? "page" : undefined}>{item.label}</LocalizedClientLink>)}
              </>}
            </PopoverPanel>
          </Popover>
        </div>
      </nav>
    </header>
  )
}
