import { listProductReviews } from "@lib/data/product-reviews"
import { retrieveCustomer } from "@lib/data/customer"
import ReviewSection from "./review-section"

export default async function ProductReviews({ productId, locale }: { productId: string; locale: string }) {
  const [initialPage, customer] = await Promise.all([listProductReviews(productId), retrieveCustomer()])
  return <ReviewSection key={productId} productId={productId} locale={locale} initialPage={initialPage} signedIn={!!customer} />
}
