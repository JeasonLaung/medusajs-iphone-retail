"use client"

import { HttpTypes } from "@medusajs/types"
import Image from "next/image"
import { useEffect, useState } from "react"
import { useParams, usePathname, useSearchParams } from "next/navigation"
import { variantFromLocation } from "@lib/util/product-variants"
import { getProductCopy } from "../../i18n"
import styles from "../product-detail/styles.module.css"

export default function ImageGallery({ images, variants = [], initialVariantId, title = "" }: { images: HttpTypes.StoreProductImage[]; variants?: HttpTypes.StoreProductVariant[]; initialVariantId?: string; title?: string }) {
  const { countryCode } = useParams<{ countryCode: string }>()
  const copy = getProductCopy(countryCode)
  const [active, setActive] = useState(0)
  const pathname = usePathname()
  const searchParams = useSearchParams()
  const variant = variantFromLocation(variants, pathname, searchParams.get("v_id"), initialVariantId)
  const validImages = (variant?.images?.length ? variant.images : images).filter((image) => image.url)
  useEffect(() => setActive(0), [variant?.id])
  const index = Math.min(active, Math.max(0, validImages.length - 1))
  const image = validImages[index]
  return (
    <section className={styles.gallery} aria-label={copy.gallery}>
      <div className={styles.stage}>
        {image ? <Image key={image.id} src={image.url} alt={title + " — " + copy.image + " " + (index + 1)} fill priority sizes="(max-width: 767px) 100vw, 600px" /> : <p className={styles.muted}>{copy.noImage}</p>}
        {validImages.length > 1 && <>
          <button type="button" className={styles.galleryArrow} aria-label={copy.previous} onClick={() => setActive((index - 1 + validImages.length) % validImages.length)}>‹</button>
          <button type="button" className={styles.galleryArrow} aria-label={copy.next} onClick={() => setActive((index + 1) % validImages.length)}>›</button>
          <span className={styles.counter}>{index + 1} / {validImages.length}</span>
        </>}
      </div>
      {validImages.length > 1 && <div className={styles.thumbnails}>{validImages.map((image, i) => <button type="button" key={image.id} className={styles.thumbnail} aria-pressed={i === index} aria-label={copy.image + " " + (i + 1)} onClick={() => setActive(i)}><Image src={image.url} alt="" fill sizes="64px" /></button>)}</div>}
    </section>
  )
}
