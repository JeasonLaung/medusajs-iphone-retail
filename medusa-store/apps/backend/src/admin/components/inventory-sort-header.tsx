import { useTranslation } from "react-i18next"
import { useSearchParams } from "react-router-dom"

export const InventorySortHeader = ({ field }: {
  field: "reserved_quantity" | "stocked_quantity"
}) => {
  const { t } = useTranslation()
  const [params, setParams] = useSearchParams()
  const order = params.get("order")
  const ascending = order === field
  const descending = order === `-${field}`
  const label = t(field === "reserved_quantity" ? "inventory.reserved" : "fields.inStock")

  return (
    <button
      type="button"
      className="inline-flex items-center gap-1 rounded-sm"
      aria-label={`${label}: ${t(ascending ? "general.descending" : "general.ascending")}`}
      onClick={() => setParams((previous) => {
        const next = new URLSearchParams(previous)
        next.set("order", ascending ? `-${field}` : field)
        next.delete("offset")
        return next
      })}
    >
      {label}
      <svg width="15" height="15" viewBox="0 0 15 15" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden="true">
        {!descending && <path d="M4 12V3m-3 3 3-3 3 3" />}
        {!ascending && <path d="M11 3v9m-3-3 3 3 3-3" />}
      </svg>
    </button>
  )
}
