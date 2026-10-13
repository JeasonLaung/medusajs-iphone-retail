import { Metadata } from "next"
import { notFound } from "next/navigation"

import { getRegion } from "@lib/data/regions"
import StoreTemplate from "@modules/store/templates"

export const metadata: Metadata = {
  title: "Refurbished iPhones",
  description: "Browse the imported iPhone catalog.",
}

export default async function StorePage(props: {
  params: Promise<{ countryCode: string }>
}) {
  const { countryCode } = await props.params
  const region = await getRegion(countryCode)

  if (!region) {
    notFound()
  }

  return <StoreTemplate currencyCode={region.currency_code} />
}
