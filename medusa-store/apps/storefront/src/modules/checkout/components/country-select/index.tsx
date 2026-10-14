"use client"

import {
  Combobox,
  ComboboxButton,
  ComboboxInput,
  ComboboxOption,
  ComboboxOptions,
} from "@headlessui/react"
import { ChevronUpDown, Check } from "@medusajs/icons"
import { HttpTypes } from "@medusajs/types"
import {
  forwardRef,
  useEffect,
  useImperativeHandle,
  useMemo,
  useRef,
  useState,
} from "react"
import ReactCountryFlag from "react-country-flag"

import { getCountryOptions, normalizeCountrySearch } from "@lib/util/countries"
import { getLocale } from "@lib/data/locale-actions"
import { countrySelectTranslations } from "@lib/data/country-select-translations"
import { NativeSelectProps } from "@modules/common/components/native-select"
import { clx } from "@modules/common/components/ui"

type CountrySelectProps = NativeSelectProps & {
  region?: HttpTypes.StoreRegion
  regions?: HttpTypes.StoreRegion[]
  "data-testid"?: string
}

const CountrySelect = forwardRef<HTMLSelectElement, CountrySelectProps>(
  (
    {
      placeholder: customPlaceholder,
      region,
      regions,
      defaultValue,
      value,
      onChange,
      className,
      disabled,
      required,
      errors,
      touched,
      ...props
    },
    ref
  ) => {
    const innerRef = useRef<HTMLSelectElement>(null)
    const inputRef = useRef<HTMLInputElement>(null)
    const [internalValue, setInternalValue] = useState(
      String(defaultValue ?? "").toLowerCase()
    )
    const [query, setQuery] = useState("")
    const [invalid, setInvalid] = useState(false)
    const [locale, setLocale] = useState("en")
    const copy =
      countrySelectTranslations[locale.startsWith("zh") ? "zh" : "en"]
    const placeholder = customPlaceholder ?? copy.country

    useEffect(() => {
      let active = true
      getLocale()
        .then((currentLocale) => {
          if (active) setLocale(currentLocale?.toLowerCase() || "en")
        })
        .catch(() => {})
      return () => {
        active = false
      }
    }, [])
    const selectedValue = String(value ?? internalValue).toLowerCase()
    const options = useMemo(
      () => getCountryOptions(regions ?? (region ? [region] : []), locale),
      [region, regions, locale]
    )
    const selected = options.find((option) => option.value === selectedValue)
    const filtered = options.filter((option) =>
      option.search.includes(normalizeCountrySearch(query))
    )

    useImperativeHandle(ref, () => innerRef.current!, [])

    useEffect(() => {
      const form = innerRef.current?.form
      const reset = () => {
        setInternalValue(String(defaultValue ?? "").toLowerCase())
        setQuery("")
        setInvalid(false)
        // Native reset runs after this event, including for controlled selects.
        setTimeout(() => {
          const resetValue = String(value ?? defaultValue ?? "").toLowerCase()
          if (innerRef.current) {
            innerRef.current.value = resetValue
          }
          if (inputRef.current) {
            inputRef.current.value =
              options.find((option) => option.value === resetValue)?.label ?? ""
          }
        }, 0)
      }
      form?.addEventListener("reset", reset)
      return () => form?.removeEventListener("reset", reset)
    }, [defaultValue, value, options])

    const choose = (code: string | null) => {
      if (!code || !innerRef.current) return
      innerRef.current.value = code
      innerRef.current.dispatchEvent(new Event("change", { bubbles: true }))
      setQuery("")
      setInvalid(false)
    }

    return (
      <div className={clx("relative min-w-0", className)}>
        <select
          {...props}
          ref={innerRef}
          className="sr-only"
          tabIndex={-1}
          aria-hidden="true"
          disabled={disabled}
          required={required}
          value={selected?.value ?? ""}
          onChange={(event) => {
            setInternalValue(event.target.value)
            setInvalid(false)
            onChange?.(event)
          }}
          onInvalid={(event) => {
            event.preventDefault()
            setInvalid(true)
            inputRef.current?.focus()
            props.onInvalid?.(event)
          }}
        >
          <option value="">{placeholder}</option>
          {options.map((option) => (
            <option key={option.value} value={option.value}>
              {option.code} {option.label}
            </option>
          ))}
        </select>
        <Combobox
          value={selected?.value ?? ""}
          onChange={choose}
          onClose={() => setQuery("")}
          disabled={disabled}
          immediate
        >
          <div
            className={clx(
              "flex min-h-[44px] items-center gap-2 rounded-md border border-ui-border-base bg-ui-bg-subtle px-4 focus-within:ring-2 focus-within:ring-gray-900",
              disabled && "cursor-not-allowed opacity-50",
              invalid && "border-rose-500"
            )}
          >
            {selected && !query && (
              <span
                className="flex shrink-0 items-center gap-2"
                aria-hidden="true"
              >
                <span className="text-xs font-medium text-ui-fg-subtle">
                  {selected.code}
                </span>
                <ReactCountryFlag
                  svg
                  countryCode={selected.code}
                  style={{ width: "20px", height: "15px", objectFit: "cover" }}
                />
              </span>
            )}
            <ComboboxInput
              ref={inputRef}
              id={props.id ? `${props.id}-search` : undefined}
              aria-label={props["aria-label"] ?? placeholder}
              aria-required={required}
              aria-invalid={invalid || undefined}
              aria-describedby={props["aria-describedby"]}
              placeholder={placeholder}
              defaultValue={
                options.find(
                  (option) =>
                    option.value ===
                    String(value ?? defaultValue ?? "").toLowerCase()
                )?.label ?? ""
              }
              displayValue={(code: string) =>
                options.find((option) => option.value === code)?.label ?? ""
              }
              onChange={(event) => setQuery(event.target.value)}
              className="min-w-0 w-full border-none bg-transparent py-2.5 text-base outline-none placeholder:text-ui-fg-muted"
              autoComplete="off"
              data-testid={
                props["data-testid"]
                  ? `${props["data-testid"]}-search`
                  : undefined
              }
            />
            <ComboboxButton
              className="flex shrink-0 items-center text-ui-fg-subtle"
              aria-label={copy.showCountries}
            >
              <ChevronUpDown />
            </ComboboxButton>
          </div>
          <ComboboxOptions
            anchor={{ to: "bottom start", gap: 6, padding: 12 }}
            className="z-[100] max-h-64 w-[var(--input-width)] min-w-64 max-w-[calc(100vw-24px)] overflow-y-auto rounded-md border border-ui-border-base bg-white p-1 text-sm shadow-lg outline-none"
          >
            <div className="px-3 py-2 text-xs text-ui-fg-muted">
              {copy.searchHint}
            </div>
            {filtered.length === 0 && (
              <div role="status" className="px-3 py-4 text-ui-fg-subtle">
                {options.length ? copy.noResults : copy.noCountries}
              </div>
            )}
            {filtered.map((option) => (
              <ComboboxOption
                key={option.value}
                value={option.value}
                className="group flex cursor-pointer items-center gap-3 rounded px-3 py-2.5 data-[focus]:bg-gray-100 data-[selected]:font-medium"
              >
                <span className="w-6 shrink-0 text-xs text-ui-fg-subtle">
                  {option.code}
                </span>
                <ReactCountryFlag
                  svg
                  countryCode={option.code}
                  style={{
                    width: "20px",
                    height: "15px",
                    objectFit: "cover",
                    flexShrink: 0,
                  }}
                  aria-hidden="true"
                />
                <span className="flex-1">{option.label}</span>
                <Check className="invisible shrink-0 group-data-[selected]:visible" />
              </ComboboxOption>
            ))}
          </ComboboxOptions>
        </Combobox>
        {invalid && (
          <p role="alert" className="mt-1 text-xs text-rose-500">
            {copy.required}
          </p>
        )}
      </div>
    )
  }
)

CountrySelect.displayName = "CountrySelect"

export default CountrySelect
