"use client"

import { ReactNode, RefObject, useEffect, useState } from "react"
import styles from "../product-detail/styles.module.css"

export default function StickyPurchaseBar({ anchor, title, summary, price, label, buttonText, disabled, onAdd, children }: {
  anchor: RefObject<HTMLDivElement | null>
  title: string
  summary?: string
  price: string
  label: string
  buttonText: string
  disabled: boolean
  onAdd: () => void
  children?: ReactNode
}) {
  const [top, setTop] = useState<number | null>(null)

  useEffect(() => {
    const element = anchor.current
    const header = document.querySelector("header")
    if (!element) return
    let observer: IntersectionObserver | undefined
    const observe = () => {
      observer?.disconnect()
      const offset = header?.getBoundingClientRect().height ?? 0
      const update = () => setTop(element.getBoundingClientRect().bottom <= offset ? offset : null)
      update()
      observer = new IntersectionObserver(update, { rootMargin: `-${offset}px 0px 0px 0px` })
      observer.observe(element)
    }
    observe()
    const resize = new ResizeObserver(observe)
    if (header) resize.observe(header)
    return () => {
      observer?.disconnect()
      resize.disconnect()
    }
  }, [anchor])

  if (top === null) return null

  return <aside className={styles.desktopBar} style={{ top }} aria-label={label} data-testid="sticky-purchase-bar">
    <div className={styles.desktopBarInner}>
      <div className={styles.desktopBarSelection}><strong>{title}</strong><p className={styles.muted}>{summary}</p></div>
      <strong className={styles.purchasePrice}>{price}</strong>
      <button type="button" className={styles.add} disabled={disabled} onClick={onAdd}>{buttonText}</button>
      {children && <div className={styles.desktopBarFeedback}>{children}</div>}
    </div>
  </aside>
}
