import { listCategories } from "@lib/data/categories"
import { storeSeriesUrl } from "@lib/util/store-series-url"
import { listCollections } from "@lib/data/collections"
import { Text, clx } from "@modules/common/components/ui"

import LocalizedClientLink from "@modules/common/components/localized-client-link"
import { getLocale } from "@lib/data/locale-actions"
import { getLayoutCopy } from "@modules/layout/i18n"
import BrandLogo from "@modules/common/components/brand-logo"
import styles from "./styles.module.css"

export default async function Footer() {
  const copy = getLayoutCopy(await getLocale())
  const { collections } = await listCollections({
    fields: "*products",
  })
  const productCategories = await listCategories()
  const iphoneCategory = productCategories.find((c) => c.handle === "iphone")
  const groups = [
    { title: copy.help, links: [
      { href: "/help#service-title", label: copy.contact },
      { href: "/help#shopping", label: copy.shopping },
      { href: "/help#account", label: copy.orders },
    ] },
    { title: copy.knowledge, links: [
      { href: "/blog/choose-your-storage", label: copy.storageGuide },
      { href: "/blog/compare-iphone-configurations", label: copy.configurationGuide },
      { href: "/blog/before-you-order", label: copy.beforeOrder },
      { href: "/blog", label: copy.allArticles },
    ] },
  ]

  return (
    <footer className="border-t border-ui-border-base w-full">
      <div className="content-container flex flex-col w-full">
        <div className={`${styles.main} items-start py-40`}>
          <div className={styles.brand}>
            <LocalizedClientLink
              href="/"
              aria-label="EuroRefurb"
              className="txt-compact-xlarge-plus text-ui-fg-subtle hover:text-ui-fg-base"
            >
              <BrandLogo />
            </LocalizedClientLink>
            <p className={`${styles.description} text-small-regular text-ui-fg-subtle`}>{copy.footerDescription}</p>
          </div>
          <div className={`${styles.links} text-small-regular`}>
            {groups.map(group => (
              <div key={group.title} className="flex flex-col gap-y-2">
                <h2 className="txt-small-plus text-ui-fg-base">{group.title}</h2>
                <ul className="grid grid-cols-1 gap-2 text-ui-fg-subtle txt-small">
                  {group.links.map(link => <li key={link.href}><LocalizedClientLink href={link.href} className="hover:text-ui-fg-base">{link.label}</LocalizedClientLink></li>)}
                </ul>
              </div>
            ))}
            {iphoneCategory && (
              <div className="flex flex-col gap-y-2">
                <LocalizedClientLink
                  className="txt-small-plus txt-ui-fg-base hover:text-ui-fg-subtle"
                  href="/store"
                >
                  {iphoneCategory.name}
                </LocalizedClientLink>
                <ul
                  className="grid grid-cols-1 gap-2"
                  data-testid="footer-categories"
                >
                  {iphoneCategory.category_children?.map((c) => (
                    <li
                      className="flex flex-col gap-2 text-ui-fg-subtle txt-small"
                      key={c.id}
                    >
                      <LocalizedClientLink
                        className="hover:text-ui-fg-base"
                        href={storeSeriesUrl(c.name)}
                        data-testid="category-link"
                      >
                        {c.name}
                      </LocalizedClientLink>
                    </li>
                  ))}
                </ul>
              </div>
            )}
            {collections && collections.length > 0 && (
              <div className="flex flex-col gap-y-2">
                <span className="txt-small-plus txt-ui-fg-base">
                  {copy.collections}
                </span>
                <ul
                  className={clx(
                    "grid grid-cols-1 gap-2 text-ui-fg-subtle txt-small",
                    {
                      "grid-cols-2": (collections?.length || 0) > 3,
                    }
                  )}
                >
                  {collections?.slice(0, 6).map((c) => (
                    <li key={c.id}>
                      <LocalizedClientLink
                        className="hover:text-ui-fg-base"
                        href={`/collections/${c.handle}`}
                      >
                        {c.title}
                      </LocalizedClientLink>
                    </li>
                  ))}
                </ul>
              </div>
            )}
            <div className="flex flex-col gap-y-2">
              <span className="txt-small-plus txt-ui-fg-base">EuroRefurb</span>
              <ul className="grid grid-cols-1 gap-y-2 text-ui-fg-subtle txt-small">
                {[
                  { href: "/about", label: copy.about },
                  { href: "/help", label: copy.help },
                  { href: "/account", label: copy.account },
                  { href: "/store", label: copy.buy },
                  { href: "/sell", label: copy.sell },
                  { href: "/reviews", label: copy.reviews },
                ].map((link) => (
                  <li key={link.href}>
                    <LocalizedClientLink href={link.href} className="hover:text-ui-fg-base">
                      {link.label}
                    </LocalizedClientLink>
                  </li>
                ))}
              </ul>
            </div>
          </div>
        </div>
        <div className="flex w-full mb-16 justify-between text-ui-fg-muted">
          <Text className="txt-compact-small">
            © {new Date().getFullYear()} EuroRefurb. {copy.rights}
          </Text>
        </div>
      </div>
    </footer>
  )
}
