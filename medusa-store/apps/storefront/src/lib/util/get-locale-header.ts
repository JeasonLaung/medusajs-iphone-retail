import { getLocale } from "@lib/data/locale-actions"
import { localeFromSegment } from "@lib/util/storefront-route"

export async function getLocaleHeader() {
  // Browser API calls must not invoke a Server Action just to read a locale.
  // That action renders the current route again and can restart search requests.
  let locale: string | null
  if (typeof window !== "undefined") {
    const segment = window.location.pathname.split("/")[1] || ""
    const cookie = document.cookie.split("; ").find((entry) => entry.startsWith("_medusa_locale="))
    locale = /^[a-z]{2}-(en|zh)$/i.test(segment)
      ? localeFromSegment(segment)
      : cookie ? decodeURIComponent(cookie.slice("_medusa_locale=".length)) : null
  } else {
    locale = await getLocale()
  }
  return {
    "x-medusa-locale": locale,
  } as const
}
