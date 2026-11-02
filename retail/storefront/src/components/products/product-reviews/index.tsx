import Theme from '../storefront-theme'
import ReviewSection from './review-section'
import type {ProductReviewPage} from '../../../lib/product-reviews'
export default function ProductReviews({productId,locale,initialPage}:{productId:string;locale:string;initialPage:ProductReviewPage|null}){
 return <Theme locale={locale.endsWith('-zh')?'zh':locale}><ReviewSection productId={productId} locale={locale} initialPage={initialPage}/></Theme>
}
