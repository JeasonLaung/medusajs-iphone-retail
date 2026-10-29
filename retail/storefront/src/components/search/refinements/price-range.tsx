"use client"

import * as Slider from "@radix-ui/react-slider"
import { useEffect, useState } from "react"
import { useInstantSearch, useRange } from "react-instantsearch"
import { useSearchRoute as useParams } from "../context"

import { indexedCurrency, priceAttribute } from "../client"
import { useStoreSearchData } from "../data"

function formatAmount(amount: number, currency: string) {
    try {
      return new Intl.NumberFormat(undefined, {
        style: "currency",
        currency: currency.toUpperCase(),
        maximumFractionDigits: 0,
      }).format(amount)
    } catch {
      // An unrecognised currency code shouldn't take the sidebar down.
      return `${Math.round(amount)} ${currency.toUpperCase()}`
    }
}

/**
 * Price filter over the region currency's `min_price_*` field, which is
 * declared `facetable({ types: ["stats"] })` — the stats facet is what
 * supplies the slider's bounds. Refines on release rather than per pixel.
 */
const PriceRange = ({ currencyCode }: { currencyCode: string }) => {
  const { countryCode } = useParams()
  const zh = countryCode?.endsWith("-zh")
  const { results } = useInstantSearch()
  const { distribution } = useStoreSearchData()
  const attribute = priceAttribute("min_price", currencyCode)
  const { start, range, canRefine, refine } = useRange({
    attribute,
  })
  const currency = indexedCurrency(currencyCode)
  const format = (amount: number) => formatAmount(amount, currency)

  const min = Math.floor(range.min ?? 0)
  const max = Math.ceil(range.max ?? 0)

  // `start` holds ±Infinity for an unset bound, so fall back to the extremes.
  const from = Number.isFinite(start[0]) ? (start[0] as number) : min
  const to = Number.isFinite(start[1]) ? (start[1] as number) : max

  const [value, setValue] = useState<number[]>([from, to])
  const [prices, setPrices] = useState<number[]>(distribution.prices)
  // Ignore this slider's refinement and pagination when building its context.
  // The distribution must cover every matching product, not just visible cards.
  // Build the distribution only from an actual, completed search. During mount
  // widgets and URL routing register in stages and expose transient helper state.
  const hasResults = !Boolean(
    (results as typeof results & { __isArtificial?: boolean }).__isArtificial
  )
  useEffect(() => {
    if (!hasResults) return
    const raw = (results as unknown as {_rawResults?:{__gin_distribution?:number[]}[]})._rawResults?.[0]
    setPrices(raw?.__gin_distribution ?? [])
  },[results,hasResults])

  const bins = Array.from({ length: 24 }, () => 0)
  for (const price of prices) {
    const index = Math.max(0, Math.min(bins.length - 1, Math.floor((price - min) / (max - min || 1) * bins.length)))
    bins[index]++
  }
  const peak = Math.max(1, ...bins)

  // Follow the refinement when it changes elsewhere — clearing all filters, or
  // a bound arriving from the URL on load.
  useEffect(() => {
    setValue([from, to])
  }, [from, to])

  const sliderDisabled = !canRefine || min >= max
  const hasPrices = prices.length > 0

  return (
    <div className="flex min-h-40 shrink-0 flex-col gap-y-4 border-t border-ui-border-base pt-4" data-testid="price-filter">
      <span className="text-base font-semibold leading-6 text-ui-fg-base">{zh ? "价格" : "Price"}</span>

      <div className="flex flex-col gap-y-3 px-2 small:pl-0 small:pr-6">
          <div
            className="flex h-12 shrink-0 items-end gap-px px-2"
            role="img"
            aria-label={zh ? "商品价格分布" : "Product price distribution"}
            data-testid="price-histogram"
          >
            {bins.map((count, index) => {
              const low = min + index * (max - min) / bins.length
              const high = min + (index + 1) * (max - min) / bins.length
              const selected = high >= value[0] && low <= value[1]
              return <span
                key={index}
                className="min-w-0 flex-1 rounded-t-full transition-colors"
                style={{ height: count ? `${Math.max(8, count / peak * 100)}%` : 0, backgroundColor: selected ? "#302cb5" : "#dddde7" }}
                title={`${format(low)} – ${format(high)}: ${count}`}
                data-count={count}
              />
            })}
          </div>
        <Slider.Root
          className="relative flex h-5 w-full touch-none select-none items-center"
          value={value}
          min={min}
          max={Math.max(max, min + 1)}
          disabled={sliderDisabled}
          step={1}
          minStepsBetweenThumbs={0}
          onValueChange={setValue}
          // Only refine when the thumb is released, so dragging doesn't fire a
          // search per pixel.
          onValueCommit={(committed) => refine([committed[0], committed[1]])}
          aria-label={zh ? "价格区间" : "Price range"}
          data-testid="price-range"
        >
          <Slider.Track className="relative h-0.5 w-full grow rounded-full bg-ui-border-base">
            <Slider.Range className="absolute h-full rounded-full bg-ui-fg-interactive" />
          </Slider.Track>
          {value.map((_, index) => (
            <Slider.Thumb
              key={index}
              className="block h-4 w-4 rounded-full border border-ui-border-interactive bg-ui-bg-base shadow-elevation-card-rest outline-none focus-visible:ring-2 focus-visible:ring-ui-fg-interactive"
              aria-label={index === 0 ? (zh ? "最低价格" : "Minimum price") : (zh ? "最高价格" : "Maximum price")}
            />
          ))}
        </Slider.Root>

        <div className="flex items-center justify-between">
          <span className="text-small-regular text-ui-fg-subtle">
            {hasPrices ? format(value[0]) : "—"}
          </span>
          <span className="text-small-regular text-ui-fg-subtle">
            {hasPrices ? format(value[1]) : "—"}
          </span>
        </div>
      </div>
    </div>
  )
}

export default PriceRange
