import {
  adaptSearchRequest,
  type SearchQuery,
  type SearchResult,
} from "@medusajs/instantsearch-adapter"
import qs from "qs"

import { sdk } from "@lib/config"
import { PRODUCT_INDEX_NAME, priceAttribute } from "@lib/search-client"
import { createStoreSearchRouting } from "@lib/util/store-search-routing"
import { normalizeProductSearchRequest } from "@lib/util/product-search-query"
import {
  priceDistributionKey,
  type StoreFilterOptions,
  type StoreSearchData,
} from "@lib/util/store-search-data"

// Catalog options and the initial histogram are part of the first HTML response.
export async function getStoreSearchData(
  url: string,
  currencyCode: string
): Promise<StoreSearchData> {
  const route = qs.parse(new URL(url).search.slice(1), { comma: true })
  const state =
    createStoreSearchRouting(PRODUCT_INDEX_NAME).stateMapping.routeToState(
      route
    )[PRODUCT_INDEX_NAME]
  const attribute = priceAttribute("min_price", currencyCode)
  const fields = [
    "category",
    "model",
    "storage_options",
    "battery_options",
    "display_size",
    "color_families",
    "rear_cameras",
    "condition_options",
    "release_year",
  ]
  const catalogQuery: SearchQuery = {
    entity: PRODUCT_INDEX_NAME,
    filters: { category: "iPhone" },
    pagination: { take: 0 },
    search_options: {
      facets: fields.map((field) => ({ field, type: "value", limit: 100 })),
    },
  }
  const distributionQuery = adaptSearchRequest(
    {
      indexName: PRODUCT_INDEX_NAME,
      params: {
        query: state.query,
        hitsPerPage: 100,
        attributesToRetrieve: [attribute],
        facetFilters: [
          "category:iPhone",
          ...Object.entries(state.refinementList ?? {}).map(([field, values]) =>
            values.map((value) => `${field}:${value}`)
          ),
        ],
      },
    },
    {
      additionalSearchParameters: {
        search_options: { match_strategy: "last" },
      },
    }
  )
  const search = (queries: SearchQuery[]) =>
    sdk.client.fetch<{ results: SearchResult[] }>("/store/search", {
      method: "POST",
      body: { queries: queries.map(normalizeProductSearchRequest) },
      cache: "no-store",
    })
  const [response, catalog] = await Promise.all([
    search([catalogQuery, distributionQuery]),
    sdk.store.category.list({
      limit: 100,
      fields: "id,name,handle,parent_category_id",
    }),
  ])
  const options: StoreFilterOptions = {}
  for (const [field, facet] of Object.entries(
    response.results[0].facets ?? {}
  )) {
    if (facet.type === "value") options[field] = { values: facet.values }
  }
  const parent = catalog.product_categories.find(
    (category) => category.handle === "iphone"
  )
  if (parent) {
    options.category = {
      values: catalog.product_categories
        .filter((category) => category.parent_category_id === parent.id)
        .map((category) => ({ value: category.name, count: 0 })),
    }
  }
  const prices: number[] = []
  let result = response.results[1]
  let skip = 0
  while (true) {
    for (const hit of result.hits) {
      const price = hit.document[attribute]
      if (typeof price === "number" && Number.isFinite(price))
        prices.push(price)
    }
    skip += result.hits.length
    if (
      result.hits.length < 100 ||
      (result.metadata.count !== null && skip >= result.metadata.count)
    )
      break
    const next = await search([
      { ...distributionQuery, pagination: { take: 100, skip } },
    ])
    result = next.results[0]
  }
  return {
    options,
    distribution: {
      key: priceDistributionKey(attribute, state.query, state.refinementList),
      prices,
    },
  }
}
