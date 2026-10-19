"use client"

import { useParams } from "next/navigation"
import { localeFromSegment } from "@lib/util/storefront-route"
import { getAccountCopy } from "./i18n"

export function useAccountCopy() {
  const { countryCode } = useParams<{ countryCode: string }>()
  return getAccountCopy(localeFromSegment(countryCode || ""))
}
