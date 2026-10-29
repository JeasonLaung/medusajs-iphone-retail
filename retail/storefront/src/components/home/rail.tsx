"use client"

import { ArrowLeft, ArrowRight } from "../products/icons"
import { type ReactNode, useEffect, useId, useRef, useState } from "react"
import type { HomeCopy } from "../journal/i18n"
import styles from "./gallery.module.css"

export default function SeriesRail({ children, copy }: { children: ReactNode; copy: HomeCopy }) {
  const track = useRef<HTMLUListElement>(null)
  const id = useId()
  const [edges, setEdges] = useState({ left: false, right: false })
  const update = () => {
    const element = track.current
    // Scroll snapping can leave a few pixels of inset at either boundary.
    if (element) setEdges({ left: element.scrollLeft > 8, right: element.scrollLeft + element.clientWidth < element.scrollWidth - 8 })
  }
  useEffect(() => {
    const element = track.current
    if (!element) return
    update()
    const observer = new ResizeObserver(update)
    observer.observe(element)
    return () => observer.disconnect()
  }, [children])
  const move = (direction: number) => {
    const element = track.current
    if (element) element.scrollBy({ left: direction * Math.max(140, element.clientWidth * .75), behavior: window.matchMedia("(prefers-reduced-motion: reduce)").matches ? "instant" : "smooth" })
  }
  return <div className={styles.rail}>
    <ul id={id} ref={track} className={styles.grid} onScroll={update} aria-label={copy.title} data-testid="series-gallery" onKeyDown={(event) => {
      if (event.key === "ArrowLeft" || event.key === "ArrowRight") { event.preventDefault(); move(event.key === "ArrowRight" ? 1 : -1) }
    }}>{children}</ul>
    <button type="button" className={`${styles.railArrow} ${styles.railPrevious}`} aria-label={copy.previousSeries} aria-controls={id} aria-hidden={!edges.left} disabled={!edges.left} onClick={() => move(-1)}><ArrowLeft width={18} height={18} aria-hidden="true" /></button>
    <button type="button" className={`${styles.railArrow} ${styles.railNext}`} aria-label={copy.nextSeries} aria-controls={id} aria-hidden={!edges.right} disabled={!edges.right} onClick={() => move(1)}><ArrowRight width={18} height={18} aria-hidden="true" /></button>
  </div>
}
