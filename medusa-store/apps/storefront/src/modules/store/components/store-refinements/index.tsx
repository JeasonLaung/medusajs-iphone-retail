"use client"

import { useInstantSearch, useSortBy } from "react-instantsearch"
import { useParams } from "next/navigation"
import { PRODUCT_INDEX_NAME } from "@lib/search-client"
import { getCapacityNote, getFilterActionsCopy, getSortLabel } from "./i18n"

import {
  BATTERY_OPTIONS_ATTRIBUTE,
  COLOR_OPTIONS_ATTRIBUTE,
  CATEGORY_ATTRIBUTE,
  CONDITION_OPTIONS_ATTRIBUTE,
  DISPLAY_SIZE_ATTRIBUTE,
  REAR_CAMERAS_ATTRIBUTE,
  RELEASE_YEAR_ATTRIBUTE,
  STORAGE_OPTIONS_ATTRIBUTE,
  getSortOptions,
} from "./attributes"
import PriceRange from "./price-range"
import RefinementGroup from "./refinement-group"
import MobileFilters from "./mobile-filters"
import { useMobileFilter } from "./mobile-filter-context"
import styles from "./mobile-filters.module.css"

const SortProducts = ({ currencyCode }: { currencyCode: string }) => {
  const { countryCode } = useParams<{ countryCode: string }>()
  const copy = getFilterActionsCopy(countryCode)
  const items = getSortOptions(currencyCode)
  const { currentRefinement, refine } = useSortBy({ items })
  const mobileFilter = useMobileFilter()

  if (mobileFilter?.panel === "sort") {
    return <div className={styles.options} role="group" aria-label={copy.sort}>
      {items.map((item) => <label key={item.value} className={styles.option}>
        <span>{getSortLabel(item.value, item.label, countryCode)}</span>
        <input
          type="radio"
          name="mobile-product-sort"
          value={item.value}
          checked={currentRefinement === item.value}
          onClick={() => { if (currentRefinement === item.value) mobileFilter.onSelect() }}
          onChange={() => { refine(item.value); mobileFilter.onSelect() }}
          data-testid="mobile-sort-option"
        />
      </label>)}
    </div>
  }

  return (
    <label className="flex flex-col gap-3 border-t border-ui-border-base pt-4 text-small-regular text-ui-fg-subtle">
      <span className="text-base font-semibold leading-6 text-ui-fg-base">{copy.sort}</span>
      <select
        value={currentRefinement}
        onChange={(event) => refine(event.target.value)}
        className="h-11 w-full rounded-md border border-ui-border-base bg-ui-bg-base px-3 text-base text-ui-fg-base small:h-10 small:text-sm"
        data-testid="sort-by-container"
      >
        {items.map((item) => (
          <option key={item.value} value={item.value}>
            {getSortLabel(item.value, item.label, countryCode)}
          </option>
        ))}
      </select>
    </label>
  )
}

const StoreRefinements = ({ currencyCode }: { currencyCode: string }) => {
  const { countryCode } = useParams<{ countryCode: string }>()
  const { setIndexUiState } = useInstantSearch()
  const clearFilters = () => setIndexUiState({
    sortBy: `${PRODUCT_INDEX_NAME}/sort/created_at:desc`,
  })
  const copy = getFilterActionsCopy(countryCode)
  return (
    <MobileFilters>
    <div className="mb-8 flex w-full min-w-0 flex-col gap-6 py-4 small:w-[250px] small:shrink-0">
      <div className="flex items-center justify-between gap-3" data-filter-section="heading">
        <h2 className="text-xl-semi">{copy.heading}</h2>
        <button
          type="button"
          onClick={clearFilters}
          className="min-h-11 shrink-0 rounded px-1 py-2 text-sm font-medium leading-5 text-ui-fg-subtle focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ui-border-interactive small:min-h-0"
          data-testid="clear-refinements"
        >
          {copy.clear}
        </button>
      </div>
      <div className="contents" data-filter-section="sort"><SortProducts currencyCode={currencyCode} /></div>
      <div className="contents" data-filter-section="model"><RefinementGroup attribute={CATEGORY_ATTRIBUTE} title="Series" /></div>
      <div className="contents" data-filter-section="price"><PriceRange currencyCode={currencyCode} /></div>
      <div className="contents" data-filter-section="other">
      <RefinementGroup attribute={STORAGE_OPTIONS_ATTRIBUTE} title="Storage" />
      <RefinementGroup attribute={BATTERY_OPTIONS_ATTRIBUTE} title="Battery" />
      <RefinementGroup
        attribute={DISPLAY_SIZE_ATTRIBUTE}
        title="Display Size"
      />
      <RefinementGroup attribute={COLOR_OPTIONS_ATTRIBUTE} title="Colour" />
      <RefinementGroup attribute={REAR_CAMERAS_ATTRIBUTE} title="Camera" />
      </div>
      <div className="contents" data-filter-section="condition"><RefinementGroup
        attribute={CONDITION_OPTIONS_ATTRIBUTE}
        title="Condition"
      /></div>
      <div className="contents" data-filter-section="other"><RefinementGroup
        attribute={RELEASE_YEAR_ATTRIBUTE}
        title="Release year"
      />
      <p className="text-small-regular text-ui-fg-muted small:pr-6">
        {getCapacityNote(countryCode)}
      </p>
      </div>
    </div>
    </MobileFilters>
  )
}

export default StoreRefinements
