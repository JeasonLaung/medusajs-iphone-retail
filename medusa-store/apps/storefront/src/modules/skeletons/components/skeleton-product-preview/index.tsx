import card from "@modules/store/components/store-hits/styles.module.css"
import styles from "./styles.module.css"

const SkeletonProductPreview = ({ layout = "card" }: { layout?: "card" | "preview" }) => {
  if (layout === "card") return (
    <div className={`${card.card} ${styles.skeleton}`} data-testid="product-card-skeleton" aria-hidden="true">
      <div className={card.contents}>
        <div className={card.imageFrame}>
          <div className={`${styles.phone} ${styles.block}`} />
        </div>
        <div className={card.details}>
          <div className={`${styles.title} ${styles.block}`} />
          <div className={card.capacities}>
            {[0, 1, 2].map(index => <span key={index} className={`${styles.capacity} ${styles.block}`} />)}
          </div>
          <div className={card.colours}>
            <div className={`${styles.caption} ${styles.block}`} />
            <div className="flex flex-wrap items-center gap-1.5">
              {[0, 1, 2, 3].map(index => <span key={index} className={`${styles.colour} ${styles.block}`} />)}
            </div>
          </div>
          <div className={card.priceRow}>
            <div className={`${styles.price} ${styles.block}`} />
            <div className={`${styles.button} ${styles.block}`} />
          </div>
        </div>
      </div>
    </div>
  )

  return (
    <div className="motion-safe:animate-pulse" aria-hidden="true">
      <div className="rounded-lg p-4 aspect-[9/16] w-full bg-gray-100 bg-ui-bg-subtle" />
      <div className="flex justify-between text-base-regular mt-2">
        <div className="w-2/5 h-6 bg-gray-100"></div>
        <div className="w-1/5 h-6 bg-gray-100"></div>
      </div>
    </div>
  )
}

export default SkeletonProductPreview
