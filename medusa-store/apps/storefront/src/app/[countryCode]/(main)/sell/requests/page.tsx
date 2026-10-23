import { redirect } from "next/navigation"

export default async function Page({ params }: { params: Promise<{ countryCode: string }> }) {
  const { countryCode } = await params
  redirect(`/${countryCode}/account/buyback`)
}
