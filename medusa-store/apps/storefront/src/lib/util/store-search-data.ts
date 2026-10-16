export type StoreFilterOptions = Record<
  string,
  {
    values?: { value: string; count: number }[]
  }
>

export type StoreSearchData = {
  options: StoreFilterOptions
  distribution: { key: string; prices: number[] }
}

export function priceDistributionKey(
  attribute: string,
  query?: string,
  refinements: Record<string, string[]> = {}
) {
  return JSON.stringify({
    attribute,
    query: query || "",
    refinements: Object.entries(refinements)
      .filter(([, values]) => values.length)
      .sort(([a], [b]) => a.localeCompare(b))
      .map(([name, values]) => [name, [...values].sort()]),
  })
}
