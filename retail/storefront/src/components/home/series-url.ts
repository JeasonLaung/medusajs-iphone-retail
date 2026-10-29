export function storeSeriesUrl(series?: string) {
  if (!series) return "/store"

  const query = new URLSearchParams({
    series,
  })
  return `/store?${query}`
}
