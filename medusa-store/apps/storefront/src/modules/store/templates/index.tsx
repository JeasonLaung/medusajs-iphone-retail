"use client"

import type { SearchClient } from "instantsearch.js"
import { Configure, InstantSearch } from "react-instantsearch"

import { PRODUCT_INDEX_NAME, searchClient } from "@lib/search-client"
import StoreHits from "@modules/store/components/store-hits"
import StoreRefinements from "@modules/store/components/store-refinements"
import StoreSearchBox from "@modules/store/components/store-search-box"

const PRODUCT_LIMIT = 12

const StoreTemplate = ({ currencyCode }: { currencyCode: string }) => {
  return (
    <div className="py-6 content-container" data-testid="category-container">
      <div className="mb-8 text-2xl-semi">
        <h1 data-testid="store-page-title">Refurbished iPhones</h1>
        <p className="mt-2 text-small-regular text-ui-fg-subtle">
          Demo store: inventory is test data for checkout flow testing.
        </p>
      </div>

      <div className="flex flex-col small:flex-row small:items-start">
        <InstantSearch
          indexName={PRODUCT_INDEX_NAME}
          searchClient={searchClient as unknown as SearchClient}
          routing
          future={{ preserveSharedStateOnUnmount: true }}
        >
          <Configure
            hitsPerPage={PRODUCT_LIMIT}
            facetFilters={["category:Swappie iPhones"]}
          />
          <StoreRefinements currencyCode={currencyCode} />
          <div className="w-full min-w-0">
            <StoreSearchBox />
            <StoreHits
              hitsPerPage={PRODUCT_LIMIT}
              currencyCode={currencyCode}
            />
          </div>
        </InstantSearch>
      </div>
    </div>
  )
}

export default StoreTemplate
