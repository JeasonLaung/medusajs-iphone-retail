import { Text } from "./ui"
import { productPrice } from "../../lib/products"
import type {Product} from "../../lib/products"
import type {Region} from "../../lib/regions"
import Link from "../products/localized-link"
import Thumbnail from "./thumbnail"
import PreviewPrice from "./preview-price"
import ColorSwatches, { isModelColor } from "../search/color-swatches"

export default function ProductPreview({
  product,
  segment,
  isFeatured,
  region: _region,
}: {
  segment: string
  product: Product
  isFeatured?: boolean
  region: Region
}) {
  const {cheapestPrice}=productPrice(product)
  const colors=product.display.colors.filter(isModelColor)

  return (
    <Link segment={segment} href={`/products/${product.handle}`} className="group">
      <div data-testid="product-wrapper">
        <Thumbnail
          thumbnail={product.thumbnail}
          images={product.images}
          size="full"
          isFeatured={isFeatured}
        />
        <div className="flex txt-compact-medium mt-4 justify-between">
          <Text className="text-ui-fg-subtle" data-testid="product-title">
            {product.title}
          </Text>
          <div className="flex items-center gap-x-2">
            {cheapestPrice && <PreviewPrice price={cheapestPrice} />}
          </div>
        </div>
        {colors.length > 0 && (
          <div className="mt-3">
            <ColorSwatches swatches={colors} />
          </div>
        )}
      </div>
    </Link>
  )
}
