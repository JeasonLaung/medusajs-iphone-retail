"use client"

import { ArrowLeft, ArrowRight } from "@medusajs/icons"
import { ReactNode, useEffect, useId, useRef, useState } from "react"

export default function RelatedProductsRail({ children, label, previous, next }: {
  children: ReactNode
  label: string
  previous: string
  next: string
}) {
  const track = useRef<HTMLUListElement>(null)
  const id = useId()
  const [edges, setEdges] = useState({ left: false, right: false })
  const update = () => {
    const element = track.current
    if (element) setEdges({
      left: element.scrollLeft > 2,
      right: element.scrollLeft + element.clientWidth < element.scrollWidth - 2,
    })
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
    if (element) element.scrollBy({
      left: direction * element.clientWidth,
      behavior: window.matchMedia("(prefers-reduced-motion: reduce)").matches ? "instant" : "smooth",
    })
  }

  const buttonClass = "flex h-11 w-11 shrink-0 items-center justify-center rounded-md disabled:opacity-30 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2"

  return (
    <div className="flex items-center gap-2 small:gap-4">
      <button type="button" className={buttonClass} aria-label={previous} aria-controls={id} disabled={!edges.left} onClick={() => move(-1)}>
        <ArrowLeft width={20} height={20} aria-hidden="true" />
      </button>
      <ul id={id} ref={track} aria-label={label} tabIndex={0} onScroll={update}
        className="flex min-w-0 flex-1 snap-x snap-mandatory gap-4 overflow-x-auto p-1 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden [&>li]:w-36 [&>li]:shrink-0 [&>li]:snap-start small:[&>li]:w-44"
        onKeyDown={(event) => {
          if (event.key === "ArrowLeft" || event.key === "ArrowRight") {
            event.preventDefault()
            move(event.key === "ArrowRight" ? 1 : -1)
          }
        }}>
        {children}
      </ul>
      <button type="button" className={buttonClass} aria-label={next} aria-controls={id} disabled={!edges.right} onClick={() => move(1)}>
        <ArrowRight width={20} height={20} aria-hidden="true" />
      </button>
    </div>
  )
}
