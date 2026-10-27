export function routeLanguage(locale: string | null | undefined) {
  return locale?.toLowerCase().startsWith("zh") ? "zh" : "en"
}

export function routeCountry(segment: string) {
  return segment.toLowerCase().split("-")[0]
}

export function storefrontSegment(country: string, locale: string | null | undefined) {
  return `${routeCountry(country)}-${routeLanguage(locale)}`
}

export function localeFromSegment(segment: string) {
  const language = segment.toLowerCase().slice(3)
  return language === "zh" ? "zh-CN" : "en"
}
