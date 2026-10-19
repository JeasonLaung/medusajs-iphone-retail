import { listCategories } from "@lib/data/categories"
import { storeSeriesUrl } from "@lib/util/store-series-url"
import { listCollections } from "@lib/data/collections"
import { Text, clx } from "@modules/common/components/ui"

import LocalizedClientLink from "@modules/common/components/localized-client-link"
import { getLocale } from "@lib/data/locale-actions"
import { getLayoutCopy } from "@modules/layout/i18n"

export default async function Footer() {
  const copy = getLayoutCopy(await getLocale())
  const { collections } = await listCollections({
    fields: "*products",
  })
  const productCategories = await listCategories()
  const iphoneCategory = productCategories.find((c) => c.handle === "iphone")

  return (
    <footer className="border-t border-ui-border-base w-full">
      <div className="content-container flex flex-col w-full">
        <div className="flex flex-col gap-y-6 xsmall:flex-row items-start justify-between py-40">
          <div>
            <LocalizedClientLink
              href="/"
              className="txt-compact-xlarge-plus text-ui-fg-subtle hover:text-ui-fg-base"
            >
              <span className="brand-wordmark">EuroRefurb</span>
            </LocalizedClientLink>
          </div>
          <div className="text-small-regular gap-10 md:gap-x-16 grid grid-cols-2 sm:grid-cols-3">
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
