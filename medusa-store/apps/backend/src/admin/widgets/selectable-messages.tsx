import { useEffect } from "react"
import { defineWidgetConfig } from "@medusajs/admin-sdk"
import "../styles/messages.css"

const SelectableMessages = () => {
  useEffect(() => {
    const selectText = (event: PointerEvent) => {
      const target = event.target
      if (!(target instanceof Element) || !target.closest("[data-sonner-toast]")) return
      if (target.closest("button, a, input, textarea, select, [role='button']")) return
      // Sonner captures the pointer to swipe-dismiss. Let the browser select text instead.
      event.stopPropagation()
    }
    document.addEventListener("pointerdown", selectText, true)
    return () => document.removeEventListener("pointerdown", selectText, true)
  }, [])
  return null
}

export default SelectableMessages
export const config = defineWidgetConfig({ zone: "topbar", id: "messages:selectable" })
