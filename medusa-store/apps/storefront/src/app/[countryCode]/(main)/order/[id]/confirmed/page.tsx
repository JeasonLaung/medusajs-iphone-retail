import { withSEO } from "@lib/data/seo"
import { retrieveOrder } from "@lib/data/orders"
import OrderCompletedTemplate from "@modules/order/templates/order-completed-template"
import { Metadata } from "next"
import { notFound } from "next/navigation"
import { getLocale } from "@lib/data/locale-actions"
import { getAccountCopy } from "@modules/account/i18n"

type Props = {
  params: Promise<{ id: string }>
}
async function defaultPageMetadata(): Promise<Metadata> {
  const { t } = getAccountCopy(await getLocale())
  return {
    title: t("orderConfirmed"),
    description: t("orderPlaced"),
  }
}

export default async function OrderConfirmedPage(props: Props) {
  const params = await props.params
  const order = await retrieveOrder(params.id).catch(() => null)

  if (!order) {
    return notFound()
  }

  return <OrderCompletedTemplate order={order} />
}

export async function generateMetadata(...args: Parameters<typeof defaultPageMetadata>): Promise<Metadata> {
  return withSEO(await defaultPageMetadata(...args))
}
