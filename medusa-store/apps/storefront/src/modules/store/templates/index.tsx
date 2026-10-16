"use client"

import type { SearchClient } from "instantsearch.js"
import { useMemo } from "react"
import { Configure } from "react-instantsearch"
import { InstantSearchNext } from "react-instantsearch-nextjs"

import { PRODUCT_INDEX_NAME, searchClient } from "@lib/search-client"
import { createStoreSearchRouting } from "@lib/util/store-search-routing"
import type { StoreSearchData } from "@lib/util/store-search-data"
import { StoreSearchDataProvider } from "@modules/store/components/store-search-data"
import StoreHits from "@modules/store/components/store-hits"
import StoreRefinements from "@modules/store/components/store-refinements"
import StoreSearchBox from "@modules/store/components/store-search-box"
import useIsMobile from "@lib/hooks/use-is-mobile"

const PRODUCT_LIMIT = 50

type StoreTemplateProps = {
  currencyCode: string
  requestUrl: string
  searchData: StoreSearchData
}

const StoreTemplate = ({
  currencyCode,
  requestUrl,
  searchData,
}: StoreTemplateProps) => {
  const isMobile = useIsMobile()
  const searchRouting = useMemo(() => {
    const routing = createStoreSearchRouting(PRODUCT_INDEX_NAME)
    return {
      ...routing,
      router: {
        ...routing.router,
        // Preserve literal array separators and escaped commas in values.
        getLocation: () =>
          typeof window === "undefined"
            ? (new URL(requestUrl) as unknown as Location)
            : window.location,
      },
    }
  }, [requestUrl])
  return (
    <StoreSearchDataProvider value={searchData}>
      <div className="py-6 content-container" data-testid="category-container">
        <div className="mb-8 text-2xl-semi">
          <h1 data-testid="store-page-title">Refurbished iPhones</h1>
          <p className="mt-2 text-small-regular text-ui-fg-subtle">
            Demo store: inventory is test data for checkout flow testing.
          </p>
        </div>

        <div className="flex flex-col small:flex-row small:items-start small:gap-x-8">
          <InstantSearchNext
            indexName={PRODUCT_INDEX_NAME}
            searchClient={searchClient as unknown as SearchClient}
            routing={searchRouting}
            future={{ preserveSharedStateOnUnmount: true }}
          >
            <Configure
              hitsPerPage={PRODUCT_LIMIT}
              facetFilters={["category:iPhone"]}
            />
            <StoreRefinements currencyCode={currencyCode} />
            <div className="w-full min-w-0">
              {!isMobile && <div className="hidden small:block"><StoreSearchBox /></div>}
              <StoreHits
                hitsPerPage={PRODUCT_LIMIT}
                currencyCode={currencyCode}
              />
            </div>
          </InstantSearchNext>
        </div>
      </div>
    </StoreSearchDataProvider>
  )
}

export default StoreTemplate
