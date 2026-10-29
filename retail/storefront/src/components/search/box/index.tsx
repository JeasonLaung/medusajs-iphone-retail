"use client"

import { MagnifyingGlass, XMarkMini } from "../icons"
import { useCallback, useEffect, useRef, useState } from "react"
import { useSearchBox } from "react-instantsearch"
import { useSearchRoute as useParams } from "../context"
import { getSearchCopy } from "./i18n"

const DEBOUNCE_MS = 250

/**
 * Free-text search over the listing, refining the same InstantSearch state the
 * sidebar filters do. The input is held locally so typing stays responsive
 * while the query itself is debounced.
 */
const StoreSearchBox = ({ compact = false }: { compact?: boolean }) => {
  const { countryCode } = useParams()
  const copy = getSearchCopy(countryCode)
  const timer = useRef<number | undefined>(undefined)

  const queryHook = useCallback(
    (nextQuery: string, search: (value: string) => void) => {
      window.clearTimeout(timer.current)
      timer.current = window.setTimeout(() => search(nextQuery), DEBOUNCE_MS)
    },
    []
  )

  const { query, refine } = useSearchBox({ queryHook })
  const [inputValue, setInputValue] = useState(query)

  useEffect(() => () => window.clearTimeout(timer.current), [])

  // Follow the query when it changes elsewhere — cleared with the filters, or
  // arriving from the URL on load, which `routing` restores after mount.
  useEffect(() => {
    setInputValue(query)
  }, [query])

  const clear = () => {
    window.clearTimeout(timer.current)
    setInputValue("")
    refine("")
  }

  return (
    <div className={compact
      ? "flex w-full items-center gap-x-2 rounded-md border border-ui-border-base bg-ui-bg-subtle px-3"
      : "mb-6 flex items-center gap-x-3 border-b border-ui-border-base"}>
      <MagnifyingGlass className="shrink-0 text-ui-fg-muted" />
      <input
        type="search"
        value={inputValue}
        onChange={(event) => {
          setInputValue(event.target.value)
          refine(event.target.value)
        }}
        placeholder={copy.placeholder}
        aria-label={copy.placeholder}
        className={`${compact ? "h-11 text-base" : "txt-medium py-3"} w-full min-w-0 bg-transparent text-ui-fg-base outline-none placeholder:text-ui-fg-muted [&::-webkit-search-cancel-button]:hidden`}
        data-testid="store-search-input"
      />
      {inputValue && (
        <button
          type="button"
          onClick={clear}
          aria-label={copy.clear}
          className="flex h-11 w-8 shrink-0 items-center justify-center text-ui-fg-muted hover:text-ui-fg-base"
          data-testid="store-search-clear"
        >
          <XMarkMini />
        </button>
      )}
    </div>
  )
}

export default StoreSearchBox
