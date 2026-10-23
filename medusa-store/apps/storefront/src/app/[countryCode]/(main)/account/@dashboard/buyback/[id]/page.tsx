import { getLocale } from "@lib/data/locale-actions"
import { retrieveCustomer } from "@lib/data/customer"
import BuybackRequests from "@modules/sell/components/buyback-requests"
import { getBuybackCopy } from "@modules/sell/buyback-copy"

export async function generateMetadata() {
  return { title: getBuybackCopy(await getLocale()).requestTitle, robots: { index: false } }
}
export default async function Page({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params
  return <BuybackRequests requestId={id} locale={await getLocale()} signedIn={Boolean(await retrieveCustomer())} />
}
