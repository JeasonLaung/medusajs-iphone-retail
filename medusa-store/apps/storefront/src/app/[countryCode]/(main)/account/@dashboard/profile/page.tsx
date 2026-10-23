import { Divider } from "antd"
import { withSEO } from "@lib/data/seo"
import { getLocale } from "@lib/data/locale-actions"
import { getAccountCopy } from "@modules/account/i18n"
import { Metadata } from "next"

import ProfilePhone from "@modules/account//components/profile-phone"
import ProfileBillingAddress from "@modules/account/components/profile-billing-address"
import ProfileEmail from "@modules/account/components/profile-email"
import ProfileName from "@modules/account/components/profile-name"
import { notFound } from "next/navigation"
import { listRegions } from "@lib/data/regions"
import { retrieveCustomer } from "@lib/data/customer"

async function defaultPageMetadata(): Promise<Metadata> {
  const { t } = getAccountCopy(await getLocale())
  return { title: t("profile") }
}

export default async function Profile() {
  const { t } = getAccountCopy(await getLocale())
  const customer = await retrieveCustomer()
  const regions = await listRegions()

  if (!customer || !regions) {
    notFound()
  }

  return (
    <div className="w-full" data-testid="profile-page-wrapper">
      <div className="mb-8 flex flex-col gap-y-4">
        <h1 className="text-2xl-semi">{t("profile")}</h1>
        <p className="text-base-regular">{t("profileDescription")}</p>
      </div>
      <div className="flex flex-col gap-y-8 w-full">
        <ProfileName customer={customer} />
        <Divider />
        <ProfileEmail customer={customer} />
        <Divider />
        <ProfilePhone customer={customer} />
        <Divider />
        {/* <ProfilePassword customer={customer} />
        <Divider /> */}
        <ProfileBillingAddress customer={customer} regions={regions} />
      </div>
    </div>
  )
}


export async function generateMetadata(...args: Parameters<typeof defaultPageMetadata>): Promise<Metadata> {
  return withSEO(await defaultPageMetadata(...args))
}
