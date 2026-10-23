"use client"

import { createContext, useContext, useEffect, useState } from "react"
import type { Dispatch, ReactNode, SetStateAction } from "react"
import { Progress } from "antd"
import styles from "../flow.module.css"

const ProgressContext = createContext<{
  percent: number | null
  setPercent: Dispatch<SetStateAction<number | null>>
} | null>(null)

// The shared layout survives step navigation, unlike the questionnaire page.
export function FlowProgressProvider({ children }: { children: ReactNode }) {
  const [percent, setPercent] = useState<number | null>(null)
  return <ProgressContext.Provider value={{ percent, setPercent }}>{children}</ProgressContext.Provider>
}

export default function FlowProgress({ percent, label }: { percent: number; label: string }) {
  const progress = useContext(ProgressContext)
  const setPercent = progress?.setPercent

  useEffect(() => {
    // Paint the previous width before updating a newly mounted progress bar.
    let nextFrame = 0
    const frame = requestAnimationFrame(() => {
      nextFrame = requestAnimationFrame(() => setPercent?.(percent))
    })
    return () => { cancelAnimationFrame(frame); cancelAnimationFrame(nextFrame) }
  }, [percent, setPercent])

  return <Progress className={styles.progress} percent={progress?.percent ?? percent} showInfo={false} aria-label={label} aria-valuenow={percent} />
}
