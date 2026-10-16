"use client"

import type { Hit } from "instantsearch.js"
import { useEffect, useRef } from "react"
import Image from "next/image"
import { useParams } from "next/navigation"
import { useHits, useInstantSearch } from "react-instantsearch"

import useSearchSettled from "@lib/hooks/use-search-settled"
import LocalizedClientLink from "@modules/common/components/localized-client-link"
import { Text } from "@modules/common/components/ui"
import ColorSwatches from "@modules/products/components/color-swatches"
import SkeletonProductGrid from "@modules/skeletons/templates/skeleton-product-grid"
import SearchPagination from "./pagination"
import HitPrice, { HitPricing } from "./price"
import { getProductCardCopy } from "./i18n"
import styles from "./styles.module.css"

type ProductHit = Hit<
  {
    title: string | null
    handle: string | null
    thumbnail: string | null
    color_options?: string[]
    color_hexes?: string[]
    storage_options?: string[]
    model_storage_options?: string[]
    sku_storage_options?: string[]
    stocked_storage_options?: string[]
    sku_color_options?: string[]
  } & HitPricing
>

type StoreHitsProps = {
  hitsPerPage: number
  currencyCode: string
}

const StoreHits = ({ hitsPerPage, currencyCode }: StoreHitsProps) => {
  const { countryCode } = useParams<{ countryCode: string }>()
  const copy = getProductCardCopy(countryCode)
  const { items } = useHits<ProductHit>()
  const { status, error, indexUiState } = useInstantSearch()
  const { isSearching, hasNoResultsYet } = useSearchSettled()

  const { page, ...refinements } = indexUiState
  const currentPage = page ?? 1
  const refinementKey = JSON.stringify(refinements)

  const settled = useRef({ page: currentPage, refinementKey })

  useEffect(() => {
    if (!isSearching) {
      settled.current = { page: currentPage, refinementKey }
    }
  }, [isSearching, currentPage, refinementKey])

  const isPagingOnly =
    refinementKey === settled.current.refinementKey &&
    currentPage !== settled.current.page

  const showSkeleton = hasNoResultsYet || (isSearching && isPagingOnly)

  if (status === "error") {
    return (
      <Text
        className="py-16 text-center text-ui-fg-error"
        data-testid="products-error"
      >
        Couldn&apos;t load products
        {error?.message ? `: ${error.message}` : "."}
      </Text>
    )
  }

  return (
    <>
      {showSkeleton ? (
        <SkeletonProductGrid numberOfProducts={hitsPerPage} />
      ) : !items.length ? (
        <Text
          className="py-16 text-center text-ui-fg-subtle"
          data-testid="no-products"
        >
          No products matched these filters.
        </Text>
      ) : (
        <ul
          className={styles.grid}
          data-testid="products-list"
        >
          {items.map((hit) =>
            hit.handle ? (
              <li key={hit.objectID}>
                <LocalizedClientLink
                  href={`/products/${hit.handle}`}
                  className={styles.card}
                >
                  <div className={styles.contents} data-testid="product-wrapper">
                    <div className={styles.imageFrame}>
                      {hit.thumbnail && <Image
                        src={hit.thumbnail}
                        alt={hit.title ?? "iPhone"}
                        fill
                        sizes="(max-width: 599px) 35vw, (max-width: 1199px) 30vw, 240px"
                        className={styles.image}
                      />}
                    </div>
                    <div className={styles.details}>
                      <Text
                        className={styles.title}
                        data-testid="product-title"
                      >
                        {hit.title}
                      </Text>
                      {(hit.model_storage_options ?? hit.storage_options ?? []).length > 0 && (
                        <div className={styles.capacities} role="group" aria-label={copy.storage}>
                          {(hit.model_storage_options ?? hit.storage_options ?? []).map((storage) => {
                            const disabled = !(hit.stocked_storage_options ?? []).includes(storage)
                            return <span
                              className={styles.tag}
                              key={storage}
                              aria-disabled={disabled || undefined}
                              data-disabled={disabled}
                              title={disabled ? copy.outOfStock : undefined}
                              data-testid="product-storage"
                            >{storage}</span>
                          })}
                        </div>
                      )}
                    {Array.isArray(hit.color_options) &&
                      Array.isArray(hit.color_hexes) &&
                      hit.color_options.length === hit.color_hexes.length &&
                      hit.color_options.length > 0 && (
                        <div className={styles.colours}>
                          <span className={styles.label}>{copy.modelColours}</span>
                          <ColorSwatches
                            unavailableLabel={copy.noSku}
                            swatches={hit.color_options.map((name, index) => ({
                              name,
                              hex: hit.color_hexes![index],
                              disabled: !(hit.sku_color_options ?? []).some((value) => value.trim().toLowerCase() === name.trim().toLowerCase()),
                            }))}
                          />
                        </div>
                      )}
                      <div className={styles.priceRow}>
                        <HitPrice hit={hit} currencyCode={currencyCode} />
                        <span className={styles.viewButton}>{copy.view}</span>
                      </div>
                    </div>
                  </div>
                </LocalizedClientLink>
              </li>
            ) : null
          )}
        </ul>
      )}
      <SearchPagination />
    </>
  )
}

export default StoreHits
