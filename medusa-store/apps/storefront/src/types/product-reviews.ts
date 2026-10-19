export type ProductReview = {
  id: string
  author: string
  rating: number
  title: string
  content: string
  created_at: string
  verified_purchase: boolean
}

export type OwnProductReview = {
  product_id: string
  status: "pending" | "approved" | "rejected"
  verified_purchase: boolean
}
export type ReviewPurchases = {
  orders: {
    id: string
    number: string
    created_at: string
    eligible: boolean
  }[]
  review: {
    id: string
    status: OwnProductReview["status"]
    verified_purchase: boolean
  } | null
  count: number
  offset: number
  limit: number
}
export type OrderReviewContext = {
  eligible: boolean
  reviews: OwnProductReview[]
}

export type ProductReviewPage = {
  reviews: ProductReview[]
  count: number
  offset: number
  limit: number
  average: number
  distribution: { rating: number; count: number }[]
}
