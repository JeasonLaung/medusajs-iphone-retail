import { withSEO } from "@lib/data/seo"
import { getLocale } from "@lib/data/locale-actions"
import { getAccountCopy } from "@modules/account/i18n"
import { Metadata } from "next"
import { Suspense } from "react"

import VerifyAccount from "@modules/account/components/verify-account"

async function defaultPageMetadata(): Promise<Metadata> {
  const { t } = getAccountCopy(await getLocale())
  return {
    title: t("verificationTitle"),
    description: t("verificationDescription"),
  }
}

export default async function VerifyAccountPage() {
  const { t } = getAccountCopy(await getLocale())
  return (
    <div className="w-full flex justify-center px-8 py-12">
      <Suspense
        fallback={
          <p className="text-base-regular text-ui-fg-base">{t("verifying")}</p>
        }
      >
        <VerifyAccount />
      </Suspense>
    </div>
  )
}

export async function generateMetadata(...args: Parameters<typeof defaultPageMetadata>): Promise<Metadata> {
  return withSEO(await defaultPageMetadata(...args))
}
