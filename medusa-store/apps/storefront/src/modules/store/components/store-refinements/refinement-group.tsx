"use client"

import clsx from "clsx"
import { ChevronDownMini } from "@medusajs/icons"
import { useState } from "react"
import { useRefinementList } from "react-instantsearch"

type RefinementGroupProps = {
  attribute: string
  title: string
}

const RefinementGroup = ({ attribute, title }: RefinementGroupProps) => {
  const [open, setOpen] = useState(title === "Model")
  const { items, refine } = useRefinementList({
    attribute,
    limit: 100,
    sortBy:
      title === "Release year" || title === "Condition"
        ? ["name:desc"]
        : title === "Display Size" || title === "Camera"
        ? ["name:asc"]
        : ["count:desc", "name:asc"],
  })

  if (!items.length) {
    return null
  }

  const selectedCount = items.filter((item) => item.isRefined).length

  return (
    <div className="flex flex-col gap-y-3 pr-6">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        aria-expanded={open}
        className="flex w-full items-center justify-between text-left txt-compact-small-plus text-ui-fg-subtle"
      >
        <span>{title}{selectedCount > 0 ? ` (${selectedCount})` : ""}</span>
        <ChevronDownMini className={clsx("transition-transform", { "rotate-180": open })} />
      </button>
      {open && <ul className="flex max-h-52 flex-col gap-y-2 overflow-y-auto">
        {items.map((item) => (
          <li key={item.value}>
            <label className="flex cursor-pointer items-start gap-x-2">
              <input
                type="checkbox"
                checked={item.isRefined}
                onChange={() => refine(item.value)}
                className="mt-1 shrink-0 accent-ui-fg-interactive"
                data-testid={`refinement-${attribute}`}
              />
              <span
                className={clsx("text-small-regular min-w-0 break-words", {
                  "text-ui-fg-base": item.isRefined,
                  "text-ui-fg-subtle": !item.isRefined,
                })}
              >
                {item.label}
              </span>
              <span className="text-small-regular shrink-0 text-ui-fg-muted">
                ({item.count})
              </span>
            </label>
          </li>
        ))}
      </ul>}
    </div>
  )
}

export default RefinementGroup
