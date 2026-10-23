import type { ReactNode } from "react"
import { FlowProgressProvider } from "@modules/sell/components/flow-progress"

export default function Layout({ children }: { children: ReactNode }) {
  return <FlowProgressProvider>{children}</FlowProgressProvider>
}
