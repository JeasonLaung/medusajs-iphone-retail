import { ArrowRight } from "@medusajs/icons"
import Image from "next/image"
import { listCategories } from "@lib/data/categories"
import { storeSeriesUrl } from "@lib/util/store-series-url"
import LocalizedClientLink from "@modules/common/components/localized-client-link"
import type { HomeCopy } from "@modules/home/i18n"
import HomeCarousel from "@modules/home/components/home-carousel"
import SeriesRail from "@modules/home/components/series-rail"
import HomeReviews from "@modules/home/components/home-reviews"
import BlogSection from "@modules/home/components/blog-section"
import { Suspense } from "react"
import styles from "./styles.module.css"

const families = ["17", "Air", "16", "15", "14", "13", "12", "11", "SE"]
const seriesImages: Record<string, string> = {
  "17": "/iphone-series/iphone-17/series.webp",
  Air: "/iphone-series/iphone-air/series.webp",
  "16": "/iphone-series/iphone-16/series.webp",
  "15": "/iphone-series/iphone-15/series.webp",
  "14": "/iphone-series/iphone-14/series.webp",
  "13": "/iphone-series/iphone-13.png",
  "12": "/iphone-series/iphone-12.png",
  "11": "/iphone-series/iphone-11.png",
  SE: "/iphone-series/iphone-se.png",
}

export default async function SeriesGallery({ copy }: { copy: HomeCopy }) {
  const categories = await listCategories({
    fields: "id,name,handle,parent_category_id,*products",
  })
  const parent = categories.find((category) => category.handle === "iphone")
  const series = families.flatMap((family) => {
    const category = categories.find(
      (entry) =>
        entry.handle === `iphone-${family.toLowerCase()}-series` &&
        entry.parent_category_id === parent?.id
    )
    return category ? [{ family, category }] : []
  })

  return (
    <main className={styles.home}>
      <div className={styles.container}>
        <HomeCarousel
          copy={copy}
          slides={series.slice(0, 3).map(({ family, category }) => ({
            id: category.id,
            title: `iPhone ${family}`,
            image: seriesImages[family],
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
                      src={seriesImages[family]}
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
        <Suspense fallback={<section className={styles.description} aria-busy="true">{copy.reviewsLoading}</section>}>
          <HomeReviews copy={copy} products={Array.from(new Map(series.flatMap(({ category }) => category.products ?? []).map((product) => [product.id, product])).values())} />
        </Suspense>
        <BlogSection copy={copy} />
      </div>
    </main>
  )
}
