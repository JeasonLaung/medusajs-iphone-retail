"use client"

import { ArrowLeft, ArrowRight } from "../products/icons"
import Image from "../products/product-image"
import { useRef, useState } from "react"
import LocalizedClientLink from "./localized-link"
import type { HomeCopy } from "../journal/i18n"
import styles from "./carousel.module.css"

export type HomeSlide = {
  id: string
  title: string
  image: string
  href: string
}

export default function HomeCarousel({ slides, copy }: { slides: HomeSlide[]; copy: HomeCopy }) {
  const track = useRef<HTMLDivElement>(null)
  const [active, setActive] = useState(0)

  const goTo = (index: number) => {
    const element = track.current
    if (!element) return
    const next = (index + slides.length) % slides.length
    element.scrollTo({
      left: next * element.clientWidth,
      behavior: window.matchMedia("(prefers-reduced-motion: reduce)").matches ? "instant" : "smooth",
    })
  }

  if (!slides.length) return <div className={styles.placeholder} aria-hidden="true" />

  return (
    <section className={styles.carousel} aria-label={copy.carouselLabel} aria-roledescription={copy.carouselRole} data-testid="home-carousel">
      <div
        className={styles.track}
        ref={track}
        onScroll={(event) => {
          const element = event.currentTarget
          setActive(Math.round(element.scrollLeft / element.clientWidth))
        }}
        onKeyDown={(event) => {
          if (event.key === "ArrowLeft" || event.key === "ArrowRight") {
            event.preventDefault()
            goTo(active + (event.key === "ArrowRight" ? 1 : -1))
          }
        }}
      >
        {slides.map((slide, index) => (
          <LocalizedClientLink key={slide.id} href={slide.href} className={styles.slide}>
            <div className={styles.copy}>
              <p className={styles.eyebrow}>{copy.eyebrow}</p>
              <h2>{slide.title}</h2>
              <span className={styles.cta}>{copy.explore} <ArrowRight aria-hidden="true" width={20} height={20} className="inline-block align-middle shrink-0" /></span>
            </div>
            <div className={styles.imageFrame}>
              <Image src={slide.image} alt={copy.imageAlt.replace("{name}", slide.title)} fill sizes="(max-width: 599px) 80vw, 45vw" priority={index === 0} className={styles.image} />
            </div>
          </LocalizedClientLink>
        ))}
      </div>
      {slides.length > 1 && (
        <div className={styles.controls}>
          <button type="button" onClick={() => goTo(active - 1)} aria-label={copy.previousSlide}><ArrowLeft aria-hidden="true" width={20} height={20} className="inline-block align-middle shrink-0" /></button>
          <div className={styles.dots}>
            {slides.map((slide, index) => (
              <button key={slide.id} type="button" onClick={() => goTo(index)} aria-label={copy.goToSlide.replace("{name}", slide.title)} aria-current={index === active ? "true" : undefined}>
                <span className={index === active ? styles.activeDot : styles.dot} />
              </button>
            ))}
          </div>
          <button type="button" onClick={() => goTo(active + 1)} aria-label={copy.nextSlide}><ArrowRight aria-hidden="true" width={20} height={20} className="inline-block align-middle shrink-0" /></button>
        </div>
      )}
    </section>
  )
}
