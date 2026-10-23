import { redirect } from "next/navigation"

export default async function Page({ params }: { params: Promise<{ countryCode: string; id: string }> }) {
  const { countryCode, id } = await params
  redirect(`/${countryCode}/account/buyback/${id}`)
}
