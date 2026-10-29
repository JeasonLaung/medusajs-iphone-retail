"use client"

import type { SearchClient } from "instantsearch.js"
import { ArrowUturnLeft, ShieldCheck, Star } from "../icons"
import { theme } from "antd"
import { useSearchRoute as useParams } from "../context"
import { useMemo } from "react"
import { Configure } from "react-instantsearch"
import { InstantSearch } from "react-instantsearch"
import { history } from "instantsearch.js/es/lib/routers"

import { PRODUCT_INDEX_NAME } from "../client"
import { createStoreSearchRouting } from "../routing"
import type { StoreSearchData } from "../search-data"
import { StoreSearchDataProvider } from "../data"
import StoreHits from "../hits"
import StoreRefinements from "../refinements"
import StoreSearchBox from "../box"
import useIsMobile from "../use-is-mobile"
import { getStoreCopy } from "../i18n"
export type BuyPolicies = {warranty_months:number;return_days:number;warranty_note:{en:string;zh:string};return_note:{en:string;zh:string}}
import styles from "./styles.module.css"

const PRODUCT_LIMIT = 50

export type StoreTemplateProps = {
  searchClient:SearchClient
  currencyCode: string
  requestUrl: string
  searchData: StoreSearchData
  policies: BuyPolicies | null
}

const StoreTemplate = ({
  currencyCode,
  searchClient,
  requestUrl,
  searchData,
  policies,
}: StoreTemplateProps) => {
  const { countryCode } = useParams()
  const copy = getStoreCopy(countryCode)
  const { token } = theme.useToken()
  const locale = countryCode?.endsWith("-zh") ? "zh" : "en"
  const note = (value: BuyPolicies["warranty_note"] | undefined) => value?.[locale] || value?.en || ""
  const number = new Intl.NumberFormat(locale)
  const period = (count: number | undefined, unit: "months" | "days") => {
    if (count === undefined) return copy.policyUnavailable
    if (count === 0) return unit === "months" ? copy.noWarranty : copy.noReturns
    if (count === 1) return unit === "months" ? copy.oneMonth : copy.oneDay
    return copy[unit].replace("{count}", number.format(count))
  }
  const highlights = [
    { Icon: Star, title: "4.4", description: copy.rating, note: "" },
    { Icon: ShieldCheck, title: period(policies?.warranty_months, "months"), description: copy.warranty, note: note(policies?.warranty_note) },
    { Icon: ArrowUturnLeft, title: period(policies?.return_days, "days"), description: copy.returns, note: note(policies?.return_note) },
  ]
  const isMobile = useIsMobile()
  const searchRouting = useMemo(() => {
    const routing = createStoreSearchRouting(PRODUCT_INDEX_NAME)
    return {
      ...routing,
      router: history<Record<string,unknown>>({
        ...routing.router,
        // Preserve literal array separators and escaped commas in values.
        getLocation: () =>
          typeof window === "undefined"
            ? (new URL(requestUrl) as unknown as Location)
            : window.location,
        cleanUrlOnDispose:false,
        // Commit URL updates promptly after popstate; delayed history writes
        // otherwise swallow a fast filter/sort action immediately after Back.
        writeDelay:0,
      }),
    }
  }, [requestUrl])
  return (
    <StoreSearchDataProvider value={searchData}>
      <div className="py-6 content-container" data-testid="category-container">
        <div className="mb-8 text-center text-2xl-semi">
          <h1 data-testid="store-page-title">{copy.title}</h1>
        </div>

        <ul
          className={styles.highlights}
          aria-label={copy.highlightsLabel}
          style={{ background: token.colorBgLayout, color: token.colorText }}
        >
          {highlights.map(({ Icon, title, description, note }) => (
            <li key={description} className={styles.highlight}>
              <span aria-hidden="true" style={{ color: token.colorPrimary }}><Icon /></span>
              <h2>{title}</h2>
              <p style={{ color: token.colorTextSecondary }}>{description}</p>
              {note && <p className={styles.note} style={{ color: token.colorTextSecondary }}>{note}</p>}
            </li>
          ))}
        </ul>

        <div className="flex flex-col small:flex-row small:items-start small:gap-x-8">
          <InstantSearch
            indexName={PRODUCT_INDEX_NAME}
            searchClient={searchClient as unknown as SearchClient}
            routing={searchRouting}
            future={{ preserveSharedStateOnUnmount: true }}
            insights={false}
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
          </InstantSearch>
        </div>
      </div>
    </StoreSearchDataProvider>
  )
}

export default StoreTemplate
