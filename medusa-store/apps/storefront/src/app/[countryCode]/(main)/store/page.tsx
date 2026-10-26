import { withSEO } from "@lib/data/seo"
import { Metadata } from "next"
import { notFound } from "next/navigation"
import { headers } from "next/headers"

import { getRegion } from "@lib/data/regions"
import { getStoreSearchData } from "@lib/data/store-search"
import { getBuyPolicies } from "@lib/data/buy-policies"
import StoreTemplate from "@modules/store/templates"

const defaultPageMetadata: Metadata = {
  title: "Refurbished iPhones",
  description: "Browse the imported iPhone catalog.",
}

export const dynamic = "force-dynamic"

export default async function StorePage(props: {
  params: Promise<{ countryCode: string }>
}) {
  const { countryCode } = await props.params
  const region = await getRegion(countryCode)

  if (!region) {
    notFound()
  }

  const requestUrl = (await headers()).get("x-storefront-url")!
  const [searchData, policies] = await Promise.all([
    getStoreSearchData(requestUrl, region.currency_code),
    getBuyPolicies(),
  ])

  return (
    <StoreTemplate
      currencyCode={region.currency_code}
      requestUrl={requestUrl}
      searchData={searchData}
      policies={policies}
    />
  )
}

export async function generateMetadata(): Promise<Metadata> { return withSEO(defaultPageMetadata) }
