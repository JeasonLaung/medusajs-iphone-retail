import { ClearableInput } from "./clearable-input"
import { Button, Popover, Text } from "@medusajs/ui"
import { useQuery } from "@tanstack/react-query"
import { useEffect, useId, useRef, useState } from "react"
import { useTranslation } from "react-i18next"
import { sdk } from "../lib/sdk"

export type DeviceVariantChoice = {
  id: string
  sku: string
  title: string
  product: { title: string }
  requires_imei?: boolean
  options: { value: string }[]
  inventory_items: {
    inventory: {
      location_levels: {
        location_id: string
        stocked_quantity: number
        reserved_quantity: number
      }[]
    }
  }[]
}

export function deviceVariantLabel(variant: DeviceVariantChoice) {
  return `${variant.product?.title || ""} · ${variant.title || variant.options?.map((option) => option.value).join(" / ")} · ${variant.sku || variant.id}`
}

type Results = { variants: DeviceVariantChoice[]; count: number; offset: number; limit: number }

export default function DeviceVariantSelect({ value, onChange, disabled, scanSearch }: {
  value?: DeviceVariantChoice
  onChange: (variant: DeviceVariantChoice | undefined) => void
  disabled?: boolean
  scanSearch?: { query: string; revision: number }
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState("")
  const [debouncedSearch, setDebouncedSearch] = useState("")
  const [offset, setOffset] = useState(0)
  const [active, setActive] = useState(-1)
  const input = useRef<HTMLInputElement>(null)
  const listId = useId()
  const helpId = useId()

  useEffect(() => {
    if (!scanSearch?.query) return
    setSearch(scanSearch.query)
    setDebouncedSearch(scanSearch.query)
    setOffset(0)
    setActive(-1)
    setOpen(true)
  }, [scanSearch])

  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearch(search)
      setOffset(0)
      setActive(-1)
    }, 250)
    return () => clearTimeout(timer)
  }, [search])

  const results = useQuery<Results>({
    queryKey: ["device-variant-search", debouncedSearch, offset],
    queryFn: () => sdk.client.fetch("/admin/devices/variants", {
      query: { q: debouncedSearch, offset },
    }),
    enabled: open,
    staleTime: 30000,
  })
  const waiting = search !== debouncedSearch || results.isFetching
  const variants = results.data?.variants || []

  useEffect(() => {
    if (active >= 0) document.getElementById(`${listId}-${active}`)?.scrollIntoView({ block: "nearest" })
  }, [active, listId])

  function choose(variant?: DeviceVariantChoice) {
    onChange(variant)
    setOpen(false)
  }

  return (
    <div className="min-w-0 space-y-1">
      <Text size="small" leading="compact" weight="plus">{t("deviceScan.sku")}</Text>
      <Popover open={open && !disabled} onOpenChange={(next) => {
        setOpen(next)
        setActive(-1)
      }}>
        <Popover.Trigger asChild>
          <Button type="button" size="small" variant="secondary" disabled={disabled}
            aria-label={t("deviceScan.sku")} aria-haspopup="dialog" aria-expanded={open}
            className="h-auto min-h-9 w-full min-w-0 justify-between gap-2 text-left">
            <Text size="small" className="min-w-0 whitespace-normal break-words">
              {value ? deviceVariantLabel(value) : t("deviceScan.select")}
            </Text>
            <svg width="16" height="16" viewBox="0 0 20 20" fill="none" className="shrink-0" aria-hidden="true">
              <path d="m6 8 4 4 4-4" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
            </svg>
          </Button>
        </Popover.Trigger>
        <Popover.Content align="start" className="w-[36rem] max-w-[calc(100vw-2rem)] p-0"
          aria-label={t("deviceScan.sku")} onOpenAutoFocus={(event) => {
            event.preventDefault()
            input.current?.focus()
          }}>
          <div className="space-y-2 border-b border-ui-border-base p-3">
            <ClearableInput medusa ref={input} value={search} maxLength={200} autoComplete="off"
              placeholder={t("deviceScan.skuSearchPlaceholder")} aria-label={t("deviceScan.skuSearch")}
              role="combobox" aria-expanded={open} aria-autocomplete="list" aria-controls={listId}
              aria-describedby={helpId} aria-activedescendant={active >= 0 && !waiting ? `${listId}-${active}` : undefined}
              onChange={(event) => { setSearch(event.target.value); setActive(-1) }}
              onKeyDown={(event) => {
                if (event.nativeEvent.isComposing) return
                if (event.key === "ArrowDown" || event.key === "ArrowUp") {
                  event.preventDefault()
                  if (!waiting && variants.length) setActive((previous) =>
                    event.key === "ArrowDown"
                      ? (previous + 1) % variants.length
                      : (previous <= 0 ? variants.length : previous) - 1)
                } else if (event.key === "Enter") {
                  event.preventDefault()
                  if (!waiting && !results.isError && variants.length && (active >= 0 || variants.length === 1)) choose(variants[active >= 0 ? active : 0])
                }
              }} />
            <Text id={helpId} size="small" className="text-ui-fg-subtle">{t("deviceScan.skuSearchHelp")}</Text>
          </div>
          <div className="px-3 py-2" role="status" aria-live="polite">
            <Text size="small" className="text-ui-fg-subtle">
              {waiting || results.isPending ? t("deviceScan.loading") : results.isError
                ? t("deviceScan.skuSearchError") : t("deviceScan.skuSearchCount", { count: results.data?.count || 0 })}
            </Text>
          </div>
          {results.isError && (
            <Button type="button" size="small" variant="secondary" className="mx-3 mb-3" onClick={() => results.refetch()}>{t("orderDevices.retry")}</Button>
          )}
          <div id={listId} role="listbox" aria-label={t("deviceScan.sku")} aria-busy={waiting}
            className="max-h-64 overflow-y-auto overscroll-contain px-2 pb-2">
            {!waiting && !results.isError && variants.map((variant, index) => (
              <Button key={variant.id} id={`${listId}-${index}`} role="option" aria-selected={value?.id === variant.id}
                type="button" variant="transparent" size="small" tabIndex={-1}
                className={`h-auto w-full justify-start whitespace-normal break-words px-3 py-2 text-left ${active === index || value?.id === variant.id ? "bg-ui-bg-base-hover" : ""}`}
                onMouseEnter={() => setActive(index)} onClick={() => choose(variant)}>
                <Text size="small">{deviceVariantLabel(variant)}</Text>
              </Button>
            ))}
            {!waiting && !results.isPending && !results.isError && !variants.length && (
              <Text size="small" className="px-3 py-4 text-ui-fg-subtle">{t("deviceScan.skuSearchEmpty")}</Text>
            )}
          </div>
          <div className="flex items-center justify-between gap-2 border-t border-ui-border-base p-3">
            <Button type="button" size="small" variant="transparent" disabled={!value} onClick={() => choose()}>{t("deviceScan.skuClear")}</Button>
            <div className="flex gap-2">
              <Button type="button" size="small" variant="secondary" disabled={waiting || offset === 0}
                onClick={() => { setOffset(Math.max(0, offset - 20)); setActive(-1) }}>{t("deviceScan.previous")}</Button>
              <Button type="button" size="small" variant="secondary" disabled={waiting || !results.data || offset + results.data.limit >= results.data.count}
                onClick={() => { setOffset(offset + 20); setActive(-1) }}>{t("deviceScan.next")}</Button>
            </div>
          </div>
        </Popover.Content>
      </Popover>
    </div>
  )
}
