import { getLocale } from "@lib/data/locale-actions"
import { getAccountCopy } from "@modules/account/i18n"
import React from "react"

import UnderlineLink from "@modules/common/components/interactive-link"

import AccountNav from "../components/account-nav"
import { HttpTypes } from "@medusajs/types"

interface AccountLayoutProps {
  customer: HttpTypes.StoreCustomer | null
  children: React.ReactNode
}

const AccountLayout: React.FC<AccountLayoutProps> = async ({
  customer,
  children,
}) => {
  const { t } = getAccountCopy(await getLocale())
  return (
    <div className="flex-1 small:py-12" data-testid="account-page">
      <div className="flex-1 content-container h-full mx-auto bg-white flex flex-col">
        <div className="grid grid-cols-1  small:grid-cols-[240px_1fr] py-12">
          <div>{customer && <AccountNav customer={customer} />}</div>
          <div className="flex-1">{children}</div>
        </div>
        <div className="flex flex-col small:flex-row items-end justify-between small:border-t border-gray-200 py-12 gap-8">
          <div>
            <h3 className="text-xl-semi mb-4">{t("questions")}</h3>
            <span className="txt-medium">{t("helpDescription")}</span>
          </div>
          <div>
            <UnderlineLink href="/help">{t("customerService")}</UnderlineLink>
          </div>
        </div>
      </div>
    </div>
  )
}

export default AccountLayout
