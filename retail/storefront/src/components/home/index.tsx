import { ArrowRight } from "../products/icons"
import Image from "../products/product-image"
import type { HomeData } from "../../lib/home"
import { storeSeriesUrl } from "./series-url"
import LocalizedClientLink from "./localized-link"
import type { HomeCopy } from "../journal/i18n"
import HomeCarousel from "./carousel"
import SeriesRail from "./rail"
import HomeReviews from "./reviews"
import BlogSection from "./blog"
import { HomeRoute } from "./localized-link"
import StorefrontTheme from "../products/storefront-theme"
import styles from "./gallery.module.css"

export default function Home({ segment, copy, data }: { segment:string; copy:HomeCopy; data:HomeData }) {
  const {series, reviews}=data
  return (
    <StorefrontTheme locale={segment}><HomeRoute segment={segment}><main className={styles.home}>
      <div className={styles.container}>
        <HomeCarousel
          copy={copy}
          slides={series.slice(0, 3).map(({ family, category }) => ({
            id: category.id,
            title: `iPhone ${family}`,
            image: category.image,
            href: storeSeriesUrl(category.name),
          }))}
        />
        <header className={styles.intro}>
          <div>
            <h1>{copy.title}</h1>
          </div>
          <LocalizedClientLink href="/store" className={styles.allModels}>
            {copy.allModels}
            <ArrowRight aria-hidden="true" width={20} height={20} className="inline-block align-middle shrink-0" />
          </LocalizedClientLink>
        </header>

        <SeriesRail copy={copy}>
          {series.map(({ family, category }, index) => {
            return (
              <li key={category.id}>
                <LocalizedClientLink
                  href={storeSeriesUrl(category.name)}
                  className={styles.card}
                  data-testid={`series-card-${family.toLowerCase()}`}
                >
                  <div className={styles.imageFrame}>
                    <Image
                      src={category.image}
                      alt={copy.imageAlt.replace("{name}", category.name)}
                      fill
                      sizes="100px"
                      priority={index < 5}
                      className={styles.image}
                    />
                  </div>
                  <div className={styles.cardHeading}>
                    <h2>{copy.seriesName.replace("{name}", `iPhone ${family}`)}</h2>
                  </div>
                </LocalizedClientLink>
              </li>
            )
          })}
        </SeriesRail>
        {!series.length && <p className={styles.description}>{copy.empty}</p>}
        <HomeReviews copy={copy} {...reviews} />
        <BlogSection copy={copy} />
      </div>
    </main></HomeRoute></StorefrontTheme>
  )
}
