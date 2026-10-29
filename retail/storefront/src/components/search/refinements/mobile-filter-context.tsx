"use client"

import { createContext, useContext } from "react"

export const MobileFilterContext = createContext<{
  panel: string | null
  onSelect: () => void
} | null>(null)

export const useMobileFilter = () => useContext(MobileFilterContext)
