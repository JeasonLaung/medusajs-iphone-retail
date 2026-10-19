import { getLocale } from "@lib/data/locale-actions"
import { getAccountCopy } from "@modules/account/i18n"
import { Metadata } from "next"
import { notFound } from "next/navigation"

import AddressBook from "@modules/account/components/address-book"

import { getRegion } from "@lib/data/regions"
import { retrieveCustomer } from "@lib/data/customer"

export async function generateMetadata(): Promise<Metadata> {
  const { t } = getAccountCopy(await getLocale())
  return { title: t("addresses") }
}

export default async function Addresses(props: {
  params: Promise<{ countryCode: string }>
}) {
  const { t } = getAccountCopy(await getLocale())
  const params = await props.params
  const { countryCode } = params
  const customer = await retrieveCustomer()
  const region = await getRegion(countryCode)

  if (!customer || !region) {
    notFound()
  }

  return (
    <div className="w-full" data-testid="addresses-page-wrapper">
      <div className="mb-8 flex flex-col gap-y-4">
        <h1 className="text-2xl-semi">{t("shippingAddresses")}</h1>
        <p className="text-base-regular">{t("addressDescription")}</p>
      </div>
      <AddressBook customer={customer} region={region} />
    </div>
  )
}
