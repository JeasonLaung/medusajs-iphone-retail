const repeat = (count: number) => Array.from({length:count},(_,i)=>i)
import SkeletonProductPreview from "./skeleton-preview"
import card from "./hits/styles.module.css"

const SkeletonProductGrid = ({
  numberOfProducts = 8,
  layout = "preview",
}: {
  numberOfProducts?: number
  layout?: "card" | "preview"
}) => {
  return (
    <ul
      className={layout === "card" ? card.grid : "grid grid-cols-1 small:grid-cols-2 gap-x-6 gap-y-8 flex-1"}
      data-testid="products-list-loader"
    >
      {repeat(numberOfProducts).map((index) => (
        <li key={index}>
          <SkeletonProductPreview layout={layout} />
        </li>
      ))}
    </ul>
  )
}

export default SkeletonProductGrid
