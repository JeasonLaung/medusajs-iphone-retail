"use client"

import { createContext, useContext, useState } from "react"

import type {CartCommands} from './types'

const closed:CartCommands={enabled:false,updateLineItem:async()=>{throw Error('Cart closed')},deleteLineItem:async()=>{throw Error('Cart closed')}}
const CartInteractionsContext = createContext({ busy: false, setBusy: (_busy: boolean) => {}, commands:closed })

export function useCartInteractions() {
  return useContext(CartInteractionsContext)
}

export default function CartInteractions({ children, commands=closed }: { children: React.ReactNode; commands?:CartCommands }) {
  const [busy, setBusy] = useState(false)
  return <CartInteractionsContext.Provider value={{ busy, setBusy, commands }}>{children}</CartInteractionsContext.Provider>
}
