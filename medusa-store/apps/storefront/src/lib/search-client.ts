import {
  createInstantSearchAdapter,
  parseSearchParams,
  type MedusaSdkLike,
} from "@medusajs/instantsearch-adapter"

import { sdk } from "@lib/config"
import { normalizeProductSearchRequest } from "@lib/util/product-search-query"

export const PRODUCT_INDEX_NAME = "product"

/**
 * The currencies the product index holds prices in, one field set each. Keep
 * in sync with `PRICE_CURRENCIES` in the backend's `src/search/product.ts`.
 */
export const SEARCH_PRICE_CURRENCIES = ["eur", "usd"]

export type PriceField =
  | "min_price"
  | "max_price"
  | "original_price"
  | "on_sale"

/**
 * The indexed currency to read prices in for a region. A region whose currency
 * the index doesn't hold falls back to the first one, so the listing still
 * shows a price rather than none.
 */
export const indexedCurrency = (currencyCode: string) => {
  const code = currencyCode.toLowerCase()
  return SEARCH_PRICE_CURRENCIES.includes(code)
    ? code
    : SEARCH_PRICE_CURRENCIES[0]
}

export const priceAttribute = (field: PriceField, currencyCode: string) =>
  `${field}_${indexedCurrency(currencyCode)}`

/**
 * Shared by the navbar search drawer and the store listing. An empty query
 * searches rather than short-circuiting, which is what the listing needs to
 * show every product before anything is refined.
 */
export const { searchClient } = createInstantSearchAdapter({
  sdk: sdk as unknown as MedusaSdkLike,
  path: "/store/search",
  transformQuery: normalizeProductSearchRequest,
  // Keep the user's original wording in InstantSearch state and shared URLs.
  transformResponse: (response, _result, request) => ({
    ...response,
    query: parseSearchParams(request.params).query ?? "",
  }),
  additionalSearchParameters: {
    search_options: {
      match_strategy: "last",
    },
  },
  // Range widgets read `facets_stats`, which the adapter only produces for
  // fields listed here. The price fields are declared with a `stats` facet on
  // the index, which is what makes the stats available at all.
  numericAttributes: SEARCH_PRICE_CURRENCIES.flatMap((currency) => [
    `min_price_${currency}`,
    `max_price_${currency}`,
  ]),
})
