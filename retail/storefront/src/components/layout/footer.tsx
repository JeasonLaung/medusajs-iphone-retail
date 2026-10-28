// Original footer body and CSS from medusa-store, with Gin data passed by the SSR caller.
import LocalizedClientLink from '../products/localized-link'
import BrandLogo from './brand-logo'
import {getLayoutCopy} from './i18n'
import {localeFromSegment} from '../../lib/storefront-route'
import {storeSeriesUrl} from '../../lib/store-series-url'
import type {FooterData} from '../../lib/footer-data'
import styles from './footer.module.css'

export default function Footer({data,segment,year=new Date().getFullYear()}:{data:FooterData;segment:string;year?:number}) {
  const copy=getLayoutCopy(localeFromSegment(segment))
  const {collections,iphoneCategory}=data
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
            <LocalizedClientLink segment={segment}
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
                  {group.links.map(link => <li key={link.href}><LocalizedClientLink segment={segment} href={link.href} className="hover:text-ui-fg-base">{link.label}</LocalizedClientLink></li>)}
                </ul>
              </div>
            ))}
            {iphoneCategory && (
              <div className="flex flex-col gap-y-2">
                <LocalizedClientLink segment={segment}
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
                      <LocalizedClientLink segment={segment}
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
                  className={`grid grid-cols-1 gap-2 text-ui-fg-subtle txt-small${collections.length>3?' grid-cols-2':''}`}
                >
                  {collections?.slice(0, 6).map((c) => (
                    <li key={c.id}>
                      <LocalizedClientLink segment={segment}
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
                    <LocalizedClientLink segment={segment} href={link.href} className="hover:text-ui-fg-base">
                      {link.label}
                    </LocalizedClientLink>
                  </li>
                ))}
              </ul>
            </div>
          </div>
        </div>
        <div className="flex w-full mb-16 justify-between text-ui-fg-muted">
          <p className="txt-medium txt-compact-small">
            © {year} EuroRefurb. {copy.rights}
          </p>
        </div>
      </div>
    </footer>
  )
}
