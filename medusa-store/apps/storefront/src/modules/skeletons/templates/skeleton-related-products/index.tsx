import repeat from "@lib/util/repeat"
import SkeletonProductPreview from "@modules/skeletons/components/skeleton-product-preview"

const SkeletonRelatedProducts = () => {
  return (
    <div className="product-page-constraint">
      <div className="flex flex-col gap-2 items-center text-center mb-6">
        <div className="w-20 h-6 animate-pulse bg-gray-100"></div>
        <div className="flex flex-col gap-2 items-center text-center">
          <div className="w-64 h-8 animate-pulse bg-gray-100"></div>
          <div className="w-48 h-6 animate-pulse bg-gray-100"></div>
        </div>
      </div>
      <ul className="flex gap-4 overflow-hidden px-14">
        {repeat(6).map((index) => (
          <li key={index} className="w-36 small:w-44 shrink-0">
            <SkeletonProductPreview layout="preview" />
          </li>
        ))}
      </ul>
    </div>
  )
}

export default SkeletonRelatedProducts
