"use client"

import { useLayoutEffect } from "react"

/** Reset only when the page content mounts, after its loading UI is replaced. */
export default function PageEntryScroll() {
  useLayoutEffect(() => {
    // Keep explicit links to product sections and reviews working.
    if (!window.location.hash) {
      window.scrollTo({ top: 0, left: 0, behavior: "instant" })
    }
  }, [])

  return null
}
