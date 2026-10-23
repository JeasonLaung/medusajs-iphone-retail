import { HttpTypes } from "@medusajs/types"
import { NextRequest, NextResponse } from "next/server"
import { localeFromSegment, routeCountry, routeLanguage, storefrontSegment } from "@lib/util/storefront-route"

const BACKEND_URL = process.env.NEXT_PUBLIC_MEDUSA_BACKEND_URL
const PUBLISHABLE_API_KEY = process.env.NEXT_PUBLIC_MEDUSA_PUBLISHABLE_KEY
const DEFAULT_REGION = process.env.NEXT_PUBLIC_DEFAULT_REGION || "dk"

const regionMapCache = {
  regionMap: new Map<string, HttpTypes.StoreRegion>(),
  regionMapUpdated: Date.now(),
}

async function getRegionMap(cacheId: string) {
  const { regionMap, regionMapUpdated } = regionMapCache

  if (!BACKEND_URL) {
    throw new Error(
      "Middleware.ts: Error fetching regions. Did you set up regions in your Medusa Admin and define a NEXT_PUBLIC_MEDUSA_BACKEND_URL environment variable."
    )
  }

  if (
    !regionMap.keys().next().value ||
    regionMapUpdated < Date.now() - 3600 * 1000
  ) {
    // Fetch regions from Medusa. We can't use the JS client here because middleware is running on Edge and the client needs a Node environment.
    const response = await fetch(`${BACKEND_URL}/store/regions`, {
      method: "GET",
      headers: {
        "x-publishable-api-key": PUBLISHABLE_API_KEY!,
      },
      next: {
        revalidate: 3600,
        tags: [`regions-${cacheId}`],
      },
      cache: "force-cache",
    })

    if (!response.ok) {
      throw new Error(`Backend returned ${response.status}`)
    }

    const json = await response.json()

    const { regions } = json

    if (!regions?.length) {
      return new Map<string, HttpTypes.StoreRegion>()
    }

    // Create a map of country codes to regions.
    regions.forEach((region: HttpTypes.StoreRegion) => {
      region.countries?.forEach((c) => {
        regionMapCache.regionMap.set(c.iso_2 ?? "", region)
      })
    })

    regionMapCache.regionMapUpdated = Date.now()
  }

  return regionMapCache.regionMap
}

/**
 * Fetches regions from Medusa and sets the region cookie.
 * @param request
 * @param response
 */
async function getCountryCode(
  request: NextRequest,
  regionMap: Map<string, HttpTypes.StoreRegion | number>
) {
  let countryCode

  const urlCountryCode = routeCountry(request.nextUrl.pathname.split("/")[1] || "")

  // Cloudflare Workers provides country via request.cf.country
  const cloudflareCountryCode = (request as { cf?: { country?: string } }).cf?.country?.toLowerCase()

  // Vercel provides x-vercel-ip-country header
  const vercelCountryCode = request.headers
    .get("x-vercel-ip-country")
    ?.toLowerCase()

  if (urlCountryCode && regionMap.has(urlCountryCode)) {
    countryCode = urlCountryCode
  } else if (cloudflareCountryCode && regionMap.has(cloudflareCountryCode)) {
    countryCode = cloudflareCountryCode
  } else if (vercelCountryCode && regionMap.has(vercelCountryCode)) {
    countryCode = vercelCountryCode
  } else if (regionMap.has(DEFAULT_REGION)) {
    countryCode = DEFAULT_REGION
  } else if (regionMap.keys().next().value) {
    countryCode = regionMap.keys().next().value
  }

  return countryCode
}

/**
 * Middleware to handle region selection and onboarding status.
 */
export async function middleware(request: NextRequest) {
  const verification = request.nextUrl.pathname.match(/^\/([A-Za-z0-9-]{8,128})\.txt$/)
  if (verification) {
    const target = request.nextUrl.clone()
    target.pathname = `/api/seo-artifacts/${verification[1]}`
    return NextResponse.rewrite(target)
  }
  if (request.nextUrl.pathname.includes(".")) {
    return NextResponse.next()
  }

  const cacheIdCookie = request.cookies.get("_medusa_cache_id")
  const cacheId = cacheIdCookie?.value || crypto.randomUUID()

  const regionMap = await getRegionMap(cacheId)
  const countryCode = await getCountryCode(request, regionMap)

  // if the country code is available, use it, otherwise use the default region
  const country = countryCode || DEFAULT_REGION
  const firstPathSegment = request.nextUrl.pathname.split("/")[1]?.toLowerCase()
  const hasCountry = regionMap.has(routeCountry(firstPathSegment || ""))
  const requestedLanguage = hasCountry && firstPathSegment.includes("-")
    ? firstPathSegment.slice(3)
    : request.cookies.get("_medusa_locale")?.value
  const segment = storefrontSegment(country, requestedLanguage)

  if (firstPathSegment !== segment) {
    const redirectUrl = request.nextUrl.clone()
    const rest = hasCountry
      ? request.nextUrl.pathname.slice(firstPathSegment.length + 1)
      : request.nextUrl.pathname === "/" ? "" : request.nextUrl.pathname
    redirectUrl.pathname = `/${segment}${rest}`
    return NextResponse.redirect(redirectUrl, 307)
  }

  // The URL is authoritative even when the browser's locale cookie differs.
  const headers = new Headers(request.headers)
  headers.set("x-storefront-locale", localeFromSegment(segment))
  headers.set("x-storefront-country", country)
  headers.set("x-storefront-url", request.url)
  const response = NextResponse.next({ request: { headers } })
  if (!cacheIdCookie) response.cookies.set("_medusa_cache_id", cacheId, { maxAge: 60 * 60 * 24 })
  // A locale-changing Server Action owns its response cookie. The old URL
  // must not overwrite the new preference while that action is redirecting.
  if (!request.headers.has("next-action") && routeLanguage(request.cookies.get("_medusa_locale")?.value) !== routeLanguage(requestedLanguage)) {
    response.cookies.set("_medusa_locale", localeFromSegment(segment), {
      maxAge: 60 * 60 * 24 * 365, sameSite: "strict", secure: process.env.NODE_ENV === "production",
    })
  }
  return response
}

export const config = {
  matcher: [
    "/((?!api|_next/static|_next/image|favicon.ico|images|assets|png|svg|jpg|jpeg|gif|webp).*)",
  ],
}
