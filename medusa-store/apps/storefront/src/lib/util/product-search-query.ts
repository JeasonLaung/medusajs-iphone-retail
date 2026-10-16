import type { SearchQuery } from "@medusajs/instantsearch-adapter"

const suffixes: Record<string, string> = {
  p: " Pro",
  pro: " Pro",
  pm: " Pro Max",
  promax: " Pro Max",
  plus: " Plus",
  mini: " mini",
  e: "e",
}

export function normalizeProductSearchRequest(query: SearchQuery): SearchQuery {
  const original = query.filters?.q
  if (!original) return query
  const expanded = normalizeProductSearchQuery(original)
  if (expanded === original) return query
  return {
    ...query,
    filters: { ...query.filters, q: expanded },
    search_options: { ...query.search_options, match_strategy: "all" },
  }
}

export function normalizeProductSearchQuery(query: string): string {
  return query.replace(
    /\bi(?:phone)?\s*(\d{1,2})(?:\s*(pro\s*max|pm|pro|plus|mini|p|e))?\b/gi,
    (_, model: string, suffix?: string) =>
      `iPhone ${model}${suffixes[suffix?.replace(/\s/g, "").toLowerCase() ?? ""] ?? ""}`
  )
}
