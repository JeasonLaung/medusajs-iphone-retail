import { defineWidgetConfig } from "@medusajs/admin-sdk"
import type { AdminProduct, DetailWidgetProps } from "@medusajs/framework/types"
import { Badge, Button, Container, Text } from "@medusajs/ui"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { useTranslation } from "react-i18next"
import { Link } from "react-router-dom"
import { reviewSdk } from "../lib/reviews"

type Status = "pending" | "approved" | "rejected"
type Review = { id: string; author: string; title: string; content: string; rating: number; status: Status; created_at: string; order_id: string | null; verified_purchase: boolean }

const ProductReviews = ({ data: product }: DetailWidgetProps<AdminProduct>) => {
  const { t, i18n } = useTranslation()
  const client = useQueryClient()
  const [offset, setOffset] = useState(0)
  const key = ["product-reviews", product.id]
  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: [...key, offset],
    queryFn: () => reviewSdk.client.fetch<{ reviews: Review[]; count: number }>("/admin/product-reviews", { query: { product_id: product.id, limit: 5, offset } }),
  })
  const mutation = useMutation({
    mutationFn: ({ id, status }: { id: string; status: Status }) => reviewSdk.client.fetch(`/admin/product-reviews/${id}`, { method: "POST", body: { status } }),
    onSuccess: () => client.invalidateQueries({ queryKey: key }),
  })
  return <Container className="divide-y divide-ui-border-base p-0">
    <div className="px-6 py-4"><Text size="small" weight="plus">{t("reviews.heading")}</Text><Text size="small" className="text-ui-fg-subtle">{t("reviews.description")}</Text></div>
    <div className="space-y-4 px-6 py-4">
      {isLoading && <Text size="small" role="status">{t("reviews.loading")}</Text>}
      {isError && <div><Text size="small" role="alert">{t("reviews.loadError")}</Text><Button size="small" variant="secondary" onClick={() => refetch()}>{t("reviews.retry")}</Button></div>}
      {mutation.isError && <Text size="small" role="alert" className="text-ui-fg-error">{t("reviews.updateError")}</Text>}
      {data?.count === 0 && <Text size="small" className="text-ui-fg-subtle">{t("reviews.empty")}</Text>}
      {data?.reviews.map((review) => <div key={review.id} className="space-y-2 border-b border-ui-border-base pb-4">
        <div className="flex flex-wrap items-center justify-between gap-2"><Text size="small" weight="plus">{review.author || t("reviews.customer")}</Text><Badge color={review.status === "approved" ? "green" : review.status === "rejected" ? "red" : "orange"}>{t(`reviews.${review.status}`)}</Badge></div>
        <Text size="small" className="text-ui-fg-subtle">{t("reviews.rating", { rating: review.rating })} · {new Date(review.created_at).toLocaleDateString(i18n.language.startsWith("zh") ? "zh-CN" : "en-GB")}</Text>
        <div className="flex flex-wrap items-center gap-2">
          <Badge color={review.verified_purchase ? "green" : "grey"}>{t(review.verified_purchase ? "reviews.verified" : "reviews.openReview")}</Badge>
          {review.order_id && <Link to={`/orders/${review.order_id}`} className="text-ui-fg-interactive"><Text size="small">{t("reviews.order", { id: review.order_id })}</Text></Link>}
        </div>
        <Text size="small" weight="plus" className="break-words">{review.title}</Text><Text size="small" className="whitespace-pre-wrap break-words">{review.content}</Text>
        <div className="flex flex-wrap gap-2 pt-2">{(["approved", "rejected", "pending"] as const).filter((status) => status !== review.status).map((status) => <Button key={status} size="small" variant="secondary" disabled={mutation.isPending} onClick={() => mutation.mutate({ id: review.id, status })}>{t(`reviews.action_${status}`)}</Button>)}</div>
      </div>)}
      {data && data.count > 5 && <div className="flex items-center justify-between gap-3"><Button size="small" variant="secondary" disabled={!offset || isLoading} onClick={() => setOffset(offset - 5)}>{t("reviews.previous")}</Button><Text size="small">{t("reviews.page", { page: offset / 5 + 1, pages: Math.ceil(data.count / 5) })}</Text><Button size="small" variant="secondary" disabled={offset + 5 >= data.count || isLoading} onClick={() => setOffset(offset + 5)}>{t("reviews.next")}</Button></div>}
    </div>
  </Container>
}

export const config = defineWidgetConfig({ zone: "product.details.after" })
export default ProductReviews
