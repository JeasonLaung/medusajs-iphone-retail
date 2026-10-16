"use client"

import clsx from "clsx"
import { useParams } from "next/navigation"
import { useRefinementList } from "react-instantsearch"
import { getRefinementLabel, getRefinementTitle } from "./i18n"
import useFilterOptions from "./use-filter-options"
import { useMobileFilter } from "./mobile-filter-context"
import styles from "./mobile-filters.module.css"

type RefinementGroupProps = {
  attribute: string
  title: string
}

const RefinementGroup = ({ attribute, title }: RefinementGroupProps) => {
  const { countryCode } = useParams<{ countryCode: string }>()
  const catalogOptions = useFilterOptions(attribute)
  const mobileFilter = useMobileFilter()
  const { items, refine } = useRefinementList({
    attribute,
    operator: "or",
    limit: 100,
    sortBy:
      attribute === "condition_options"
        ? ["name:desc"]
        : attribute === "display_size" ||
            attribute === "rear_cameras" ||
            attribute === "color_families"
          ? ["name:asc"]
          : ["count:desc", "name:asc"],
  })
  const allItems = [
    ...catalogOptions.map((option) => items.find((item) => item.value === option.value) ?? {
      value: option.value, label: option.value, count: 0, isRefined: false,
    }),
    ...items.filter((item) => !catalogOptions.some((option) => option.value === item.value)),
  ]
  const visibleItems =
    attribute === "category"
      ? allItems.filter((item) => /^iPhone (\d+|Air|SE)$/.test(item.value))
      : allItems

  if (attribute === "release_year") {
    visibleItems.sort((a, b) => Number(a.value) - Number(b.value))
  }

  if (!visibleItems.length) {
    return null
  }

  const isDropdown = (mobileFilter?.panel === "model" && attribute === "category")
    || (mobileFilter?.panel === "condition" && attribute === "condition_options")
  if (isDropdown) {
    return <fieldset className={styles.options} data-testid={`filter-group-${attribute}`}>
      <legend className="sr-only">{getRefinementTitle(attribute, countryCode) ?? title}</legend>
      {visibleItems.map((item) => <label key={item.value} className={styles.option} data-disabled={item.count === 0 && !item.isRefined}>
        <span>{getRefinementLabel(attribute, item.label, countryCode)}</span>
        <input
          type="checkbox"
          checked={item.isRefined}
          disabled={item.count === 0 && !item.isRefined}
          onChange={() => refine(item.value)}
          data-testid={`mobile-option-${attribute}`}
        />
      </label>)}
    </fieldset>
  }

  return (
    <fieldset
      className="min-w-0 small:pr-6"
      data-testid={`filter-group-${attribute}`}
    >
      <legend className="mb-3 w-full border-t border-ui-border-base pt-4 text-base font-semibold leading-6 text-ui-fg-base">
        {getRefinementTitle(attribute, countryCode) ?? title}
      </legend>
      <ul className="flex flex-wrap gap-2">
        {visibleItems.map((item) => (
          <li key={item.value}>
            <button
              type="button"
              aria-pressed={item.isRefined}
              disabled={item.count === 0 && !item.isRefined}
              onClick={() => refine(item.value)}
              className={clsx(
                "min-h-11 rounded-full border px-3 py-2 text-sm leading-4 transition-colors small:min-h-9 small:text-[13px] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ui-border-interactive focus-visible:ring-offset-2 disabled:opacity-40 disabled:cursor-not-allowed disabled:hover:border-[#a1a1aa] disabled:hover:bg-ui-bg-base",
                item.isRefined
                  ? "border-ui-fg-base bg-ui-fg-base text-ui-bg-base"
                  : "border-[#a1a1aa] bg-ui-bg-base text-ui-fg-subtle hover:border-[#71717a] hover:bg-ui-bg-subtle",
              )}
              data-testid={`refinement-${attribute}`}
            >
              {getRefinementLabel(attribute, item.label, countryCode)}
            </button>
          </li>
        ))}
      </ul>
    </fieldset>
  )
}

export default RefinementGroup
