"use client"

import { useStoreSearchData } from "@modules/store/components/store-search-data"

export default function useFilterOptions(attribute: string) {
  return useStoreSearchData().options[attribute]?.values ?? []
}
