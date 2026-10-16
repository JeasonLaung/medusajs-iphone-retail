"use client"

import { createContext, useContext, useState } from "react"

const CartInteractionsContext = createContext({ busy: false, setBusy: (_busy: boolean) => {} })

export function useCartInteractions() {
  return useContext(CartInteractionsContext)
}

export default function CartInteractions({ children }: { children: React.ReactNode }) {
  const [busy, setBusy] = useState(false)
  return <CartInteractionsContext.Provider value={{ busy, setBusy }}>{children}</CartInteractionsContext.Provider>
}
