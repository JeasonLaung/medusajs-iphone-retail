import { permanentRedirect } from "next/navigation"
import { getCategoryByHandle } from "@lib/data/categories"
import { storeSeriesUrl } from "@lib/util/store-series-url"

export default async function CategoryPage({
  params,
}: {
  params: Promise<{ category: string[]; countryCode: string }>
}) {
  const { category, countryCode } = await params
  const productCategory = await getCategoryByHandle(category)
  const series =
    productCategory?.parent_category?.handle === "iphone"
      ? productCategory.name
      : undefined

  permanentRedirect(`/${countryCode}${storeSeriesUrl(series)}`)
}
