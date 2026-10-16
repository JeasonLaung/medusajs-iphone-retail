"use client"

import { createContext, useContext } from "react"
import type { StoreSearchData } from "@lib/util/store-search-data"

const SearchDataContext = createContext<StoreSearchData | null>(null)

export const StoreSearchDataProvider = SearchDataContext.Provider

export function useStoreSearchData() {
  const data = useContext(SearchDataContext)
  if (!data) throw new Error("Store search data provider is missing")
  return data
}
