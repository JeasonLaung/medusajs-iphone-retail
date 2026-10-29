"use client"

import { XMark } from "../icons"
import { useSearchRoute as useParams } from "../context"
import { useCallback, useEffect, useId, useRef, useState, type ReactNode } from "react"
import { createPortal } from "react-dom"
import { useInstantSearch } from "react-instantsearch"
import { getFilterActionsCopy } from "./i18n"
import styles from "./mobile-filters.module.css"
import { MobileFilterContext } from "./mobile-filter-context"
import useIsMobile from "../use-is-mobile"
import StoreSearchBox from "../box"

const MobileFilters = ({ children }: { children: ReactNode }) => {
  const isMobile = useIsMobile()
  const [activePanel, setActivePanel] = useState<"sort" | "price" | "model" | "condition" | "all" | null>(null)
  const [panelTop, setPanelTop] = useState(0)
  const toolbar = useRef<HTMLDivElement>(null)
  const trigger = useRef<HTMLButtonElement | null>(null)
  const closeButton = useRef<HTMLButtonElement>(null)
  const panelElement = useRef<HTMLElement>(null)
  const panelId = useId()
  const { indexUiState, setIndexUiState } = useInstantSearch()
  const { countryCode } = useParams()
  const copy = getFilterActionsCopy(countryCode)
  const close = useCallback(() => {
    setActivePanel(null)
    trigger.current?.focus({ preventScroll: true })
  }, [])
  const isOpen = activePanel !== null
  const clearCurrentFilter = () => {
    setIndexUiState((previous) => {
      const next = { ...previous, page: 1 }
      if (activePanel === "all") {
        delete next.refinementList
        delete next.range
      } else if (activePanel === "price") {
        next.range = Object.fromEntries(Object.entries(previous.range ?? {})
          .filter(([attribute]) => !attribute.startsWith("min_price_")))
      } else if (activePanel === "model" || activePanel === "condition") {
        next.refinementList = { ...previous.refinementList }
        delete next.refinementList[activePanel === "model" ? "category" : "condition_options"]
      }
      return next
    })
    close()
  }
  const isDropdown = activePanel === "sort" || activePanel === "model" || activePanel === "condition"
  const open = (panel: NonNullable<typeof activePanel>, button: HTMLButtonElement) => {
    if (activePanel === panel) {
      close()
      return
    }
    trigger.current = button
    setPanelTop(toolbar.current?.getBoundingClientRect().bottom ?? 0)
    setActivePanel(panel)
  }
  useEffect(() => {
    if (!isOpen || !isMobile) return
    const focusTarget = isDropdown
      ? panelElement.current?.querySelector<HTMLElement>("input:checked, input:not(:disabled)")
      : closeButton.current
    focusTarget?.focus({ preventScroll: true })
    const updatePosition = () => setPanelTop(toolbar.current?.getBoundingClientRect().bottom ?? 0)
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault()
        close()
      }
    }
    window.addEventListener("scroll", updatePosition, { passive: true })
    window.addEventListener("resize", updatePosition)
    window.addEventListener("keydown", onKeyDown)
    return () => {
      window.removeEventListener("scroll", updatePosition)
      window.removeEventListener("resize", updatePosition)
      window.removeEventListener("keydown", onKeyDown)
    }
  }, [activePanel, isOpen, isMobile, isDropdown, close])
  const refinements = indexUiState.refinementList ?? {}
  const count = Object.values(refinements).reduce((total, values) => total + values.length, 0)
    + Object.values(indexUiState.range ?? {}).filter(Boolean).length
  const tabs = [
    { id: "sort", label: copy.sort, selected: Boolean(indexUiState.sortBy?.includes("/sort/")) },
    { id: "price", label: copy.price, selected: Object.values(indexUiState.range ?? {}).some(Boolean) },
    { id: "model", label: copy.model, selected: Boolean(refinements.category?.length) },
    { id: "condition", label: copy.condition, selected: Boolean(refinements.condition_options?.length) },
  ] as const

  if (!isMobile) {
    return <div className="hidden small:block small:w-[250px] small:shrink-0">{children}</div>
  }

  return (
    <>
      <div ref={toolbar} className={styles.toolbar} data-testid="mobile-filter-toolbar">
        <StoreSearchBox compact />
        <div className={styles.toolbarRow} role="group" aria-label={copy.toolbar}>
        <div className={styles.rail}>
          {tabs.map((tab) => <button
            key={tab.id}
            type="button"
            onClick={(event) => open(tab.id, event.currentTarget)}
            aria-haspopup="dialog"
            aria-controls={isOpen ? panelId : undefined}
            aria-expanded={activePanel === tab.id}
            data-active={tab.selected}
            className={styles.chip}
            data-testid={`mobile-filter-${tab.id}`}
          >
            {tab.id === "sort" && <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.3" aria-hidden="true"><path d="M5 13V3m-3 3 3-3 3 3m3-3v10m-3-3 3 3 3-3" /></svg>}
            {tab.label}<span className={styles.chevron} aria-hidden="true" />
          </button>)}
        </div>
        <button
          type="button"
          onClick={(event) => open("all", event.currentTarget)}
          aria-label={copy.open}
          aria-haspopup="dialog"
          aria-controls={isOpen ? panelId : undefined}
          aria-expanded={activePanel === "all"}
          data-active={count > 0}
          className={`${styles.chip} ${styles.all}`}
          data-testid="open-mobile-filters"
        >
          <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.3" aria-hidden="true"><path d="M2 3h12L9.5 8v4.5l-3 1V8L2 3Z" /></svg>
          {copy.all}{count > 0 && <span className={styles.count}>{count}</span>}
        </button>
        </div>
      </div>
      {createPortal(<div hidden={!isOpen} className={styles.layer} style={{ top: panelTop }}>
        <div className={styles.backdrop} onClick={close} aria-hidden="true" />
        <div className={styles.panelFrame}>
          <section ref={panelElement} id={panelId} role="dialog" aria-modal="false" aria-labelledby={`${panelId}-title`} className={styles.panel} data-testid="mobile-filters-panel">
            <div className={isDropdown ? "sr-only" : "flex shrink-0 items-center justify-between border-b border-ui-border-base px-5 py-3"}>
              <h2 id={`${panelId}-title`} className="text-base font-semibold text-ui-fg-base">{tabs.find((tab) => tab.id === activePanel)?.label ?? copy.open}</h2>
              {!isDropdown && <button
                ref={closeButton}
                type="button"
                onClick={close}
                aria-label={copy.close}
                className="flex h-11 w-11 items-center justify-center rounded-full text-ui-fg-subtle focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ui-border-interactive"
                data-testid="close-mobile-filters"
              >
                <XMark />
              </button>}
            </div>
            <MobileFilterContext.Provider value={{ panel: activePanel, onSelect: close }}>
              <div className={`${styles.sections} min-h-0 overflow-y-auto overscroll-contain px-5`} data-panel={activePanel ?? "all"}>{children}</div>
            </MobileFilterContext.Provider>
            {activePanel !== "sort" && <div className={`${styles.footer} shrink-0 border-t border-ui-border-base px-5 pt-3 pb-[max(1rem,env(safe-area-inset-bottom))]`}>
              <button
                type="button"
                onClick={clearCurrentFilter}
                className="min-h-11 rounded-md border border-ui-border-base bg-ui-bg-base px-4 py-3 text-base font-semibold text-ui-fg-base focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ui-border-interactive focus-visible:ring-offset-2"
                data-testid="cancel-current-filter"
              >
                {copy.cancel}
              </button>
              <button
                type="button"
                onClick={close}
                className="min-h-11 w-full rounded-md bg-ui-fg-base px-4 py-3 text-base font-semibold text-ui-bg-base focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ui-border-interactive focus-visible:ring-offset-2"
                data-testid="show-filter-results"
              >
                {copy.confirm}
              </button>
            </div>}
          </section>
        </div>
      </div>, document.body)}
    </>
  )
}

export default MobileFilters
