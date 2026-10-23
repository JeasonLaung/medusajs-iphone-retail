import { withSEO } from "@lib/data/seo"
import { getLocale } from "@lib/data/locale-actions"
import { getAccountCopy } from "@modules/account/i18n"
import { getOrderNumber } from "@lib/util/order-number"
import { retrieveOrder, retrieveOrderTimeline } from "@lib/data/orders"
import OrderDetailsTemplate from "@modules/order/templates/order-details-template"
import { Metadata } from "next"
import { notFound } from "next/navigation"

type Props = {
  params: Promise<{ id: string }>
}

async function defaultPageMetadata(props: Props): Promise<Metadata> {
  const params = await props.params
  const { t } = getAccountCopy(await getLocale())
  const order = await retrieveOrder(params.id).catch(() => null)

  if (!order) {
    notFound()
  }

  return {
    title: t("orderTitle", { number: getOrderNumber(order) }),
    description: t("viewOrder"),
  }
}

export default async function OrderDetailPage(props: Props) {
  const params = await props.params
  const [order, timeline] = await Promise.all([
    retrieveOrder(params.id).catch(() => null),
    retrieveOrderTimeline(params.id),
  ])

  if (!order) {
    notFound()
  }

  return <OrderDetailsTemplate order={order} timeline={timeline} />
}

export async function generateMetadata(...args: Parameters<typeof defaultPageMetadata>): Promise<Metadata> {
  return withSEO(await defaultPageMetadata(...args))
}
