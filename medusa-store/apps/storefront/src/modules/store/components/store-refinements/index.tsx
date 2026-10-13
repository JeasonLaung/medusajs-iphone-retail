"use client"

import { useSortBy } from "react-instantsearch"

import {
  BATTERY_OPTIONS_ATTRIBUTE,
  COLOR_OPTIONS_ATTRIBUTE,
  CONDITION_OPTIONS_ATTRIBUTE,
  DISPLAY_SIZE_ATTRIBUTE,
  MODEL_ATTRIBUTE,
  REAR_CAMERAS_ATTRIBUTE,
  RELEASE_YEAR_ATTRIBUTE,
  STORAGE_OPTIONS_ATTRIBUTE,
  getSortOptions,
} from "./attributes"
import CurrentRefinements from "./current-refinements"
import PriceRange from "./price-range"
import RefinementGroup from "./refinement-group"

const SortProducts = ({ currencyCode }: { currencyCode: string }) => {
  const items = getSortOptions(currencyCode)
  const { currentRefinement, refine } = useSortBy({ items })

  return (
    <label className="flex flex-col gap-2 text-small-regular text-ui-fg-subtle">
      Sort by
      <select
        value={currentRefinement}
        onChange={(event) => refine(event.target.value)}
        className="h-10 w-full rounded-md border border-ui-border-base bg-ui-bg-base px-3 text-ui-fg-base"
        data-testid="sort-by-container"
      >
        {items.map((item) => (
          <option key={item.value} value={item.value}>
            {item.label}
          </option>
        ))}
      </select>
    </label>
  )
}

const StoreRefinements = ({ currencyCode }: { currencyCode: string }) => {
  return (
    <div className="flex flex-col gap-6 py-4 mb-8 small:px-0 pl-6 small:w-[250px] small:shrink-0 small:ml-[1.675rem]">
      <h2 className="text-xl-semi">Filter by</h2>
      <SortProducts currencyCode={currencyCode} />
      <CurrentRefinements currencyCode={currencyCode} />
      <RefinementGroup attribute={MODEL_ATTRIBUTE} title="Model" />
      <PriceRange currencyCode={currencyCode} />
      <RefinementGroup attribute={STORAGE_OPTIONS_ATTRIBUTE} title="Storage" />
      <RefinementGroup attribute={BATTERY_OPTIONS_ATTRIBUTE} title="Battery" />
      <RefinementGroup attribute={DISPLAY_SIZE_ATTRIBUTE} title="Display Size" />
      <RefinementGroup attribute={COLOR_OPTIONS_ATTRIBUTE} title="Colour" />
      <RefinementGroup attribute={REAR_CAMERAS_ATTRIBUTE} title="Camera" />
      <RefinementGroup attribute={CONDITION_OPTIONS_ATTRIBUTE} title="Condition" />
      <RefinementGroup attribute={RELEASE_YEAR_ATTRIBUTE} title="Release year" />
      <p className="text-small-regular text-ui-fg-muted pr-6">
        Storage, battery, colour and condition describe model options. This
        snapshot provides one priced SKU per model.
      </p>
    </div>
  )
}

export default StoreRefinements
