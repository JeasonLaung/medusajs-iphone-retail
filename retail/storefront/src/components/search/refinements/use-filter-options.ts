"use client"

import { useStoreSearchData } from "../data"

export default function useFilterOptions(attribute: string) {
  return useStoreSearchData().options[attribute]?.values ?? []
}
