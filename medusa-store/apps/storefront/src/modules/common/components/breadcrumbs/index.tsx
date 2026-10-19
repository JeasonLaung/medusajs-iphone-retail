import { ChevronRightMini } from "@medusajs/icons"
import LocalizedClientLink from "@modules/common/components/localized-client-link"
import { getBreadcrumbCopy } from "./i18n"
import styles from "./styles.module.css"

export type BreadcrumbItem = {
  label: string
  /** Storefront-relative path, without the country/language prefix. */
  href?: string
}

type BreadcrumbsProps = {
  items: readonly BreadcrumbItem[]
  locale: string
  className?: string
}

/** Ancestors are links; the final item always represents the current page. */
export default function Breadcrumbs({ items, locale, className }: BreadcrumbsProps) {
  if (!items.length) return null

  return (
    <nav aria-label={getBreadcrumbCopy(locale).label} className={[styles.breadcrumbs, className].filter(Boolean).join(" ")}>
      <ol className={styles.list}>
        {items.map((item, index) => {
          const isCurrent = index === items.length - 1
          return (
            <li key={`${item.href ?? ""}-${index}`} className={styles.item}>
              {index > 0 && <ChevronRightMini className={styles.separator} aria-hidden="true" width={14} height={14} />}
              {isCurrent ? (
                <span className={styles.current} aria-current="page">{item.label}</span>
              ) : item.href ? (
                <LocalizedClientLink href={item.href} className={styles.link}>{item.label}</LocalizedClientLink>
              ) : (
                <span>{item.label}</span>
              )}
            </li>
          )
        })}
      </ol>
    </nav>
  )
}
